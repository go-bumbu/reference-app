package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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

// forEachDB runs fn once per test database, each with a fresh database of its own.
func forEachDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	for _, target := range testdbs.DBs() {
		t.Run(target.DbType(), func(t *testing.T) {
			fn(t, target.ConnDbName(t.Name()))
		})
	}
}

var (
	acme   = Actor{TenantID: "tenant-acme"}
	globex = Actor{TenantID: "tenant-globex"}
)

// fakeNotes holds the note ids of each tenant.
type fakeNotes map[string][]string

func (f fakeNotes) NoteIDs(_ context.Context, tenantID string) ([]string, error) {
	return f[tenantID], nil
}

var someNotes = fakeNotes{
	acme.TenantID:   {"a1", "a2", "a3"},
	globex.TenantID: {"g1"},
}

// recorder records every note a scan handed to ScanNote.
type recorder struct {
	mu      sync.Mutex
	scanned []string
}

func (r *recorder) scan(_ context.Context, tenantID, noteID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanned = append(r.scanned, tenantID+"/"+noteID)
	return nil
}

func (r *recorder) got() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.scanned)
}

// blockingScan holds every note until release is closed or the run is stopped.
func blockingScan(release <-chan struct{}) func(context.Context, string, string) error {
	return func(ctx context.Context, _, _ string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}
}

// newService builds and starts a Service on db, logging to a temp dir, and
// shuts it down when the test ends.
func newService(t *testing.T, db *gorm.DB, cfg Cfg) *Service {
	t.Helper()
	if cfg.Notes == nil {
		cfg.Notes = someNotes
	}
	if cfg.LogDir == "" {
		cfg.LogDir = t.TempDir()
	}
	svc, err := New(db, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { shutDown(t, svc) })
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return svc
}

func shutDown(t *testing.T, svc *Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := svc.ShutDown(ctx); err != nil {
		t.Errorf("ShutDown: %v", err)
	}
}

// waitRun polls the run until cond holds; runs are asynchronous.
func waitRun(t *testing.T, svc *Service, a Actor, id string, cond func(Run) bool) Run {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		r, err := svc.Run(context.Background(), a, id)
		if err != nil {
			t.Fatalf("run %s: %v", id, err)
		}
		if cond(r) {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s: condition not met, last seen %+v", id, r)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func hasStatus(want string) func(Run) bool {
	return func(r Run) bool { return r.Status == want }
}

func trigger(t *testing.T, svc *Service, a Actor) (Run, bool) {
	t.Helper()
	r, reused, err := svc.Trigger(context.Background(), a, ScanNotes)
	if err != nil {
		t.Fatalf("trigger for %s: %v", a.TenantID, err)
	}
	return r, reused
}

func TestScanNotesScansTheTenantsNotes(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		rec := &recorder{}
		svc := newService(t, db, Cfg{ScanNote: rec.scan})

		run, reused := trigger(t, svc, acme)
		if reused || run.Job != ScanNotes || run.ID == "" || run.QueuedAt.IsZero() {
			t.Fatalf("got %+v, reused %v; want a new queued run", run, reused)
		}
		done := waitRun(t, svc, acme, run.ID, hasStatus("complete"))

		want := []string{"tenant-acme/a1", "tenant-acme/a2", "tenant-acme/a3"}
		if got := rec.got(); !slices.Equal(got, want) {
			t.Errorf("scanned %v, want %v", got, want)
		}
		if done.StartedAt.IsZero() || done.EndedAt.IsZero() || done.Progress != nil {
			t.Errorf("a finished run: got %+v; want start and end times, no progress", done)
		}
		if done.QueuedAt.Location() != time.UTC {
			t.Errorf("times must be UTC, got %v", done.QueuedAt.Location())
		}

		lines, err := svc.Log(context.Background(), acme, run.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		text := logText(lines)
		for _, want := range []string{
			"scanning notes notes=3",
			"note scanned note=a1 n=1 of=3",
			"note scanned note=a2 n=2 of=3",
			"note scanned note=a3 n=3 of=3",
			"scan finished scanned=3",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("log lacks %q:\n%s", want, text)
			}
		}
	})
}

func logText(lines []LogLine) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Level + " " + l.Message + "\n")
	}
	return b.String()
}

