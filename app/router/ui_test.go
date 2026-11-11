package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUIIsTheCatchAllOutsideTheAPI checks the UI gets every GET outside the
// API, while unknown API paths keep their problem+json answers.
func TestUIIsTheCatchAllOutsideTheAPI(t *testing.T) {
	deps := newDeps(t)
	ui := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ui"))
	})
	h, err := New(Cfg{Notes: deps.notes, Tags: deps.notes, Jobs: deps.jobs, Tenants: deps.tenants, UI: ui})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		method   string
		path     string
		wantCode int
		wantUI   bool
	}{
		{method: http.MethodGet, path: "/", wantCode: http.StatusOK, wantUI: true},
		{method: http.MethodGet, path: "/notes/abc", wantCode: http.StatusOK, wantUI: true},
		{method: http.MethodHead, path: "/", wantCode: http.StatusOK, wantUI: true},
		{method: http.MethodGet, path: "/api/v0/notes", wantCode: http.StatusOK},
		{method: http.MethodGet, path: "/api/v0/nope", wantCode: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/v0", wantCode: http.StatusNotFound},
		{method: http.MethodDelete, path: "/api/v0/notes", wantCode: http.StatusMethodNotAllowed},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.wantCode {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantCode)
			}
			if gotUI := rec.Body.String() == "ui"; gotUI != tc.wantUI {
				t.Errorf("served by the UI: got %v, want %v (body %q)", gotUI, tc.wantUI, rec.Body.String())
			}
			if !tc.wantUI && rec.Code >= 400 && rec.Header().Get("Content-Type") != "application/problem+json" {
				t.Errorf("content type: got %q, want problem+json", rec.Header().Get("Content-Type"))
			}
		})
	}
}
