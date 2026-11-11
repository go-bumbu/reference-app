package jobs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-bumbu/tempo/schedule"
	"github.com/google/uuid"
)

// MaxCronLen caps the length of a cron expression.
const MaxCronLen = 100

// Schedule runs a job for its tenant whenever Cron fires.
type Schedule struct {
	ID  string
	Job string
	// Cron is the stored 6-field form (seconds first).
	Cron    string
	Enabled bool
	// NextRunAt is when it fires next; zero while it is disabled.
	NextRunAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ScheduleInput holds the fields of a new schedule.
type ScheduleInput struct {
	Cron    string
	Enabled bool
}

// SchedulePatch changes a schedule; a nil field is left as it is.
type SchedulePatch struct {
	Cron    *string
	Enabled *bool
}

// Jobs returns every job of the catalogue with the actor's tenant's schedules
// of it.
func (s *Service) Jobs(ctx context.Context, a Actor) ([]Job, error) {
	if a.TenantID == "" {
		return nil, ErrNoTenant
	}
	owned, err := s.tenantSchedules(ctx, a.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(s.defs))
	for _, d := range s.defs {
		j := Job{Name: d.name, Title: d.title, Description: d.description, Schedules: []Schedule{}}
		for _, sc := range owned {
			if sc.Job == d.name {
				j.Schedules = append(j.Schedules, sc)
			}
		}
		out = append(out, j)
	}
	return out, nil
}

// CreateSchedule adds a schedule that runs the job for the actor's tenant.
func (s *Service) CreateSchedule(ctx context.Context, a Actor, job string, in ScheduleInput) (Schedule, error) {
	if err := s.checkJob(a, job); err != nil {
		return Schedule{}, err
	}
	cron, err := validateCron(in.Cron)
	if err != nil {
		return Schedule{}, err
	}
	params, err := paramsFor(a.TenantID)
	if err != nil {
		return Schedule{}, err
	}

	s.schedMu.Lock()
	defer s.schedMu.Unlock()
	owned, err := s.tenantSchedules(ctx, a.TenantID)
	if err != nil {
		return Schedule{}, err
	}
	n := 0
	for _, sc := range owned {
		if sc.Job == job {
			n++
		}
	}
	if n >= MaxSchedulesPerJob {
		return Schedule{}, ErrTooManySchedules
	}
	info, err := s.sched.Create(ctx, schedule.Schedule{TaskName: job, Cron: cron, Params: params, Enabled: in.Enabled})
	if err != nil {
		return Schedule{}, fmt.Errorf("create schedule: %w", err)
	}
	return toSchedule(info), nil
}

// UpdateSchedule changes the cron expression or the enabled flag of one of
// the actor's tenant's schedules of the job.
func (s *Service) UpdateSchedule(ctx context.Context, a Actor, job, id string, p SchedulePatch) (Schedule, error) {
	if err := s.checkJob(a, job); err != nil {
		return Schedule{}, err
	}
	var cron string
	if p.Cron != nil {
		c, err := validateCron(*p.Cron)
		if err != nil {
			return Schedule{}, err
		}
		cron = c
	}

	s.schedMu.Lock()
	defer s.schedMu.Unlock()
	info, err := s.ownedSchedule(ctx, a, job, id)
	if err != nil {
		return Schedule{}, err
	}
	// tempo's Update replaces the whole schedule: start from the stored one
	upd := info.Schedule
	if p.Cron != nil {
		upd.Cron = cron
	}
	if p.Enabled != nil {
		upd.Enabled = *p.Enabled
	}
	out, err := s.sched.Update(ctx, upd)
	if errors.Is(err, schedule.ErrScheduleNotFound) {
		return Schedule{}, ErrScheduleNotFound
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("update schedule: %w", err)
	}
	return toSchedule(out), nil
}

// DeleteSchedule deletes one of the actor's tenant's schedules of the job.
// Runs it queued already are not affected.
func (s *Service) DeleteSchedule(ctx context.Context, a Actor, job, id string) error {
	if err := s.checkJob(a, job); err != nil {
		return err
	}
	s.schedMu.Lock()
	defer s.schedMu.Unlock()
	info, err := s.ownedSchedule(ctx, a, job, id)
	if err != nil {
		return err
	}
	err = s.sched.Delete(ctx, info.ID)
	if errors.Is(err, schedule.ErrScheduleNotFound) {
		return ErrScheduleNotFound
	}
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	return nil
}

func (s *Service) checkJob(a Actor, job string) error {
	if a.TenantID == "" {
		return ErrNoTenant
	}
	if _, ok := s.definition(job); !ok {
		return ErrJobNotFound
	}
	return nil
}

// tenantSchedules returns the tenant's schedules of every job, oldest first.
// The scheduler has no lookup by tenant (nor by job): it lists them all, and
// the tenant comes from each schedule's params.
func (s *Service) tenantSchedules(ctx context.Context, tenantID string) ([]Schedule, error) {
	all, err := s.sched.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	var out []Schedule
	for _, info := range all {
		if tenantOf(info.Params) == tenantID {
			out = append(out, toSchedule(info))
		}
	}
	slices.SortFunc(out, func(x, y Schedule) int {
		if c := x.CreatedAt.Compare(y.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(x.ID, y.ID)
	})
	return out, nil
}

// ownedSchedule returns the schedule id names when it is the actor's tenant's
// schedule of job; anything else is ErrScheduleNotFound.
func (s *Service) ownedSchedule(ctx context.Context, a Actor, job, id string) (schedule.ScheduleInfo, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return schedule.ScheduleInfo{}, ErrScheduleNotFound
	}
	info, err := s.sched.Get(ctx, uid)
	if errors.Is(err, schedule.ErrScheduleNotFound) {
		return schedule.ScheduleInfo{}, ErrScheduleNotFound
	}
	if err != nil {
		return schedule.ScheduleInfo{}, fmt.Errorf("read schedule: %w", err)
	}
	if info.TaskName != job || tenantOf(info.Params) != a.TenantID {
		return schedule.ScheduleInfo{}, ErrScheduleNotFound
	}
	return info, nil
}

// validateCron checks a cron expression and returns its stored 6-field form.
// Besides being valid, it must fire at most once a minute: the seconds field
// is one number. A tenant cannot keep the shared workers busy every second.
func validateCron(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	switch {
	case expr == "":
		return "", &ValidationError{Field: "cron", Msg: "must not be empty"}
	case utf8.RuneCountInString(expr) > MaxCronLen:
		return "", &ValidationError{Field: "cron", Msg: fmt.Sprintf("must be at most %d characters", MaxCronLen)}
	}
	if err := schedule.ValidateCron(expr); err != nil {
		return "", &ValidationError{Field: "cron", Msg: err.Error()}
	}
	cron := schedule.NormalizeCron(expr)
	seconds := strings.Fields(cron)[0]
	if strings.Trim(seconds, "0123456789") != "" {
		return "", &ValidationError{Field: "cron", Msg: "may fire at most once a minute: the seconds field must be one number"}
	}
	return cron, nil
}

// toSchedule maps a tempo schedule onto a Schedule. Its times are truncated to
// microseconds, the finest every database keeps: tempo returns a created
// schedule with nanoseconds, which Postgres would not give back on a later read.
func toSchedule(info schedule.ScheduleInfo) Schedule {
	sc := Schedule{
		ID:        info.ID.String(),
		Job:       info.TaskName,
		Cron:      info.Cron,
		Enabled:   info.Enabled,
		CreatedAt: utc(info.CreatedAt).Truncate(time.Microsecond),
		UpdatedAt: utc(info.UpdatedAt).Truncate(time.Microsecond),
	}
	if info.NextFireAt != nil {
		sc.NextRunAt = info.NextFireAt.UTC()
	}
	return sc
}
