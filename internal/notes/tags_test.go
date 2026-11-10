package notes

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-bumbu/db/gormdb/gormdbtest"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"github.com/go-bumbu/testdbs"
	"gorm.io/gorm"
)

func ptr[T any](v T) *T { return &v }

// mustTag creates a tag or fails the test.
func mustTag(t *testing.T, svc *Service, a Actor, name string, parentID uint) Tag {
	t.Helper()
	tag, err := svc.CreateTag(context.Background(), a, TagInput{Name: name, ParentID: parentID})
	if err != nil {
		t.Fatalf("create tag %q: %v", name, err)
	}
	return tag
}

// mustNote creates a note or fails the test.
func mustNote(t *testing.T, svc *Service, a Actor, title string) Note {
	t.Helper()
	n, err := svc.Create(context.Background(), a, Input{Title: title})
	if err != nil {
		t.Fatalf("create note %q: %v", title, err)
	}
	return n
}

// outline renders a tree as "a(b,c),d", to compare shapes and order at a glance.
func outline(tags []Tag) string {
	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		part := tag.Name
		if len(tag.Children) > 0 {
			part += "(" + outline(tag.Children) + ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ",")
}

func treeOutline(t *testing.T, svc *Service, a Actor) string {
	t.Helper()
	tree, err := svc.Tags(context.Background(), a)
	if err != nil {
		t.Fatalf("tags: %v", err)
	}
	return outline(tree)
}

func wantValidation(t *testing.T, err error, field string) {
	t.Helper()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %v, want a *ValidationError on %s", err, field)
	}
	if verr.Field != field {
		t.Errorf("field: got %q, want %q", verr.Field, field)
	}
}

func TestTagTree(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})

		empty, err := svc.Tags(ctx, acme)
		if err != nil {
			t.Fatal(err)
		}
		if empty == nil || len(empty) != 0 {
			t.Errorf("empty tree: got %#v, want a non-nil empty slice", empty)
		}

		work := mustTag(t, svc, acme, "  work ", 0)
		if work.Name != "work" || work.ParentID != 0 || work.ID == 0 || work.Children == nil {
			t.Errorf("created tag: got %+v", work)
		}
		home := mustTag(t, svc, acme, "home", 0)
		mustTag(t, svc, acme, "reports", work.ID)
		meetings, err := svc.CreateTag(ctx, acme, TagInput{Name: "meetings", ParentID: work.ID})
		if err != nil {
			t.Fatal(err)
		}
		if meetings.ParentID != work.ID {
			t.Errorf("parent: got %d, want %d", meetings.ParentID, work.ID)
		}
		// after a sibling, instead of first
		if _, err := svc.CreateTag(ctx, acme, TagInput{Name: "garden", ParentID: home.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CreateTag(ctx, acme, TagInput{Name: "last", AfterID: ptr(work.ID)}); err != nil {
			t.Fatal(err)
		}

		// a new tag goes first among its siblings unless placed after one
		if got, want := treeOutline(t, svc, acme), "home(garden),work(meetings,reports),last"; got != want {
			t.Errorf("tree: got %s, want %s", got, want)
		}

		// the same name under another parent is fine
		mustTag(t, svc, acme, "garden", work.ID)
	})
}

