# Developing reference-app

## Overview

The Go code lives in three tiers: `app/` (CLI, servers, handlers, wiring), `internal/` (business modules) and
`libs/` (agnostic foundation). The Vue web UI lives in `webui/`, packaging and sample config in `zarf/`, and the
OpenAPI contract and docs in `docs/`. [docs/agents/architecture.md](docs/agents/architecture.md) explains each
folder, the rules between them, and links to every topic doc. [docs/reference/](docs/reference/README.md) explains
how to build another app from this one.

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
make verify         # the full check: tests, license check, lint, benchmarks, coverage gate, spec lint, UI tests
```

Run `make verify` before opening a PR. It runs every check and fails at the end if any failed, so one run
reports all of them. CI runs the `go` and `webui` workflows on every PR and push to `main`. They run tests,
coverage, lint, the license check, the spec lint and the UI tests, but not the benchmarks or Postgres. See
[docs/agents/testing.md](docs/agents/testing.md).

## Run locally

```sh
make run            # start the service with zarf/config.yaml
make run-ui         # build the web UI, then start the service with it embedded
```

The sample config listens on <http://localhost:8081>, with the observability server (`/metrics`, `/health`,
`/ready`) on `localhost:9090`. It uses SQLite under `zarf/appData/` and `Auth.Method: none`, so every request is
user `admin` of tenant `default`.

For UI work with hot reload, run `make run` in one terminal and `make ui-dev` in another (run `make ui-prepare`
once first); the Vite dev server proxies `/api` to the backend. See [docs/agents/web-ui.md](docs/agents/web-ui.md) for
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
On the tag, `.github/workflows/release.yml` builds the zips, debs and container image with goreleaser. What each
artifact contains, and the traps, are in [docs/agents/releasing.md](docs/agents/releasing.md).
