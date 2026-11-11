// Package jobs is the business module of background work: the jobs the app
// defines, their runs, logs and progress, and the cron schedules that start
// them. It runs on github.com/go-bumbu/tempo, which owns the queue, the
// workers, the scheduler and their tables (tempo_tasks, tempo_schedules).
//
// Every run, log and schedule belongs to one tenant: a run carries its
// tenant's id in its params, works on that tenant's data only, and is visible
// to that tenant only. tempo's tables have no tenant column, so this package
// applies the filter on every read, as the notes store does for its rows:
// another tenant's run or schedule does not exist for the caller.
//
// The module follows the shape of every module in internal/: it knows nothing
// about HTTP, it reports failures as typed errors, and the shared runtime
// (db, logger) is passed in by app/. The notes it scans come through the
// NoteSource interface, never by importing internal/notes.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/go-bumbu/tempo"
	"github.com/go-bumbu/tempo/dbqueue"
	"github.com/go-bumbu/tempo/dbschedule"
	"github.com/go-bumbu/tempo/filelog"
	"github.com/go-bumbu/tempo/schedule"
	"gorm.io/gorm"
)

const (
	// DefaultParallelism is how many runs execute at once, every job and tenant together.
	DefaultParallelism = 2
	// DefaultQueueSize is how many runs may wait at once.
	DefaultQueueSize = 100
	// DefaultHistorySize is how many finished runs are kept, every job and
	// tenant together; older ones are removed with their logs.
	DefaultHistorySize = 100
	// MaxSchedulesPerJob caps the schedules a tenant has of one job.
	MaxSchedulesPerJob = 5
)

var (
	// ErrNoTenant is returned when a call has no tenant to scope it to: a
	// wiring bug, never a user error.
	ErrNoTenant = errors.New("jobs: actor has no tenant")
	// ErrJobNotFound is returned for a job name the app does not define.
	ErrJobNotFound = errors.New("job not found")
	// ErrRunNotFound is returned for a run that does not exist, or belongs to
	// another tenant, or left the history.
	ErrRunNotFound = errors.New("job run not found")
	// ErrScheduleNotFound is returned for a schedule that does not exist, or
	// belongs to another tenant or another job.
	ErrScheduleNotFound = errors.New("job schedule not found")
	// ErrQueueFull is returned when no more runs may wait.
	ErrQueueFull = errors.New("job queue is full")
	// ErrTooManySchedules is returned by a create past MaxSchedulesPerJob.
	ErrTooManySchedules = fmt.Errorf("a job has at most %d schedules per tenant", MaxSchedulesPerJob)
)

// ValidationError reports an input that breaks a rule; Field is the input
// field at fault.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Msg)
}

// Actor is who a call is made for: the tenant whose runs and schedules it may
// touch. The id comes from the tenants module, resolved by app/; this module
// uses it as an opaque scope key.
type Actor struct {
	TenantID string
}

// NoteSource is what the scan-notes job needs from the notes module.
type NoteSource interface {
	// NoteIDs returns the ids of every note of the tenant.
	NoteIDs(ctx context.Context, tenantID string) ([]string, error)
}

// Cfg holds the dependencies and limits of the Service; zero limits get the
// defaults above.
type Cfg struct {
	// Notes is where the scan-notes job reads the notes; required.
	Notes NoteSource
	// LogDir holds one log file per run; required, created when missing. One
	// directory belongs to one Service: at startup it deletes the logs of
	// runs it does not know.
	LogDir string

	Parallelism int
	QueueSize   int
	HistorySize int

	// Location is the time zone of every cron expression; nil is time.Local.
	Location *time.Location
	// Logger receives the scheduler's fire and skip messages; nil discards them.
	Logger *slog.Logger
	// ScanNote is the work scan-notes does on each note; nil is the stub. Tests
	// inject their own.
	ScanNote func(ctx context.Context, tenantID, noteID string) error
}

// Job is a kind of background work the app defines, with the caller's
// schedules of it.
type Job struct {
	Name        string
	Title       string
	Description string
	// Schedules are the tenant's schedules of the job, oldest first; never nil.
	Schedules []Schedule
}

// definition is a job of the catalogue: what the API shows and what a run does.
type definition struct {
	name        string
	title       string
	description string
	run         func(ctx context.Context, log *slog.Logger, prog tempo.Progress, tenantID string) error
}

