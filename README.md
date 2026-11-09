# reference-app

A living reference of best practices, code structure and guides for my Go projects.
It doubles as an opinionated boilerplate for Go applications (originally "bgb", Bott's Go Boilerplate).

## Features

- Start a new Go service from a working app: CLI, layered config, structured logging, graceful shutdown
- Design the REST API contract-first, with tests that keep the OpenAPI spec and the routes in sync
- Store data in SQLite by default or Postgres by config, with every record isolated per tenant
- Delegate login to a reverse proxy instead of handling passwords and sessions
- Ship a Vue web UI embedded in a single binary, released as zips, debs and a container image
- Follow a clear folder structure that keeps business logic out of handlers

## Quick start

Needs Go and Node.

```sh
git clone https://github.com/go-bumbu/reference-app.git
cd reference-app
make run-ui
```

Open <http://localhost:8081>; the sample config signs every request in as `admin`.

## Documentation

- [Build a new app from this reference](docs/reference/README.md): step-by-step guides for reusing each part
- [Architecture](docs/agents/architecture.md): folder tiers, design decisions, and links to every topic doc
- [Features](docs/agents/features.md): what is implemented, partial, or deliberately left out
- [API](docs/agents/api.md) and its [OpenAPI spec](docs/openapi/api-v0.yaml)
- [Authentication and tenancy](docs/agents/auth-and-tenancy.md)
- [Database](docs/agents/database.md)
- [Web UI](docs/agents/web-ui.md)
- [Server lifecycle](docs/agents/server-lifecycle.md)

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md).

## License

Licensed under the [GPL-3.0 License](LICENSE).
