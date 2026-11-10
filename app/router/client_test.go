package router

import (
	"context"
	"errors"
	"net/http/httptest"
	"reflect"
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
	if !reflect.DeepEqual(got, created) {
		t.Errorf("get: got %+v, want %+v", got, created)
	}
	list, err := alice.ListNotes(ctx, client.ListNotesOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || !reflect.DeepEqual(list[0], created) {
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
	if !reflect.DeepEqual(updated, want) {
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
	if _, err := anonymous(t, srv.URL).ListNotes(ctx, client.ListNotesOptions{}); !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("no identity: got %v, want ErrUnauthorized", err)
	}

	// tags: a tree per tenant, notes listed by tag, the subtree included
	work, err := alice.CreateTag(ctx, client.TagInput{Name: "work"})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	reports, err := bob.CreateTag(ctx, client.TagInput{Name: "reports", ParentID: work.ID})
	if err != nil {
		t.Fatalf("create child tag: %v", err)
	}
	if _, err := alice.CreateTag(ctx, client.TagInput{Name: "work"}); !errors.Is(err, client.ErrConflict) {
		t.Errorf("a sibling's name: got %v, want ErrConflict", err)
	}
	tagged, err := alice.AddNoteTag(ctx, created.ID, reports.ID)
	if err != nil {
		t.Fatalf("tag the note: %v", err)
	}
	if !reflect.DeepEqual(tagged.TagIDs, []uint{reports.ID}) {
		t.Errorf("tagged note: got tags %v", tagged.TagIDs)
	}
	underWork, err := bob.ListNotes(ctx, client.ListNotesOptions{Tag: work.ID})
	if err != nil {
		t.Fatalf("list by tag: %v", err)
	}
	if len(underWork) != 1 || underWork[0].ID != created.ID {
		t.Errorf("list by tag: got %+v", underWork)
	}
	name := "papers"
	renamed, err := alice.UpdateTag(ctx, reports.ID, client.TagPatch{Name: &name})
	if err != nil || renamed.Name != "papers" {
		t.Errorf("rename tag: got %+v, %v", renamed, err)
	}
	tree, err := alice.Tags(ctx)
	if err != nil {
		t.Fatalf("tags: %v", err)
	}
	if len(tree) != 1 || len(tree[0].Children) != 1 || tree[0].Children[0].Name != "papers" {
		t.Errorf("tags: got %+v", tree)
	}
	if foreign, err := as("globex", "carol").Tags(ctx); err != nil || len(foreign) != 0 {
		t.Errorf("another tenant's tags: got %+v, %v", foreign, err)
	}
	if err := as("globex", "carol").DeleteTag(ctx, work.ID); !errors.Is(err, client.ErrNotFound) {
		t.Errorf("delete another tenant's tag: got %v, want ErrNotFound", err)
	}
	if _, err := alice.RemoveNoteTag(ctx, created.ID, reports.ID); err != nil {
		t.Errorf("untag the note: %v", err)
	}
	if err := alice.DeleteTag(ctx, work.ID); err != nil {
		t.Errorf("delete tag: %v", err)
	}
}

// anonymous returns a client that sends no identity.
func anonymous(t *testing.T, url string) *client.Client {
	t.Helper()
	c, err := client.New(url)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
