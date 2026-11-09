package tenants

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/go-bumbu/testdbs"
	"gorm.io/gorm"
)

// TestMain prepares the databases every test runs against: SQLite always, and
// Postgres (in a container, needs docker) with TESTDBS_ALL=1.
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

func forEachDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	for _, target := range testdbs.DBs() {
		t.Run(target.DbType(), func(t *testing.T) {
			fn(t, target.ConnDbName(t.Name()))
		})
	}
}

func newService(t *testing.T, db *gorm.DB) *Service {
	t.Helper()
	svc, err := New(context.Background(), db, Cfg{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func TestProvisionCreatesThenReuses(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db)

		first, err := svc.Provision(ctx, " acme ", " alice ")
		if err != nil {
			t.Fatalf("provision: %v", err)
		}
		if first.TenantID == "" || first.UserID == "" {
			t.Fatalf("expected generated ids, got %+v", first)
		}
		if first.TenantSlug != "acme" || first.Subject != "alice" {
			t.Errorf("values are not trimmed: %+v", first)
		}

		again, err := svc.Provision(ctx, "acme", "alice")
		if err != nil {
			t.Fatalf("provision again: %v", err)
		}
		if again != first {
			t.Errorf("second login: got %+v, want the same principal %+v", again, first)
		}
	})
}

func TestProvisionIsolation(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		svc := newService(t, db)

		alice, err := svc.Provision(ctx, "acme", "alice")
		if err != nil {
			t.Fatal(err)
		}
		bob, err := svc.Provision(ctx, "acme", "bob")
		if err != nil {
			t.Fatal(err)
		}
		aliceElsewhere, err := svc.Provision(ctx, "globex", "alice")
		if err != nil {
			t.Fatal(err)
		}

		if bob.TenantID != alice.TenantID {
			t.Error("two users of one tenant got different tenant ids")
		}
		if bob.UserID == alice.UserID {
			t.Error("two users of one tenant share a user id")
		}
		if aliceElsewhere.TenantID == alice.TenantID {
			t.Error("two tenants share a tenant id")
		}
		if aliceElsewhere.UserID == alice.UserID {
			t.Error("the same subject in two tenants must be two users")
		}
	})
}

// TestProvisionSurvivesRestart opens a second Service on the same database:
// the principal is stable across restarts and migrating again is harmless.
func TestProvisionSurvivesRestart(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		first, err := newService(t, db).Provision(ctx, "acme", "alice")
		if err != nil {
			t.Fatal(err)
		}
		again, err := newService(t, db).Provision(ctx, "acme", "alice")
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Errorf("got %+v, want %+v", again, first)
		}
	})
}

// TestProvisionConcurrentFirstLogin races many first logins of one identity:
// all must succeed and agree on a single tenant and user.
func TestProvisionConcurrentFirstLogin(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db)
		const n = 8
		var wg sync.WaitGroup
		results := make([]Principal, n)
		errs := make([]error, n)
		for i := range n {
			wg.Go(func() {
				results[i], errs[i] = svc.Provision(context.Background(), "acme", "alice")
			})
		}
		wg.Wait()
		for i := range n {
			if errs[i] != nil {
				t.Fatalf("provision %d: %v", i, errs[i])
			}
			if results[i] != results[0] {
				t.Fatalf("provision %d: got %+v, want %+v", i, results[i], results[0])
			}
		}
	})
}

func TestProvisionValidation(t *testing.T) {
	tests := []struct {
		name      string
		tenant    string
		subject   string
		wantField string
	}{
		{name: "empty tenant", tenant: "  ", subject: "alice", wantField: "tenant"},
		{name: "tenant too long", tenant: strings.Repeat("t", MaxSlugLen+1), subject: "alice", wantField: "tenant"},
		{name: "empty subject", tenant: "acme", subject: "", wantField: "subject"},
		{name: "subject too long", tenant: "acme", subject: strings.Repeat("s", MaxSubjectLen+1), wantField: "subject"},
	}
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db)
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := svc.Provision(context.Background(), tc.tenant, tc.subject)
				var verr *ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("got %v, want a *ValidationError", err)
				}
				if verr.Field != tc.wantField {
					t.Errorf("field: got %q, want %q", verr.Field, tc.wantField)
				}
			})
		}
	})
}

// TestMaxLengthFits stores the longest valid values, multi-byte included: the
// column sizes must hold what validation lets through.
func TestMaxLengthFits(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		slug := strings.Repeat("é", MaxSlugLen)
		subject := strings.Repeat("é", MaxSubjectLen)
		p, err := newService(t, db).Provision(context.Background(), slug, subject)
		if err != nil {
			t.Fatalf("provision: %v", err)
		}
		if p.TenantSlug != slug || p.Subject != subject {
			t.Error("the stored identity was truncated")
		}
	})
}

func TestNewRequiresDB(t *testing.T) {
	if _, err := New(context.Background(), nil, Cfg{}); err == nil {
		t.Fatal("expected an error without a db")
	}
}

func TestCancelledContext(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := svc.Provision(ctx, "acme", "alice"); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	})
}
