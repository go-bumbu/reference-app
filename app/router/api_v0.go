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
func attachApiV0(r *mux.Router, notes handlers.NotesService, tags handlers.TagsService, jobs handlers.JobsService,
	guard *authGuard, problems *problemjson.Writer, logger *slog.Logger,
) {
	api := func(path string) *mux.Route { return r.Path(apiV0MountPrefix + path) }

	api("/info").Methods(http.MethodGet).Handler(handlers.InfoHandler())
	api("/me").Methods(http.MethodGet).Handler(guard.protect(handlers.MeHandler(problems)))

	nh := &handlers.NotesHandler{Notes: notes, Problems: problems, Logger: logger}
	api("/notes").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(nh.List)))
	api("/notes").Methods(http.MethodPost).Handler(guard.protect(http.HandlerFunc(nh.Create)))
	api("/notes/{noteId}").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(nh.Get)))
	api("/notes/{noteId}").Methods(http.MethodPut).Handler(guard.protect(http.HandlerFunc(nh.Update)))
	api("/notes/{noteId}").Methods(http.MethodDelete).Handler(guard.protect(http.HandlerFunc(nh.Delete)))
	api("/notes/{noteId}/tags/{tagId}").Methods(http.MethodPut).Handler(guard.protect(http.HandlerFunc(nh.AddTag)))
	api("/notes/{noteId}/tags/{tagId}").Methods(http.MethodDelete).Handler(guard.protect(http.HandlerFunc(nh.RemoveTag)))

	th := &handlers.TagsHandler{Tags: tags, Problems: problems, Logger: logger}
	api("/tags").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(th.List)))
	api("/tags").Methods(http.MethodPost).Handler(guard.protect(http.HandlerFunc(th.Create)))
	api("/tags/{tagId}").Methods(http.MethodPatch).Handler(guard.protect(http.HandlerFunc(th.Update)))
	api("/tags/{tagId}").Methods(http.MethodDelete).Handler(guard.protect(http.HandlerFunc(th.Delete)))

	jh := &handlers.JobsHandler{Jobs: jobs, Problems: problems, Logger: logger}
	api("/jobs").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(jh.List)))
	api("/jobs/{jobName}/runs").Methods(http.MethodPost).Handler(guard.protect(http.HandlerFunc(jh.RunJob)))
	api("/jobs/{jobName}/schedules").Methods(http.MethodPost).Handler(guard.protect(http.HandlerFunc(jh.CreateSchedule)))
	api("/jobs/{jobName}/schedules/{scheduleId}").Methods(http.MethodPatch).
		Handler(guard.protect(http.HandlerFunc(jh.UpdateSchedule)))
	api("/jobs/{jobName}/schedules/{scheduleId}").Methods(http.MethodDelete).
		Handler(guard.protect(http.HandlerFunc(jh.DeleteSchedule)))
	api("/job-runs").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(jh.ListRuns)))
	api("/job-runs/{runId}").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(jh.GetRun)))
	api("/job-runs/{runId}/cancel").Methods(http.MethodPost).Handler(guard.protect(http.HandlerFunc(jh.CancelRun)))
	api("/job-runs/{runId}/log").Methods(http.MethodGet).Handler(guard.protect(http.HandlerFunc(jh.RunLog)))
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