func TestCreateTagValidation(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		globex := Actor{TenantID: "tenant-globex", UserID: "user-carol"}
		work := mustTag(t, svc, acme, "work", 0)
		home := mustTag(t, svc, acme, "home", 0)
		foreign := mustTag(t, svc, globex, "foreign", 0)

		tests := []struct {
			name      string
			in        TagInput
			wantField string
		}{
			{name: "empty name", in: TagInput{Name: "  "}, wantField: "name"},
			{name: "name too long", in: TagInput{Name: strings.Repeat("a", MaxTagNameLen+1)}, wantField: "name"},
			{name: "missing parent", in: TagInput{Name: "x", ParentID: 99999}, wantField: "parentId"},
			{name: "parent of another tenant", in: TagInput{Name: "x", ParentID: foreign.ID}, wantField: "parentId"},
			{name: "after a tag under another parent", in: TagInput{Name: "x", ParentID: work.ID, AfterID: ptr(home.ID)},
				wantField: "afterId"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := svc.CreateTag(ctx, acme, tc.in)
				wantValidation(t, err, tc.wantField)
			})
		}

		if _, err := svc.CreateTag(ctx, acme, TagInput{Name: " work "}); !errors.Is(err, ErrTagExists) {
			t.Errorf("sibling's name: got %v, want ErrTagExists", err)
		}
		// nothing invalid was stored, not even by the create refused after its insert
		if got, want := treeOutline(t, svc, acme), "home,work"; got != want {
			t.Errorf("tree: got %s, want %s", got, want)
		}
	})
}

// TestTagNameMaxLengthFits stores the longest valid name: the column must hold
// what validation lets through, multi-byte characters included.
func TestTagNameMaxLengthFits(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		name := strings.Repeat("é", MaxTagNameLen)
		mustTag(t, svc, acme, name, 0)
		if got := treeOutline(t, svc, acme); got != name {
			t.Errorf("the stored name was changed: %q", got)
		}
	})
}

func TestUpdateTag(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		home := mustTag(t, svc, acme, "home", 0)
		work := mustTag(t, svc, acme, "work", 0)
		reports := mustTag(t, svc, acme, "reports", work.ID)
		q1 := mustTag(t, svc, acme, "q1", reports.ID)
		// work(reports(q1)),home

		renamed, err := svc.UpdateTag(ctx, acme, reports.ID, TagPatch{Name: ptr(" papers ")})
		if err != nil {
			t.Fatalf("rename: %v", err)
		}
		want := Tag{ID: reports.ID, ParentID: work.ID, Name: "papers",
			Children: []Tag{{ID: q1.ID, ParentID: reports.ID, Name: "q1", Children: []Tag{}}}}
		if !reflect.DeepEqual(renamed, want) {
			t.Errorf("rename: got %+v, want %+v", renamed, want)
		}

		// the subtree moves with the tag
		moved, err := svc.UpdateTag(ctx, acme, reports.ID, TagPatch{ParentID: ptr(home.ID)})
		if err != nil {
			t.Fatalf("move: %v", err)
		}
		if moved.ParentID != home.ID || len(moved.Children) != 1 {
			t.Errorf("move: got %+v", moved)
		}
		if got, want := treeOutline(t, svc, acme), "work,home(papers(q1))"; got != want {
			t.Errorf("after move: got %s, want %s", got, want)
		}

		// to the root, after a sibling
		if _, err := svc.UpdateTag(ctx, acme, q1.ID, TagPatch{ParentID: ptr(uint(0)), AfterID: ptr(work.ID)}); err != nil {
			t.Fatalf("move to root: %v", err)
		}
		if got, want := treeOutline(t, svc, acme), "work,q1,home(papers)"; got != want {
			t.Errorf("after move to root: got %s, want %s", got, want)
		}

		// reorder alone, and first among the siblings
		if _, err := svc.UpdateTag(ctx, acme, home.ID, TagPatch{AfterID: ptr(uint(0))}); err != nil {
			t.Fatalf("reorder: %v", err)
		}
		if got, want := treeOutline(t, svc, acme), "home(papers),work,q1"; got != want {
			t.Errorf("after reorder: got %s, want %s", got, want)
		}

		// a rename to its own name, a move to its own parent and an empty patch change nothing
		for _, p := range []TagPatch{{Name: ptr("home")}, {ParentID: ptr(uint(0))}, {}} {
			got, err := svc.UpdateTag(ctx, acme, home.ID, p)
			if err != nil {
				t.Fatalf("no-op patch %+v: %v", p, err)
			}
			if got.Name != "home" || got.ParentID != 0 {
				t.Errorf("no-op patch %+v: got %+v", p, got)
			}
		}
		if got, want := treeOutline(t, svc, acme), "home(papers),work,q1"; got != want {
			t.Errorf("after no-op patches: got %s, want %s", got, want)
		}
	})
}

