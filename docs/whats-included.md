# What's included

* **CLI** with [cobra](https://github.com/spf13/cobra): `start` and `version` commands (`app/cmd`).
* **Layered config** with [go-bumbu/config](https://github.com/go-bumbu/config): defaults → `.env` file → config file → env vars
  (prefix `REFAPP_`, e.g. `REFAPP_ENV_LOGLEVEL=debug`).
  * `-c <file>` is mandatory: a missing file fails loudly instead of silently falling back to defaults.
  * without `-c`, the first existing of `./config.yaml` (development) and `/etc/refapp/config.yaml` (packaged) is used;
    with none, only defaults and env vars apply.
  * validation and normalization of the values belong to the loader (`getAppCfg`), so the server only sees a valid config.
* **Structured logging** with `log/slog`: a human-friendly console handler on a TTY, JSON otherwise (`app/logger`).
* **Two HTTP servers** (`app/cmd/serve.go`, see [Server lifecycle](server-lifecycle.md)):
  * main API server (default `:8080`) with request logging and Prometheus middleware,
  * observability server (default `:9090`, opt-in via `Observability.Enabled`) exposing `/metrics`, `/health` and `/ready`;
    when enabled, keep it off the public interface.
* **REST API** under `/api/v0`, contract-first with an OpenAPI 3.1 spec (see [API](api.md)).
* **Persistence** with [GORM](https://gorm.io): SQLite by default (pure Go, no CGO), Postgres by config
  (see [Database](database.md)).
* **Multi-tenancy**: `internal/tenants` owns tenants and their users; every note belongs to a tenant
  (see [Authentication and tenancy](auth-and-tenancy.md)).
* **Delegated authentication**: `none` for demos, `proxy` for identity headers of a trusted reverse proxy,
  `oidc` reserved.
* **Prototype business module** `internal/notes`, consumed by the API handlers.
* **Web UI** in `webui/`, embedded into the binary (see [Web UI](web-ui.md)).
* **Go API client** in `libs/client`.
* **Build metadata** injected via `-ldflags` at release time (`app/metainfo`), served by `GET /api/v0/info`.
* **Tooling**: golangci-lint v2 config, goreleaser config, license allow-list, per-package coverage gate,
  spectral lint of the OpenAPI spec (`make spec-lint`).

Where each of these lives, and why, is in [Folder structure](folder-structure.md).
