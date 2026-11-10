package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/internal/notes"
)

// notesErrors maps the errors of the internal/notes module onto problems, in
// one place for every handler of the module (notes and tags).
type notesErrors struct {
	problems *problemjson.Writer
	logger   *slog.Logger
	area     string // the handler, in the log line of a 500
}

// subject names what a request was about, for the detail of a 404.
type subject struct {
	noteID string
	tagID  string
}

// write maps err: ErrNotFound and ErrTagNotFound are 404 for the note or the
// tag, ErrTagExists 409, ErrTooManyTags 422, a *ValidationError 422 with the
// field's pointer, anything else a logged 500.
func (e notesErrors) write(w http.ResponseWriter, r *http.Request, about subject, err error) {
	var verr *notes.ValidationError
	switch {
	case errors.Is(err, notes.ErrNotFound):
		e.problems.Write(w, r, http.StatusNotFound, problemjson.SlugNotFound, "note "+about.noteID+" not found")
	case errors.Is(err, notes.ErrTagNotFound):
		e.problems.Write(w, r, http.StatusNotFound, problemjson.SlugNotFound, "tag "+about.tagID+" not found")
	case errors.Is(err, notes.ErrTagExists):
		e.problems.Write(w, r, http.StatusConflict, problemjson.SlugConflict, err.Error())
	case errors.Is(err, notes.ErrTooManyTags):
		e.problems.Write(w, r, http.StatusUnprocessableEntity, problemjson.SlugValidationError, err.Error())
	case errors.As(err, &verr):
		e.problems.WriteValidation(w, r, verr.Error(),
			problemjson.FieldError{Pointer: "/" + verr.Field, Detail: verr.Msg})
	default:
		e.internal(w, r, err)
	}
}

// internal logs the real error and answers with a generic 500: internal
// details never reach the client.
func (e notesErrors) internal(w http.ResponseWriter, r *http.Request, err error) {
	e.logger.Error(e.area, slog.String("path", r.URL.Path), slog.String("error", err.Error()))
	e.problems.Write(w, r, http.StatusInternalServerError, problemjson.SlugInternal, "unexpected error")
}
