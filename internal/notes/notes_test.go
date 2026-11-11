package notes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-bumbu/testdbs"
	"gorm.io/gorm"
)

// TestMain prepares the databases every test runs against: SQLite always, and
// Postgres (in a container, needs docker) with `go test -alldbs` or TESTDBS_ALL=1.
func TestMain(m *testing.M) {
	testdbs.InitCustomDbs(
		[]testdbs.TargetDb{&testdbs.SqliteNoCgo{}},
		[]testdbs.TargetDb{testdbs.NewPostgres("")},
	)
	code := m.Run()
	if err := testdbs.Clean(); err != nil {
		fmt.Fprintf(os.Stderr, "clean test dbs: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// forEachDB runs fn once per test database, each with a fresh database of its own.
func forEachDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	for _, target := range testdbs.DBs() {
		t.Run(target.DbType(), func(t *testing.T) {
			fn(t, target.ConnDbName(t.Name()))
		})
	}
}

func newService(t *testing.T, db *gorm.DB, cfg Cfg) *Service {
	t.Helper()
	svc, err := New(context.Background(), db, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// acme is the actor most tests act as.
var acme = Actor{TenantID: "tenant-acme", UserID: "user-alice"}

func fixedClock(start time.Time) func() time.Time {
	t := start
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}
}

func TestCreateAndGet(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		// nanoseconds and a non-UTC zone: the stored note must still equal the returned one
		start := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.FixedZone("CET", 3600))
		svc := newService(t, db, Cfg{Now: fixedClock(start)})

		created, err := svc.Create(ctx, acme, Input{Title: "  hello ", Body: "world"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if created.ID == "" {
			t.Error("expected a generated id")
		}
		if created.Title != "hello" {
			t.Errorf("title: got %q, want the trimmed %q", created.Title, "hello")
		}
		if created.TenantID != acme.TenantID || created.CreatedBy != acme.UserID {
			t.Errorf("owner: got tenant %q by %q, want %+v", created.TenantID, created.CreatedBy, acme)
		}
		if created.CreatedAt.Location() != time.UTC {
			t.Errorf("createdAt: got zone %s, want UTC", created.CreatedAt.Location())
		}

		got, err := svc.Get(ctx, acme, created.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !reflect.DeepEqual(got, created) {
			t.Errorf("get: got %+v, want %+v", got, created)
		}
	})
}

// TestNotesSurviveRestart opens a second Service on the same database, as a
// restarted process would: the data is there and migrating again is harmless.
func TestNotesSurviveRestart(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		created, err := newService(t, db, Cfg{}).Create(ctx, acme, Input{Title: "persisted"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		got, err := newService(t, db, Cfg{}).Get(ctx, acme, created.ID)
		if err != nil {
			t.Fatalf("get after restart: %v", err)
		}
		if !reflect.DeepEqual(got, created) {
			t.Errorf("got %+v, want %+v", got, created)
		}
	})
}

func TestGetNotFound(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		_, err := newService(t, db, Cfg{}).Get(context.Background(), acme, "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})
}

func TestUpdate(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{Now: fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))})
		created, err := svc.Create(ctx, acme, Input{Title: "draft", Body: "old"})
		if err != nil {
			t.Fatal(err)
		}

		// another user of the tenant edits it: notes are shared
		bob := Actor{TenantID: acme.TenantID, UserID: "user-bob"}
		updated, err := svc.Update(ctx, bob, created.ID, Input{Title: "  final ", Body: "new"})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		want := created
		want.Title, want.Body = "final", "new"
		if !reflect.DeepEqual(updated, want) {
			t.Errorf("update: got %+v, want %+v (author and creation time unchanged)", updated, want)
		}

		got, err := svc.Get(ctx, acme, created.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("get after update: got %+v, want %+v", got, want)
		}

		// the same values again still find the note
		if _, err := svc.Update(ctx, acme, created.ID, Input{Title: "final", Body: "new"}); err != nil {
			t.Errorf("unchanged update: %v", err)
		}
	})
}

