# Architecture — tiers, package roles, design decisions

`reference-app` (module `github.com/go-bumbu/reference-app`, binary `refapp`) is a working Go service that serves as
the living reference and boilerplate for the author's Go applications: CLI, layered config, contract-first REST API,
multi-tenant persistence, delegated auth and an embedded Vue UI, released as zips, debs and a container image.
Other projects consume it by copying its patterns; [../reference/](../reference/README.md) is the guide for that.
The docs in this folder are for working **on this repo**.

Start here, then read the topic doc for the area you change:

| Doc | Read before changing |
|---|---|
| [features.md](features.md) | anything user-facing — check whether a gap is deliberate first |
| [api.md](api.md) | routes, handlers, the OpenAPI spec, `libs/client` |
| [auth-and-tenancy.md](auth-and-tenancy.md) | the auth guard, `internal/tenants`, anything tenant-scoped |
| [database.md](database.md) | `go-bumbu/db/gormdb`, a module's store, migrations |
| [server-lifecycle.md](server-lifecycle.md) | `app/cmd`: CLI, config, startup, shutdown, logging |
| [web-ui.md](web-ui.md) | `webui/`, `app/spa` |
| [testing.md](testing.md) | before calling any change done |
| [releasing.md](releasing.md) | goreleaser, `zarf/`, workflows, tags |

## Layering

    main.go ─► app/cmd        CLI, config loading, composition root, server lifecycle
                 │
               app/router     routes, auth guard, problem+json, UI mount, observability handler
               app/router/handlers   HTTP ⇄ module translation, wire structs
               app/spa  app/logger  app/metainfo  app/reqctx
                 │
               internal/notes   internal/tenants   internal/jobs   business modules
                 │
               libs/middleware/{log,prometheus}  libs/client   foundation
               (external: go-bumbu/db/gormdb, go-bumbu/http, go-bumbu/config, go-bumbu/tempo, ...)

Dependencies only point down: `app/` → `internal/` → `libs/`. `.go-deps-view.config` lays the graph out along these
tiers for [go-deps-view](https://github.com/candy-tools/go-deps-view); nothing enforces them in lint yet (see debt).

- **`app/` is glue, never business logic.** It instantiates shared resources (logger, db), wires modules into
  handlers, and translates requests or triggers into module calls. Sharing inside `app/` is fine: one handlers
  package, one file per area. If a handler starts deciding *business* rules, the rule belongs in a module.
- **`internal/<module>` owns one concern: its logic, its tables, a small public API.** Modules never import `app/`
  and never import each other. When a module needs another's capability it declares a small interface and `app/`
  passes the implementation in. `internal/notes` never imports `internal/tenants`: it takes tenant and user IDs as
  opaque scope keys in `notes.Actor`. `internal/jobs` never imports `internal/notes`: it declares `NoteSource`
  (`NoteIDs`), which `*notes.Service` satisfies.
- **Interfaces live on the consumer side.** `handlers.NotesService` and `router.TenantProvisioner` are declared in
  `app/`, where they are used, so handlers test against fakes and modules export concrete `*Service` types.
- **`libs/` is agnostic foundation.** Describable without a business word; never imports `internal/` or `app/`;
  every app-specific value is injected through a `Cfg` or interface; minimal dependencies; no logger unless
  unavoidable (then a `*slog.Logger` from the caller). `libs/client` is the one lib that names domain concepts: it is
  the public SDK of the wire contract and still imports nothing from `app/` or `internal/`.
- **Shared runtime dependencies are created once, in `app/cmd/server.go` (`runServer`).** Modules and libs never
  open a db, build a logger or read config themselves.

Other top-level folders: `webui/` (Vue sources, see [web-ui.md](web-ui.md)); `zarf/` (everything that "holds" the
project: sample config, Dockerfile, deb packaging); `docs/` (these docs, [../reference/](../reference/README.md) and
the OpenAPI contract in `docs/openapi/`). This flat `app/internal/libs` layout fits a project that builds a binary;
it does not fit a pure library.

## Design decisions

All settled at bootstrap (2026-11-09, commit `650c99c`) unless dated otherwise.

- **Authentication is delegated** (`none` / `proxy` / reserved `oidc`): the app never stores passwords or sessions,
  it maps an asserted identity onto a tenant and a user — [auth-and-tenancy.md](auth-and-tenancy.md).
- **Tenant isolation is enforced in the module store**, not in handlers: no caller can forget the filter.
- **The API is contract-first**: the hand-written `docs/openapi/api-v0.yaml` is the source of truth and tests fail
  when routes or responses drift from it — [api.md](api.md).
- **Wire types are never shared**: handlers, `libs/client` and `webui/src/lib/api/types.ts` each map their own
  structs, so a module's model can change without changing the wire format.
- **Persistence models are private to their module** (`noteRow`, `tenantRow`, `userRow`); the domain types carry no
  GORM tags — [database.md](database.md).
- **Pure Go, `CGO_ENABLED=0`**: SQLite through `go-bumbu/db/gormdb/sqlite` (glebarez), static binaries cross-compiled from any
  host — [releasing.md](releasing.md).
- **Errors, not panics, at startup**: every constructor (`router.New`, `notes.New`, `gormdb.Open`) validates its
  dependencies and returns an error, so a bad wiring fails the `start` command with one message.
- **Tags are part of `internal/notes`** (2026-11-10): one tree per tenant in a closure table
  (`go-bumbu/closure-tree`), after codex's media tags without tag groups or facets. Listing a tag's notes is a join
  across the tag tree and the notes, so tags stay in the module that owns the notes instead of becoming a module of
  their own — [database.md](database.md).
- **Background jobs are a tenant-scoped module on `go-bumbu/tempo`** (2026-11-10): `internal/jobs` holds tempo's
  runner, scheduler, file logs and progress, and the job catalogue (one job, `scan-notes`, with a stub per note).
  Every run and schedule carries its tenant in tempo's params and is filtered by it on every read, as a store
  filters its rows; a tenant has at most one waiting or running run per job (manual runs and schedule fires
  alike), so no tenant fills the shared queue. The jobs API is the tenant API (`/jobs`, `/job-runs`), not an ops
  port, because the web UI drives it. This departs from
  [../reference/background-jobs.md](../reference/background-jobs.md) on purpose: the engine is not a bare holder
  of tempo types, since the tenant filter must sit between tempo and every caller.
- **Placeholders a copy must rename** are marked `// TODO replace` (`appName`, `EnvVarPrefix`, `problemBaseURI`).

## Known debt

- Layer rules are documented and visualized, not enforced: depguard (or go-arch-lint) is planned in `TODO.md`.
- `make verify` runs the test suite twice (test + coverage) and skips `spec-lint`; CI does not call `make verify`
  — details and the planned fixes in [testing.md](testing.md).
- Schema changes rely on GORM `AutoMigrate` only; there are no versioned migrations — [database.md](database.md).
- `/health` and `/ready` always answer `OK` — [server-lifecycle.md](server-lifecycle.md).
