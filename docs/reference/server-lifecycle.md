# Server lifecycle — composition root, servers, graceful shutdown

How a new app starts, serves and stops. Copy `app/cmd/serve.go` (+ `serve_test.go`) unchanged. It is pure
mechanism. Then write your own `runServer` in `app/cmd/server.go` following the shape below. Copy
`app/router/observability.go` for metrics and probes. Background:
[../agents/server-lifecycle.md](../agents/server-lifecycle.md).

## Split of responsibilities

- **`app/router` builds `http.Handler`s and never owns a listener.** `router.New(Cfg) (http.Handler, error)` takes
  every dependency in a `Cfg` struct, checks the required ones, and returns an error, not a panic.
- **`app/cmd` owns the `http.Server`s**, their ports, timeouts and shutdown, so the whole lifecycle sits in one place.

## The composition root (`runServer`)

Keep this order. Each step returns an error, and nothing panics:

    resolve config file → load + validate config → logger → log startup and buffered config messages
      → open shared resources (db; defer Close right away)
      → build modules (each migrates its own tables) → build handlers (router.New)
      → assemble servers → start background workers → serveWithGracefulShutdown(servers, shutdownHook)

- Create every shared resource here, once. Modules and libs receive them and never create their own.
- Defer `Close` for a resource right after opening it, joining its error into the return value. A failed later
  step still releases it.
- Set `ReadHeaderTimeout` on every `http.Server` (5 s here, gosec G112). Add `ReadTimeout`, `WriteTimeout` and
  `IdleTimeout` if your endpoints have known bounds. Leave `WriteTimeout` unset for streaming responses.

## Serving and shutdown (`app/cmd/serve.go`)

What the copied code guarantees. Keep these guarantees if you change it:

- **Bind first, then serve.** `net.Listen` runs synchronously, so "port in use" comes back as an error instead of
  being lost in a goroutine.
- **One `errgroup` for all servers, under a SIGINT/SIGTERM context.** A signal or a failing server stops all of
  them. A process with one dead server and one live one is worse than a clean exit.
- **Each server has a bounded grace period (10 s), then is closed.** `Shutdown` only waits for in-flight handlers.
  `Close` is what cancels a stuck handler's context and lets the process exit.
- **Then the shutdown hook runs, with its own budget (30 s).** Stop background workers there: schedulers, queue
  consumers, task runners. It is a no-op in reference-app because there are none yet. Stop a scheduler before you
  drain its runner, and join both errors, so a stuck scheduler never skips the drain
  ([background-jobs.md](background-jobs.md)).
- **The db closes last**, through the `defer`, after every server and worker that uses it has stopped.
- **A second Ctrl-C kills the process immediately**, because default signal handling is restored when shutdown
  starts.

## Adding a background worker

Start it in `runServer` after the resources it needs, with a context that the shutdown hook cancels. In the hook,
cancel it and wait for it within the hook's `ctx`. Have the worker log with its own `component`. Add a test like
`TestServeWithGracefulShutdownStopsAllOnFailure` for the worker's stop path. For queued, scheduled or user-visible
jobs, use a task runner instead of a hand-written goroutine: [background-jobs.md](background-jobs.md).

## Optional dependencies

Some features need something the host may not have: an external binary, a provider API key, a model download. The
server must still start without it. *(aether: `fpcalc` for audio identification, the fanart.tv key)*

- **Decide once, at startup.** Probe for the dependency in `runServer`. If it is missing, pass `nil` for that
  service in the router's `Cfg`, together with a reason the user can read.
- **Handlers treat `nil` as "feature off".** Answer `503` with that reason. Never panic, and never retry the probe
  on each request.
- **Tell the UI.** A small `GET …/capabilities` endpoint returns `{feature: false, feature_unavailable_reason}`.
  The UI then greys the control out and shows the reason, instead of hiding the control. *(aether:
  `GET /api/v0/metadata/capabilities`)*
- Log the missing dependency once at startup as a `WARN`. State in your docs that installing it needs a restart.
- Tests that need the dependency **skip** when it is absent. A fresh checkout and CI must stay green.

## Data directory: authoritative data and caches apart

When the app keeps files beside its database, give each kind its own tree under one `DataDir`, and write down which
trees are **authoritative** (the only copy: uploads, the database) and which are **cache** (derived, rebuildable).
Then an operator knows what is safe to delete and what must be backed up. *(aether: `metadata/` holds manual
uploads and is never cleared; `image-cache/` holds re-encoded copies and is safe to wipe)*

## Observability server

- A second `http.Server` on its own port (default 9090), opt-in (`Observability.Enabled`). Keep it off the public
  interface. Bind it to localhost or a private network.
- `/metrics` uses the Prometheus default registry. The `go-bumbu/http/middleware` request middleware records request
  durations through a `middleware/metrics` observer, labelled by **route pattern** (not the raw path), so label
  cardinality stays bounded. Create the observer once per process: it registers in the registry.
- With gorilla/mux (or chi), report the matched template yourself: a router-level middleware calls
  `middleware.SetRoute(r, template)`, or every request is labelled `unmatched` (see `reportRoute` in reference-app).
  A plain `http.ServeMux` under the middleware needs nothing.
- `/ready` should reflect startup work: caches warmed, migrations done.
- `/health` should reflect the process itself. **Do not tie it to the database**: one network blip would fail every
  replica's probe and restart all of them.
- Both are a static `OK` in reference-app. Replace them when you have real state to report.
- **An ops API goes on a second port, in its own package.** Operators may need routes that cross tenants: task
  management, an inspector. Build them as a separate router in their own package (codex: `app/adminrouter`), not
  on the tenant API. Then a tenant-scoped route cannot be mounted there by mistake. That router has no auth
  guard. The control is the network: it is disabled unless its address is set, and that address is localhost or a
  private network. Document this in the spec too, so the contract stays honest. See
  [background-jobs.md](background-jobs.md).

## Tests to copy

`TestServeHTTPForceClosesStuckHandler`, `TestServeHTTPListenError`, `TestServeWithGracefulShutdownStopsAllOnFailure`,
and a `TestRunServerWiresTheRouter`-style test. That test runs the real `runServer` up to `router.New`, and uses a
config the router is known to refuse (`oidc`). Getting exactly that error proves every dependency was passed in,
without binding a port.
