# Build and release — verify gates, CI, goreleaser, deb, container image

How a new app checks its quality and ships. Copy the `Makefile`, `.golangci.yaml`, `.spectral.yaml`,
`zarf/allowedLicenses.json`, `.github/workflows/{go,webui,release}.yml`, both `.goreleaser*.yaml` files and
`zarf/{packaging,docker}/`. Then apply the renames in [README.md](README.md). Background:
[../agents/testing.md](../agents/testing.md) and [../agents/releasing.md](../agents/releasing.md).

## Local gates

| Target | Runs |
|---|---|
| `make test` / `make test-alldbs` | Go tests; the second adds Postgres through testdbs (needs docker) |
| `make lint` | golangci-lint v2 (pin the same version locally and in CI) |
| `make spec-lint` | spectral on the OpenAPI spec |
| `make ui-test` | `vue-tsc` + Vitest |
| `make verify` | test, license-check, lint, benchmark, coverage, spec-lint, ui-test |

- **Coverage gate: at least 70% per package** across `internal/` and `libs/` (`COVERAGE_THRESHOLD`). `app/` is
  covered by router and process tests but not gated. Raise the threshold over time, and never lower it to get a
  green build.
- **Lint**: the `standard` linters plus `gosec`, complexity limits (`gocyclo` 15, `gocognit` 15, `nestif` 6, all
  skipped in tests), and `nolintlint` set so every `//nolint` names its linter and gives a reason.
- **License check**: `go-licence-detector` checks every module, indirect ones included, against
  `zarf/allowedLicenses.json`. Review the list for your project's license.
- `verify` runs every target, collects the failures and fails at the end, so one run reports all of them.
- Known gaps you inherit: `verify` runs tests twice, and `check-git-clean` ignores untracked files. The
  reference-app `TODO.md` → "Fix the verify process" lists the fixes. Apply them in your copy if you can.

## CI (`.github/workflows/`)

- `go.yml` (PRs and pushes to `main`): test + coverage, golangci-lint (version pinned), license check, spec lint,
  each as its own job, so they fail independently.
- `webui.yml`: `npm ci`, `make ui-test`, `make package-ui`.
- `release.yml`: runs on `v*.*.*` tags.
- Better than the reference: have CI call the same make targets as local `verify`, so they cannot drift.
- When the repo grows a second component (a desktop app, a website), give each its own workflow. A check then
  reads `<component> / <job>` on a PR. *(aether: `server.yml`, `webui.yml`, `player.yml`)*
- **A job that must run after the release goes inside `release.yml`** with `needs:`. goreleaser publishes with
  `GITHUB_TOKEN`, and events caused by that token start no workflows, so `on: release` never fires. *(aether: the
  GitHub Pages `site` job)*

## Release (`make tag version="vX.Y.Z"` → `release.yml` → goreleaser)

- `make tag` requires `main`, no unstaged changes and a `vX.Y.Z[-pre]` version. It relies on `main` having passed CI
  and does not run `verify` itself.
- Both release jobs run `make package-ui` **before** goreleaser. The embedded UI is whatever is in
  `app/spa/files/ui` at build time.
- Linux and windows build on Ubuntu (`.goreleaser.yaml`), macOS on a macOS runner (`.goreleaser-darwin.yaml`),
  which appends to the same GitHub release.

Keep these build decisions:

- **`CGO_ENABLED=0` everywhere**: static binaries, no glibc floor, cross-compiled from one host. Check that
  `file dist/linux*/<app>` says "statically linked" before you add a dependency that might need CGO.
- **`CGO_ENABLED=0` alone does not guarantee a static binary.** A library that loads a C library at runtime through
  `purego` (aether: `gen2brain/webp`) still links the binary against glibc, and it then fails on Alpine with
  `no such file or directory`. Such libraries offer a build tag to turn this off (`nodynamic`); add it to every
  Linux build. The `file` check above catches this.
- The counter-example: codex uses the CGO SQLite driver, so its release is linux/amd64 only and inherits the build
  runner's glibc.
- **`GOAMD64` v1 and v3 only**: v4 does not start on CPUs without AVX-512.
- **`-s -w` plus `-X <module>/app/metainfo.{Version,BuildTime,ShaVer}`** in every build. Update the path after
  renaming the module.

## Debian package (`zarf/packaging/`, nfpm through goreleaser)

The binary goes to `/usr/bin/<app>`, and the config to `/etc/<app>/config.yaml` as a **conffile**, so admin edits
survive upgrades. A hardened systemd unit (`ProtectSystem`, `ReadWritePaths=/var/lib/<app>`, …) runs as a
dedicated system user created by `preinstall.sh`, with the database under `/var/lib/<app>` (mode 0750). `postinstall`
enables the service on install and restarts it on upgrade. A purge removes the config and the user, but **keeps the
data directory**. Rename the user, paths and unit file together.

Keep these details of the copied scripts:

- `preremove` stops the service **only** when `$1 = remove`. On upgrade the service keeps running: replacing the
  binary under a running process is safe on Linux. Stopping it there would make `postinstall`'s `is-active` check see
  a stopped service and leave it down after every upgrade.
- `postinstall` restarts on upgrade **only if the service was active**, so an admin's choice to disable it
  survives.
- The unit passes `--config /etc/<app>/config.yaml` explicitly, so a stray `config.yaml` in the working directory
  cannot shadow the packaged one.

Lessons from aether's package:

- **Optional runtime tools are `Recommends`**, not `Depends`. apt installs them by default, and the app still
  starts without them ([server-lifecycle.md](server-lifecycle.md), optional dependencies).
- **The systemd sandbox breaks features without an error.** `ProtectSystem=strict` makes everything outside
  `ReadWritePaths` read-only. A feature that writes elsewhere (aether's tag editor writes into music folders) needs
  those paths added with `systemctl edit`. Say so in the docs. `MemoryDenyWriteExecute=true` breaks any JIT (aether
  runs a wasm module with wazero, so it sets it to `false`).
- Two amd64 debs (v1 and v3) share one package name and architecture, and differ only by file name
  (`nfpms[].file_name_template`). That is fine for downloads. **If you ever serve them from an apt repository**, drop
  the v3 deb or give it its own suite, because apt sees two candidates for the same package.

## Container image (`zarf/docker/`, goreleaser `dockers_v2`)

The Dockerfile compiles nothing. goreleaser puts the prebuilt static binary in the build context, so the image is
Alpine (with `ca-certificates` and `tzdata`) plus the binary. It is multi-arch (amd64 + arm64) and runs as uid 1000
with `WORKDIR` and `VOLUME` at `/var/lib/<app>`. The baked-in config listens on `0.0.0.0:8080`, and
`<PREFIX>_*` env vars override it. `MAJOR.MINOR` and `latest` tags move only on non-prerelease versions. On GHCR the
first push creates a **private** package, and making it public is a one-time step in the package settings.

## License

The license ships in every archive, in the deb (`/usr/share/doc/<app>/LICENSE`) and as the image's
`org.opencontainers.image.licenses` annotation. reference-app is GPL-3.0. Set your own in all three places.

## Local release dry run

`make build` builds the current platform into `dist/`. To run the whole release without publishing:
`make package-ui && goreleaser release --snapshot --clean`.
