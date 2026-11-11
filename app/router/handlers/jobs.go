package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/app/reqctx"
	"github.com/go-bumbu/reference-app/internal/jobs"
	"github.com/gorilla/mux"
)

// JobsService is what the jobs handler needs from the internal/jobs module.
type JobsService interface {
	Jobs(ctx context.Context, a jobs.Actor) ([]jobs.Job, error)
	Trigger(ctx context.Context, a jobs.Actor, job string) (jobs.Run, bool, error)
	Runs(ctx context.Context, a jobs.Actor) ([]jobs.Run, error)
	Run(ctx context.Context, a jobs.Actor, id string) (jobs.Run, error)
	Cancel(ctx context.Context, a jobs.Actor, id string) error
	Log(ctx context.Context, a jobs.Actor, id string, tail int) ([]jobs.LogLine, error)
	CreateSchedule(ctx context.Context, a jobs.Actor, job string, in jobs.ScheduleInput) (jobs.Schedule, error)
	UpdateSchedule(ctx context.Context, a jobs.Actor, job, id string, p jobs.SchedulePatch) (jobs.Schedule, error)
	DeleteSchedule(ctx context.Context, a jobs.Actor, job, id string) error
}

// maxLogTail is the most log lines GET /job-runs/{runId}/log returns, and its default.
const maxLogTail = 1000

// SlugQueueFull is the problem slug of a 429 from a full job queue.
const SlugQueueFull = "queue_full"

// JobsHandler translates the /jobs and /job-runs HTTP API into calls to the
// jobs module. Its routes must sit behind the auth guard.
type JobsHandler struct {
	Jobs     JobsService
	Problems *problemjson.Writer
	Logger   *slog.Logger
}

