# Folder Structure

Note about flat vs nested structure: I have experienced that this structure is useful for projects that will
generate an executable binary; for libraries that are only referenced by other projects this structure is not practical.

Code lives in three tiers and dependencies only point down: `app/` → `internal/` → `libs/`.

### app

The glue. The app folder contains the highest level of application files: here you instantiate shared resources like
loggers, db connections etc., pass references between them, and hold the handlers, routes, CLI commands and jobs.
`app/` holds no business logic; it translates a request or trigger and delegates.

If your project builds multiple artifacts, e.g. a backend and a cli, you can create folders for every artifact.

### internal

The business modules. Each module owns one business concern: its logic, its data and a small public API,
e.g. a package that manages bookmarks and writes them to a database.

Packages in this folder:
* should not instantiate shared runtime dependencies, e.g. a logger or a db connection; `app/` passes them in
* never import `app/`, and never reach into another module: when a module needs another module's capability,
  it declares a small interface and `app/` passes the implementation in
* own their data: only a module's own store reads or writes its tables
* cannot be referenced outside the project (are not public)

`internal/notes` is a prototype module: a small API (`Create`, `Get`, `List`), typed errors
(`ErrNotFound`, `*ValidationError`) that the handlers map to status codes, and injected dependencies (a db handle,
a clock). Its GORM store lives in `store.go`, private to the module. It never imports `internal/tenants`:
it only receives the tenant and user IDs as opaque scope keys.

`internal/tenants` owns the isolation root: tenants, their users and the just-in-time provisioning of both.

### docs

Documentation: the guides next to this file, and the documentation that is part of the contract,
e.g. the OpenAPI spec in `docs/openapi/`.

### libs

The foundation. Capabilities you can describe without a business word: HTTP middleware, file storage,
a provider client, a file-format codec. In this repo, `libs/middleware/{log,prometheus}` and `libs/gormdb` are examples.
`libs/client`, the client of this app's own API, is the exception that names domain concepts: it is the public
SDK of the wire contract, and still imports nothing from `app/` or `internal/`.

Packages in this folder:
* never import `internal/` or `app/`; every app-specific value is injected via a constructor or interface
* ideally do not need a logger, but expose proper error handling; if logging is unavoidable,
  accept a `*slog.Logger` (or a small interface) from the caller
* keep their dependency surface minimal, and avoid depending on each other
* are promoted to a separately released module only once a second project needs them

### webui

The web UI's sources and tooling (see [Web UI](web-ui.md)); its build output is embedded by `app/spa`.

### zarf

_...is a holder, usually of ornamental metal, for a coffee cup without a handle ([wikipedia](https://en.wikipedia.org/wiki/Zarf))_

Inspired by: https://github.com/ardanlabs/service

This folder contains all the extras needed to "hold" the project, e.g. docker files, build scripts, config samples etc.
Here: `config.yaml` (the sample used by `make run`), `docker/` (the release container image) and `packaging/`
(the deb's systemd unit, config and maintainer scripts).
