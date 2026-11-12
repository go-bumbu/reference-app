# Features — what exists, what is partial, what is deliberately missing

Status of every capability, and where it lives. Check here before adding one: the gap may be deliberate, or
already catalogued with a direction in `TODO.md`. Layering and decisions: [architecture.md](architecture.md).

## Status

| Feature | Status | Where |
|---|---|---|
| CLI: `start`, `version` | Implemented | `app/cmd/root.go`, `app/cmd/server.go` — cobra |
| Layered config (defaults → `.env` → file → `REFAPP_*` env) | Implemented | `app/cmd/config.go` — [server-lifecycle.md](server-lifecycle.md) |
| Structured logging | Implemented | `app/logger` — console handler on a TTY, JSON otherwise |
| Main HTTP server + graceful shutdown | Implemented | `app/cmd/serve.go` |
| Observability server (`/metrics`, `/health`, `/ready`) | Implemented, opt-in | `app/router/observability.go`; `Observability.Enabled` |
| Real health / readiness checks | Partial | both probes answer a static `OK`; wire real state when startup work appears |
| Background jobs: queue, workers, cron schedules, per-run logs and progress | Implemented | `internal/jobs` on `go-bumbu/tempo` (tables `tempo_tasks`, `tempo_schedules`; logs in `Jobs.LogDir`); started last in `runServer`, stopped in the shutdown hook |
| Job `scan-notes`: walks every note of the tenant | Implemented, stub work | `internal/jobs/scannotes.go`; `scanNoteStub` waits 50 ms per note in place of real work; logs one line per scanned note, so the log endpoint (at most 1000 lines) shows only the last ones of a large tenant |
| Jobs per tenant: runs, logs, schedules visible to their tenant only; one waiting or running run per tenant and job | Implemented | the tenant id rides in tempo's params; `/jobs`, `/job-runs` — [api.md](api.md) |
| Jobs admin / ops view across tenants | Not implemented | no admin role; every job API is tenant-scoped |
| Run history per tenant | Partial | `HistorySize` is shared by every tenant: a busy tenant pushes out the others' old runs and their logs |
| Missed schedule fires replayed after a restart | Not implemented, by tempo's design | a fire due while the server was down is skipped |
| Jobs across several server processes | Not implemented | one process per database and log directory: tempo's queue and scheduler hold within one process |
| Request logging, panic recovery, Prometheus request metrics | Implemented | `go-bumbu/http/middleware` wrapping the whole router in `router.New`; metrics only with `Observability.Enabled`, labelled by gorilla route template (`reportRoute`), `unmatched` for 404/405 |
| REST API `/api/v0`: `info`, `me`, notes list / create / get / update / delete | Implemented | `app/router/api_v0.go` — [api.md](api.md) |
| Tags: one tree per tenant, any number per note, notes listed by tag (subtree included) | Implemented | `internal/notes/tags.go`, `tagstore.go` (closure table via `go-bumbu/closure-tree`); `/tags`, `/notes/{id}/tags/{tagId}`, `GET /notes?tag=` |
| Tag groups (several trees, codex's media) and tag facets / counts | Not implemented, by design | one tree of tags for notes; the sidebar is a plain tree, no per-tag counts |
| Notes list pagination | Not implemented | `List` returns the whole tenant in one page; the UI's search filters on the client |
| RFC 9457 problem+json errors, masked in production | Implemented | `go-bumbu/http/problemjson`; `Env.Production` |
| Request ids, unmasked problem log | Implemented | `withRequestID` (`app/router/requestid.go`): `req-id` in the request log = problem `reference` = `Request-Id` response header; `logProblem` logs every problem in full |
| Client ip behind a reverse proxy | Implemented | `Server.TrustedProxies` → `middleware.ClientIPFromXFF`; empty logs the direct peer |
| OpenAPI contract tests + spectral lint | Implemented | `app/router/openapi_test.go`, `make spec-lint` |
| Go API client | Implemented | `libs/client` |
| SQLite (default) / Postgres persistence | Implemented | `github.com/go-bumbu/db/gormdb` — [database.md](database.md) |
| Versioned schema migrations | Not implemented | each module `AutoMigrate`s its own tables at startup |
| Multi-tenancy, just-in-time tenant + user provisioning | Implemented | `internal/tenants`, auth guard — [auth-and-tenancy.md](auth-and-tenancy.md) |
| Auth `none` (fixed `admin` of `default`) | Implemented | demos and local development only |
| Auth `proxy` (trusted reverse-proxy headers) | Implemented | `app/router/auth.go` via `go-bumbu/userauth/headerauth` |
| Auth `oidc` | Not implemented | reserved: config accepts it, `router.New` refuses it, so `start` fails |
| Login forms, passwords, sessions | DIY by design | delegated to the reverse proxy / identity provider |
| Tenant / user administration API | Not implemented | tenants and users only appear through provisioning |
| Roles or per-user permissions inside a tenant | Not implemented | every user of a tenant reads and writes all its notes, by design of the prototype |
| Vue web UI, embedded in the binary | Implemented | `webui/`, `app/spa` — [web-ui.md](web-ui.md) |
| Web UI search (notes, client-side `?q=` filter) | Implemented | `webui/src/App.vue`, `views/NotesView.vue` |
| Web UI settings: theme, light / dark / auto mode | Implemented, per device | `webui/src/views/settings/`; kept in `localStorage`, no user settings API |
| Web UI settings: Jobs | Implemented | `webui/src/views/settings/JobsView.vue`, `components/jobs/`, after aether's Tasks page: run now with progress, schedules, runs with cancel and log |
| Web UI tags: sidebar tree, drag a note onto a tag, tag editor dialog | Implemented | `webui/src/components/AppNav.vue`, `components/tags/` — [web-ui.md](web-ui.md); nothing about tags lives in the settings |
| Moving tags without a pointer drag | Partial | "Move to…" in the editor nests a tag anywhere (keyboard, touch); reordering among siblings is drag only |
| More than 1000 tags per tenant | Not implemented, by design | `notes.MaxTagsPerTenant` keeps `GET /tags` one unpaginated response |
| A 422 listing every failing field | Not implemented | `notes.ValidationError` holds one field; the spec says the server stops at the first |
| Case-insensitive tag names, a unique constraint on sibling names | Not implemented | names are unique among siblings as written; the check runs in the transaction, not as a constraint (Postgres can race two renames) |
| Build metadata (`version`, `GET /info`) | Implemented | `app/metainfo`, set by goreleaser `-ldflags` |
| Release: zips, debs, container image, UI zip | Implemented | goreleaser — [releasing.md](releasing.md) |
| docker compose setup | Not implemented | `TODO.md` → "pending" |
| Layer rules enforced in lint | Not implemented | `.go-deps-view.config` only visualizes; depguard planned in `TODO.md` |

## Not implemented (catalogued in TODO.md)

`TODO.md` → "Verify and code quality" holds the chosen direction for: running the suite once in `verify`, a
formatting gate, pinning tool versions in one place, CI calling the same make targets, `go-test-coverage` instead of
the awk parser, `check-git-clean` catching untracked files, `govulncheck`, `-race -shuffle=on`, extra linters, and
workflow / secret / spelling checks. Implement from there; do not redesign them.
