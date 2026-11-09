// Package router builds the HTTP handlers of the application.
//
// It only assembles routes and middleware into an http.Handler; it does not own
// a listener. Creating the http.Server, binding the port and the graceful
// shutdown live in app/cmd, so the lifecycle of every server is handled in one
// place (see serveWithGracefulShutdown).
package router

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-bumbu/reference-app/app/router/handlers"
	"github.com/go-bumbu/reference-app/libs/middleware/log"
	"github.com/go-bumbu/reference-app/libs/middleware/prometheus"
	"github.com/gorilla/mux"
)

// Cfg holds the dependencies of the main application handler.
// app/cmd instantiates them and passes the references in.
type Cfg struct {
	Logger *slog.Logger
	// Notes is the notes module (internal/notes); required.
	Notes handlers.NotesService
	// Tenants resolves the authenticated identity into tenant and user IDs
	// (internal/tenants); required.
	Tenants TenantProvisioner
	// Auth selects the authentication method; the zero value is AuthNone.
	Auth AuthCfg
	// UI serves the web UI (app/spa) for every GET outside the API; nil serves
	// no UI.
	UI http.Handler
	// Production masks the problem+json error bodies: status and instance only,
	// no internal detail. Sourced from Env.Production.
	Production bool
}

// New builds the main application handler. It returns an error so that wiring
// which can fail (e.g. a missing dependency) is reported at startup instead of
// panicking on the first request.
func New(cfg Cfg) (http.Handler, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Notes == nil {
		return nil, errors.New("router: Notes is required")
	}
	if cfg.Tenants == nil {
		return nil, errors.New("router: Tenants is required")
	}
	problems, err := newProblems(cfg.Production)
	if err != nil {
		return nil, fmt.Errorf("router: problem writer: %w", err)
	}
	guard, err := newAuthGuard(cfg.Auth, cfg.Tenants, problems, logger)
	if err != nil {
		return nil, fmt.Errorf("router: auth: %w", err)
	}

	r := mux.NewRouter()
	// add logging middleware
	r.Use(func(handler http.Handler) http.Handler {
		return log.LoggingMiddleware(handler, logger)
	})

	// add prometheus middleware
	r.Use(func(handler http.Handler) http.Handler {
		return prometheus.PromMiddleware(handler)
	})

	attachApiV0(r, cfg.Notes, guard, problems, logger)
	if cfg.UI != nil {
		attachUI(r, cfg.UI)
	}

	r.NotFoundHandler = notFound(problems)
	r.MethodNotAllowedHandler = methodNotAllowed(problems)

	return r, nil
}

// attachUI mounts the web UI as the catch-all of every GET and HEAD outside the
// API, after the API routes. An unknown /api path must stay a problem+json 404,
// never the UI's index.html, so the API prefix is excluded explicitly.
//
// The exclusion is the first matcher on purpose: in gorilla/mux every matcher
// that succeeds clears the "method mismatch" an earlier route recorded, so with
// PathPrefix("/") first, DELETE /api/v0/notes would turn from 405 into 404.
func attachUI(r *mux.Router, ui http.Handler) {
	r.MatcherFunc(func(req *http.Request, _ *mux.RouteMatch) bool { return !isAPIPath(req) }).
		PathPrefix("/").
		Methods(http.MethodGet, http.MethodHead).
		Handler(ui)
}
