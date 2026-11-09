# Developing reference-app

## Overview

The Go code lives in three tiers: `app/` (CLI, servers, handlers, wiring), `internal/` (business modules) and
`libs/` (agnostic foundation). The Vue web UI lives in `webui/`, packaging and sample config in `zarf/`, and the
OpenAPI contract and guides in `docs/`. [docs/folder-structure.md](docs/folder-structure.md) explains each folder
and the rules between them.

## Prerequisites

- Go 1.26 (from `go.mod`)
- Node 22 and npm, for the web UI and the spec lint
- [golangci-lint](https://github.com/golangci/golangci-lint) v2 (CI pins v2.13.2)
- [go-licence-detector](https://github.com/elastic/go-licence-detector) (CI uses v0.10.0)
- [goreleaser](https://goreleaser.com), for `make build`
- docker, only for `make test-alldbs`

## Common tasks

```sh
make help           # list all targets
make test           # fast Go tests
make test-alldbs    # Go tests on every database, Postgres included (needs docker)
make lint           # golangci-lint
make spec-lint      # lint the OpenAPI spec with spectral
make ui-test        # web UI type check and unit tests
make verify         # the full check: tests, license check, lint, benchmarks, coverage gate, UI tests
```

Run `make verify` before opening a PR. CI runs the same checks in the `go` and `webui` workflows on every PR and
push to `main`.

## Run locally

```sh
make run            # start the service with zarf/config.yaml
make run-ui         # build the web UI, then start the service with it embedded
```

The sample config listens on <http://localhost:8081>, with the observability server (`/metrics`, `/health`,
`/ready`) on `localhost:9090`. It uses SQLite under `zarf/appData/` and `Auth.Method: none`, so every request is
user `admin` of tenant `default`.

For UI work with hot reload, run `make run` in one terminal and `make ui-dev` in another (run `make ui-prepare`
once first); the Vite dev server proxies `/api` to the backend. See [docs/web-ui.md](docs/web-ui.md) for
playing the auth proxy from the dev server.

## Build

```sh
make build          # build the web UI, then the binary for the current OS/arch into dist/
make clean          # remove dist/
```

Try the whole release locally without publishing: `make package-ui && goreleaser release --snapshot --clean`.

## Release

Releases are built in CI by goreleaser, triggered by pushing a `v*.*.*` tag:

```sh
make tag version="v1.2.3"   # on main with a clean tree; (re)creates and pushes the tag
```

`make tag` does **not** run `make verify`; the `go` and `webui` workflows gate every push to `main` and every PR.
On the tag, `.github/workflows/release.yml` runs `make package-ui` and then goreleaser:

| Job              | Config                     | Publishes                                                                 |
|------------------|----------------------------|---------------------------------------------------------------------------|
| `linux + windows`| `.goreleaser.yaml`         | the GitHub release; zips for linux (amd64 v1/v3, arm64) and windows (amd64 v1/v3); debs; `refapp-spa.zip`; the container image |
| `macos`          | `.goreleaser-darwin.yaml`  | darwin zips (amd64, arm64), appended to the same release                  |

- **Pure Go, `CGO_ENABLED=0` everywhere**: static binaries with no glibc floor, cross-compiled from any host.
- **GOAMD64 v1 and v3 only**: v1 runs on any x86-64 CPU; v4 would not start on CPUs without AVX-512.
- **Debian package** (`zarf/packaging/`): `/usr/bin/refapp`, `/etc/refapp/config.yaml` (a conffile, edits survive
  upgrades), a hardened systemd unit running as the `refapp` system user, the database under `/var/lib/refapp`.
  A purge keeps `/var/lib/refapp`.
- **Container image** `ghcr.io/go-bumbu/refapp` (`zarf/docker/`), linux/amd64 + arm64: Alpine plus the prebuilt
  binary, uid 1000, the database in the `/var/lib/refapp` volume, listening on `0.0.0.0:8080`. `MAJOR.MINOR` and
  `latest` only move on non-prerelease tags. The first push creates a **private** package; making it public is a
  one-time step in the package settings.
- The embedded UI is whatever `app/spa/files/ui` holds at build time, so every release path keeps the
  `package-ui → goreleaser` order.
- The license text ships in every release archive, in the deb (`/usr/share/doc/refapp/LICENSE`) and as the
  container image's `org.opencontainers.image.licenses` annotation.
