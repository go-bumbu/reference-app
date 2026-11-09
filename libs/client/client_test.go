package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeServer answers the API paths with canned responses and records the last
// request it saw.
type fakeServer struct {
	t    *testing.T
	last *http.Request
	body string
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.last = r
	b, _ := io.ReadAll(r.Body)
	f.body = string(b)
	switch r.Method + " " + r.URL.EscapedPath() {
	case "GET /base/api/v0/info":
		writeJSON(w, http.StatusOK, `{"version":"v1.2.3","buildTime":"today","commit":"abc","goVersion":"go1.26"}`)
	case "GET /base/api/v0/me":
		if r.Header.Get("Remote-User") == "" {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		writeJSON(w, http.StatusOK, `{"userId":"u1","subject":"`+r.Header.Get("Remote-User")+`","tenantId":"t1","tenant":"acme"}`)
	case "GET /base/api/v0/notes":
		writeJSON(w, http.StatusOK, `{"items":[{"id":"n1","createdBy":"u1","title":"hi","body":"","createdAt":"2026-01-02T03:04:05Z"}]}`)
	case "POST /base/api/v0/notes":
		writeJSON(w, http.StatusCreated, `{"id":"n2","createdBy":"u1","title":"new","body":"text","createdAt":"2026-01-02T03:04:05Z"}`)
	case "GET /base/api/v0/notes/n1":
		writeJSON(w, http.StatusOK, `{"id":"n1","createdBy":"u1","title":"hi","body":"","createdAt":"2026-01-02T03:04:05Z"}`)
	case "PUT /base/api/v0/notes/n1":
		writeJSON(w, http.StatusOK, `{"id":"n1","createdBy":"u1","title":"edited","body":"more","createdAt":"2026-01-02T03:04:05Z"}`)
	case "DELETE /base/api/v0/notes/n1":
		w.WriteHeader(http.StatusNoContent)
	case "GET /base/api/v0/notes/a%2Fb", "DELETE /base/api/v0/notes/a%2Fb":
		writeProblem(w, http.StatusNotFound, "not_found", "note a/b not found")
	case "GET /base/api/v0/notes/bad":
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"type":"https://x/probs/validation_error","title":"Validation error","status":422,`+
			`"errors":[{"pointer":"/title","detail":"must not be empty"}]}`)
	case "GET /base/api/v0/notes/proxy-down":
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "<html>"+strings.Repeat("é", maxDetailLen)+"</html>")
	case "GET /base/api/v0/notes/garbled":
		writeJSON(w, http.StatusOK, `{"id":`)
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body) //nolint:gosec // G705: a JSON test fixture, never rendered as HTML
}

func writeProblem(w http.ResponseWriter, status int, slug, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]any{
		"type": "https://x/probs/" + slug, "title": http.StatusText(status), "status": status, "detail": detail,
	})
	_, _ = w.Write(b)
}

// newTestClient serves the fake under a /base prefix, so the tests also cover a
// server mounted below the root.
func newTestClient(t *testing.T, opts ...Option) (*Client, *fakeServer) {
	t.Helper()
	fake := &fakeServer{t: t}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL+"/base/", opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c, fake
}

func TestInfo(t *testing.T) {
	c, _ := newTestClient(t)
	got, err := c.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Info{Version: "v1.2.3", BuildTime: "today", Commit: "abc", GoVersion: "go1.26"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMeSendsHeaders(t *testing.T) {
	c, fake := newTestClient(t, WithHeader("Remote-User", "alice"))
	got, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "alice" || got.Tenant != "acme" {
		t.Errorf("got %+v", got)
	}
	if accept := fake.last.Header.Get("Accept"); !strings.Contains(accept, "application/problem+json") {
		t.Errorf("accept: got %q", accept)
	}
}

func TestUnauthorized(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.Me(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
	var p *Problem
	if !errors.As(err, &p) || p.Slug() != "unauthorized" || p.Detail != "authentication required" {
		t.Errorf("problem: got %+v", p)
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("a 401 must not match ErrNotFound")
	}
}

func TestNotes(t *testing.T) {
	c, fake := newTestClient(t)
	ctx := context.Background()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	list, err := c.ListNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "n1" || !list[0].CreatedAt.Equal(created) {
		t.Errorf("list: got %+v", list)
	}

	n, err := c.GetNote(ctx, "n1")
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "n1" || n.CreatedBy != "u1" {
		t.Errorf("get: got %+v", n)
	}

	n, err = c.CreateNote(ctx, NoteInput{Title: "new", Body: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "n2" {
		t.Errorf("create: got %+v", n)
	}
	if ct := fake.last.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("create content type: got %q", ct)
	}
	if fake.body != `{"title":"new","body":"text"}` {
		t.Errorf("create body: got %s", fake.body)
	}

	n, err = c.UpdateNote(ctx, "n1", NoteInput{Title: "edited", Body: "more"})
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "n1" || n.Title != "edited" {
		t.Errorf("update: got %+v", n)
	}
	if fake.body != `{"title":"edited","body":"more"}` {
		t.Errorf("update body: got %s", fake.body)
	}

	if err := c.DeleteNote(ctx, "n1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := c.DeleteNote(ctx, "a/b"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: got %v, want ErrNotFound", err)
	}
}

func TestGetNoteEscapesTheID(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GetNote(context.Background(), "a/b")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestValidationProblem(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GetNote(context.Background(), "bad")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("got %v, want ErrValidation", err)
	}
	var p *Problem
	if !errors.As(err, &p) || len(p.Errors) != 1 || p.Errors[0].Pointer != "/title" {
		t.Errorf("problem: got %+v", p)
	}
}

// TestNonProblemError covers an error page that is not problem+json, e.g. from
// a proxy: the status survives and the body is cut to valid UTF-8.
func TestNonProblemError(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GetNote(context.Background(), "proxy-down")
	var p *Problem
	if !errors.As(err, &p) {
		t.Fatalf("got %v, want a *Problem", err)
	}
	if p.Status != http.StatusBadGateway || p.Title != "Bad Gateway" {
		t.Errorf("got status %d title %q", p.Status, p.Title)
	}
	if len(p.Detail) > maxDetailLen || !strings.HasPrefix(p.Detail, "<html>") {
		t.Errorf("detail not truncated: %d bytes", len(p.Detail))
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error message: %q", err.Error())
	}
}

func TestDecodeError(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GetNote(context.Background(), "garbled")
	var p *Problem
	if err == nil || errors.As(err, &p) {
		t.Fatalf("got %v, want a decode error", err)
	}
}

func TestTransportError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listens any more
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Info(context.Background()); err == nil {
		t.Fatal("expected a transport error")
	}
}

func TestWithHTTPClient(t *testing.T) {
	var used bool
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})}
	c, _ := newTestClient(t, WithHTTPClient(hc))
	if _, err := c.Info(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Error("the custom http.Client was not used")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewRejectsBadBaseURLs(t *testing.T) {
	for _, base := range []string{"", "localhost:8080", "ftp://host", "http://", "http://host?x=1", "http://host#f", "http://h\x7f"} {
		if _, err := New(base); err == nil {
			t.Errorf("New(%q): expected an error", base)
		}
	}
}
