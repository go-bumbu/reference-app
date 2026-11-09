# Testing — gates, test layers, conventions

How to verify a change before calling it done, and what each test layer protects.
See [architecture.md](architecture.md) for the tiers the layers follow.

## Gates

| Target | What it runs |
|---|---|
| `make test` | `go test ./... -cover` — the fast suite, SQLite only |
| `make test-alldbs` | the same with `TESTDBS_ALL=1`: module tests on Postgres too (needs docker) |
| `make lint` | `golangci-lint run` (v2; CI pins v2.13.2) |
| `make spec-lint` | spectral on `docs/openapi/api-v0.yaml` with `.spectral.yaml`, failing on warnings |
| `make ui-test` | `vue-tsc --noEmit` + `vitest run` in `webui/` |
| `make verify` | `test` + `license-check` + `lint` + `benchmark` + `coverage` + `spec-lint` + `ui-test`; runs every check, then fails if any failed |

**Before calling work done run `make verify`, plus `make test-alldbs` when a store or migration changed.**
`verify` does not stop at the first failure: it runs every check and reports all failures in one run.

- **Coverage gate**: every package under `internal/` and `libs/` must reach **70%** on its own
  (`COVERAGE_THRESHOLD`); `app/` is not gated. If a package falls below, write the missing tests — never lower the
  threshold or exclude the package.
- **License check**: `go-licence-detector` against `zarf/allowedLicenses.json`, indirect dependencies included. A new
  dependency with a license outside the list fails; ask before widening the list.
- **Lint**: `standard` linters plus `nolintlint`, `gocyclo` (15), `gocognit` (15), `nestif` (6) and `gosec`;
  complexity linters are skipped in `_test.go`. A `//nolint` must name the linter and explain itself — fix the code
  instead when you can.

## CI vs local (known debt)

CI (`.github/workflows/go.yml`, `webui.yml`) runs on every PR and push to `main`: `make test` + `make coverage`, the
golangci-lint action, `make license-check`, `make spec-lint`, and `make ui-test` + `make package-ui`. It does not run
`make verify` or the benchmarks, and it never runs Postgres. `verify` runs the suite twice (`test`, then
`coverage` per package). The planned fixes are in `TODO.md` → "Fix the verify process"; follow them rather than
inventing another scheme.

## Test layers

| Layer | Where | Protects |
|---|---|---|
| Modules | `internal/*/…_test.go` | business rules and persistence on every db: validation, max lengths fit the column, data survives a new `Service` on the same db, cancelled contexts, tenant isolation, fail-closed without a tenant, concurrent provisioning |
| Router | `app/router/*_test.go` | real modules on one SQLite file, no mocks: auth methods, isolation over HTTP, `404` vs `405`, UI catch-all |
| Contract | `app/router/openapi_test.go` | spec ⇄ routes coverage and response schemas ([api.md](api.md)) |
| Client | `app/router/client_test.go`, `libs/client` | `libs/client` against the real router; client error decoding |
| Process | `app/cmd/*_test.go` | config precedence and validation, bind errors, forced close of stuck handlers, one failing server stopping all, `runServer` wiring |
| Libs | `libs/**/…_test.go` | each lib alone, no app fixtures |
| UI | `webui/src/__tests__/` | views and `apiError.ts` (Vitest + jsdom) |

## Conventions

- **Module tests use the real database, not mocks**: `TestMain` registers the testdbs targets, `forEachDB` runs the
  body once per db with a fresh database ([database.md](database.md)).
- **Fixed clocks**: inject `Cfg.Now` (`fixedClock` advances one second per call) instead of sleeping or comparing
  with `time.Now`.
- **Handler-level tests go through the router** built with `router.New`, so middleware, the guard and problem+json
  mapping are exercised as in production.
- **Test files resolve repo files relative to themselves** (`runtime.Caller`), never relative to `go test`'s working
  directory (see `specDoc` in `openapi_test.go`).
- Table-driven tests are welcome; their branching is why the complexity linters skip test files.
- A new tenant-scoped module copies `TestTenantIsolation` and `TestNoTenantFailsClosed` from `internal/notes`.
