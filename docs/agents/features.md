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
| Background workers (schedulers, task runners) | Not implemented | the shutdown hook in `runServer` is the place to stop them |
| Request logging + Prometheus request metrics | Implemented | `libs/middleware/{log,prometheus}`, mounted in `router.New` |
| REST API `/api/v0`: `info`, `me`, notes list / create / get / update / delete | Implemented | `app/router/api_v0.go` — [api.md](api.md) |
| Notes list pagination | Not implemented | `List` returns the whole tenant in one page; the UI's search filters on the client |
| RFC 9457 problem+json errors, masked in production | Implemented | `go-bumbu/http/problemjson`; `Env.Production` |
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
| Web UI settings: Jobs | Placeholder | sample jobs whose Run now only toasts; needs the background workers below |
| Tags (sidebar) | Placeholder | `webui/src/components/AppNav.vue` shows a fixed list; no API or module support yet |
| Build metadata (`version`, `GET /info`) | Implemented | `app/metainfo`, set by goreleaser `-ldflags` |
| Release: zips, debs, container image, UI zip | Implemented | goreleaser — [releasing.md](releasing.md) |
| docker compose setup | Not implemented | `TODO.md` → "pending" |
| Layer rules enforced in lint | Not implemented | `.go-deps-view.config` only visualizes; depguard planned in `TODO.md` |

## Not implemented (catalogued in TODO.md)

`TODO.md` → "Verify and code quality" holds the chosen direction for: running the suite once in `verify`, a
formatting gate, pinning tool versions in one place, CI calling the same make targets, `go-test-coverage` instead of
the awk parser, `check-git-clean` catching untracked files, `govulncheck`, `-race -shuffle=on`, extra linters, and
workflow / secret / spelling checks. Implement from there; do not redesign them.
