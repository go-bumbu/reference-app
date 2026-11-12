package router

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-bumbu/http/middleware"
)

// statusObserver records the status and route label of every request the
// middleware observes.
type statusObserver struct {
	mu       sync.Mutex
	statuses []int
	routes   []string
}

func (o *statusObserver) Observe(status int, r *http.Request, _ time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.statuses = append(o.statuses, status)
	o.routes = append(o.routes, middleware.Route(r))
}

// TestMiddlewareCoversUnmatchedRoutes checks the request middleware wraps the
// whole router: gorilla/mux skips r.Use middleware for 404 and 405, so those
// answers are only logged and measured when the middleware sits outside it.
func TestMiddlewareCoversUnmatchedRoutes(t *testing.T) {
	deps := newDeps(t)
	var logs bytes.Buffer
	obs := &statusObserver{}
	h, err := New(Cfg{
		Logger:  slog.New(slog.NewJSONHandler(&logs, nil)),
		Notes:   deps.notes,
		Tags:    deps.notes,
		Jobs:    deps.jobs,
		Tenants: deps.tenants,
		Metrics: obs,
	})
	if err != nil {
		t.Fatal(err)
	}

	// route is the metric label: gorilla's path template for a matched route
	// (reportRoute), empty ("unmatched") for 404 and 405, never the raw path.
	requests := []struct {
		method string
		path   string
		want   int
		route  string
	}{
		{method: http.MethodGet, path: "/api/v0/notes", want: http.StatusOK, route: "/api/v0/notes"},
		{method: http.MethodGet, path: "/api/v0/notes/42", want: http.StatusNotFound, route: "/api/v0/notes/{noteId}"},
		{method: http.MethodGet, path: "/api/v0/nope", want: http.StatusNotFound, route: ""},
		{method: http.MethodDelete, path: "/api/v0/notes", want: http.StatusMethodNotAllowed, route: ""},
	}
	for _, rq := range requests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(rq.method, rq.path, nil))
		if rec.Code != rq.want {
			t.Fatalf("%s %s: got %d, want %d", rq.method, rq.path, rec.Code, rq.want)
		}
	}

	if len(obs.statuses) != len(requests) {
		t.Fatalf("observed %v, want one observation per request", obs.statuses)
	}
	for i, rq := range requests {
		if obs.statuses[i] != rq.want {
			t.Errorf("observation %d: got %d, want %d", i, obs.statuses[i], rq.want)
		}
		if obs.routes[i] != rq.route {
			t.Errorf("%s %s: route label %q, want %q", rq.method, rq.path, obs.routes[i], rq.route)
		}
	}
	for _, rq := range requests {
		line := `"url":"` + rq.path + `"`
		if !strings.Contains(logs.String(), line) {
			t.Errorf("no request log line for %s %s; logs:\n%s", rq.method, rq.path, logs.String())
		}
	}
}

// logLines decodes the JSON log lines written to buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		lines = append(lines, m)
	}
	return lines
}

// newLoggedRouter builds the router logging JSON into the returned buffer.
func newLoggedRouter(t *testing.T, cfg Cfg) (http.Handler, *bytes.Buffer) {
	t.Helper()
	deps := newDeps(t)
	var logs bytes.Buffer
	cfg.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	cfg.Notes, cfg.Tags, cfg.Jobs, cfg.Tenants = deps.notes, deps.notes, deps.jobs, deps.tenants
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return h, &logs
}

// TestRequestLogClientIP checks the logged ip: X-Forwarded-For only counts when
// the peer is a trusted proxy, so a direct client cannot forge it.
func TestRequestLogClientIP(t *testing.T) {
	tests := map[string]struct {
		trusted []netip.Prefix
		want    string
	}{
		"no trusted proxies logs the peer": {want: "10.0.0.1"},
		"trusted proxy logs the client":    {trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, want: "203.0.113.7"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			h, logs := newLoggedRouter(t, Cfg{TrustedProxies: tc.trusted})
			req := httptest.NewRequest(http.MethodGet, "/api/v0/info", nil)
			req.RemoteAddr = "10.0.0.1:4321"
			req.Header.Set("X-Forwarded-For", "203.0.113.7")
			h.ServeHTTP(httptest.NewRecorder(), req)

			lines := logLines(t, logs)
			if got := lines[len(lines)-1]["ip"]; got != tc.want {
				t.Errorf("ip: got %v, want %q", got, tc.want)
			}
		})
	}
}

// TestProblemLoggedUnmasked checks a masked production problem stays traceable:
// the body has no detail but a reference, which is the Request-Id response
// header, the req-id of the request log line, and the reference of the
// "problem" log line that carries the detail.
func TestProblemLoggedUnmasked(t *testing.T) {
	h, logs := newLoggedRouter(t, Cfg{Production: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v0/notes/42", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ref := rec.Header().Get(requestIDHeader)
	if ref == "" || body["reference"] != ref {
		t.Fatalf("reference: body %v, header %q; want equal and non-empty", body["reference"], ref)
	}
	if _, ok := body["detail"]; ok {
		t.Errorf("masked body leaks detail: %v", body)
	}

	var problem, access map[string]any
	for _, l := range logLines(t, logs) {
		switch {
		case l["msg"] == "problem":
			problem = l
		case l["response-code"] != nil:
			access = l
		}
	}
	if problem == nil || problem["detail"] != "note 42 not found" || problem["reference"] != ref {
		t.Errorf("problem log line: got %v, want detail and reference %q", problem, ref)
	}
	if access == nil || access["req-id"] != ref {
		t.Errorf("request log line: got %v, want req-id %q", access, ref)
	}
}

// TestRequestID checks a well-formed incoming Request-Id is kept, and a
// malformed one is replaced so a client cannot write arbitrary text into logs.
func TestRequestID(t *testing.T) {
	h, _ := newLoggedRouter(t, Cfg{})
	for in, keep := range map[string]bool{
		"proxy-abc.123_X":                      true,
		"":                                     false,
		"bad id\nforged=entry":                 false,
		strings.Repeat("a", maxRequestIDLen+1): false,
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v0/info", nil)
		if in != "" {
			req.Header.Set(requestIDHeader, in)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		got := rec.Header().Get(requestIDHeader)
		if got == "" || (got == in) != keep {
			t.Errorf("incoming %q: got %q, keep=%v", in, got, keep)
		}
	}
}
