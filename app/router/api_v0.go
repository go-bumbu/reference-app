package router

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/app/router/handlers"
	"github.com/gorilla/mux"
)

// apiV0MountPrefix is the base of the API; it matches the `servers` entry of
// docs/openapi/api-v0.yaml, whose paths are relative to it.
const apiV0MountPrefix = "/api/v0"

// problemBaseURI namespaces the Type of every problem+json body. It is opaque,
// never fetched by clients; only the trailing slug is meaningful.
const problemBaseURI = "https://reference-app.local/probs" // TODO replace with your own namespace

const slugMethodNotAllowed = "method_not_allowed"

// newProblems returns the problem+json Writer. production masks the bodies:
// no internal detail reaches the client.
func newProblems(production bool) (*problemjson.Writer, error) {
	mode := problemjson.ModeDev
	if production {
		mode = problemjson.ModeMasked
	}
	return problemjson.New(problemjson.Cfg{
		BaseURI: problemBaseURI,
		Titles: map[string]string{
			handlers.SlugBadRequest: "Bad request",
			slugMethodNotAllowed:    "Method not allowed",
		},
		Mode: mode,
	})
}

// attachApiV0 mounts the /api/v0 routes. Every route here must be documented in
// docs/openapi/api-v0.yaml; TestOpenAPICoversAllRoutes enforces it.
//
// Routes are public unless wrapped in guard.protect, which authenticates the
// caller and puts its tenant and user on the request context. Protecting each
// route explicitly keeps the public ones visible at a glance.
//
// The routes are registered on the root router with the full path instead of on
// a PathPrefix subrouter: subroutes inherit the prefix matcher, and when a later
// subroute's prefix matches, gorilla/mux drops the "method mismatch" of an
// earlier one, so a wrong method ends up as 404 instead of 405.
func attachApiV0(r *mux.Router, notes handlers.NotesService, guard *authGuard, problems *problemjson.Writer, logger *slog.Logger) {
	api := func(path string) *mux.Route { return r.Path(apiV0MountPrefix + path) }

	api("/info").Methods(http.MethodGet).Handler(handlers.InfoHandler())
	api("/me").Methods(http.MethodGet).Handler(guard.protect(handlers.MeHandler(problems)))

	nh := &handlers.NotesHandler{Notes: notes, Problems: problems, Logger: logger}
	api("/notes").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(nh.List)))
	api("/notes").Methods(http.MethodPost).Handler(guard.protect(http.HandlerFunc(nh.Create)))
	api("/notes/{noteId}").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(nh.Get)))
}

// isAPIPath reports whether the request targets the API.
func isAPIPath(r *http.Request) bool {
	return r.URL.Path == apiV0MountPrefix || strings.HasPrefix(r.URL.Path, apiV0MountPrefix+"/")
}

// notFound answers unknown routes with problem+json on the API, and plain text elsewhere.
func notFound(problems *problemjson.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r) {
			problems.Write(w, r, http.StatusNotFound, problemjson.SlugNotFound, "no such endpoint")
			return
		}
		http.NotFound(w, r)
	})
}

// methodNotAllowed answers a wrong method with problem+json on the API, and plain text elsewhere.
func methodNotAllowed(problems *problemjson.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r) {
			problems.Write(w, r, http.StatusMethodNotAllowed, slugMethodNotAllowed, r.Method+" is not allowed on this endpoint")
			return
		}
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})
}
