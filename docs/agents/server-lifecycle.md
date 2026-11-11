# Server lifecycle — CLI, config, composition root, graceful shutdown

`app/cmd` owns the process: the cobra CLI, config loading, the composition root (`runServer`) and every
`http.Server`. `app/router` only builds `http.Handler`s and never owns a listener.
See [architecture.md](architecture.md) for the tiers, [database.md](database.md) for the db section of the config.

## CLI

- `refapp start [-c file]` runs the servers; `refapp seed [-c file]` adds demo notes and tags for the auth-none
  identity (`DefaultTenant`/`DefaultUser`) through the modules' APIs and is not idempotent: `make run` runs it
  once per database, tracked by the `.demo-data` marker next to the db, and `make clean` deletes the db dir;
  `refapp version` prints the build metadata (`app/metainfo`, set by
  goreleaser `-ldflags`; `dev-build` otherwise).
- The root command sets `SilenceUsage` and `SilenceErrors`: a runtime error (bad config, port in use) is not a usage
  error, so `Execute` prints it once and exits `1`.

## Config (`app/cmd/config.go`)

- Precedence, low to high: `defaultCfg` → `.env` in the working directory → config file → env vars prefixed
  `REFAPP_` (e.g. `REFAPP_ENV_LOGLEVEL=debug`, `REFAPP_DB_POSTGRES_PASSWORD`), via `go-bumbu/config`.
- **`-c <file>` is mandatory when given**: a missing file fails loudly, never falls back to defaults. Without `-c`,
  the first existing of `./config.yaml` (development) and `/etc/refapp/config.yaml` (packaged) is used; with none,
  defaults and env vars only.
- **Validation and normalization belong to the loader** (`getAppCfg` → `normalizeDBCfg`, `normalizeAuthCfg`):
  `runServer` only ever sees a valid config.
- Each section is mapped onto its consumer's config by a method (`dbCfg.gormCfg`, `authCfg.routerCfg`), so the file
  format stays the app's own.
- `go-bumbu/config` walks every field, unexported ones included: never cache parsed values on a config struct.
- Loader messages are buffered in `AppCfg.Msgs` and logged once the logger exists.
- Three sample configs must stay in step with `AppCfg`: `zarf/config.yaml` (`make run`), `zarf/packaging/config.yaml`
  (deb), `zarf/docker/config.yaml` (image).

## Logging (`app/logger`)

`log/slog`: a console handler on a TTY, JSON on stdout otherwise. Every log line carries
`slog.String("component", ...)`. Never log secrets or the Postgres DSN.

## Startup (`runServer`, `app/cmd/server.go`)

    resolve config file → getAppCfg → logger → startup + buffered config logs
      → openDB (deferred Close) → newModules (tenants, notes, jobs) → newMainHandler (auth, UI, router.New)
      → servers: main (+ observability if enabled) → jobs.Start → serveWithGracefulShutdown

- Every step returns an error; nothing panics. Constructors validate required dependencies (`router.New` refuses a
  nil `Notes` or `Tenants`).
- Modules are built here and migrate their own tables in `New`; `newModules` hands one module to another through
  its interface (`notes.Service` is the jobs' `NoteSource`).
- **Background workers start last** (`jobs.Start`: the runner's workers, then the scheduler), once nothing else can
  fail before serving. A failing `Start` shuts the jobs down again and ends the command.
- The `Jobs` section (`Parallelism` 2, `QueueSize` 100, `HistorySize` 100, `LogDir` `data/job-logs`, made absolute
  by `normalizeJobsCfg`) configures `internal/jobs`. One log directory and one database per server process: tempo's
  queue, schedules and log sweep are not shared between processes.
- Servers set `ReadHeaderTimeout: 5s` (gosec G112, Slowloris).

## Servers and shutdown (`app/cmd/serve.go`)

- `serveHTTP` binds the listener **before** serving, so a bind error is returned, not lost in a goroutine.
- All servers run in one `errgroup` under a `SIGINT`/`SIGTERM` context: a signal or a failing server cancels it and
  every server shuts down (`TestServeWithGracefulShutdownStopsAllOnFailure`).
- Each server gets 10 s (`serverShutdownTimeout`). `http.Server.Shutdown` only *waits* for in-flight handlers, it
  never cancels them; when the grace period ends, `Close` drops the remaining connections, which is what cancels a
  stuck handler's context (`TestServeHTTPForceClosesStuckHandler`).
- The shutdown hook gets 30 s (`appShutdownTimeout`) to stop background workers: `jobs.ShutDown` stops the scheduler
  (no new fires), then the runner, which cancels running runs and waits for them. The hook runs on every exit of
  the servers, a failed bind included. A new worker stops in the same hook.
- The db closes last, through the `defer`, after everything that uses it stopped, also after a failed startup step.
- A second Ctrl-C during shutdown kills the process: the signal context is cancelled first, restoring default
  signal handling.

## Observability server

Opt-in (`Observability.Enabled`, default port 9090), on its own port so it can stay off the public interface:
`/metrics` (Prometheus), `/health`, `/ready`. Both probes are a static `OK` today. When making them real: `ready`
reflects startup work (caches, migrations); do **not** tie `health` to the database, or one network blip restarts
every replica. Request metrics come from `go-bumbu/http/middleware/metrics`
(`requests_http_duration_seconds`), created once in `newMainHandler` and only when the observability server is
enabled. Its route label reads `r.Pattern`, which gorilla/mux never sets, so every request is labelled `unmatched`
until go-bumbu/http fixes that; the app adds no workaround.