func TestUpdateTagRefusals(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		globex := Actor{TenantID: "tenant-globex", UserID: "user-carol"}
		home := mustTag(t, svc, acme, "home", 0)
		work := mustTag(t, svc, acme, "work", 0)
		reports := mustTag(t, svc, acme, "reports", work.ID)
		foreign := mustTag(t, svc, globex, "foreign", 0)
		if got, want := treeOutline(t, svc, acme), "work(reports),home"; got != want {
			t.Fatalf("setup: got %s, want %s", got, want)
		}

		tests := []struct {
			name      string
			id        uint
			patch     TagPatch
			wantField string
		}{
			{name: "empty name", id: work.ID, patch: TagPatch{Name: ptr(" ")}, wantField: "name"},
			{name: "under itself", id: work.ID, patch: TagPatch{ParentID: ptr(work.ID)}, wantField: "parentId"},
			{name: "under its descendant", id: work.ID, patch: TagPatch{ParentID: ptr(reports.ID)}, wantField: "parentId"},
			{name: "under a missing tag", id: work.ID, patch: TagPatch{ParentID: ptr(uint(99999))}, wantField: "parentId"},
			{name: "under another tenant's tag", id: work.ID, patch: TagPatch{ParentID: ptr(foreign.ID)},
				wantField: "parentId"},
			{name: "after a tag of another parent", id: reports.ID, patch: TagPatch{AfterID: ptr(home.ID)},
				wantField: "afterId"},
			{name: "after itself", id: work.ID, patch: TagPatch{AfterID: ptr(work.ID)}, wantField: "afterId"},
			// the whole patch or nothing: the valid rename is not stored either
			{name: "valid name, invalid move", id: work.ID, patch: TagPatch{Name: ptr("job"), ParentID: ptr(reports.ID)},
				wantField: "parentId"},
			{name: "valid move, invalid order", id: reports.ID, patch: TagPatch{ParentID: ptr(home.ID), AfterID: ptr(work.ID)},
				wantField: "afterId"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := svc.UpdateTag(ctx, acme, tc.id, tc.patch)
				wantValidation(t, err, tc.wantField)
			})
		}

		if _, err := svc.UpdateTag(ctx, acme, home.ID, TagPatch{Name: ptr("work")}); !errors.Is(err, ErrTagExists) {
			t.Errorf("rename to a sibling's name: got %v, want ErrTagExists", err)
		}
		mustTag(t, svc, acme, "reports", home.ID)
		if _, err := svc.UpdateTag(ctx, acme, reports.ID, TagPatch{ParentID: ptr(home.ID)}); !errors.Is(err, ErrTagExists) {
			t.Errorf("move next to a namesake: got %v, want ErrTagExists", err)
		}
		if _, err := svc.UpdateTag(ctx, acme, 99999, TagPatch{Name: ptr("x")}); !errors.Is(err, ErrTagNotFound) {
			t.Errorf("missing tag: got %v, want ErrTagNotFound", err)
		}
		if _, err := svc.UpdateTag(ctx, acme, foreign.ID, TagPatch{Name: ptr("x")}); !errors.Is(err, ErrTagNotFound) {
			t.Errorf("another tenant's tag: got %v, want ErrTagNotFound", err)
		}
		if got, want := treeOutline(t, svc, acme), "work(reports),home(reports)"; got != want {
			t.Errorf("a refused patch changed the tree: got %s, want %s", got, want)
		}
	})
}

