package router

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// statusObserver records the status of every request the middleware observes.
type statusObserver struct {
	mu       sync.Mutex
	statuses []int
}

func (o *statusObserver) Observe(status int, _ *http.Request, _ time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.statuses = append(o.statuses, status)
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

	requests := []struct {
		method string
		path   string
		want   int
	}{
		{method: http.MethodGet, path: "/api/v0/notes", want: http.StatusOK},
		{method: http.MethodGet, path: "/api/v0/nope", want: http.StatusNotFound},
		{method: http.MethodDelete, path: "/api/v0/notes", want: http.StatusMethodNotAllowed},
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
	}
	for _, rq := range requests {
		line := `"url":"` + rq.path + `"`
		if !strings.Contains(logs.String(), line) {
			t.Errorf("no request log line for %s %s; logs:\n%s", rq.method, rq.path, logs.String())
		}
	}
}
