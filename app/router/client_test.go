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
