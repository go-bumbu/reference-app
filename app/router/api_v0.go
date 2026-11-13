package router

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/app/router/handlers"
	"github.com/go-bumbu/reference-app/libs/keyscope"
	"github.com/gorilla/mux"
)

// apiV0MountPrefix is the base of the API; it matches the `servers` entry of
// docs/openapi/api-v0.yaml, whose paths are relative to it.
const apiV0MountPrefix = "/api/v0"

// keyMountPrefix is the key-only mount of the same API: the same routes,
// authenticated by an API key instead of the proxy's identity (keyauth.go). It
// is the spec's second `servers` entry. The proxy must let it through without a
// login, and nothing else.
const keyMountPrefix = "/api/key/v0"

// KeyMountPrefix is keyMountPrefix for the startup log and the docs of a
// deployment: the one path the proxy lets through without a login.
const KeyMountPrefix = keyMountPrefix

// scopeNotes is the API key area of the notes routes ("notes:read",
// "notes:write").
const scopeNotes = "notes"

// problemBaseURI namespaces the Type of every problem+json body. It is opaque,
// never fetched by clients; only the trailing slug is meaningful.
const problemBaseURI = "https://reference-app.local/probs" // TODO replace with your own namespace

const slugMethodNotAllowed = "method_not_allowed"

// newProblems returns the problem+json Writer. production masks the bodies:
// no internal detail reaches the client. Every problem carries the request id
// as its reference and is logged unmasked by logProblem, so a masked answer a
// user reports can be traced to its detail.
func newProblems(production bool, logger *slog.Logger) (*problemjson.Writer, error) {
	mode := problemjson.ModeDev
	if production {
		mode = problemjson.ModeMasked
	}
	return problemjson.New(problemjson.Cfg{
		BaseURI: problemBaseURI,
		Titles: map[string]string{
			handlers.SlugBadRequest: "Bad request",
			handlers.SlugQueueFull:  "Job queue full",
			slugMethodNotAllowed:    "Method not allowed",
		},
		Mode:      mode,
		RequestID: requestID,
		OnProblem: logProblem(logger),
	})
}

// logProblem logs every problem with its full, unmasked detail: at error for a
// 5xx, at info otherwise. The reference matches the req-id of the request log
// line and the reference in the body the client got.
func logProblem(logger *slog.Logger) func(*http.Request, problemjson.Problem) {
	return func(r *http.Request, p problemjson.Problem) {
		attrs := []slog.Attr{
			slog.Int("status", p.Status),
			slog.String("type", p.Type),
			slog.String("detail", p.Detail),
			slog.String("url", r.URL.EscapedPath()),
			slog.String("reference", p.Reference),
		}
		for _, f := range p.Errors {
			attrs = append(attrs, slog.String("field "+f.Pointer, f.Detail))
		}
		if p.Reason != "" {
			attrs = append(attrs, slog.String("reason", p.Reason))
		}
		if p.Err != nil {
			attrs = append(attrs, slog.String("error", p.Err.Error()))
		}
		level := slog.LevelInfo
		if p.Status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		logger.LogAttrs(r.Context(), level, "problem", attrs...)
	}
}

// apiMount registers each API route on the session mount (/api/v0, behind
// the auth guard) and, when API keys are on, on the key-only mount
// (/api/key/v0, behind the key guard), with the key rule the route declares.
//
// Routes are registered on the root router with the full path instead of on a
// PathPrefix subrouter: subroutes inherit the prefix matcher, and when a later
// subroute's prefix matches, gorilla/mux drops the "method mismatch" of an
// earlier one, so a wrong method ends up as 404 instead of 405.
type apiMount struct {
	r      *mux.Router
	guard  *authGuard
	keys   *keyGuard // nil: API keys are off and the key-only mount is absent
	scopes *keyscope.Registry
	// rules is the key rule of every protected route, by "METHOD /path"
	// (mount-relative), for the test that holds them to the spec.
	rules map[string]keyscope.Rule
}

// public mounts a route anyone may call, on both mounts.
func (m apiMount) public(method, path string, h http.Handler) {
	m.r.Path(apiV0MountPrefix + path).Methods(method).Handler(h)
	if m.keys != nil {
		m.r.Path(keyMountPrefix + path).Methods(method).Handler(h)
	}
}

// protected mounts a route that needs an authenticated caller. rule is what an
// API key needs to use it: keyscope.Closed() keeps every key off it (the route
// still exists on the key-only mount and answers 403), keyscope.Area("notes")
// opens it at the level its method implies. The rules' areas are what a key
// can be granted.
func (m apiMount) protected(method, path string, rule keyscope.Rule, h http.Handler) {
	m.scopes.Register(rule)
	m.rules[method+" "+path] = rule
	m.r.Path(apiV0MountPrefix + path).Methods(method).Handler(m.guard.protect(h))
	if m.keys != nil {
		m.r.Path(keyMountPrefix + path).Methods(method).Handler(m.keys.protect(rule, h))
	}
}

