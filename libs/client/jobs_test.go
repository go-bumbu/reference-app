package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	runJSON = `{"id":"r1","job":"scan-notes","status":"running","queuedAt":"2026-01-02T03:04:05Z",` +
		`"startedAt":"2026-01-02T03:04:06Z","progress":{"done":1,"total":3,"stage":"Scanning notes"}}`
	schedJSON = `{"id":"s1","cron":"0 0 3 * * *","enabled":true,"nextRunAt":"2026-01-03T03:00:00Z",` +
		`"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-02T03:04:05Z"}`
)

// jobsServer answers the jobs paths with canned responses. A run of "busy"
// is reused (200), "full" has a full queue (429).
func jobsServer(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.EscapedPath() + "?" + r.URL.RawQuery {
		case "GET /api/v0/jobs?":
			writeJSON(w, http.StatusOK, `{"items":[{"name":"scan-notes","title":"Scan notes","description":"d","schedules":[`+schedJSON+`]}]}`)
		case "POST /api/v0/jobs/scan-notes/runs?":
			writeJSON(w, http.StatusAccepted, runJSON)
		case "POST /api/v0/jobs/busy/runs?":
			writeJSON(w, http.StatusOK, runJSON)
		case "POST /api/v0/jobs/full/runs?":
			writeProblem(w, http.StatusTooManyRequests, "queue_full", "the job queue is full")
		case "GET /api/v0/job-runs?":
			writeJSON(w, http.StatusOK, `{"items":[`+runJSON+`]}`)
		case "GET /api/v0/job-runs/r1?":
			writeJSON(w, http.StatusOK, runJSON)
		case "POST /api/v0/job-runs/r1/cancel?", "DELETE /api/v0/jobs/scan-notes/schedules/s1?":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v0/job-runs/r1/log?tail=2":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, "line 1\nline 2\n")
		case "GET /api/v0/job-runs/gone/log?":
			writeProblem(w, http.StatusNotFound, "not_found", "job run gone not found")
		case "POST /api/v0/jobs/scan-notes/schedules?":
			writeJSON(w, http.StatusCreated, schedJSON)
		case "PATCH /api/v0/jobs/scan-notes/schedules/s1?":
			writeJSON(w, http.StatusOK, schedJSON)
		default:
			t.Errorf("unexpected request %s %s?%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery)
			http.NotFound(w, r)
		}
	})
}

func TestJobs(t *testing.T) {
	srv := httptest.NewServer(jobsServer(t))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	list, err := c.Jobs(ctx)
	if err != nil || len(list) != 1 || len(list[0].Schedules) != 1 || list[0].Schedules[0].NextRunAt == nil {
		t.Fatalf("jobs: got %+v, %v", list, err)
	}
	run, reused, err := c.RunJob(ctx, "scan-notes")
	if err != nil || reused || run.Progress == nil || run.Progress.Total != 3 || run.EndedAt != nil {
		t.Errorf("run job: got %+v reused %v, %v", run, reused, err)
	}
	if _, reused, err := c.RunJob(ctx, "busy"); err != nil || !reused {
		t.Errorf("run a busy job: reused %v, %v; want reused", reused, err)
	}
	if _, _, err := c.RunJob(ctx, "full"); !errors.Is(err, ErrQueueFull) {
		t.Errorf("run with a full queue: got %v, want ErrQueueFull", err)
	}
	if runs, err := c.JobRuns(ctx); err != nil || len(runs) != 1 {
		t.Errorf("runs: got %+v, %v", runs, err)
	}
	if got, err := c.JobRun(ctx, "r1"); err != nil || got.ID != "r1" {
		t.Errorf("run: got %+v, %v", got, err)
	}
	if err := c.CancelJobRun(ctx, "r1"); err != nil {
		t.Errorf("cancel: %v", err)
	}
	if text, err := c.JobRunLog(ctx, "r1", 2); err != nil || text != "line 1\nline 2\n" {
		t.Errorf("log: got %q, %v", text, err)
	}
	if _, err := c.JobRunLog(ctx, "gone", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("log of a missing run: got %v, want ErrNotFound", err)
	}
	if s, err := c.CreateJobSchedule(ctx, "scan-notes", JobScheduleInput{Cron: "0 3 * * *"}); err != nil || s.ID != "s1" {
		t.Errorf("create schedule: got %+v, %v", s, err)
	}
	on := true
	if s, err := c.UpdateJobSchedule(ctx, "scan-notes", "s1", JobSchedulePatch{Enabled: &on}); err != nil || !s.Enabled {
		t.Errorf("update schedule: got %+v, %v", s, err)
	}
	if err := c.DeleteJobSchedule(ctx, "scan-notes", "s1"); err != nil {
		t.Errorf("delete schedule: %v", err)
	}
}
