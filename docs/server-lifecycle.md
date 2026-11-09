# Server lifecycle

`app/router` only builds `http.Handler`s; `app/cmd` owns the `http.Server`s and their lifecycle:

* `runServer` is the composition root: load config → logger → shared resources → handlers → servers.
  Every step returns an error, nothing panics.
* `serveHTTP` binds the listener *before* serving, so a bind error (port in use) is returned instead of being lost in a goroutine.
* All servers run in one `errgroup`: SIGINT/SIGTERM or a failing server cancels the shared context, every server shuts
  down, then a shutdown hook stops the background workers (task runners, schedulers) within a bounded window;
  the db is closed last, after everything that uses it stopped.
* `http.Server.Shutdown` only *waits* for in-flight handlers, it never cancels them; after the grace period
  `Close` drops the remaining connections so a stuck handler cannot block the exit.
* A second Ctrl-C during shutdown kills the process immediately.
