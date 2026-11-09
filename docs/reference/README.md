# Reference — building a new app from reference-app

Guides for an agent or developer who builds **another** Go service from
[go-bumbu/reference-app](https://github.com/go-bumbu/reference-app), either by starting from a copy or by adopting
one part in an existing app. Paths are relative to the reference-app repo. Each guide says what to copy, what to
rename, and which rules the copy must keep. The reasons behind those rules are in [../agents/](../agents/architecture.md).

The guides also record patterns from two apps built the same way:
[aether](https://github.com/andresbott/aether) (a music server, Go module under `server/`) and codex (a multi-tenant
workspace). A rule tagged *(aether)* or *(codex)* is **not implemented in reference-app**. The paths given with it
point into that app. Copy the pattern when your app has the need, not before.

## Guides

| Guide | Covers | Builds on |
|---|---|---|
| [project-layout.md](project-layout.md) | `app/` → `internal/` → `libs/` tiers, what goes where, `zarf/` | — |
| [config-and-cli.md](config-and-cli.md) | cobra CLI, layered config, logging, build metadata | project-layout |
| [server-lifecycle.md](server-lifecycle.md) | composition root, servers, graceful shutdown, observability, optional dependencies | config-and-cli |
| [background-jobs.md](background-jobs.md) | durable task queue and schedules (go-bumbu/tempo), tenancy of jobs, ops port | server-lifecycle, auth-and-tenancy |
| [modules-and-database.md](modules-and-database.md) | writing a business module, GORM stores, SQLite/Postgres, db tests | project-layout |
| [api.md](api.md) | contract-first endpoints, handlers, problem+json, contract tests, Go client | modules-and-database |
| [auth-and-tenancy.md](auth-and-tenancy.md) | delegated auth, tenants and users, tenant isolation | api, modules-and-database |
| [web-ui.md](web-ui.md) | Vue SPA, its API layer, embedding it in the binary | api |
| [build-and-release.md](build-and-release.md) | verify gates, CI, goreleaser, deb, container image | everything above |

## Starting a new app from a copy

1. Copy the repo without its history and run `make verify` once, before you change anything. That proves the
   toolchain works.
2. Rename (checklist below), then run `make verify` again.
3. Build your first module next to `internal/notes`, following
   [modules-and-database.md](modules-and-database.md). Then design its endpoints in the spec ([api.md](api.md)).
4. Delete the notes prototype only after your module has replaced it **everywhere**: spec, handlers, routes,
   contract tests, `libs/client`, UI. The contract and client tests exercise notes until then.
5. Choose the auth method and tenancy model ([auth-and-tenancy.md](auth-and-tenancy.md)) before you store real
   data. Adding tenancy to existing data later is expensive.
6. Set up releases last ([build-and-release.md](build-and-release.md)).

## Rename checklist

| What | Where |
|---|---|
| Go module path | `go.mod` and every import (`go mod edit -module`, then replace the old path in `*.go`) |
| `appName` (binary name; `/etc/<app>/config.yaml`; default SQLite path) | `app/cmd/root.go` — marked `// TODO replace` |
| `EnvVarPrefix` (`REFAPP_*`) | `app/cmd/config.go` — marked `// TODO replace`; also `Makefile` `run` target and the comments in `zarf/*.yaml` |
| `problemBaseURI` (problem+json `type` namespace) | `app/router/api_v0.go` — marked `// TODO replace` |
| `-ldflags` import path of `app/metainfo`, binary and archive names, deb package, image name, homepage | `.goreleaser.yaml`, `.goreleaser-darwin.yaml` |
| `refapp` user, paths, unit name | `zarf/packaging/*` (rename `refapp.service` too), `zarf/docker/*` |
| Sample db name and user | `zarf/config.yaml` |
| UI title and package name | `webui/index.html`, `webui/src/App.vue`, `webui/package.json` |
| API title, `servers` | `docs/openapi/api-v0.yaml` |
| License (GPL-3.0 here) | `LICENSE`, the goreleaser license annotation, the deb doc path |

Done when `git grep -i -e refapp -e reference-app` only finds lines you meant to keep.

## Adopting one part in an existing app

Read the guide for that part. Each guide starts with what to copy and what it depends on. The pieces are
independent except for these links: the auth guard needs a tenants-style provisioner, the contract tests need the
router, and the UI embed needs the router's catch-all mount.

## Rules every app built from this keeps

- Dependencies point down: `app/` → `internal/` → `libs/`. Modules never import each other.
- A module owns its tables. No other code queries them.
- Tenant-scoped data is filtered in the module's store, and a missing tenant fails closed.
- The OpenAPI spec is the contract, and tests fail when the code drifts from it.
- The app never handles passwords or sessions. A proxy or identity provider does that.
- A variable-length list never goes in a URL. It goes in a request body.
- Startup returns errors, it never panics. Every resource is closed on the way out.
- Pure Go (`CGO_ENABLED=0`), so one build gives static binaries for every platform.

## When this template does not fit

- **A library, not a binary**: do not use the `app/internal/libs` layout; publish flat packages.
- **No UI**: remove `webui/`, `app/spa`, the `UI` field in `router.Cfg`, `package-ui` from `build`, the `webui`
  workflow and the spa archive in `.goreleaser.yaml`.
- **Single tenant**: keep the tenant scoping anyway. Run with `Auth.Method: none`, or `proxy` without a
  `TenantHeader`, and every user lands in tenant `default`. Removing the scoping saves little, and adding it back
  later is expensive.