// attachApiV0 mounts the /api/v0 routes. Every route here must be documented in
// docs/openapi/api-v0.yaml, with the API key scope its rule declares;
// TestOpenAPICoversAllRoutes and TestOpenAPIKeyScopes enforce both.
//
// Routes are public unless mounted with protected, which authenticates the
// caller and puts its tenant and user on the request context. Each protected
// route states, on its own line, whether an API key may use it.
func attachApiV0(m apiMount, notes handlers.NotesService, tags handlers.TagsService, jobs handlers.JobsService,
	apiKeys handlers.APIKeysService, problems *problemjson.Writer, logger *slog.Logger,
) {
	var (
		closed   = keyscope.Closed()
		anyKey   = keyscope.AnyKey()
		notesKey = keyscope.Area(scopeNotes)
	)

	m.public(http.MethodGet, "/info", handlers.InfoHandler())
	m.protected(http.MethodGet, "/me", anyKey, handlers.MeHandler(problems))

	nh := &handlers.NotesHandler{Notes: notes, Problems: problems, Logger: logger}
	m.protected(http.MethodGet, "/notes", notesKey, http.HandlerFunc(nh.List))
	m.protected(http.MethodPost, "/notes", notesKey, http.HandlerFunc(nh.Create))
	m.protected(http.MethodGet, "/notes/{noteId}", notesKey, http.HandlerFunc(nh.Get))
	m.protected(http.MethodPut, "/notes/{noteId}", notesKey, http.HandlerFunc(nh.Update))
	m.protected(http.MethodDelete, "/notes/{noteId}", notesKey, http.HandlerFunc(nh.Delete))
	m.protected(http.MethodPut, "/notes/{noteId}/tags/{tagId}", notesKey, http.HandlerFunc(nh.AddTag))
	m.protected(http.MethodDelete, "/notes/{noteId}/tags/{tagId}", notesKey, http.HandlerFunc(nh.RemoveTag))

	th := &handlers.TagsHandler{Tags: tags, Problems: problems, Logger: logger}
	m.protected(http.MethodGet, "/tags", closed, http.HandlerFunc(th.List))
	m.protected(http.MethodPost, "/tags", closed, http.HandlerFunc(th.Create))
	m.protected(http.MethodPatch, "/tags/{tagId}", closed, http.HandlerFunc(th.Update))
	m.protected(http.MethodDelete, "/tags/{tagId}", closed, http.HandlerFunc(th.Delete))

	jh := &handlers.JobsHandler{Jobs: jobs, Problems: problems, Logger: logger}
	m.protected(http.MethodGet, "/jobs", closed, http.HandlerFunc(jh.List))
	m.protected(http.MethodPost, "/jobs/{jobName}/runs", closed, http.HandlerFunc(jh.RunJob))
	m.protected(http.MethodPost, "/jobs/{jobName}/schedules", closed, http.HandlerFunc(jh.CreateSchedule))
	m.protected(http.MethodPatch, "/jobs/{jobName}/schedules/{scheduleId}", closed, http.HandlerFunc(jh.UpdateSchedule))
	m.protected(http.MethodDelete, "/jobs/{jobName}/schedules/{scheduleId}", closed, http.HandlerFunc(jh.DeleteSchedule))
	m.protected(http.MethodGet, "/job-runs", closed, http.HandlerFunc(jh.ListRuns))
	m.protected(http.MethodGet, "/job-runs/{runId}", closed, http.HandlerFunc(jh.GetRun))
	m.protected(http.MethodPost, "/job-runs/{runId}/cancel", closed, http.HandlerFunc(jh.CancelRun))
	m.protected(http.MethodGet, "/job-runs/{runId}/log", closed, http.HandlerFunc(jh.RunLog))

	// Key management exists only when API keys are on, and is never open to a key.
	if apiKeys != nil {
		kh := &handlers.APIKeysHandler{Keys: apiKeys, Scopes: m.scopes, Problems: problems, Logger: logger}
		m.protected(http.MethodGet, "/api-keys", closed, http.HandlerFunc(kh.List))
		m.protected(http.MethodPost, "/api-keys", closed, http.HandlerFunc(kh.Create))
		m.protected(http.MethodGet, "/api-keys/scopes", closed, http.HandlerFunc(kh.ListScopes))
		m.protected(http.MethodDelete, "/api-keys/{keyId}", closed, http.HandlerFunc(kh.Revoke))
	}
}

// isAPIPath reports whether the request targets the API, on either mount.
func isAPIPath(r *http.Request) bool {
	for _, prefix := range []string{apiV0MountPrefix, keyMountPrefix} {
		if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
			return true
		}
	}
	return false
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
