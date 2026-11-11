package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/go-bumbu/tempo"
	"github.com/google/uuid"
)

// Run is one execution of a job.
type Run struct {
	ID  string
	Job string
	// Status is tempo's: waiting, running, complete, failed, panicked,
	// canceled or cancel_error (a cancel that timed out; it still ends with
	// the status its own return gives).
	Status   string
	QueuedAt time.Time
	// StartedAt and EndedAt are zero until the run starts and ends.
	StartedAt time.Time
	EndedAt   time.Time
	// Progress is the latest progress of a running run; nil otherwise.
	Progress *Progress
}

// Progress is how far a running run is: Done of Total units of its current
// Stage. Total is 0 while the size is not known.
type Progress struct {
	Done  int64
	Total int64
	Stage string
}

// LogLine is one line of a run's log.
type LogLine struct {
	At      time.Time
	Level   string
	Message string
}

// runParams is the payload of every run: the tenant it works for.
type runParams struct {
	TenantID string `json:"tenant_id"`
}

// paramsFor returns the params of a run, or a schedule, for the tenant.
func paramsFor(tenantID string) ([]byte, error) {
	b, err := json.Marshal(runParams{TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("encode run params: %w", err)
	}
	return b, nil
}

// tenantOf returns the tenant of a run's params, empty when they carry none.
func tenantOf(params []byte) string {
	var p runParams
	if err := json.Unmarshal(params, &p); err != nil {
		return ""
	}
	return p.TenantID
}

// Trigger queues a run of the job for the actor's tenant. While the tenant has
// a run of the job waiting or running, it returns that run with reused true
// instead of queueing a second one.
func (s *Service) Trigger(_ context.Context, a Actor, job string) (run Run, reused bool, err error) {
	if a.TenantID == "" {
		return Run{}, false, ErrNoTenant
	}
	if _, ok := s.definition(job); !ok {
		return Run{}, false, ErrJobNotFound
	}
	info, reused, err := s.enqueue(job, a.TenantID)
	if err != nil {
		return Run{}, false, err
	}
	return s.toRun(info), reused, nil
}

// enqueue queues a run of job for the tenant unless one is waiting or running
// already. The check and the enqueue happen under one lock, so two concurrent
// triggers cannot both queue. Schedule fires come through here too, so a
// tenant has at most one waiting or running run per job: one tenant cannot
// fill the queue every other tenant shares.
func (s *Service) enqueue(job, tenantID string) (tempo.TaskInfo, bool, error) {
	s.enqueueMu.Lock()
	defer s.enqueueMu.Unlock()
	for _, t := range s.runner.List() {
		active := t.Status == tempo.TaskStatusWaiting || t.Status == tempo.TaskStatusRunning
		if active && t.Name == job && tenantOf(t.Params) == tenantID {
			return t, true, nil
		}
	}
	params, err := paramsFor(tenantID)
	if err != nil {
		return tempo.TaskInfo{}, false, err
	}
	id, _, err := s.runner.AddRaw(job, params)
	if errors.Is(err, tempo.ErrQueueFull) {
		return tempo.TaskInfo{}, false, ErrQueueFull
	}
	if err != nil {
		return tempo.TaskInfo{}, false, fmt.Errorf("enqueue %s: %w", job, err)
	}
	info, err := s.runner.GetTask(id)
	if err != nil {
		return tempo.TaskInfo{}, false, fmt.Errorf("read queued run: %w", err)
	}
	return info, false, nil
}

// fireEnqueuer is what the scheduler enqueues through: each fire goes through
// enqueue's per-tenant check, so a fire while the tenant's run is still going
// is skipped (logged by the scheduler) instead of piling up.
type fireEnqueuer struct{ s *Service }

func (f fireEnqueuer) AddRaw(job string, params []byte) (uuid.UUID, bool, error) {
	tenantID := tenantOf(params)
	if tenantID == "" {
		return uuid.Nil, false, errors.New("jobs: schedule has no tenant")
	}
	info, reused, err := f.s.enqueue(job, tenantID)
	return info.ID, reused, err
}

// Runs returns the actor's tenant's runs, newest first: those waiting or
// running and the history the runner keeps.
func (s *Service) Runs(_ context.Context, a Actor) ([]Run, error) {
	if a.TenantID == "" {
		return nil, ErrNoTenant
	}
	tasks := s.runner.List()
	slices.SortFunc(tasks, func(x, y tempo.TaskInfo) int {
		if c := y.QueuedAt.Compare(x.QueuedAt); c != 0 {
			return c
		}
		return strings.Compare(x.ID.String(), y.ID.String())
	})
	out := make([]Run, 0, len(tasks))
	for _, t := range tasks {
		if _, ok := s.definition(t.Name); ok && tenantOf(t.Params) == a.TenantID {
			out = append(out, s.toRun(t))
		}
	}
	return out, nil
}

// Run returns one run of the actor's tenant, or ErrRunNotFound.
func (s *Service) Run(_ context.Context, a Actor, id string) (Run, error) {
	t, err := s.ownedTask(a, id)
	if err != nil {
		return Run{}, err
	}
	return s.toRun(t), nil
}

// Cancel cancels a waiting run, or stops a running one and waits for it to
// end, bounded by ctx. A run that already ended is left as it is.
func (s *Service) Cancel(ctx context.Context, a Actor, id string) error {
	t, err := s.ownedTask(a, id)
	if err != nil {
		return err
	}
	if ended(t.Status) {
		return nil
	}
	if err := s.runner.Cancel(ctx, t.ID); err != nil {
		// it may have ended between the read and the cancel
		if now, gerr := s.runner.GetTask(t.ID); gerr == nil && ended(now.Status) {
			return nil
		}
		return fmt.Errorf("cancel run %s: %w", id, err)
	}
	return nil
}

// Log returns the last tail lines of a run's log (every line when tail < 1).
func (s *Service) Log(ctx context.Context, a Actor, id string, tail int) ([]LogLine, error) {
	t, err := s.ownedTask(a, id)
	if err != nil {
		return nil, err
	}
	entries, err := s.logs.Logs(ctx, t.ID)
	if err != nil {
		return nil, fmt.Errorf("read run log: %w", err)
	}
	if tail > 0 && len(entries) > tail {
		entries = entries[len(entries)-tail:]
	}
	out := make([]LogLine, len(entries))
	for i, e := range entries {
		out[i] = LogLine{At: e.At.UTC(), Level: e.Level, Message: e.Message}
	}
	return out, nil
}

// ownedTask returns the run id names when it is a job run of the actor's
// tenant; anything else is ErrRunNotFound.
func (s *Service) ownedTask(a Actor, id string) (tempo.TaskInfo, error) {
	if a.TenantID == "" {
		return tempo.TaskInfo{}, ErrNoTenant
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return tempo.TaskInfo{}, ErrRunNotFound
	}
	t, err := s.runner.GetTask(uid)
	if errors.Is(err, tempo.ErrTaskNotFound) {
		return tempo.TaskInfo{}, ErrRunNotFound
	}
	if err != nil {
		return tempo.TaskInfo{}, fmt.Errorf("read run: %w", err)
	}
	if _, ok := s.definition(t.Name); !ok || tenantOf(t.Params) != a.TenantID {
		return tempo.TaskInfo{}, ErrRunNotFound
	}
	return t, nil
}

func ended(st tempo.TaskStatus) bool {
	return slices.Contains(tempo.TaskTerminalStatus, st)
}

// toRun maps a tempo task onto a Run, with the progress of a running one.
func (s *Service) toRun(t tempo.TaskInfo) Run {
	r := Run{
		ID:        t.ID.String(),
		Job:       t.Name,
		Status:    t.Status.Str(),
		QueuedAt:  utc(t.QueuedAt),
		StartedAt: utc(t.StartedAt),
		EndedAt:   utc(t.EndedAt),
	}
	// progress is a live-run thing: an ended run never shows a stale count
	if t.Status == tempo.TaskStatusRunning {
		if p, ok, err := s.progress.Progress(context.Background(), t.ID); err == nil && ok {
			r.Progress = &Progress{Done: p.Done, Total: p.Total, Stage: p.Stage}
		}
	}
	return r
}

// utc converts t to UTC, keeping the zero time zero.
func utc(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC()
}