func TestTriggerCoalescesPerTenant(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		release := make(chan struct{})
		svc := newService(t, db, Cfg{ScanNote: blockingScan(release)})

		first, _ := trigger(t, svc, acme)
		running := waitRun(t, svc, acme, first.ID, func(r Run) bool {
			return r.Status == "running" && r.Progress != nil && r.Progress.Total == 3
		})
		if running.Progress.Done != 0 || running.Progress.Stage != "Scanning notes" {
			t.Errorf("progress: got %+v", running.Progress)
		}

		again, reused := trigger(t, svc, acme)
		if !reused || again.ID != first.ID {
			t.Errorf("second trigger: got %s reused %v; want the running run %s", again.ID, reused, first.ID)
		}
		other, reused := trigger(t, svc, globex)
		if reused || other.ID == first.ID {
			t.Errorf("another tenant's trigger coalesced onto acme's run")
		}

		close(release)
		waitRun(t, svc, acme, first.ID, hasStatus("complete"))
		waitRun(t, svc, globex, other.ID, hasStatus("complete"))
		if next, reused := trigger(t, svc, acme); reused || next.ID == first.ID {
			t.Error("a trigger after the run ended must queue a new run")
		}
	})
}

func TestScheduleFiresCoalescePerTenant(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		release := make(chan struct{})
		defer close(release)
		svc := newService(t, db, Cfg{ScanNote: blockingScan(release)})
		fire := fireEnqueuer{svc}

		id, reused, err := fire.AddRaw(ScanNotes, []byte(`{"tenant_id":"tenant-acme"}`))
		if err != nil || reused {
			t.Fatalf("first fire: %v, reused %v", err, reused)
		}
		again, reused, err := fire.AddRaw(ScanNotes, []byte(`{"tenant_id":"tenant-acme"}`))
		if err != nil || !reused || again != id {
			t.Errorf("second fire: got %s reused %v err %v; want the first run %s", again, reused, err, id)
		}
		if _, _, err := fire.AddRaw(ScanNotes, []byte(`{}`)); err == nil {
			t.Error("a fire without a tenant must fail")
		}
	})
}

func TestRunsListsTheTenantsRunsNewestFirst(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{ScanNote: (&recorder{}).scan})
		first, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, first.ID, hasStatus("complete"))
		second, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, second.ID, hasStatus("complete"))
		foreign, _ := trigger(t, svc, globex)
		waitRun(t, svc, globex, foreign.ID, hasStatus("complete"))

		runs, err := svc.Runs(context.Background(), acme)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, r := range runs {
			ids = append(ids, r.ID)
		}
		if want := []string{second.ID, first.ID}; !slices.Equal(ids, want) {
			t.Errorf("got %v, want %v", ids, want)
		}
	})
}

func TestCancel(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		release := make(chan struct{})
		defer close(release)
		svc := newService(t, db, Cfg{ScanNote: blockingScan(release), Parallelism: 1})
		ctx := context.Background()

		running, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, running.ID, hasStatus("running"))
		waiting, _ := trigger(t, svc, globex) // one worker: it waits

		if err := svc.Cancel(ctx, globex, waiting.ID); err != nil {
			t.Fatalf("cancel a waiting run: %v", err)
		}
		waitRun(t, svc, globex, waiting.ID, hasStatus("canceled"))

		if err := svc.Cancel(ctx, acme, running.ID); err != nil {
			t.Fatalf("cancel a running run: %v", err)
		}
		r := waitRun(t, svc, acme, running.ID, hasStatus("canceled"))
		if r.EndedAt.IsZero() {
			t.Error("a canceled run must have an end time")
		}
		if err := svc.Cancel(ctx, acme, running.ID); err != nil {
			t.Errorf("cancel an ended run: got %v, want nil", err)
		}
	})
}

func TestFailingNoteFailsTheRun(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{ScanNote: func(_ context.Context, _, noteID string) error {
			if noteID == "a2" {
				return errors.New("unreadable")
			}
			return nil
		}})
		run, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, run.ID, hasStatus("failed"))
		lines, err := svc.Log(context.Background(), acme, run.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if text := logText(lines); !strings.Contains(text, "ERROR") || !strings.Contains(text, "note a2: unreadable") {
			t.Errorf("the log must say why the run failed:\n%s", text)
		}
	})
}

