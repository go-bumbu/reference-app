package router

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/go-bumbu/reference-app/libs/client"
)

// TestClientAgainstRouter runs libs/client against the real router: the two
// share no types, so this is where a drift between them shows up.
func TestClientAgainstRouter(t *testing.T) {
	r := newTestRouter(t, newDeps(t), false, AuthCfg{
		Method: AuthProxy,
		Proxy:  ProxyAuthCfg{UserHeader: userHeader, TenantHeader: tenantHeader},
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	as := func(tenant, user string) *client.Client {
		t.Helper()
		c, err := client.New(srv.URL, client.WithHeader(userHeader, user), client.WithHeader(tenantHeader, tenant))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	alice := as("acme", "alice")

	if _, err := alice.Info(ctx); err != nil {
		t.Fatalf("info: %v", err)
	}
	me, err := alice.Me(ctx)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if me.Subject != "alice" || me.Tenant != "acme" || me.UserID == "" || me.TenantID == "" {
		t.Errorf("me: got %+v", me)
	}

	created, err := alice.CreateNote(ctx, client.NoteInput{Title: "plan", Body: "text"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.CreatedBy != me.UserID || created.Title != "plan" || created.CreatedAt.IsZero() {
		t.Errorf("create: got %+v", created)
	}
	got, err := alice.GetNote(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != created {
		t.Errorf("get: got %+v, want %+v", got, created)
	}
	list, err := alice.ListNotes(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0] != created {
		t.Errorf("list: got %+v", list)
	}

	// another user of the tenant edits the note: notes are shared
	bob := as("acme", "bob")
	updated, err := bob.UpdateNote(ctx, created.ID, client.NoteInput{Title: "plan v2", Body: "more"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	want := created
	want.Title, want.Body = "plan v2", "more"
	if updated != want {
		t.Errorf("update: got %+v, want %+v", updated, want)
	}
	if _, err := as("globex", "carol").UpdateNote(ctx, created.ID, client.NoteInput{Title: "x"}); !errors.Is(err, client.ErrNotFound) {
		t.Errorf("update another tenant's note: got %v, want ErrNotFound", err)
	}
	if err := as("globex", "carol").DeleteNote(ctx, created.ID); !errors.Is(err, client.ErrNotFound) {
		t.Errorf("delete another tenant's note: got %v, want ErrNotFound", err)
	}
	if err := bob.DeleteNote(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := alice.GetNote(ctx, created.ID); !errors.Is(err, client.ErrNotFound) {
		t.Errorf("get after delete: got %v, want ErrNotFound", err)
	}
	if _, err := alice.UpdateNote(ctx, created.ID, client.NoteInput{Title: "x"}); !errors.Is(err, client.ErrNotFound) {
		t.Errorf("update after delete: got %v, want ErrNotFound", err)
	}
	created, err = alice.CreateNote(ctx, client.NoteInput{Title: "plan", Body: "text"})
	if err != nil {
		t.Fatalf("create again: %v", err)
	}

	// the problem responses map onto the client's errors
	_, err = alice.CreateNote(ctx, client.NoteInput{Title: "  "})
	var p *client.Problem
	if !errors.Is(err, client.ErrValidation) || !errors.As(err, &p) || len(p.Errors) == 0 || p.Errors[0].Pointer != "/title" {
		t.Errorf("empty title: got %v", err)
	}
	if _, err := as("globex", "carol").GetNote(ctx, created.ID); !errors.Is(err, client.ErrNotFound) {
		t.Errorf("another tenant's note: got %v, want ErrNotFound", err)
	}
	anonymous, err := client.New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := anonymous.ListNotes(ctx); !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("no identity: got %v, want ErrUnauthorized", err)
	}
}
