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
	"net/netip"

	"github.com/go-bumbu/http/middleware"
	"github.com/go-bumbu/reference-app/app/router/handlers"
	"github.com/gorilla/mux"
)

// Cfg holds the dependencies of the main application handler.
// app/cmd instantiates them and passes the references in.
type Cfg struct {
	Logger *slog.Logger
	// Notes is the notes module (internal/notes); required.
	Notes handlers.NotesService
	// Tags is the notes module's tag tree (internal/notes, the same Service as
	// Notes); required.
	Tags handlers.TagsService
	// Jobs is the jobs module (internal/jobs); required.
	Jobs handlers.JobsService
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
	// Metrics records one observation per request (go-bumbu/http
	// middleware/metrics); nil records no metrics. It registers in the
	// Prometheus registry, so app/cmd creates it once per process.
	Metrics middleware.Observer
	// TrustedProxies are the reverse proxies in front of the server: the
	// request log takes the client ip from X-Forwarded-For, skipping these from
	// the right. Empty logs the direct peer and ignores the header, which any
	// client can forge.
	TrustedProxies []netip.Prefix
}

// New builds the main application handler: the routes, wrapped in the
// go-bumbu/http request middleware (request log, metrics, panic recovery), all
// behind withRequestID, so the request log, the problem log and a problem's
// reference share one id. It returns an error so that wiring which can fail
// (e.g. a missing dependency) is reported at startup instead of panicking on the
// first request.
func New(cfg Cfg) (http.Handler, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	r, err := newRoutes(cfg)
	if err != nil {
		return nil, err
	}
	// The middleware wraps the whole router rather than going through r.Use:
	// gorilla/mux only runs r.Use middleware on a matched route, so 404 and 405
	// would go unlogged.
	mw := middleware.New(middleware.Cfg{
		PanicRecover: true,
		Logger:       cfg.Logger,
		Metrics:      cfg.Metrics,
		RequestID:    requestID,
		ClientIP:     middleware.ClientIPFromXFF(cfg.TrustedProxies...),
	})
	return withRequestID(mw.Wrap(r)), nil
}

// newRoutes validates the dependencies and mounts every route on a gorilla/mux
// router. cfg.Logger must be set (New defaults it).
func newRoutes(cfg Cfg) (*mux.Router, error) {
	logger := cfg.Logger
	if cfg.Notes == nil {
		return nil, errors.New("router: Notes is required")
	}
	if cfg.Tags == nil {
		return nil, errors.New("router: Tags is required")
	}
	if cfg.Jobs == nil {
		return nil, errors.New("router: Jobs is required")
	}
	if cfg.Tenants == nil {
		return nil, errors.New("router: Tenants is required")
	}
	problems, err := newProblems(cfg.Production, logger)
	if err != nil {
		return nil, fmt.Errorf("router: problem writer: %w", err)
	}
	guard, err := newAuthGuard(cfg.Auth, cfg.Tenants, problems, logger)
	if err != nil {
		return nil, fmt.Errorf("router: auth: %w", err)
	}

	r := mux.NewRouter()
	r.Use(reportRoute)
	attachApiV0(r, cfg.Notes, cfg.Tags, cfg.Jobs, guard, problems, logger)
	if cfg.UI != nil {
		attachUI(r, cfg.UI)
	}

	r.NotFoundHandler = notFound(problems)
	r.MethodNotAllowedHandler = methodNotAllowed(problems)
	return r, nil
}

// reportRoute hands gorilla's matched path template (e.g. "/api/v0/notes/{noteId}")
// to the request middleware, which labels the request metrics with it. gorilla/mux
// does not set r.Pattern, so without it every request would record as "unmatched".
// gorilla runs r.Use middleware only after a match, so 404 and 405 report nothing
// and stay "unmatched" — the raw path is never used as a label.
func reportRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if route := mux.CurrentRoute(req); route != nil {
			if tmpl, err := route.GetPathTemplate(); err == nil {
				middleware.SetRoute(req, tmpl)
			}
		}
		next.ServeHTTP(w, req)
	})
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