func TestLogTail(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{ScanNote: (&recorder{}).scan})
		run, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, run.ID, hasStatus("complete"))
		all, err := svc.Log(context.Background(), acme, run.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		last, err := svc.Log(context.Background(), acme, run.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) < 2 || len(last) != 1 || last[0] != all[len(all)-1] {
			t.Errorf("tail 1: got %v of %v", last, all)
		}
	})
}

func TestQueueFull(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		release := make(chan struct{})
		defer close(release)
		svc := newService(t, db, Cfg{ScanNote: blockingScan(release), Parallelism: 1, QueueSize: 1})
		running, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, running.ID, hasStatus("running"))
		trigger(t, svc, globex) // waits, filling the queue
		if _, _, err := svc.Trigger(context.Background(), Actor{TenantID: "tenant-third"}, ScanNotes); !errors.Is(err, ErrQueueFull) {
			t.Errorf("got %v, want ErrQueueFull", err)
		}
	})
}

func TestRunTenantIsolation(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{ScanNote: (&recorder{}).scan})
		ctx := context.Background()
		run, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, run.ID, hasStatus("complete"))

		for _, id := range []string{run.ID, "not-a-uuid", "00000000-0000-0000-0000-000000000001"} {
			who := globex
			if id != run.ID {
				who = acme
			}
			if _, err := svc.Run(ctx, who, id); !errors.Is(err, ErrRunNotFound) {
				t.Errorf("run %s: got %v, want ErrRunNotFound", id, err)
			}
			if err := svc.Cancel(ctx, who, id); !errors.Is(err, ErrRunNotFound) {
				t.Errorf("cancel %s: got %v, want ErrRunNotFound", id, err)
			}
			if _, err := svc.Log(ctx, who, id, 0); !errors.Is(err, ErrRunNotFound) {
				t.Errorf("log %s: got %v, want ErrRunNotFound", id, err)
			}
		}
		runs, err := svc.Runs(ctx, globex)
		if err != nil || len(runs) != 0 {
			t.Errorf("another tenant lists %v, %v; want none", runs, err)
		}
	})
}

func TestRunsSurviveRestart(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		logDir := t.TempDir()
		svc := newService(t, db, Cfg{ScanNote: (&recorder{}).scan, LogDir: logDir})
		run, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, run.ID, hasStatus("complete"))
		shutDown(t, svc)

		restarted := newService(t, db, Cfg{LogDir: logDir})
		got, err := restarted.Run(context.Background(), acme, run.ID)
		if err != nil || got.Status != "complete" {
			t.Fatalf("after a restart: got %+v, %v", got, err)
		}
		lines, err := restarted.Log(context.Background(), acme, run.ID, 0)
		if err != nil || len(lines) == 0 {
			t.Errorf("the log must survive a restart: got %d lines, %v", len(lines), err)
		}
	})
}

func TestShutDownStopsARunningRun(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		release := make(chan struct{})
		defer close(release)
		svc := newService(t, db, Cfg{ScanNote: blockingScan(release)})
		run, _ := trigger(t, svc, acme)
		waitRun(t, svc, acme, run.ID, hasStatus("running"))

		shutDown(t, svc)
		if got, err := svc.Run(context.Background(), acme, run.ID); err != nil || got.Status != "canceled" {
			t.Errorf("after shutdown: got %+v, %v; want canceled", got, err)
		}
		if err := svc.ShutDown(context.Background()); err != nil {
			t.Errorf("a second ShutDown: %v", err)
		}
	})
}

