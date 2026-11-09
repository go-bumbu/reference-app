# Project layout — three tiers, dependencies point down

Use this layout for an app that builds a binary. Code lives in three tiers, and imports only point down:
`app/` → `internal/` → `libs/`. The app stays one deployable unit, while business rules, wiring and generic
mechanisms stay apart. Why: [../agents/architecture.md](../agents/architecture.md).

    main.go          calls app/cmd.Execute(), nothing else
    app/             glue: CLI, config, composition root, routes, handlers, middleware wiring, jobs
    internal/<mod>/  one business module per concern: logic, its tables, a small public API
    libs/<name>/     agnostic foundation: no business words, no app imports
    webui/           frontend sources (optional)
    zarf/            what "holds" the project: sample config, Dockerfile, deb packaging, license allow-list
    docs/            agent docs, reference guides, the OpenAPI contract (docs/openapi/)

## Where does it go?

| You are writing… | Put it in |
|---|---|
| a business rule, validation, a domain type, a query on your data | `internal/<module>` |
| HTTP decoding, status mapping, wire structs | `app/router/handlers/<area>.go` |
| a route, middleware order, auth wrapping | `app/router` |
| a CLI command, config section, construction of shared resources | `app/cmd` |
| a mechanism you can describe without a business word (retry, a codec, a provider client, middleware) | `libs/<name>` |
| a feature that combines two modules (a dashboard of A and B) | `app/` orchestrates both APIs; once it grows business rules of its own, make it a module that receives the others through interfaces |
| a new feature in an existing business area | the existing module — split only when its store grows dozens of methods, it changes for two unrelated reasons, or it keeps breaking another module |

## Rules for `app/`

- Holds no business logic: it translates a request or trigger and delegates.
- Creates every shared runtime resource (logger, db, clients) once, in the composition root, and passes them on
  ([server-lifecycle.md](server-lifecycle.md)).
- Declares the small interfaces it needs from modules on the consumer side (`handlers.NotesService`), so handlers
  test against fakes.
- Building several artifacts (server + CLI tool)? Give each its own folder under `app/`.
- **Is the repo more than one Go module** (a Rust desktop app, a website)? Move the Go module into `server/` with
  its own `Makefile` and tool configs, and keep `webui/`, the other components, `docs/`, `.github/` and the
  goreleaser files at the root. The root `Makefile` delegates to `server/Makefile`, and its `verify` covers every
  component. *(aether: `server/`, `webui/`, `player/`, `site/`)*

## Rules for `internal/`

- One concern per module, with a small public API, typed errors and a `New(ctx, db, Cfg)` constructor
  ([modules-and-database.md](modules-and-database.md)).
- Never import `app/` or another module. When a module needs another's capability, it declares an interface for
  just that, and `app/` passes the implementation in. Prefer passing plain IDs: `internal/notes` gets tenant IDs as
  opaque keys and never imports `internal/tenants`.
- Never instantiate a logger, db or clock. Accept them from `app/`.
- Use direct in-process calls through the public API. Add an event bus only for a real need: several consumers, or
  work that must not block the caller.

## Rules for `libs/`

- Never import `internal/` or `app/`. Inject every app-specific value (names, URIs, thresholds) through a `Cfg`
  struct or an interface. Accept interfaces, return structs.
- Keep dependencies minimal, and avoid depending on other libs. A dependency of a lib is a dependency of every
  future consumer.
- Prefer returning errors to logging. If logging is unavoidable, accept a `*slog.Logger` from the caller.
- Exception: an SDK of your own API (`libs/client`) may name domain concepts, because the wire contract is public.
  It still imports nothing from `app/` or `internal/`.
- Promote a lib to its own released module only when a second **project** needs it.
- Small, stdlib-only helpers grow into libs fast once a second module needs them. codex has `libs/keyedlock` (one
  mutex per key, released when unused), `libs/uploadname` (a safe stored name for an uploaded file) and
  `libs/archive`. They are good candidates to copy.

## Using go-bumbu libraries: fix upstream, don't wrap *(codex)*

Use the go-bumbu libraries (`config`, `http`, `userauth`, `tempo`, `closure-tree`, …) directly. Thin wiring is
fine: building a component from your config, or a handler over the library's own API. When you need a change in
the library, **raise it during planning and fix it upstream**. Do not hide the library behind a local wrapper or
shim. A wrapper lives in one app, and the fix then never reaches the others. aether's history shows the payoff: its
own `internal/upstream` retry and throttle code moved into `go-bumbu/http/outbound`, and every client migrated to it.

## Docs an agent needs

reference-app's `docs/agents/` is the template: an `architecture.md` that every task reads first, one doc per area,
a `features.md` with the status of each capability, and a `testing.md` with the gates. Two additions proved useful
in the apps:

- **Decisions with their status.** Write a decided rule as decided ("do not redesign"), with its reason and date,
  and list the open questions apart (codex: "Open decisions" in `multitenancy.md`). An agent then keeps a rule
  instead of reopening it.
- **Design records apart from current truth.** Dated specs and plans (`docs/superpowers/specs/`, `plans/`) record
  why something was built. Say in `architecture.md` that the code wins when they disagree. Living convention
  registries are different: keep them up to date. *(aether)*

## Making the tiers visible

Copy `.go-deps-view.config` and adjust the package globs. [go-deps-view](https://github.com/candy-tools/go-deps-view)
then draws the import graph along the tiers. It does not enforce them. To turn the rules into lint failures, add a
`depguard` rule to `.golangci.yaml` (planned, not done, in reference-app).