// TestTagOrderSurvivesManyReorders places a tag between the same two
// siblings far more often than float64 halvings allow: the order is respaced
// on the way and stays right.
func TestTagOrderSurvivesManyReorders(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		c := mustTag(t, svc, acme, "c", 0)
		b := mustTag(t, svc, acme, "b", 0)
		a := mustTag(t, svc, acme, "a", 0)
		// a,b,c: move c and b, in turn, right after a
		for i := range 80 {
			id, want := c.ID, "a,c,b"
			if i%2 == 1 {
				id, want = b.ID, "a,b,c"
			}
			if _, err := svc.UpdateTag(ctx, acme, id, TagPatch{AfterID: ptr(a.ID)}); err != nil {
				t.Fatalf("reorder %d: %v", i, err)
			}
			if got := treeOutline(t, svc, acme); got != want {
				t.Fatalf("reorder %d: got %s, want %s", i, got, want)
			}
		}
	})
}

func TestDeleteTag(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		work := mustTag(t, svc, acme, "work", 0)
		reports := mustTag(t, svc, acme, "reports", work.ID)
		home := mustTag(t, svc, acme, "home", 0)
		note := mustNote(t, svc, acme, "plan")
		for _, id := range []uint{reports.ID, home.ID} {
			if _, err := svc.AddNoteTag(ctx, acme, note.ID, id); err != nil {
				t.Fatal(err)
			}
		}

		if err := svc.DeleteTag(ctx, acme, work.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if got := treeOutline(t, svc, acme); got != "home" {
			t.Errorf("tree after delete: got %s, want home", got)
		}
		got, err := svc.Get(ctx, acme, note.ID)
		if err != nil {
			t.Fatalf("the note went with its tag: %v", err)
		}
		if !reflect.DeepEqual(got.TagIDs, []uint{home.ID}) {
			t.Errorf("tags after delete: got %v, want [%d]", got.TagIDs, home.ID)
		}
		var links int64
		if err := db.Table("note_tag_links").Where("tag_id IN ?", []uint{work.ID, reports.ID}).Count(&links).Error; err != nil {
			t.Fatal(err)
		}
		if links != 0 {
			t.Errorf("%d links to the deleted tags were left behind", links)
		}
		if err := svc.DeleteTag(ctx, acme, work.ID); !errors.Is(err, ErrTagNotFound) {
			t.Errorf("second delete: got %v, want ErrTagNotFound", err)
		}
	})
}