func TestSchedules(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		ctx := context.Background()

		created, err := svc.CreateSchedule(ctx, acme, ScanNotes, ScheduleInput{Cron: " 0 3 * * * ", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		if created.Cron != "0 0 3 * * *" || !created.Enabled || created.NextRunAt.IsZero() || created.Job != ScanNotes {
			t.Errorf("created: got %+v; want the 6-field form, enabled, with a next run", created)
		}
		paused, err := svc.CreateSchedule(ctx, acme, ScanNotes, ScheduleInput{Cron: "0 0 4 * * *"})
		if err != nil {
			t.Fatal(err)
		}
		if paused.Enabled || !paused.NextRunAt.IsZero() {
			t.Errorf("a disabled schedule: got %+v; want no next run", paused)
		}

		jobs, err := svc.Jobs(ctx, acme)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 1 || jobs[0].Name != ScanNotes || len(jobs[0].Schedules) != 2 ||
			jobs[0].Schedules[0].ID != created.ID {
			t.Fatalf("jobs: got %+v", jobs)
		}
		foreignJobs, err := svc.Jobs(ctx, globex)
		if err != nil || len(foreignJobs[0].Schedules) != 0 {
			t.Errorf("another tenant sees schedules: %+v, %v", foreignJobs, err)
		}

		cron, on := "*/5 * * * *", true
		updated, err := svc.UpdateSchedule(ctx, acme, ScanNotes, paused.ID, SchedulePatch{Cron: &cron, Enabled: &on})
		if err != nil {
			t.Fatal(err)
		}
		if updated.Cron != "0 */5 * * * *" || !updated.Enabled || updated.CreatedAt != paused.CreatedAt {
			t.Errorf("updated: got %+v", updated)
		}
		off := false
		if kept, err := svc.UpdateSchedule(ctx, acme, ScanNotes, paused.ID, SchedulePatch{Enabled: &off}); err != nil ||
			kept.Cron != "0 */5 * * * *" || kept.Enabled {
			t.Errorf("a patch of enabled only: got %+v, %v", kept, err)
		}

		if err := svc.DeleteSchedule(ctx, acme, ScanNotes, created.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.DeleteSchedule(ctx, acme, ScanNotes, created.ID); !errors.Is(err, ErrScheduleNotFound) {
			t.Errorf("delete twice: got %v, want ErrScheduleNotFound", err)
		}
	})
}

func TestScheduleTenantIsolation(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		ctx := context.Background()
		sc, err := svc.CreateSchedule(ctx, acme, ScanNotes, ScheduleInput{Cron: "0 3 * * *", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		on := true
		for _, id := range []string{sc.ID, "not-a-uuid", "00000000-0000-0000-0000-000000000001"} {
			if _, err := svc.UpdateSchedule(ctx, globex, ScanNotes, id, SchedulePatch{Enabled: &on}); !errors.Is(err, ErrScheduleNotFound) {
				t.Errorf("update %s from another tenant: got %v, want ErrScheduleNotFound", id, err)
			}
			if err := svc.DeleteSchedule(ctx, globex, ScanNotes, id); !errors.Is(err, ErrScheduleNotFound) {
				t.Errorf("delete %s from another tenant: got %v, want ErrScheduleNotFound", id, err)
			}
		}
		if _, err := svc.UpdateSchedule(ctx, acme, ScanNotes, sc.ID, SchedulePatch{}); err != nil {
			t.Errorf("the owner's schedule must be untouched: %v", err)
		}
	})
}

func TestScheduleValidation(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		ctx := context.Background()
		for _, cron := range []string{"", "  ", "not a cron", "* * * * * *", "*/10 * * * * *", strings.Repeat("1", MaxCronLen+1)} {
			var verr *ValidationError
			if _, err := svc.CreateSchedule(ctx, acme, ScanNotes, ScheduleInput{Cron: cron}); !errors.As(err, &verr) || verr.Field != "cron" {
				t.Errorf("create with cron %q: got %v, want a ValidationError on cron", cron, err)
			}
		}
		sc, err := svc.CreateSchedule(ctx, acme, ScanNotes, ScheduleInput{Cron: "0 3 * * *"})
		if err != nil {
			t.Fatal(err)
		}
		bad := "* * * * * *"
		var verr *ValidationError
		if _, err := svc.UpdateSchedule(ctx, acme, ScanNotes, sc.ID, SchedulePatch{Cron: &bad}); !errors.As(err, &verr) {
			t.Errorf("update to an every-second cron: got %v, want a ValidationError", err)
		}

		for range MaxSchedulesPerJob - 1 {
			if _, err := svc.CreateSchedule(ctx, acme, ScanNotes, ScheduleInput{Cron: "0 3 * * *"}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := svc.CreateSchedule(ctx, acme, ScanNotes, ScheduleInput{Cron: "0 3 * * *"}); !errors.Is(err, ErrTooManySchedules) {
			t.Errorf("one schedule too many: got %v, want ErrTooManySchedules", err)
		}
		if _, err := svc.CreateSchedule(ctx, globex, ScanNotes, ScheduleInput{Cron: "0 3 * * *"}); err != nil {
			t.Errorf("the cap is per tenant: %v", err)
		}
	})
}

func TestUnknownJob(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		ctx := context.Background()
		on := true
		if _, _, err := svc.Trigger(ctx, acme, "nope"); !errors.Is(err, ErrJobNotFound) {
			t.Errorf("trigger: got %v", err)
		}
		if _, err := svc.CreateSchedule(ctx, acme, "nope", ScheduleInput{Cron: "0 3 * * *"}); !errors.Is(err, ErrJobNotFound) {
			t.Errorf("create schedule: got %v", err)
		}
		if _, err := svc.UpdateSchedule(ctx, acme, "nope", "x", SchedulePatch{Enabled: &on}); !errors.Is(err, ErrJobNotFound) {
			t.Errorf("update schedule: got %v", err)
		}
		if err := svc.DeleteSchedule(ctx, acme, "nope", "x"); !errors.Is(err, ErrJobNotFound) {
			t.Errorf("delete schedule: got %v", err)
		}
		// a schedule of one job is not found under another name
		sc, err := svc.CreateSchedule(ctx, acme, ScanNotes, ScheduleInput{Cron: "0 3 * * *"})
		if err != nil {
			t.Fatal(err)
		}
		svc.defs = append(svc.defs, definition{name: "other"})
		if err := svc.DeleteSchedule(ctx, acme, "other", sc.ID); !errors.Is(err, ErrScheduleNotFound) {
			t.Errorf("delete under another job: got %v, want ErrScheduleNotFound", err)
		}
	})
}

