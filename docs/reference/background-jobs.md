# Background jobs — durable tasks and schedules with go-bumbu/tempo

How a new app runs work outside a request: queued tasks, cron schedules, progress and logs that users can read.
aether and codex use [go-bumbu/tempo](https://github.com/go-bumbu/tempo) the same way, and this guide records their
common shape. Lifecycle background: [server-lifecycle.md](server-lifecycle.md).

**reference-app's own jobs are the tenant-facing variant** (`internal/jobs`, one job `scan-notes`, the web UI's
Settings → Jobs after aether's Tasks page). Copy it when tenants run and watch their own jobs. It differs from the
shape below in three deliberate ways, recorded in [../agents/architecture.md](../agents/architecture.md):

- the module wraps tempo instead of exposing it, because the tenant filter must sit between tempo and every caller;
- the API is the tenant API (`/api/v0/jobs`, `/api/v0/job-runs`), not the ops port, because the web UI drives it;
- instead of `WithSingleton` (which would drop other tenants' runs), the module itself keeps one waiting or running
  run per tenant and job, and the scheduler enqueues through that same check.

For ops work across tenants (the shape below), read codex's `internal/tasks/` and its `docs/agents/tasks.md`.

## When to use a task runner

- **Use tempo** when work outlives a request, runs on a schedule, must survive a restart, or users want to see its
  progress, logs or history.
- **Use a plain goroutine** for an in-process loop that nobody watches, such as a cache refresher. Follow
  "Adding a background worker" in [server-lifecycle.md](server-lifecycle.md).

## Wiring (`internal/tasks`, constructed in `app/cmd/server.go`)

- **Keep the engine a holder, not an abstraction.** `NewEngine(EngineCfg) (*Engine, error)` exposes tempo's own
  types (`Runner`, `Scheduler`, `Logs`, `Progress`) and has no forwarding methods. Callers use tempo's API
  directly. If tempo lacks something, fix it upstream rather than wrapping it ([project-layout.md](project-layout.md)).
  *(aether wraps tempo in `internal/taskrunner` to allow several schedules per task. Check whether tempo does that
  now before you write such a wrapper.)*
- **Build it on the shared `*gorm.DB`**, right after the database opens: `dbqueue` (durable queue, table
  `tempo_tasks`), `filelog` (one JSON-Lines log per run under `<DataDir>/task-logs`), the queue runner with a
  progress sink, the task registrations, `dbschedule` (table `tempo_schedules`), then the scheduler.
- **Pass the runner to the tasks as their `Enqueuer`**, so a task can queue a follow-up on the runner it runs on.
- **Run the engine always**, even when the ops port is off. Code in the app can then always enqueue.
- **Start the scheduler before you touch schedules.** `Scheduler.Create` and `Delete` need a started scheduler.
- **Reconcile code-defined schedules at startup** (`ensureSchedules`): create the ones that are missing and delete
  the ones the code no longer wants.
- **Shut down in order:** stop the scheduler (no new fires), then drain the runner, and join both errors. Give it its
  own timeout in the shutdown hook, separate from the HTTP drain.
- Config: `Tasks.Parallelism` (2), `QueueSize` (100), `HistorySize` (50), `LogDir` (empty → `<DataDir>/task-logs`).
  All are optional.

## Defining a task

1. Add a `TaskDef{Name, Title, Description}` to the catalogue. The name is the task's stable id.
2. Write the handler: `func(ctx, *slog.Logger, tempo.Progress, params []byte) error`. Decode `params` yourself and
   do the work through the module's API, like a handler would.
3. Register it with the right concurrency option:
   - `tempo.WithSingleton()` coalesces queued triggers **by task name and ignores `params`**. See the tenancy trap
     below.
   - Tasks that must never overlap each other share `tempo.WithMaxParallelism(1)` and one
     `tempo.WithExclusionGroup`. *(aether: `scan`, `scan-full`, `reindex` and `prune` share `library-writes`)*
4. Report progress as `{done, total, stage}`. Set `total` back to 0 for a step without one, so an old count never
   reads as the next step's.

- **Prefer two parameterless tasks over one task with a mode flag** when the modes need their own schedules or
  their own singleton behaviour. *(aether: `scan` and `scan-full`)*
- **Chain follow-up work best-effort.** Queue it from the task that produced its input. If the queue refuses it,
  log that and carry on. A user's create request never fails because a follow-up could not be queued. *(codex:
  creating a bookmark queues a scrape, and a successful scrape queues a classify)*

## Tenancy of jobs

- tempo's tables are app-global and have no `tenant_id`. That is fine, because they hold no domain data.
- **Every task's `params` carries `tenant_id`**, plus any sub-partition (codex: `space_id`). The handler decodes it
  and calls the tenant-scoped store. A per-request enqueue takes these IDs from the request identity, never from the
  body.
- **A user-facing jobs API filters `Runner.List()` by the decoded params.** It needs no run table of its own. A run of
  another tenant is a `404`. A task whose params lack the IDs is invisible to users by construction. That is
  correct only for ops work.
- **The singleton trap:** a singleton task enqueued once per tenant collapses into a single run, and the other
  tenants' work is silently dropped. A scheduled task that serves every tenant fires once and **fans out inside the
  run** over `AllTenants()`. That is the one cross-tenant store method. Document it as "task fan-out only; never call
  from a request handler".
- **A per-tenant schedule is a row owned by the tenant** (with `tenant_id`). Only a route that checked the tenant
  writes it, and it is deleted with its owner. The startup reconcile is the only code that reads all of them.

## Run history and logs

- **The history cap is shared by all tasks** (`HistorySize`). A schedule that fires every few minutes fills it and
  pushes the other tasks' runs out, and their logs are reaped with them. Do not schedule frequent housekeeping that
  nobody looks at. *(codex removed its 10-minute sweeps for this reason.)*
- Remove orphaned log files at startup and delete logs together with their history. *(aether: `RetainOnly`,
  `RemoveTasks`)*
- Logs that users read must not show server paths or internal addresses. Name a configured root by its name.
  *(codex: `scanRun.shownPath`)*
- A log endpoint takes a bounded `?tail=` (1 to 1000), so the UI can poll the last lines of a running job.

## Ops API (admin port)

Serve task management on the ops port ([server-lifecycle.md](server-lifecycle.md)), never on the tenant API. The
handler only translates JSON and calls tempo:

| Route | Notes |
|---|---|
| `GET /taskrunner/tasks[/{name}]` | the catalogue with each task's schedules |
| `GET /taskrunner/executions` | filter by `?task=`, `?status=` |
| `POST /taskrunner/executions` | body `{task, params}` or `{schedule_id}`; `201` + `Location`, or `200` when a running singleton was reused; `429` when the queue is full |
| `DELETE /taskrunner/executions/{id}` | cancel; `204` even if the run already ended |
| `GET /taskrunner/executions/{id}/log` | `text/plain` |
| `/taskrunner/schedules[/{id}]` | CRUD; validate the cron expression and the task name |

Executions and schedules have tempo's UUID string ids. If your spec says "ids are integers", record this as the
exception.

## Tests

- A catalogue test: every registered task is in the catalogue and the reverse.
- An engine test that builds a real, throwaway engine with `NewEngine` on `t.TempDir()` and runs the task to
  completion. Do not mock tempo.
- A handler test for the ops API, built the same way.
