# Releasing — tags, goreleaser, packages, container image

Releases are built in CI by goreleaser when a `v*.*.*` tag is pushed. This doc covers the mechanics and the traps;
the gates a change must pass first are in [testing.md](testing.md).

    make tag version="v1.2.3"   # on main with a clean tree; (re)creates and pushes the tag

- `make tag` refuses unless you are on `main`, the tree has no unstaged changes, and the version looks like
  `v1.2.3` (optionally `-rc.1`). It **does not run `make verify`**: a release is cut from a `main` that already passed
  the `go` and `webui` workflows. It deletes and re-pushes an existing tag of the same name — never re-tag a version
  that has been published.
- `check-git-clean` uses `git diff --quiet`: untracked files do not block a tag (debt, in `TODO.md`).
- Never tag or push on your own; releasing is the maintainer's call.

## What the tag publishes (`.github/workflows/release.yml`)

Both jobs run `make package-ui` first, then goreleaser:

| Job | Config | Publishes |
|---|---|---|
| `linux + windows` | `.goreleaser.yaml` | the GitHub release; zips for linux (amd64 v1/v3, arm64) and windows (amd64 v1/v3); debs; `refapp-spa.zip`; the container image |
| `macos` | `.goreleaser-darwin.yaml` | darwin zips (amd64, arm64), appended to the same release |

- **Pure Go, `CGO_ENABLED=0` everywhere**: static binaries with no glibc floor, cross-compiled from any host. Adding
  a CGO dependency breaks this; pick a pure-Go alternative (as with `gormdb/sqlite`, not `gormdb/sqlitecgo`).
- **GOAMD64 v1 and v3 only**: v1 runs on any x86-64 CPU; v4 would not start on CPUs without AVX-512.
- **Build metadata** goes in through `-ldflags -X .../app/metainfo.{Version,BuildTime,ShaVer}`; keep the import path
  in both goreleaser configs in step with the module path.
- **The embedded UI is whatever `app/spa/files/ui` holds at build time**, so every release path keeps the
  `package-ui → goreleaser` order ([web-ui.md](web-ui.md)).

## Debian package (`zarf/packaging/`)

`/usr/bin/refapp`; `/etc/refapp/config.yaml` as a conffile (edits survive upgrades); a hardened systemd unit
(`refapp.service`) running as the `refapp` system user; the database under `/var/lib/refapp`. A purge keeps
`/var/lib/refapp`. Maintainer scripts: `preinstall.sh`, `postinstall.sh`, `preremove.sh`, `postremove.sh`.

## Container image (`zarf/docker/`)

`ghcr.io/go-bumbu/refapp`, linux/amd64 + arm64: Alpine plus the prebuilt binary (not built inside the image), uid
1000, the database in the `/var/lib/refapp` volume, listening on `0.0.0.0:8080`. `MAJOR.MINOR` and `latest` only
move on non-prerelease tags. The first push creates a **private** package; making it public is a one-time step in
the package settings.

## License

The license text ships in every release archive, in the deb (`/usr/share/doc/refapp/LICENSE`) and as the image's
`org.opencontainers.image.licenses` annotation. Changing the license means changing all three.

## Try it locally

    make build                                               # current OS/arch into dist/, UI included
    make package-ui && goreleaser release --snapshot --clean # the whole release, nothing published