// TestNoTenantFailsClosed checks a call without a tenant is refused instead of
// reaching every tenant's runs.
func TestNoTenantFailsClosed(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		svc := newService(t, db, Cfg{})
		ctx := context.Background()
		none := Actor{}
		on := true
		checks := map[string]error{}
		_, _, checks["trigger"] = svc.Trigger(ctx, none, ScanNotes)
		_, checks["runs"] = svc.Runs(ctx, none)
		_, checks["run"] = svc.Run(ctx, none, "x")
		checks["cancel"] = svc.Cancel(ctx, none, "x")
		_, checks["log"] = svc.Log(ctx, none, "x", 0)
		_, checks["jobs"] = svc.Jobs(ctx, none)
		_, checks["create schedule"] = svc.CreateSchedule(ctx, none, ScanNotes, ScheduleInput{Cron: "0 3 * * *"})
		_, checks["update schedule"] = svc.UpdateSchedule(ctx, none, ScanNotes, "x", SchedulePatch{Enabled: &on})
		checks["delete schedule"] = svc.DeleteSchedule(ctx, none, ScanNotes, "x")
		for name, err := range checks {
			if !errors.Is(err, ErrNoTenant) {
				t.Errorf("%s: got %v, want ErrNoTenant", name, err)
			}
		}
	})
}

func TestNewRequiresItsDependencies(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		for name, tc := range map[string]struct {
			db  *gorm.DB
			cfg Cfg
		}{
			"db":     {cfg: Cfg{Notes: someNotes, LogDir: t.TempDir()}},
			"notes":  {db: db, cfg: Cfg{LogDir: t.TempDir()}},
			"logDir": {db: db, cfg: Cfg{Notes: someNotes}},
		} {
			if _, err := New(tc.db, tc.cfg); err == nil {
				t.Errorf("without %s: want an error", name)
			}
		}
		// a ShutDown without Start does nothing
		svc, err := New(db, Cfg{Notes: someNotes, LogDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.ShutDown(context.Background()); err != nil {
			t.Errorf("ShutDown without Start: %v", err)
		}
	})
}

func TestScanNoteStub(t *testing.T) {
	if err := scanNoteStub(context.Background(), "t", "n"); err != nil {
		t.Errorf("stub: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := scanNoteStub(ctx, "t", "n"); !errors.Is(err, context.Canceled) {
		t.Errorf("stub on a canceled context: got %v", err)
	}
}