// jobResponse is schema Job.
type jobResponse struct {
	Name        string             `json:"name"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Schedules   []scheduleResponse `json:"schedules"`
}

type jobListResponse struct {
	Items []jobResponse `json:"items"`
}

// scheduleResponse is schema JobSchedule.
type scheduleResponse struct {
	ID        string     `json:"id"`
	Cron      string     `json:"cron"`
	Enabled   bool       `json:"enabled"`
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// scheduleRequest is the body of POST /jobs/{jobName}/schedules (schema JobScheduleInput).
type scheduleRequest struct {
	Cron    string `json:"cron"`
	Enabled *bool  `json:"enabled"`
}

// schedulePatchRequest is schema JobSchedulePatch; a null field decodes as absent.
type schedulePatchRequest struct {
	Cron    *string `json:"cron"`
	Enabled *bool   `json:"enabled"`
}

// runResponse is schema JobRun.
type runResponse struct {
	ID        string            `json:"id"`
	Job       string            `json:"job"`
	Status    string            `json:"status"`
	QueuedAt  time.Time         `json:"queuedAt"`
	StartedAt *time.Time        `json:"startedAt,omitempty"`
	EndedAt   *time.Time        `json:"endedAt,omitempty"`
	Progress  *progressResponse `json:"progress,omitempty"`
}

// progressResponse is schema JobProgress.
type progressResponse struct {
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Stage string `json:"stage,omitempty"`
}

type runListResponse struct {
	Items []runResponse `json:"items"`
}

// optionalTime maps a zero time onto an absent field.
func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func toScheduleResponse(s jobs.Schedule) scheduleResponse {
	return scheduleResponse{
		ID: s.ID, Cron: s.Cron, Enabled: s.Enabled, NextRunAt: optionalTime(s.NextRunAt),
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func toRunResponse(r jobs.Run) runResponse {
	out := runResponse{
		ID: r.ID, Job: r.Job, Status: r.Status, QueuedAt: r.QueuedAt,
		StartedAt: optionalTime(r.StartedAt), EndedAt: optionalTime(r.EndedAt),
	}
	if r.Progress != nil {
		out.Progress = &progressResponse{Done: r.Progress.Done, Total: r.Progress.Total, Stage: r.Progress.Stage}
	}
	return out
}

// jobsActor maps the authenticated caller onto the jobs module's Actor.
func jobsActor(r *http.Request) (jobs.Actor, bool) {
	id, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		return jobs.Actor{}, false
	}
	return jobs.Actor{TenantID: id.TenantID}, true
}

// List handles GET /jobs.
func (h *JobsHandler) List(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	list, err := h.Jobs.Jobs(r.Context(), a)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	resp := jobListResponse{Items: make([]jobResponse, 0, len(list))}
	for _, j := range list {
		jr := jobResponse{Name: j.Name, Title: j.Title, Description: j.Description,
			Schedules: make([]scheduleResponse, 0, len(j.Schedules))}
		for _, s := range j.Schedules {
			jr.Schedules = append(jr.Schedules, toScheduleResponse(s))
		}
		resp.Items = append(resp.Items, jr)
	}
	writeJSON(w, http.StatusOK, resp)
}

// RunJob handles POST /jobs/{jobName}/runs: 202 with a new run, or 200 with
// the run the tenant already has waiting or running.
func (h *JobsHandler) RunJob(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	run, reused, err := h.Jobs.Trigger(r.Context(), a, mux.Vars(r)["jobName"])
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if reused {
		writeJSON(w, http.StatusOK, toRunResponse(run))
		return
	}
	w.Header().Set("Location", jobRunsPath(r)+"/"+run.ID)
	writeJSON(w, http.StatusAccepted, toRunResponse(run))
}

// jobRunsPath is the URL path of /job-runs under the mount prefix the request
// came through (the part of its path before /jobs/).
func jobRunsPath(r *http.Request) string {
	prefix, _, _ := strings.Cut(r.URL.Path, "/jobs/")
	return prefix + "/job-runs"
}

// ListRuns handles GET /job-runs.
func (h *JobsHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	list, err := h.Jobs.Runs(r.Context(), a)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	resp := runListResponse{Items: make([]runResponse, 0, len(list))}
	for _, run := range list {
		resp.Items = append(resp.Items, toRunResponse(run))
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetRun handles GET /job-runs/{runId}.
func (h *JobsHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	run, err := h.Jobs.Run(r.Context(), a, mux.Vars(r)["runId"])
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toRunResponse(run))
}

// CancelRun handles POST /job-runs/{runId}/cancel.
func (h *JobsHandler) CancelRun(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	if err := h.Jobs.Cancel(r.Context(), a, mux.Vars(r)["runId"]); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RunLog handles GET /job-runs/{runId}/log: the last `tail` lines as text.
func (h *JobsHandler) RunLog(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	tail := maxLogTail
	if raw, set := r.URL.Query()["tail"]; set {
		n, err := strconv.Atoi(raw[0])
		if err != nil || n < 1 || n > maxLogTail || len(raw) > 1 {
			h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest,
				"tail must be one integer from 1 to "+strconv.Itoa(maxLogTail))
			return
		}
		tail = n
	}
	lines, err := h.Jobs.Log(r.Context(), a, mux.Vars(r)["runId"], tail)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.At.Format(time.RFC3339Nano))
		b.WriteByte(' ')
		b.WriteString(l.Level)
		b.WriteByte(' ')
		b.WriteString(l.Message)
		b.WriteByte('\n')
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

// CreateSchedule handles POST /jobs/{jobName}/schedules.
func (h *JobsHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req scheduleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest, err.Error())
		return
	}
	in := jobs.ScheduleInput{Cron: req.Cron, Enabled: true}
	if req.Enabled != nil {
		in.Enabled = *req.Enabled
	}
	s, err := h.Jobs.CreateSchedule(r.Context(), a, mux.Vars(r)["jobName"], in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Location", r.URL.Path+"/"+s.ID)
	writeJSON(w, http.StatusCreated, toScheduleResponse(s))
}

// UpdateSchedule handles PATCH /jobs/{jobName}/schedules/{scheduleId}.
func (h *JobsHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req schedulePatchRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest, err.Error())
		return
	}
	vars := mux.Vars(r)
	s, err := h.Jobs.UpdateSchedule(r.Context(), a, vars["jobName"], vars["scheduleId"],
		jobs.SchedulePatch{Cron: req.Cron, Enabled: req.Enabled})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toScheduleResponse(s))
}

// DeleteSchedule handles DELETE /jobs/{jobName}/schedules/{scheduleId}.
func (h *JobsHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	if err := h.Jobs.DeleteSchedule(r.Context(), a, vars["jobName"], vars["scheduleId"]); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// actor returns the caller, or answers 500: a route without identity is a
// wiring bug.
func (h *JobsHandler) actor(w http.ResponseWriter, r *http.Request) (jobs.Actor, bool) {
	a, ok := jobsActor(r)
	if !ok {
		h.writeError(w, r, errNoIdentity)
	}
	return a, ok
}

// writeError maps the errors of the jobs module onto problems: the not-found
// errors 404, ErrQueueFull 429, ErrTooManySchedules 422, a *ValidationError
// 422 with the field's pointer, anything else a logged 500.
func (h *JobsHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var verr *jobs.ValidationError
	switch {
	case errors.Is(err, jobs.ErrJobNotFound):
		h.Problems.Write(w, r, http.StatusNotFound, problemjson.SlugNotFound, "job "+mux.Vars(r)["jobName"]+" not found")
	case errors.Is(err, jobs.ErrRunNotFound):
		h.Problems.Write(w, r, http.StatusNotFound, problemjson.SlugNotFound, "job run "+mux.Vars(r)["runId"]+" not found")
	case errors.Is(err, jobs.ErrScheduleNotFound):
		h.Problems.Write(w, r, http.StatusNotFound, problemjson.SlugNotFound,
			"job schedule "+mux.Vars(r)["scheduleId"]+" not found")
	case errors.Is(err, jobs.ErrQueueFull):
		h.Problems.Write(w, r, http.StatusTooManyRequests, SlugQueueFull, "the job queue is full, try again later")
	case errors.Is(err, jobs.ErrTooManySchedules):
		h.Problems.Write(w, r, http.StatusUnprocessableEntity, problemjson.SlugValidationError, err.Error())
	case errors.As(err, &verr):
		h.Problems.WriteValidation(w, r, verr.Error(),
			problemjson.FieldError{Pointer: "/" + verr.Field, Detail: verr.Msg})
	default:
		h.Logger.Error("jobs handler", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
		h.Problems.Write(w, r, http.StatusInternalServerError, problemjson.SlugInternal, "unexpected error")
	}
}