func TestUpdateValidationAndNotFound(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		created, err := svc.Create(ctx, acme, Input{Title: "keep"})
		if err != nil {
			t.Fatal(err)
		}

		_, err = svc.Update(ctx, acme, created.ID, Input{Title: "  "})
		var verr *ValidationError
		if !errors.As(err, &verr) || verr.Field != "title" {
			t.Errorf("empty title: got %v, want a title *ValidationError", err)
		}
		if got, _ := svc.Get(ctx, acme, created.ID); got.Title != "keep" {
			t.Errorf("an invalid update was stored: title %q", got.Title)
		}

		if _, err := svc.Update(ctx, acme, "missing", Input{Title: "x"}); !errors.Is(err, ErrNotFound) {
			t.Errorf("missing note: got %v, want ErrNotFound", err)
		}
	})
}

func TestDelete(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		gone, err := svc.Create(ctx, acme, Input{Title: "gone"})
		if err != nil {
			t.Fatal(err)
		}
		kept, err := svc.Create(ctx, acme, Input{Title: "kept"})
		if err != nil {
			t.Fatal(err)
		}

		if err := svc.Delete(ctx, acme, gone.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := svc.Get(ctx, acme, gone.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("get after delete: got %v, want ErrNotFound", err)
		}
		if err := svc.Delete(ctx, acme, gone.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("second delete: got %v, want ErrNotFound", err)
		}
		list, err := svc.List(ctx, acme, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != kept.ID {
			t.Errorf("list after delete: got %+v, want only %q", list, kept.ID)
		}
	})
}

func TestCreateValidation(t *testing.T) {
	tests := []struct {
		name      string
		in        Input
		wantField string
	}{
		{name: "empty title", in: Input{Title: "   "}, wantField: "title"},
		{name: "title too long", in: Input{Title: strings.Repeat("a", MaxTitleLen+1)}, wantField: "title"},
		{name: "body too long", in: Input{Title: "ok", Body: strings.Repeat("a", MaxBodyLen+1)}, wantField: "body"},
	}
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := svc.Create(context.Background(), acme, tc.in)
				var verr *ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("got %v, want a *ValidationError", err)
				}
				if verr.Field != tc.wantField {
					t.Errorf("field: got %q, want %q", verr.Field, tc.wantField)
				}
			})
		}
		list, err := svc.List(context.Background(), acme, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 0 {
			t.Errorf("invalid input was stored: %+v", list)
		}
	})
}

// TestMaxLengthFits stores the longest valid values: the column sizes must
// hold what validation lets through, multi-byte characters included.
func TestMaxLengthFits(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		in := Input{Title: strings.Repeat("é", MaxTitleLen), Body: strings.Repeat("é", MaxBodyLen)}
		created, err := svc.Create(context.Background(), acme, in)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := svc.Get(context.Background(), acme, created.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Title != in.Title || got.Body != in.Body {
			t.Error("the stored note was truncated")
		}
	})
}

func TestListNewestFirst(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{Now: fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))})

		empty, err := svc.List(ctx, acme, ListOptions{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if empty == nil || len(empty) != 0 {
			t.Errorf("empty list: got %#v, want a non-nil empty slice", empty)
		}

		for _, title := range []string{"first", "second", "third"} {
			if _, err := svc.Create(ctx, acme, Input{Title: title}); err != nil {
				t.Fatal(err)
			}
		}
		list, err := svc.List(ctx, acme, ListOptions{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var titles []string
		for _, n := range list {
			titles = append(titles, n.Title)
		}
		if got, want := strings.Join(titles, ","), "third,second,first"; got != want {
			t.Errorf("order: got %s, want %s", got, want)
		}
	})
}

// TestCancelledContext checks the context reaches the database: a request
// the client abandoned does not run its query.
func TestCancelledContext(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := svc.Create(ctx, acme, Input{Title: "x"}); !errors.Is(err, context.Canceled) {
			t.Errorf("create: got %v, want context.Canceled", err)
		}
		if _, err := svc.Get(ctx, acme, "x"); !errors.Is(err, context.Canceled) {
			t.Errorf("get: got %v, want context.Canceled", err)
		}
		if _, err := svc.List(ctx, acme, ListOptions{}); !errors.Is(err, context.Canceled) {
			t.Errorf("list: got %v, want context.Canceled", err)
		}
		if _, err := svc.Update(ctx, acme, "x", Input{Title: "x"}); !errors.Is(err, context.Canceled) {
			t.Errorf("update: got %v, want context.Canceled", err)
		}
		if err := svc.Delete(ctx, acme, "x"); !errors.Is(err, context.Canceled) {
			t.Errorf("delete: got %v, want context.Canceled", err)
		}
		if _, err := New(ctx, db, Cfg{}); err == nil {
			t.Error("New: expected the migration to fail on a cancelled context")
		}
	})
}

