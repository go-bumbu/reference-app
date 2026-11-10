package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/app/reqctx"
	"github.com/go-bumbu/reference-app/internal/notes"
	"github.com/gorilla/mux"
)

// NotesService is what the notes handler needs from the internal/notes module.
// Declared on the consumer side, so the handler can be tested with a fake.
type NotesService interface {
	List(ctx context.Context, a notes.Actor, opts notes.ListOptions) ([]notes.Note, error)
	Get(ctx context.Context, a notes.Actor, id string) (notes.Note, error)
	Create(ctx context.Context, a notes.Actor, in notes.Input) (notes.Note, error)
	Update(ctx context.Context, a notes.Actor, id string, in notes.Input) (notes.Note, error)
	Delete(ctx context.Context, a notes.Actor, id string) error
	AddNoteTag(ctx context.Context, a notes.Actor, noteID string, tagID uint) (notes.Note, error)
	RemoveNoteTag(ctx context.Context, a notes.Actor, noteID string, tagID uint) (notes.Note, error)
}

// NotesHandler translates the /notes HTTP API into calls to the notes module.
// It holds no business logic: validation, storage and tenant scoping belong to
// the module. Its routes must sit behind the auth guard, which provides the
// caller's identity.
type NotesHandler struct {
	Notes    NotesService
	Problems *problemjson.Writer
	Logger   *slog.Logger
}

// noteResponse is the API representation of a note (schema Note in
// docs/openapi/api-v0.yaml). It is mapped explicitly from notes.Note so the
// wire format does not change when the module's model does.
type noteResponse struct {
	ID        string    `json:"id"`
	CreatedBy string    `json:"createdBy"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	TagIDs    []uint    `json:"tagIds"`
}

type noteListResponse struct {
	Items []noteResponse `json:"items"`
}

// noteRequest is the body of POST /notes and PUT /notes/{noteId} (schema NoteInput).
type noteRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func toNoteResponse(n notes.Note) noteResponse {
	tagIDs := n.TagIDs
	if tagIDs == nil { // the wire always carries a list, never null
		tagIDs = []uint{}
	}
	return noteResponse{
		ID: n.ID, CreatedBy: n.CreatedBy, Title: n.Title, Body: n.Body, CreatedAt: n.CreatedAt,
		TagIDs: tagIDs,
	}
}

// actor maps the authenticated caller onto the notes module's Actor. ok is
// false when the route is not behind the auth guard: a wiring bug.
func actor(r *http.Request) (notes.Actor, bool) {
	id, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		return notes.Actor{}, false
	}
	return notes.Actor{TenantID: id.TenantID, UserID: id.UserID}, true
}

// List handles GET /notes, narrowed to the notes under a tag by `?tagId=`.
func (h *NotesHandler) List(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	var opts notes.ListOptions
	if raw, set := r.URL.Query()["tagId"]; set {
		id, ok := parseTagID(raw[0])
		if !ok || len(raw) > 1 {
			h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest, "tagId must be one positive integer")
			return
		}
		opts.TagID = id
	}
	list, err := h.Notes.List(r.Context(), a, opts)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	resp := noteListResponse{Items: make([]noteResponse, 0, len(list))}
	for _, n := range list {
		resp.Items = append(resp.Items, toNoteResponse(n))
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /notes/{noteId}.
func (h *NotesHandler) Get(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	id := mux.Vars(r)["noteId"]
	n, err := h.Notes.Get(r.Context(), a, id)
	if err != nil {
		h.writeError(w, r, id, err)
		return
	}
	writeJSON(w, http.StatusOK, toNoteResponse(n))
}

// Create handles POST /notes.
func (h *NotesHandler) Create(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	var req noteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest, err.Error())
		return
	}
	n, err := h.Notes.Create(r.Context(), a, notes.Input{Title: req.Title, Body: req.Body})
	if err != nil {
		h.writeError(w, r, "", err)
		return
	}
	w.Header().Set("Location", r.URL.Path+"/"+n.ID)
	writeJSON(w, http.StatusCreated, toNoteResponse(n))
}

// Update handles PUT /notes/{noteId}.
func (h *NotesHandler) Update(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	var req noteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest, err.Error())
		return
	}
	id := mux.Vars(r)["noteId"]
	n, err := h.Notes.Update(r.Context(), a, id, notes.Input{Title: req.Title, Body: req.Body})
	if err != nil {
		h.writeError(w, r, id, err)
		return
	}
	writeJSON(w, http.StatusOK, toNoteResponse(n))
}

// Delete handles DELETE /notes/{noteId}.
func (h *NotesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	id := mux.Vars(r)["noteId"]
	if err := h.Notes.Delete(r.Context(), a, id); err != nil {
		h.writeError(w, r, id, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddTag handles PUT /notes/{noteId}/tags/{tagId}: the note carries the tag
// afterwards, whether it did before or not.
func (h *NotesHandler) AddTag(w http.ResponseWriter, r *http.Request) {
	h.setTag(w, r, h.Notes.AddNoteTag)
}

// RemoveTag handles DELETE /notes/{noteId}/tags/{tagId}: the note does not
// carry the tag afterwards, whether it did before or not.
func (h *NotesHandler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	h.setTag(w, r, h.Notes.RemoveNoteTag)
}

func (h *NotesHandler) setTag(w http.ResponseWriter, r *http.Request,
	apply func(context.Context, notes.Actor, string, uint) (notes.Note, error),
) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	vars := mux.Vars(r)
	noteID, rawTag := vars["noteId"], vars["tagId"]
	about := subject{noteID: noteID, tagID: rawTag}
	tagID, ok := parseTagID(rawTag)
	if !ok {
		h.errs().write(w, r, about, notes.ErrTagNotFound)
		return
	}
	n, err := apply(r.Context(), a, noteID, tagID)
	if err != nil {
		h.errs().write(w, r, about, err)
		return
	}
	writeJSON(w, http.StatusOK, toNoteResponse(n))
}

func (h *NotesHandler) errs() notesErrors {
	return notesErrors{problems: h.Problems, logger: h.Logger, area: "notes handler"}
}

func (h *NotesHandler) writeError(w http.ResponseWriter, r *http.Request, id string, err error) {
	h.errs().write(w, r, subject{noteID: id}, err)
}

func (h *NotesHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.errs().internal(w, r, err)
}