func TestNoteTags(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		work := mustTag(t, svc, acme, "work", 0)
		home := mustTag(t, svc, acme, "home", 0)
		note := mustNote(t, svc, acme, "plan")
		if note.TagIDs == nil || len(note.TagIDs) != 0 {
			t.Errorf("new note: got tags %#v, want a non-nil empty slice", note.TagIDs)
		}

		// added in any order, listed ascending; adding twice changes nothing
		for _, id := range []uint{home.ID, work.ID, home.ID} {
			if _, err := svc.AddNoteTag(ctx, acme, note.ID, id); err != nil {
				t.Fatalf("add %d: %v", id, err)
			}
		}
		wantIDs := []uint{min(work.ID, home.ID), max(work.ID, home.ID)}
		got, err := svc.Get(ctx, acme, note.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.TagIDs, wantIDs) {
			t.Errorf("get: got tags %v, want %v", got.TagIDs, wantIDs)
		}
		list, err := svc.List(ctx, acme, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || !reflect.DeepEqual(list[0].TagIDs, wantIDs) {
			t.Errorf("list: got %+v, want tags %v", list, wantIDs)
		}
		updated, err := svc.Update(ctx, acme, note.ID, Input{Title: "plan v2"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(updated.TagIDs, wantIDs) {
			t.Errorf("update: got tags %v, want %v", updated.TagIDs, wantIDs)
		}

		// removing twice changes nothing either
		for range 2 {
			removed, err := svc.RemoveNoteTag(ctx, acme, note.ID, work.ID)
			if err != nil {
				t.Fatalf("remove: %v", err)
			}
			if !reflect.DeepEqual(removed.TagIDs, []uint{home.ID}) {
				t.Errorf("remove: got tags %v, want [%d]", removed.TagIDs, home.ID)
			}
		}

		if _, err := svc.AddNoteTag(ctx, acme, "missing", work.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("missing note: got %v, want ErrNotFound", err)
		}
		if _, err := svc.AddNoteTag(ctx, acme, note.ID, 99999); !errors.Is(err, ErrTagNotFound) {
			t.Errorf("missing tag: got %v, want ErrTagNotFound", err)
		}
		if _, err := svc.RemoveNoteTag(ctx, acme, note.ID, 99999); !errors.Is(err, ErrTagNotFound) {
			t.Errorf("remove a missing tag: got %v, want ErrTagNotFound", err)
		}

		// deleting the note takes its links along
		if err := svc.Delete(ctx, acme, note.ID); err != nil {
			t.Fatal(err)
		}
		var links int64
		if err := db.Table("note_tag_links").Where("note_id = ?", note.ID).Count(&links).Error; err != nil {
			t.Fatal(err)
		}
		if links != 0 {
			t.Errorf("%d links of the deleted note were left behind", links)
		}
	})
}

func TestListByTag(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{Now: fixedClockAt2026()})
		work := mustTag(t, svc, acme, "work", 0)
		reports := mustTag(t, svc, acme, "reports", work.ID)
		q1 := mustTag(t, svc, acme, "q1", reports.ID)
		home := mustTag(t, svc, acme, "home", 0)

		tagged := map[string][]uint{
			"on work":        {work.ID},
			"on q1":          {q1.ID},
			"on q1 and work": {q1.ID, work.ID}, // listed once
			"on home":        {home.ID},
			"untagged":       nil,
		}
		for _, title := range []string{"on work", "on q1", "on q1 and work", "on home", "untagged"} {
			n := mustNote(t, svc, acme, title)
			for _, id := range tagged[title] {
				if _, err := svc.AddNoteTag(ctx, acme, n.ID, id); err != nil {
					t.Fatal(err)
				}
			}
		}

		tests := []struct {
			tag  uint
			want string
		}{
			{tag: work.ID, want: "on q1 and work,on q1,on work"},
			{tag: reports.ID, want: "on q1 and work,on q1"},
			{tag: q1.ID, want: "on q1 and work,on q1"},
			{tag: home.ID, want: "on home"},
			{tag: 99999, want: ""},
			{tag: 0, want: "untagged,on home,on q1 and work,on q1,on work"},
		}
		for _, tc := range tests {
			t.Run(fmt.Sprint(tc.tag), func(t *testing.T) {
				list, err := svc.List(ctx, acme, ListOptions{TagID: tc.tag})
				if err != nil {
					t.Fatal(err)
				}
				var titles []string
				for _, n := range list {
					titles = append(titles, n.Title)
				}
				if got := strings.Join(titles, ","); got != tc.want {
					t.Errorf("got %s, want %s", got, tc.want)
				}
			})
		}
	})
}