func TestNewRequiresDB(t *testing.T) {
	if _, err := New(context.Background(), nil, Cfg{}); err == nil {
		t.Fatal("expected an error without a db")
	}
}

// TestTenantIsolation checks a note is invisible to every other tenant, while
// shared by all users of its own.
func TestTenantIsolation(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		bob := Actor{TenantID: acme.TenantID, UserID: "user-bob"}
		globex := Actor{TenantID: "tenant-globex", UserID: "user-carol"}

		created, err := svc.Create(ctx, acme, Input{Title: "acme only"})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := svc.Get(ctx, globex, created.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("get from another tenant: got %v, want ErrNotFound", err)
		}
		list, err := svc.List(ctx, globex, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 0 {
			t.Errorf("another tenant lists %d notes, want 0", len(list))
		}
		if _, err := svc.Update(ctx, globex, created.ID, Input{Title: "hijacked"}); !errors.Is(err, ErrNotFound) {
			t.Errorf("update from another tenant: got %v, want ErrNotFound", err)
		}
		if err := svc.Delete(ctx, globex, created.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("delete from another tenant: got %v, want ErrNotFound", err)
		}
		if got, err := svc.Get(ctx, acme, created.ID); err != nil || got.Title != "acme only" {
			t.Errorf("another tenant changed the note: got %+v, %v", got, err)
		}

		got, err := svc.Get(ctx, bob, created.ID)
		if err != nil {
			t.Fatalf("get from a user of the same tenant: %v", err)
		}
		if got.CreatedBy != acme.UserID {
			t.Errorf("createdBy: got %q, want %q", got.CreatedBy, acme.UserID)
		}
	})
}

// TestNoTenantFailsClosed checks a call without a tenant is refused instead of
// reaching every tenant's data.
func TestNoTenantFailsClosed(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{})
		if _, err := svc.Create(ctx, acme, Input{Title: "x"}); err != nil {
			t.Fatal(err)
		}
		none := Actor{UserID: "user-alice"}
		if _, err := svc.Create(ctx, none, Input{Title: "x"}); !errors.Is(err, ErrNoTenant) {
			t.Errorf("create: got %v, want ErrNoTenant", err)
		}
		if _, err := svc.Get(ctx, none, "x"); !errors.Is(err, ErrNoTenant) {
			t.Errorf("get: got %v, want ErrNoTenant", err)
		}
		if _, err := svc.List(ctx, none, ListOptions{}); !errors.Is(err, ErrNoTenant) {
			t.Errorf("list: got %v, want ErrNoTenant", err)
		}
		if _, err := svc.Update(ctx, none, "x", Input{Title: "x"}); !errors.Is(err, ErrNoTenant) {
			t.Errorf("update: got %v, want ErrNoTenant", err)
		}
		if err := svc.Delete(ctx, none, "x"); !errors.Is(err, ErrNoTenant) {
			t.Errorf("delete: got %v, want ErrNoTenant", err)
		}
		if _, err := svc.NoteIDs(ctx, ""); !errors.Is(err, ErrNoTenant) {
			t.Errorf("note ids: got %v, want ErrNoTenant", err)
		}
	})
}

// TestNoteIDs checks the ids come newest first and only from the tenant asked for.
func TestNoteIDs(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db, Cfg{Now: fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))})
		var want []string
		for _, title := range []string{"first", "second"} {
			n, err := svc.Create(ctx, acme, Input{Title: title})
			if err != nil {
				t.Fatal(err)
			}
			want = append([]string{n.ID}, want...)
		}
		if _, err := svc.Create(ctx, Actor{TenantID: "tenant-globex"}, Input{Title: "foreign"}); err != nil {
			t.Fatal(err)
		}

		got, err := svc.NoteIDs(ctx, acme.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
		empty, err := svc.NoteIDs(ctx, "tenant-empty")
		if err != nil || empty == nil || len(empty) != 0 {
			t.Errorf("a tenant without notes: got %#v, %v; want an empty list", empty, err)
		}
	})
}