// Service runs and schedules the jobs.
type Service struct {
	runner   *tempo.QueueRunner
	sched    *schedule.Scheduler
	logs     *filelog.Store
	progress *tempo.MemTaskProgressSink
	defs     []definition

	notes    NoteSource
	scanNote func(ctx context.Context, tenantID, noteID string) error

	// enqueueMu makes "is a run of this job waiting or running for the tenant?"
	// and the enqueue that follows one step (enqueue).
	enqueueMu sync.Mutex
	// schedMu serializes the read-modify-write of schedules and the count
	// check of a create.
	schedMu sync.Mutex

	lifeMu  sync.Mutex
	started bool
	stopped bool
}

// New builds the Service on db: tempo's queue and schedule tables (migrated
// here), the log directory and the job handlers. Nothing runs until Start.
func New(db *gorm.DB, cfg Cfg) (*Service, error) {
	switch {
	case db == nil:
		return nil, errors.New("jobs: db is required")
	case cfg.Notes == nil:
		return nil, errors.New("jobs: Notes is required")
	case cfg.LogDir == "":
		return nil, errors.New("jobs: LogDir is required")
	}
	applyDefaults(&cfg)

	queueStore, err := dbqueue.New(db)
	if err != nil {
		return nil, fmt.Errorf("jobs: queue store: %w", err)
	}
	logs, err := filelog.New(filelog.Config{Dir: cfg.LogDir, DirPerm: 0o750, FilePerm: 0o640})
	if err != nil {
		return nil, fmt.Errorf("jobs: log directory: %w", err)
	}
	progress := tempo.NewMemTaskProgressSink()
	runner, err := tempo.NewQueueRunner(tempo.RunnerCfg{
		Parallelism:  cfg.Parallelism,
		QueueSize:    cfg.QueueSize,
		HistorySize:  cfg.HistorySize,
		Persistence:  queueStore,
		LogSink:      logs,
		ProgressSink: progress,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: runner: %w", err)
	}

	s := &Service{
		runner:   runner,
		logs:     logs,
		progress: progress,
		notes:    cfg.Notes,
		scanNote: cfg.ScanNote,
	}
	s.defs = s.catalogue()
	// every handler is registered before Start: recovered waiting runs are
	// claimed as soon as the workers start
	for _, d := range s.defs {
		runner.RegisterRaw(d.name, handler(d))
	}

	schedStore, err := dbschedule.New(db)
	if err != nil {
		return nil, fmt.Errorf("jobs: schedule store: %w", err)
	}
	s.sched, err = schedule.New(schedule.Cfg{
		Store:    schedStore,
		Enqueuer: fireEnqueuer{s},
		Logger:   cfg.Logger.With(slog.String("component", "jobs")),
		Location: cfg.Location,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: scheduler: %w", err)
	}
	return s, nil
}

func applyDefaults(cfg *Cfg) {
	if cfg.Parallelism < 1 {
		cfg.Parallelism = DefaultParallelism
	}
	if cfg.QueueSize < 1 {
		cfg.QueueSize = DefaultQueueSize
	}
	if cfg.HistorySize < 1 {
		cfg.HistorySize = DefaultHistorySize
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.ScanNote == nil {
		cfg.ScanNote = scanNoteStub
	}
}

// handler adapts a job to tempo: it decodes the tenant from the run's params.
func handler(d definition) func(context.Context, *slog.Logger, tempo.Progress, []byte) error {
	return func(ctx context.Context, log *slog.Logger, prog tempo.Progress, params []byte) error {
		tenantID := tenantOf(params)
		if tenantID == "" {
			return errors.New("the run has no tenant")
		}
		return d.run(ctx, log, prog, tenantID)
	}
}

// Start starts the workers, then the scheduler, which loads the stored
// schedules. Call ShutDown afterwards, also when Start fails.
func (s *Service) Start(ctx context.Context) error {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if s.started || s.stopped {
		return nil
	}
	s.started = true
	s.runner.StartBg()
	if err := s.sched.Start(ctx); err != nil {
		return fmt.Errorf("jobs: start scheduler: %w", err)
	}
	return nil
}

// ShutDown stops the scheduler, so no fire comes in, then the runner: waiting
// runs end canceled and running ones have their context canceled; it waits
// for them until ctx ends. Calling it again, or without Start, does nothing.
func (s *Service) ShutDown(ctx context.Context) error {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if !s.started || s.stopped {
		s.stopped = true
		return nil
	}
	s.stopped = true
	var errs []error
	if err := s.sched.ShutDown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("jobs: stop scheduler: %w", err))
	}
	if err := s.runner.ShutDown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("jobs: stop runner: %w", err))
	}
	return errors.Join(errs...)
}

// definition returns the job of the catalogue with the given name.
func (s *Service) definition(name string) (definition, bool) {
	for _, d := range s.defs {
		if d.name == name {
			return d, true
		}
	}
	return definition{}, false
}
