package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/internal/notes"
	"github.com/gorilla/mux"
)

// TagsService is what the tags handler needs from the internal/notes module,
// which owns the tag tree of the notes. Declared on the consumer side, so the
// handler can be tested with a fake.
type TagsService interface {
	Tags(ctx context.Context, a notes.Actor) ([]notes.Tag, error)
	CreateTag(ctx context.Context, a notes.Actor, in notes.TagInput) (notes.Tag, error)
	UpdateTag(ctx context.Context, a notes.Actor, id uint, p notes.TagPatch) (notes.Tag, error)
	DeleteTag(ctx context.Context, a notes.Actor, id uint) error
}

// TagsHandler translates the /tags HTTP API into calls to the notes module.
// Like NotesHandler it holds no business logic, and its routes must sit behind
// the auth guard.
type TagsHandler struct {
	Tags     TagsService
	Problems *problemjson.Writer
	Logger   *slog.Logger
}

// tagResponse is the API representation of a tag with its subtree (schema Tag).
type tagResponse struct {
	ID       uint          `json:"id"`
	ParentID uint          `json:"parentId"`
	Name     string        `json:"name"`
	Children []tagResponse `json:"children"`
}

type tagListResponse struct {
	Items []tagResponse `json:"items"`
}

// createTagRequest is the body of POST /tags (schema TagInput).
type createTagRequest struct {
	Name     string `json:"name"`
	ParentID uint   `json:"parentId"`
	AfterID  *uint  `json:"afterId"`
}

// updateTagRequest is the body of PATCH /tags/{tagId} (schema TagPatch); an
// absent field is left as it is.
type updateTagRequest struct {
	Name     *string `json:"name"`
	ParentID *uint   `json:"parentId"`
	AfterID  *uint   `json:"afterId"`
}

func toTagResponse(t notes.Tag) tagResponse {
	return tagResponse{ID: t.ID, ParentID: t.ParentID, Name: t.Name, Children: toTagResponses(t.Children)}
}

// toTagResponses maps a list of tags; a leaf's children are an empty list,
// never null.
func toTagResponses(tags []notes.Tag) []tagResponse {
	out := make([]tagResponse, 0, len(tags))
	for _, t := range tags {
		out = append(out, toTagResponse(t))
	}
	return out
}

// parseTagID reads a tag id from a path or query value; ok is false when it
// is not a positive integer the database can hold (63 bits: a signed int64).
func parseTagID(raw string) (uint, bool) {
	id, err := strconv.ParseUint(raw, 10, 63)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}

// List handles GET /tags.
func (h *TagsHandler) List(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	tree, err := h.Tags.Tags(r.Context(), a)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tagListResponse{Items: toTagResponses(tree)})
}

// Create handles POST /tags.
func (h *TagsHandler) Create(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	var req createTagRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest, err.Error())
		return
	}
	tag, err := h.Tags.CreateTag(r.Context(), a, notes.TagInput{Name: req.Name, ParentID: req.ParentID, AfterID: req.AfterID})
	if err != nil {
		h.writeError(w, r, "", err)
		return
	}
	w.Header().Set("Location", r.URL.Path+"/"+strconv.FormatUint(uint64(tag.ID), 10))
	writeJSON(w, http.StatusCreated, toTagResponse(tag))
}

// Update handles PATCH /tags/{tagId}.
func (h *TagsHandler) Update(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	raw := mux.Vars(r)["tagId"]
	id, ok := parseTagID(raw)
	if !ok {
		h.writeError(w, r, raw, notes.ErrTagNotFound)
		return
	}
	var req updateTagRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest, err.Error())
		return
	}
	tag, err := h.Tags.UpdateTag(r.Context(), a, id, notes.TagPatch{Name: req.Name, ParentID: req.ParentID, AfterID: req.AfterID})
	if err != nil {
		h.writeError(w, r, raw, err)
		return
	}
	writeJSON(w, http.StatusOK, toTagResponse(tag))
}

// Delete handles DELETE /tags/{tagId}.
func (h *TagsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(r)
	if !ok {
		h.internalError(w, r, errNoIdentity)
		return
	}
	raw := mux.Vars(r)["tagId"]
	id, ok := parseTagID(raw)
	if !ok {
		h.writeError(w, r, raw, notes.ErrTagNotFound)
		return
	}
	if err := h.Tags.DeleteTag(r.Context(), a, id); err != nil {
		h.writeError(w, r, raw, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TagsHandler) errs() notesErrors {
	return notesErrors{problems: h.Problems, logger: h.Logger, area: "tags handler"}
}

func (h *TagsHandler) writeError(w http.ResponseWriter, r *http.Request, id string, err error) {
	h.errs().write(w, r, subject{tagID: id}, err)
}

func (h *TagsHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.errs().internal(w, r, err)
}
