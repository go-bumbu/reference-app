# Config and CLI — cobra, layered config, slog, build metadata

How to give a new app the reference CLI, config loading and logging. Copy `main.go`, `app/cmd/root.go`,
`app/cmd/config.go` (+ `config_test.go`), `app/logger/` and `app/metainfo/`, then apply the renames in
[README.md](README.md). Background: [../agents/server-lifecycle.md](../agents/server-lifecycle.md).

## CLI (cobra, `app/cmd/root.go`)

- `main.go` only calls `cmd.Execute()`. Commands are `start [-c file]` and `version`.
- Keep `SilenceUsage: true` and `SilenceErrors: true` on the root command. A runtime error (bad config, port in use)
  is not a usage error: `Execute` prints it once and exits `1`. `RunE` returns errors and never calls `os.Exit`.
- A new command goes in `app/cmd/<name>.go`, is added in `rootCmd`, and reuses `getAppCfg` and the logger, so it
  reads the same config as the server.

## Config (`app/cmd/config.go`, [go-bumbu/config](https://github.com/go-bumbu/config))

Precedence, low to high: built-in `defaultCfg` → `.env` in the working directory → config file → env vars with the
app's prefix. Env var names are the uppercase key path: `REFAPP_SERVER_PORT`, `REFAPP_AUTH_METHOD`,
`REFAPP_DB_POSTGRES_PASSWORD`.

Keep these behaviours:

- **A file passed with `-c` must exist.** A typo fails loudly, and never falls back to defaults silently.
- Without `-c`, look in `./config.yaml` (development), then `/etc/<app>/config.yaml` (packaged). If neither exists,
  use defaults and env vars only.
- **Validation and normalization happen in the loader** (`getAppCfg`). The rest of the app only sees a valid config.
  Resolve relative paths to absolute ones, trim values, parse CIDRs, and check cross-field rules there.
- **Map each section to its consumer's config with a method** (`dbCfg.gormCfg()` → `gormdb.Cfg`,
  `authCfg.routerCfg()` → `router.AuthCfg`). The config file format stays yours even if a library's `Cfg` changes.
  Never put library types in `AppCfg`.
- Never cache parsed values on config structs: `go-bumbu/config` walks every field, unexported ones included.
- Keep secrets out of files: document the env var (`..._PASSWORD`). Never log a value that may hold one.
- **A secret can also live in its own file.** `go-bumbu/config` reads any string value that starts with `@` as
  the path of a file and uses the file's contents. This works with docker secrets and systemd credentials. Trim the
  value in the loader, because the file usually ends with a newline. The catch: this applies to **every** string
  value. A value that really starts with `@` (a regex, a folder name like `@eaDir`) needs a note in the sample
  config. *(aether: `FanartApiKey: "@/etc/aether/fanart.api.key"` in `zarf/packaging/config.yaml`)*
- **Use a pointer for an optional value whose zero value means something.** Since `go-bumbu/config` v0.5.0 a
  pointer field stays `nil` when no source sets the key, and an explicit `false` gives a pointer to `false`. A bool
  that defaults to `true` must be a `*bool` read as `p == nil || *p`. A plain `bool` reads an omitted key as
  `false`. *(aether: `FollowSymlinks`, `LoginThrottle.Enabled`)*
- **Fail on what the config says, warn on what the world says.** Reject bad config at load: duplicate names,
  nested paths, a regex that does not compile. Name the offending entry in the error. Do not fail when a resource
  the config names is missing at startup (a network share that mounts late, a path not created yet). Log a `WARN`
  and let the feature that needs it refuse to run. *(aether: `scanfolder.NewSet` and `warnScanFolders`,
  `app/cmd/scanfolders.go`)*
- The logger does not exist while config loads, so buffer the loader messages (`AppCfg.Msgs`) and log them after
  the logger is created.

### Adding a config section

1. Add a private section type and a field on `AppCfg` (use a `config:"Name"` tag when the file key differs).
2. Add its defaults to `defaultCfg`.
3. Add `normalizeXCfg` and call it from `getAppCfg`, wrapping errors as `config X: %w`.
4. Add the mapping method to the consumer's `Cfg`.
5. Document the keys in `zarf/config.yaml` (development), `zarf/packaging/config.yaml` (deb) and
   `zarf/docker/config.yaml` (image).
6. Add cases to `config_test.go`: defaults, file + env precedence, and every validation error.

## Logging (`app/logger`)

- `log/slog` only. `GetDefault` uses a coloured console handler on a TTY, and JSON with RFC 3339 UTC times otherwise
  (systemd, containers).
- Pass the `*slog.Logger` down from the composition root. Never use the global `slog` default in modules or libs.
- Tag every line with `slog.String("component", "<area>")` (`startup`, `config`, `db`, `auth`, `<server name>`)
  so logs can be filtered by area.
- Log at startup what an operator needs: version, auth mode, db type and location. Warn about risky setups
  (`none` auth, a proxy without trusted IPs).
- Each outbound client sends a user agent that names the app, its version and its homepage
  (`<App>/<version> (<homepage>)`). Third-party APIs such as MusicBrainz require one, and it lets them contact you
  instead of blocking you. *(aether)*

## Offline admin commands

Some fixes cannot wait for the server to run: a locked-out admin, a password hash for the config file. Add these as
CLI subcommands that load the same config and open the same database as `start` (`aether user …`). Keep each
command's logic in its module. The command only parses flags and calls it. *(aether: `app/cmd/usercmd.go`)*

## Build metadata (`app/metainfo`)

`Version`, `BuildTime` and `ShaVer` are package variables, set by goreleaser with
`-ldflags -X <module>/app/metainfo.<Var>=…`. A plain `go build` gives `dev-build` / `unknown` / `undefined`. The
`version` command and `GET /info` read them. After renaming the module, update the `-ldflags` paths, or every
release reports `dev-build` ([build-and-release.md](build-and-release.md)).