func fixedClockAt2026() func() time.Time {
	return fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

// TestTagTenantIsolation checks a tenant's tags are invisible to, and out of
// reach of, every other tenant.
func TestTagTenantIsolation(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		globex := Actor{TenantID: "tenant-globex", UserID: "user-carol"}
		work := mustTag(t, svc, acme, "work", 0)
		acmeNote := mustNote(t, svc, acme, "acme note")
		globexNote := mustNote(t, svc, globex, "globex note")
		if _, err := svc.AddNoteTag(ctx, acme, acmeNote.ID, work.ID); err != nil {
			t.Fatal(err)
		}

		if got := treeOutline(t, svc, globex); got != "" {
			t.Errorf("another tenant sees the tags %s", got)
		}
		// the same name is free in another tenant
		mustTag(t, svc, globex, "work", 0)
		if _, err := svc.UpdateTag(ctx, globex, work.ID, TagPatch{Name: ptr("x")}); !errors.Is(err, ErrTagNotFound) {
			t.Errorf("update: got %v, want ErrTagNotFound", err)
		}
		if err := svc.DeleteTag(ctx, globex, work.ID); !errors.Is(err, ErrTagNotFound) {
			t.Errorf("delete: got %v, want ErrTagNotFound", err)
		}
		if _, err := svc.AddNoteTag(ctx, globex, globexNote.ID, work.ID); !errors.Is(err, ErrTagNotFound) {
			t.Errorf("tag a note with another tenant's tag: got %v, want ErrTagNotFound", err)
		}
		if _, err := svc.AddNoteTag(ctx, globex, acmeNote.ID, work.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("tag another tenant's note: got %v, want ErrNotFound", err)
		}
		if _, err := svc.RemoveNoteTag(ctx, globex, acmeNote.ID, work.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("untag another tenant's note: got %v, want ErrNotFound", err)
		}
		list, err := svc.List(ctx, globex, ListOptions{TagID: work.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 0 {
			t.Errorf("another tenant lists %d notes under the tag, want 0", len(list))
		}
		if got := treeOutline(t, svc, acme); got != "work" {
			t.Errorf("another tenant changed the tree: %s", got)
		}
	})
}

// TestTagsNoTenantFailsClosed checks a tag call without a tenant is refused.
func TestTagsNoTenantFailsClosed(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		none := Actor{UserID: "user-alice"}
		if _, err := svc.Tags(ctx, none); !errors.Is(err, ErrNoTenant) {
			t.Errorf("tags: got %v, want ErrNoTenant", err)
		}
		if _, err := svc.CreateTag(ctx, none, TagInput{Name: "x"}); !errors.Is(err, ErrNoTenant) {
			t.Errorf("create: got %v, want ErrNoTenant", err)
		}
		if _, err := svc.UpdateTag(ctx, none, 1, TagPatch{}); !errors.Is(err, ErrNoTenant) {
			t.Errorf("update: got %v, want ErrNoTenant", err)
		}
		if err := svc.DeleteTag(ctx, none, 1); !errors.Is(err, ErrNoTenant) {
			t.Errorf("delete: got %v, want ErrNoTenant", err)
		}
		if _, err := svc.AddNoteTag(ctx, none, "x", 1); !errors.Is(err, ErrNoTenant) {
			t.Errorf("add note tag: got %v, want ErrNoTenant", err)
		}
		if _, err := svc.RemoveNoteTag(ctx, none, "x", 1); !errors.Is(err, ErrNoTenant) {
			t.Errorf("remove note tag: got %v, want ErrNoTenant", err)
		}
	})
}

// TestTagsCancelledContext checks the context reaches the database for the tag calls.
func TestTagsCancelledContext(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := svc.Tags(ctx, acme); !errors.Is(err, context.Canceled) {
			t.Errorf("tags: got %v, want context.Canceled", err)
		}
		if _, err := svc.CreateTag(ctx, acme, TagInput{Name: "x"}); !errors.Is(err, context.Canceled) {
			t.Errorf("create: got %v, want context.Canceled", err)
		}
		if _, err := svc.UpdateTag(ctx, acme, 1, TagPatch{Name: ptr("x")}); !errors.Is(err, context.Canceled) {
			t.Errorf("update: got %v, want context.Canceled", err)
		}
		if err := svc.DeleteTag(ctx, acme, 1); !errors.Is(err, context.Canceled) {
			t.Errorf("delete: got %v, want context.Canceled", err)
		}
		if _, err := svc.AddNoteTag(ctx, acme, "x", 1); !errors.Is(err, context.Canceled) {
			t.Errorf("add note tag: got %v, want context.Canceled", err)
		}
		if _, err := svc.RemoveNoteTag(ctx, acme, "x", 1); !errors.Is(err, context.Canceled) {
			t.Errorf("remove note tag: got %v, want context.Canceled", err)
		}
		if _, err := svc.List(ctx, acme, ListOptions{TagID: 1}); !errors.Is(err, context.Canceled) {
			t.Errorf("list by tag: got %v, want context.Canceled", err)
		}
	})
}

