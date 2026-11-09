# reference-app

A living reference of best practices, code structure and guides for my Go projects.
It doubles as an opinionated boilerplate for Go applications (originally "bgb" — Bott's Go Boilerplate).

## Quick start

```bash
make help        # list all targets
make run         # start the service with zarf/config.yaml
make run-ui      # build the web UI, then start the service with it embedded (needs node)
make verify      # tests, license check, lint, benchmarks and coverage gate
```

## What's included

* **CLI** with [cobra](https://github.com/spf13/cobra): `start` and `version` commands (`app/cmd`).
* **Layered config** with [go-bumbu/config](https://github.com/go-bumbu/config): defaults → `.env` file → config file → env vars
  (prefix `REFAPP_`, e.g. `REFAPP_ENV_LOGLEVEL=debug`).
  * `-c <file>` is mandatory: a missing file fails loudly instead of silently falling back to defaults.
  * without `-c`, the first existing of `./config.yaml` (development) and `/etc/refapp/config.yaml` (packaged) is used;
    with none, only defaults and env vars apply.
  * validation and normalization of the values belong to the loader (`getAppCfg`), so the server only sees a valid config.
* **Structured logging** with `log/slog`: a human-friendly console handler on a TTY, JSON otherwise (`app/logger`).
* **Two HTTP servers** (`app/cmd/serve.go`):
  * main API server (default `:8080`) with request logging and Prometheus middleware,
  * observability server (default `:9090`, opt-in via `Observability.Enabled`) exposing `/metrics`, `/health` and `/ready`;
    when enabled, keep it off the public interface.
* **REST API** under `/api/v0`, contract-first with an OpenAPI 3.1 spec (see [API](#api)).
* **Persistence** with [GORM](https://gorm.io): SQLite by default (pure Go, no CGO), Postgres by config
  (see [Database](#database)).
* **Multi-tenancy**: `internal/tenants` owns tenants and their users; every note belongs to a tenant
  (see [Authentication and tenancy](#authentication-and-tenancy)).
* **Delegated authentication**: `none` for demos, `proxy` for identity headers of a trusted reverse proxy,
  `oidc` reserved.
* **Prototype business module** `internal/notes`, consumed by the API handlers.
* **Web UI** in `webui/`, embedded into the binary (see [Web UI](#web-ui)).
* **Go API client** in `libs/client`.
* **Build metadata** injected via `-ldflags` at release time (`app/metainfo`), served by `GET /api/v0/info`.
* **Tooling**: golangci-lint v2 config, goreleaser config, license allow-list, per-package coverage gate,
  spectral lint of the OpenAPI spec (`make spec-lint`).

## API

The API is designed first: `docs/openapi/api-v0.yaml` is the hand-written contract, and the routes in
`app/router/api_v0.go` implement it.

| Endpoint                  | Description                                          |
|---------------------------|------------------------------------------------------|
| `GET /api/v0/info`        | version, build time, commit and Go version (public)  |
| `GET /api/v0/me`          | the authenticated caller: user and tenant            |
| `GET /api/v0/notes`       | list the tenant's notes, newest first                |
| `POST /api/v0/notes`      | create a note                                        |
| `GET /api/v0/notes/{id}`  | get a note                                           |

* Spec paths are mount-relative; the `/api/v0` base lives only in the spec's `servers` entry.
* Errors are RFC 9457 problem details (`application/problem+json`, via
  [go-bumbu/http/problemjson](https://github.com/go-bumbu/http)); `Env.Production: true` masks their internal detail.
* Handlers map the module's types to their own request/response structs, so the wire format does not change when
  the module's model does.
* The spec is enforced by tests in `app/router/openapi_test.go`: every mounted route must be documented and vice versa,
  and real responses are validated against the documented schema, status and content type
  (with [kin-openapi](https://github.com/getkin/kin-openapi)).
* `make spec-lint` lints the spec with [spectral](https://github.com/stoplightio/spectral) and `.spectral.yaml`.
* `libs/client` is a hand-written Go client of the API (stdlib only). It shares no types with the server;
  `app/router/client_test.go` runs it against the real router, so a drift on either side fails there.
  API errors come back as `*client.Problem`, comparable with `errors.Is` to `client.ErrNotFound`,
  `ErrUnauthorized` and `ErrValidation`.

## Authentication and tenancy

Authentication is delegated: the app never handles passwords or sessions. `Auth.Method` picks who vouches for
the caller, and the app maps that identity onto a tenant and a user:

| `Auth.Method` | Who is the caller                                                                                 |
|---------------|---------------------------------------------------------------------------------------------------|
| `none`        | default; every request is user `admin` of tenant `default`. Demos and local development only.     |
| `proxy`       | a reverse proxy (Authelia, oauth2-proxy, ...) asserts the user in `Auth.ProxyHeader.UserHeader` and, optionally, the tenant in `TenantHeader` |
| `oidc`        | reserved; the server refuses to start with it until it is implemented                             |

* `proxy` trusts the headers only from `Auth.ProxyHeader.TrustedProxies` (via
  [go-bumbu/userauth](https://github.com/go-bumbu/userauth)'s `headerauth`). The deployment must still make the
  app reachable only through the proxy, and the proxy must strip the headers from client requests.
  Without a `TenantHeader` every user lands in tenant `default`; with one, a request missing it is a 401.
* **Tenant → User**: a user belongs to exactly one tenant; the same login in two tenants is two users.
* **Just-in-time provisioning**: `internal/tenants` creates an unknown tenant or user on its first authenticated
  request, so onboarding stays with whoever authenticates. Nothing is ever created from an unauthenticated request.
* **Isolation**: the auth guard (`app/router/auth.go`) puts the resolved IDs on the request context
  (`app/reqctx`); handlers pass them to the module as a `notes.Actor`, and the module's store adds the tenant filter
  to every query itself. A note of another tenant is a 404, notes are shared by all users of their tenant, and a
  call without a tenant fails closed (`notes.ErrNoTenant`).
* Routes are public unless wrapped in `guard.protect` in `app/router/api_v0.go`.

## Web UI

`webui/` is a small Vue 3 single-page app on the stack of [aether](https://github.com/andresbott/aether):
TypeScript, Vite, PrimeVue (Aura preset), TanStack Query for server state and axios, tested with Vitest.
It lists and creates the notes of the caller's tenant and shows who is signed in.

* `make package-ui` builds it and copies it into `app/spa/files/ui`, which `app/spa` embeds with `go:embed`;
  `make build` and goreleaser run it first. A binary built without it serves a "not built" page instead.
* The router mounts the UI as the catch-all for GET/HEAD outside `/api`, so deep links and reloads reach
  `index.html` while an unknown API path stays a problem+json 404.
* `make ui-dev` runs the Vite dev server with hot reload, proxying `/api` to `make run` (`localhost:8081`).
  With the backend in `proxy` mode, the dev server can play the proxy:
  `DEV_USER=alice DEV_TENANT=acme npm run dev` (in `webui/`).
* `make ui-test` type-checks and runs the unit tests; it is part of `make verify`.
* Wire types live in `webui/src/lib/api/types.ts`, hand-written like `libs/client`; errors are read in one
  place, `webui/src/lib/apiError.ts`.

## Database

`libs/gormdb` opens a `*gorm.DB` for the driver named in the config's `DB.Type`; nothing above it knows which one it is.

| `DB.Type`  | Settings                                                    | Notes                                                       |
|------------|-------------------------------------------------------------|-------------------------------------------------------------|
| `sqlite`   | `DB.Sqlite.Path`                                            | default; the directory is created, WAL, busy timeout, FKs on |
| `postgres` | `DB.Postgres.{Host,Port,User,Password,DbName,SslMode}`      | `SslMode` defaults to `require`; pass the password by env var |

* The config loader resolves the SQLite path to an absolute one and validates the section, so a typo fails at load.
* Each module owns its tables: it takes the `*gorm.DB` in `New`, migrates its own schema and keeps its GORM row types
  private (`internal/notes/store.go`). GORM's errors are translated at the store (e.g. to `notes.ErrNotFound`) and
  never reach the handlers.
* Module tests run on every database through [go-bumbu/testdbs](https://github.com/go-bumbu/testdbs):
  SQLite always, Postgres in a container (needs docker) with `make test-alldbs`.

## Server lifecycle

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

## Releasing

Releases are built in CI by [goreleaser](https://goreleaser.com), triggered by pushing a `v*.*.*` tag:

```bash
make tag version="v1.2.3"   # on main with a clean tree; (re)creates and pushes the tag
```

`make tag` does **not** run `make verify`; the `go` and `webui` workflows gate every push to `main` and every PR.
On the tag, `.github/workflows/release.yml` runs `make package-ui` and then goreleaser:

| Job              | Config                     | Publishes                                                                 |
|------------------|----------------------------|---------------------------------------------------------------------------|
| `linux + windows`| `.goreleaser.yaml`         | the GitHub release; zips for linux (amd64 v1/v3, arm64) and windows (amd64 v1/v3); debs; `refapp-spa.zip`; the container image |
| `macos`          | `.goreleaser-darwin.yaml`  | darwin zips (amd64, arm64), appended to the same release                  |

* **Pure Go, `CGO_ENABLED=0` everywhere**: static binaries with no glibc floor, cross-compiled from any host.
* **GOAMD64 v1 and v3 only**: v1 runs on any x86-64 CPU; v4 would not start on CPUs without AVX-512.
* **Debian package** (`zarf/packaging/`): `/usr/bin/refapp`, `/etc/refapp/config.yaml` (a conffile, edits survive
  upgrades), a hardened systemd unit running as the `refapp` system user, the database under `/var/lib/refapp`.
  A purge keeps `/var/lib/refapp`.
* **Container image** `ghcr.io/go-bumbu/refapp` (`zarf/docker/`), linux/amd64 + arm64: Alpine plus the prebuilt
  binary, uid 1000, the database in the `/var/lib/refapp` volume, listening on `0.0.0.0:8080`. `MAJOR.MINOR` and
  `latest` only move on non-prerelease tags. The first push creates a **private** package; making it public is a
  one-time step in the package settings.
* The embedded UI is whatever `app/spa/files/ui` holds at build time, so every release path keeps the
  `package-ui → goreleaser` order.
* Try it locally without publishing: `make package-ui && goreleaser release --snapshot --clean`.

## License

[GPL-3.0](LICENSE). The text ships in every release archive, in the deb (`/usr/share/doc/refapp/LICENSE`) and as the
container image's `org.opencontainers.image.licenses` annotation.

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

Documentation that is part of the contract, e.g. the OpenAPI spec in `docs/openapi/`.

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

The web UI's sources and tooling (see [Web UI](#web-ui)); its build output is embedded by `app/spa`.

### zarf

_...is a holder, usually of ornamental metal, for a coffee cup without a handle ([wikipedia](https://en.wikipedia.org/wiki/Zarf))_

Inspired by: https://github.com/ardanlabs/service

This folder contains all the extras needed to "hold" the project, e.g. docker files, build scripts, config samples etc.
Here: `config.yaml` (the sample used by `make run`), `docker/` (the release container image) and `packaging/`
(the deb's systemd unit, config and maintainer scripts).
