package handlers

import (
	"context"
	"errors"
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
	List(ctx context.Context, a notes.Actor) ([]notes.Note, error)
	Get(ctx context.Context, a notes.Actor, id string) (notes.Note, error)
	Create(ctx context.Context, a notes.Actor, in notes.Input) (notes.Note, error)
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
}

type noteListResponse struct {
	Items []noteResponse `json:"items"`
}

// noteRequest is the body of POST /notes (schema NoteInput).
type noteRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func toNoteResponse(n notes.Note) noteResponse {
	return noteResponse{ID: n.ID, CreatedBy: n.CreatedBy, Title: n.Title, Body: n.Body, CreatedAt: n.CreatedAt}
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

// List handles GET /notes.
func (h *NotesHandler) List(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	list, err := h.Notes.List(r.Context(), a)
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
		if errors.Is(err, notes.ErrNotFound) {
			h.Problems.Write(w, r, http.StatusNotFound, problemjson.SlugNotFound, "note "+id+" not found")
			return
		}
		h.internalError(w, r, err)
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
		var verr *notes.ValidationError
		if errors.As(err, &verr) {
			h.Problems.WriteValidation(w, r, verr.Error(),
				problemjson.FieldError{Pointer: "/" + verr.Field, Detail: verr.Msg})
			return
		}
		h.internalError(w, r, err)
		return
	}
	w.Header().Set("Location", r.URL.Path+"/"+n.ID)
	writeJSON(w, http.StatusCreated, toNoteResponse(n))
}

// internalError logs the real error and answers with a generic 500: internal
// details never reach the client.
func (h *NotesHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.Logger.Error("notes handler", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
	h.Problems.Write(w, r, http.StatusInternalServerError, problemjson.SlugInternal, "unexpected error")
}
