package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const keyJSON = `{"id":"k1","name":"ci","scopes":["notes:read"],"usable":true,"createdAt":"2026-01-02T03:04:05Z",` +
	`"lastUsedAt":null,"expiresAt":"2027-01-02T03:04:05Z"`

// keyServer answers the API key operations on the session mount, and /me and
// /notes on the key-only mount.
func keyServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var lastBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		auth := r.Header.Get("Authorization")
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api/v0/api-keys":
			writeJSON(w, http.StatusOK, `{"items":[`+keyJSON+`}]}`)
		case "POST /api/v0/api-keys":
			writeJSON(w, http.StatusCreated, keyJSON+`,"token":"refapp_k1_secret"}`)
		case "GET /api/v0/api-keys/scopes":
			writeJSON(w, http.StatusOK, `{"items":[{"area":"notes","levels":["read","write"]}]}`)
		case "DELETE /api/v0/api-keys/k1":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/v0/api-keys/a%2Fb":
			writeProblem(w, http.StatusNotFound, "not_found", "API key a/b not found")
		case "GET /api/key/v0/me":
			if auth != "Bearer refapp_k1_secret" {
				writeProblem(w, http.StatusUnauthorized, "unauthorized", "a valid API key is required")
				return
			}
			writeJSON(w, http.StatusOK, `{"userId":"u1","subject":"alice","tenantId":"t1","tenant":"acme",`+
				`"apiKey":{"id":"k1","name":"ci","scopes":["notes:read"]}}`)
		case "POST /api/key/v0/notes":
			writeProblem(w, http.StatusForbidden, "forbidden", "this API key is read-only for notes")
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &lastBody
}

func TestAPIKeyManagement(t *testing.T) {
	srv, lastBody := keyServer(t)
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	keys, err := c.APIKeys(ctx)
	if err != nil || len(keys) != 1 || keys[0].ID != "k1" || !keys[0].Usable || keys[0].LastUsedAt != nil {
		t.Fatalf("APIKeys: %+v, %v", keys, err)
	}
	exp := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	created, err := c.CreateAPIKey(ctx, APIKeyInput{Name: "ci", Scopes: []string{"notes:read"}, ExpiresAt: &exp})
	if err != nil || created.Token != "refapp_k1_secret" || created.ID != "k1" {
		t.Fatalf("CreateAPIKey: %+v, %v", created, err)
	}
	if !strings.Contains(*lastBody, `"expiresAt":"2027-01-02T03:04:05Z"`) {
		t.Errorf("create body: %s", *lastBody)
	}
	if _, err := c.CreateAPIKey(ctx, APIKeyInput{Name: "ci", Scopes: []string{"notes:read"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(*lastBody, "expiresAt") {
		t.Errorf("a nil expiry must be left out: %s", *lastBody)
	}
	scopes, err := c.APIKeyScopes(ctx)
	if err != nil || len(scopes) != 1 || scopes[0].Area != "notes" || len(scopes[0].Levels) != 2 {
		t.Fatalf("APIKeyScopes: %+v, %v", scopes, err)
	}
	if err := c.RevokeAPIKey(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	if err := c.RevokeAPIKey(ctx, "a/b"); !errors.Is(err, ErrNotFound) {
		t.Errorf("RevokeAPIKey missing: got %v, want ErrNotFound", err)
	}
}

func TestWithAPIKey(t *testing.T) {
	srv, _ := keyServer(t)
	ctx := context.Background()

	c, err := New(srv.URL+"/", WithAPIKey("refapp_k1_secret"))
	if err != nil {
		t.Fatal(err)
	}
	me, err := c.Me(ctx)
	if err != nil || me.Subject != "alice" || me.APIKey == nil || me.APIKey.ID != "k1" {
		t.Fatalf("Me with key: %+v, %v", me, err)
	}
	_, err = c.CreateNote(ctx, NoteInput{Title: "x"})
	if !errors.Is(err, ErrForbidden) || errors.Is(err, ErrUnauthorized) {
		t.Errorf("read-only key: got %v, want ErrForbidden", err)
	}

	bad, err := New(srv.URL, WithAPIKey("wrong"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bad.Me(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("bad key: got %v, want ErrUnauthorized", err)
	}
}