// TestTagIDZeroIsNoTag checks the virtual root of closure-tree, id 0, is no
// tag: deleting "it" must not take the whole tree.
func TestTagIDZeroIsNoTag(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		work := mustTag(t, svc, acme, "work", 0)
		note := mustNote(t, svc, acme, "plan")
		if _, err := svc.AddNoteTag(ctx, acme, note.ID, work.ID); err != nil {
			t.Fatal(err)
		}

		for _, id := range []uint{0, uint(maxTagID) + 1} {
			if err := svc.DeleteTag(ctx, acme, id); !errors.Is(err, ErrTagNotFound) {
				t.Errorf("delete %d: got %v, want ErrTagNotFound", id, err)
			}
			if _, err := svc.UpdateTag(ctx, acme, id, TagPatch{Name: ptr("x")}); !errors.Is(err, ErrTagNotFound) {
				t.Errorf("update %d: got %v, want ErrTagNotFound", id, err)
			}
			if _, err := svc.AddNoteTag(ctx, acme, note.ID, id); !errors.Is(err, ErrTagNotFound) {
				t.Errorf("add note tag %d: got %v, want ErrTagNotFound", id, err)
			}
			if _, err := svc.RemoveNoteTag(ctx, acme, note.ID, id); !errors.Is(err, ErrTagNotFound) {
				t.Errorf("remove note tag %d: got %v, want ErrTagNotFound", id, err)
			}
		}
		if got := treeOutline(t, svc, acme); got != "work" {
			t.Errorf("tree after the refused calls: got %q, want work", got)
		}
		got, err := svc.Get(ctx, acme, note.ID)
		if err != nil || !reflect.DeepEqual(got.TagIDs, []uint{work.ID}) {
			t.Errorf("note after the refused calls: got %+v, %v", got.TagIDs, err)
		}
	})
}

// TestTagRefsBeyondTheDatabase checks ids no database column can hold are
// refused as invalid, not passed on to fail there.
func TestTagRefsBeyondTheDatabase(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		work := mustTag(t, svc, acme, "work", 0)
		huge := uint(maxTagID) + 1
		_, err := svc.CreateTag(ctx, acme, TagInput{Name: "x", ParentID: huge})
		wantValidation(t, err, "parentId")
		_, err = svc.CreateTag(ctx, acme, TagInput{Name: "x", AfterID: &huge})
		wantValidation(t, err, "afterId")
		_, err = svc.UpdateTag(ctx, acme, work.ID, TagPatch{ParentID: &huge})
		wantValidation(t, err, "parentId")
		_, err = svc.UpdateTag(ctx, acme, work.ID, TagPatch{AfterID: &huge})
		wantValidation(t, err, "afterId")
		list, err := svc.List(ctx, acme, ListOptions{TagID: huge})
		if err != nil || len(list) != 0 {
			t.Errorf("list under a huge tag id: got %+v, %v", list, err)
		}
	})
}

// TestTagLimit checks a tenant's tree stays within MaxTagsPerTenant.
func TestTagLimit(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		// the limit counts the tenant's rows: fill all but one directly, it is quicker
		rows := make([]tagRow, 0, MaxTagsPerTenant-1)
		for i := range MaxTagsPerTenant - 1 {
			row := tagRow{Name: fmt.Sprint("t", i)}
			row.Tenant = acme.TenantID
			rows = append(rows, row)
		}
		if err := db.CreateInBatches(rows, 200).Error; err != nil {
			t.Fatal(err)
		}
		mustTag(t, svc, acme, "the last one", 0)
		if _, err := svc.CreateTag(ctx, acme, TagInput{Name: "one too many"}); !errors.Is(err, ErrTooManyTags) {
			t.Errorf("past the limit: got %v, want ErrTooManyTags", err)
		}
		// another tenant has a limit of its own
		mustTag(t, svc, Actor{TenantID: "tenant-globex"}, "free", 0)
	})
}

