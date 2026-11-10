package cmd

import (
	"context"
	"testing"

	"github.com/go-bumbu/db/gormdb/gormdbtest"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"github.com/go-bumbu/reference-app/app/router"
	"github.com/go-bumbu/reference-app/internal/notes"
	"github.com/go-bumbu/reference-app/internal/tenants"
)

// TestSeedDemoData checks the demo data lands in the tenant of auth method
// none, with the tag tree in the declared order and every note tagged.
func TestSeedDemoData(t *testing.T) {
	ctx := context.Background()
	db := gormdbtest.Sqlite(t, sqlite.Driver{})
	tenantsSvc, err := tenants.New(ctx, db, tenants.Cfg{})
	if err != nil {
		t.Fatalf("tenants.New: %v", err)
	}
	notesSvc, err := notes.New(ctx, db, notes.Cfg{})
	if err != nil {
		t.Fatalf("notes.New: %v", err)
	}

	n, err := seedDemoData(ctx, tenantsSvc, notesSvc)
	if err != nil {
		t.Fatalf("seedDemoData: %v", err)
	}
	if n != len(demoNotes) {
		t.Errorf("notes added: got %d, want %d", n, len(demoNotes))
	}

	p, err := tenantsSvc.Provision(ctx, router.DefaultTenant, router.DefaultUser)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	a := notes.Actor{TenantID: p.TenantID, UserID: p.UserID}

	tree, err := notesSvc.Tags(ctx, a)
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if got, want := tagNames(tree), tagNames(toTags(demoTags)); got != want {
		t.Errorf("tag tree: got %s, want %s", got, want)
	}

	got, err := notesSvc.List(ctx, a, notes.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != len(demoNotes) {
		t.Fatalf("notes listed: got %d, want %d", len(got), len(demoNotes))
	}
	tagged := 0
	for _, note := range got {
		tagged += len(note.TagIDs)
	}
	want := 0
	for _, dn := range demoNotes {
		want += len(dn.tags)
	}
	if tagged != want {
		t.Errorf("note tags: got %d, want %d", tagged, want)
	}
}

func toTags(dts []demoTag) []notes.Tag {
	out := make([]notes.Tag, 0, len(dts))
	for _, dt := range dts {
		out = append(out, notes.Tag{Name: dt.name, Children: toTags(dt.children)})
	}
	return out
}

// tagNames renders a tree as "a(b,c),d" so order and nesting compare at once.
func tagNames(tags []notes.Tag) string {
	s := ""
	for i, tag := range tags {
		if i > 0 {
			s += ","
		}
		s += tag.Name
		if len(tag.Children) > 0 {
			s += "(" + tagNames(tag.Children) + ")"
		}
	}
	return s
}
