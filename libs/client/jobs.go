package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Job is a kind of background work the server defines, with the caller's
// tenant's schedules of it.
type Job struct {
	Name        string        `json:"name"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Schedules   []JobSchedule `json:"schedules"`
}

// JobSchedule runs a job for the caller's tenant whenever Cron fires.
type JobSchedule struct {
	ID string `json:"id"`
	// Cron is the stored 6-field form (seconds first).
	Cron    string `json:"cron"`
	Enabled bool   `json:"enabled"`
	// NextRunAt is nil while the schedule is disabled.
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// JobScheduleInput holds the fields of a new schedule; a nil Enabled is true.
type JobScheduleInput struct {
	Cron    string `json:"cron"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// JobSchedulePatch changes a schedule; a nil field is left as it is.
type JobSchedulePatch struct {
	Cron    *string `json:"cron,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

// JobRun is one execution of a job.
type JobRun struct {
	ID  string `json:"id"`
	Job string `json:"job"`
	// Status is waiting, running, complete, failed, panicked, canceled or cancel_error.
	Status   string    `json:"status"`
	QueuedAt time.Time `json:"queuedAt"`
	// StartedAt and EndedAt are nil until the run starts and ends.
	StartedAt *time.Time `json:"startedAt,omitempty"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	// Progress is set while the run is running.
	Progress *JobProgress `json:"progress,omitempty"`
}

// JobProgress is how far a running run is: Done of Total units of Stage.
type JobProgress struct {
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Stage string `json:"stage,omitempty"`
}

// Jobs returns every job with the caller's tenant's schedules of it.
func (c *Client) Jobs(ctx context.Context) ([]Job, error) {
	var out struct {
		Items []Job `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/jobs", nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// RunJob queues a run of the job for the caller's tenant. When the tenant has
// a run of it waiting or running already, it returns that run with reused
// true. A missing job is ErrNotFound, a full queue ErrQueueFull.
func (c *Client) RunJob(ctx context.Context, job string) (run JobRun, reused bool, err error) {
	status, err := c.call(ctx, http.MethodPost, jobPath(job)+"/runs", nil, &run, http.StatusAccepted, http.StatusOK)
	return run, status == http.StatusOK, err
}

// JobRuns returns the runs of the caller's tenant, newest first.
func (c *Client) JobRuns(ctx context.Context) ([]JobRun, error) {
	var out struct {
		Items []JobRun `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/job-runs", nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// JobRun returns one run of the caller's tenant; another tenant's is ErrNotFound.
func (c *Client) JobRun(ctx context.Context, id string) (JobRun, error) {
	var out JobRun
	err := c.do(ctx, http.MethodGet, runPath(id), nil, http.StatusOK, &out)
	return out, err
}

// CancelJobRun cancels a waiting run or stops a running one; a run that ended
// already is left as it is.
func (c *Client) CancelJobRun(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, runPath(id)+"/cancel", nil, http.StatusNoContent, nil)
}

// JobRunLog returns the last tail lines of a run's log as text; tail 0 is the
// server's default (and maximum) of 1000.
func (c *Client) JobRunLog(ctx context.Context, id string, tail int) (string, error) {
	path := runPath(id) + "/log"
	if tail > 0 {
		path += "?tail=" + strconv.Itoa(tail)
	}
	resp, data, err := c.send(ctx, http.MethodGet, path, nil, "text/plain, application/problem+json")
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", newProblem(resp, data)
	}
	return string(data), nil
}

// CreateJobSchedule adds a schedule of the job for the caller's tenant; a bad
// cron expression, or one schedule too many, is ErrValidation.
func (c *Client) CreateJobSchedule(ctx context.Context, job string, in JobScheduleInput) (JobSchedule, error) {
	var out JobSchedule
	err := c.do(ctx, http.MethodPost, jobPath(job)+"/schedules", in, http.StatusCreated, &out)
	return out, err
}

// UpdateJobSchedule changes a schedule; a missing one is ErrNotFound.
func (c *Client) UpdateJobSchedule(ctx context.Context, job, id string, p JobSchedulePatch) (JobSchedule, error) {
	var out JobSchedule
	err := c.do(ctx, http.MethodPatch, schedulePath(job, id), p, http.StatusOK, &out)
	return out, err
}

// DeleteJobSchedule deletes a schedule; a missing one is ErrNotFound.
func (c *Client) DeleteJobSchedule(ctx context.Context, job, id string) error {
	return c.do(ctx, http.MethodDelete, schedulePath(job, id), nil, http.StatusNoContent, nil)
}

func jobPath(job string) string { return "/jobs/" + url.PathEscape(job) }

func schedulePath(job, id string) string {
	return jobPath(job) + "/schedules/" + url.PathEscape(id)
}

func runPath(id string) string { return "/job-runs/" + url.PathEscape(id) }