// TestConcurrentCreateOfOneName checks two creates of the same new name
// cannot both win: GetOrAdd finds and adds under the tree's tenant lock.
//
// SQLite runs as the app opens it (gormdb: one write connection that writers
// queue for); the bare test database of forEachDB has no such queue, and a
// second concurrent writer would get SQLITE_BUSY instead of waiting.
func TestConcurrentCreateOfOneName(t *testing.T) {
	t.Run("SqliteGormdb", func(t *testing.T) {
		concurrentCreateOfOneName(t, gormdbtest.Sqlite(t, sqlite.Driver{}))
	})
	for _, target := range testdbs.DBs() {
		if target.DbType() == "postgres" {
			t.Run(target.DbType(), func(t *testing.T) {
				concurrentCreateOfOneName(t, target.ConnDbName(t.Name()))
			})
		}
	}
}

func concurrentCreateOfOneName(t *testing.T, db *gorm.DB) {
	t.Helper()
	{
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		const n = 8
		errs := make(chan error, n)
		var wg sync.WaitGroup
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := svc.CreateTag(ctx, acme, TagInput{Name: "work"})
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		created := 0
		for err := range errs {
			switch {
			case err == nil:
				created++
			case !errors.Is(err, ErrTagExists):
				t.Errorf("unexpected error: %v", err)
			}
		}
		if created != 1 {
			t.Errorf("%d creates won, want 1", created)
		}
		if got := treeOutline(t, svc, acme); got != "work" {
			t.Errorf("tree: got %s, want a single work", got)
		}
	}
}

// TestListByTagFollowsMoves checks the tag filter reads the tree as it is now:
// a moved subtree takes its notes along.
func TestListByTagFollowsMoves(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		work := mustTag(t, svc, acme, "work", 0)
		home := mustTag(t, svc, acme, "home", 0)
		reports := mustTag(t, svc, acme, "reports", work.ID)
		note := mustNote(t, svc, acme, "plan")
		if _, err := svc.AddNoteTag(ctx, acme, note.ID, reports.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.UpdateTag(ctx, acme, reports.ID, TagPatch{ParentID: &home.ID}); err != nil {
			t.Fatal(err)
		}
		count := func(tag uint) int {
			list, err := svc.List(ctx, acme, ListOptions{TagID: tag})
			if err != nil {
				t.Fatal(err)
			}
			return len(list)
		}
		if got := count(work.ID); got != 0 {
			t.Errorf("under the old parent: got %d notes, want 0", got)
		}
		if got := count(home.ID); got != 1 {
			t.Errorf("under the new parent: got %d notes, want 1", got)
		}
	})
}

// TestLinkToAMissingTagIsNotReturned checks a link left behind by a
// concurrent tag delete (possible on Postgres) never reaches a caller.
func TestLinkToAMissingTagIsNotReturned(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		note := mustNote(t, svc, acme, "plan")
		orphan := noteTagRow{TenantID: acme.TenantID, NoteID: note.ID, TagID: 424242}
		if err := db.Create(&orphan).Error; err != nil {
			t.Fatal(err)
		}
		got, err := svc.Get(ctx, acme, note.ID)
		if err != nil || len(got.TagIDs) != 0 {
			t.Errorf("get: got tags %v, %v; want none", got.TagIDs, err)
		}
		list, err := svc.List(ctx, acme, ListOptions{})
		if err != nil || len(list) != 1 || len(list[0].TagIDs) != 0 {
			t.Errorf("list: got %+v, %v; want the note without tags", list, err)
		}
	})
}
