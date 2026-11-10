# Modules and database — writing a business module with its own GORM store

How to write a business module in a new app, with persistence that runs unchanged on SQLite and Postgres.
`internal/notes` is the template. Copy its shape, not its domain. Use `github.com/go-bumbu/db/gormdb` to open the
database. Background:
[../agents/database.md](../agents/database.md).

## Module shape (`internal/<module>/`)

| File | Holds |
|---|---|
| `<module>.go` | package doc stating the concern; domain types (no storage tags); `Cfg`; `Service`; `New`; typed errors; validation |
| `store.go` | private row types, `TableName()`, `migrate`, and every query; the only code that touches the module's tables |
| `<module>_test.go` | `TestMain` with testdbs, `forEachDB`, behaviour tests |

- **Constructor**: `New(ctx, db *gorm.DB, cfg Cfg) (*Service, error)`. It rejects a nil db, fills `Cfg` zero values
  with defaults (`Now: time.Now`), and migrates its own tables. The caller owns `db` and closes it.
- **Public API**: a few methods on `*Service` that take `ctx` first and, when the data is tenant-scoped, an `Actor`
  second ([auth-and-tenancy.md](auth-and-tenancy.md)). Inputs come in their own struct (`Input`), separate from the
  stored type (`Note`).
- **Typed errors** that the transport maps to status codes: a sentinel `ErrNotFound`, and
  `*ValidationError{Field, Msg}` where `Field` is the input field's JSON name, which becomes the problem pointer.
  Unexpected errors are wrapped with the operation.
- **Business rules live here**: trimming, length limits as exported constants (`MaxTitleLen`), and generated values
  (ID, author, timestamps). Handlers never apply them.
- **No HTTP, JSON or app imports.** The module has no idea it is served over HTTP.
- **Dependencies come in through `Cfg`**: a clock, other modules' capabilities as small interfaces declared in this
  package. Never import another module.

## Store rules (`store.go`)

- Row types are private (`noteRow`) and pinned with `TableName()`. Map them to domain types explicitly in both
  directions. Never return a row type, and never put GORM tags on a domain type.
- Translate GORM errors in the store: `gorm.ErrRecordNotFound` → `ErrNotFound`, and wrap everything else
  (`"select note: %w"`). Nothing GORM-typed leaves the store.
- Always call `db.WithContext(ctx)`, so a cancelled request cancels its query.
- Use `Take` for lookups by key (`First` adds a useless `ORDER BY`). Give every list a total order, with the ID as
  tie-breaker.
- Keep column sizes in step with the validation constants (`size:200` ↔ `MaxTitleLen`), with a comment saying so.
- Index the columns you filter by, the tenant first (`idx_notes_tenant_created`: `tenant_id`, `created_at`).
- Store times in UTC, truncated to microseconds, and convert them back to UTC on read. Then a returned value equals
  the stored one on every database.
- Generate IDs in the module (16 random hex chars from `crypto/rand`), not with database sequences.
- For "create if missing" under concurrency, insert with `clause.OnConflict{DoNothing: true}` and then read back.
  See `tenants.store.ensureTenant`.
- **Inside a transaction, use its `tx` for everything, never the store's `db`.** A read on `db` misses the
  transaction's writes. On SQLite with a single write connection, a write on `db` waits for the transaction that
  holds the connection, so it blocks forever with no error. *(codex)*
- **Do no slow work inside a transaction**: no network calls, file processing or hashing. A transaction holds the
  write lock, so it holds up every other writer. Do the slow part first and then write in one short transaction.

## Data that belongs to two modules

- **A parent's deletion cleans up its children through hooks.** When deleting a record of A must remove what B and
  C keep for it, A's side declares a small interface (`SpaceCleaner`) and takes a list of them in its constructor.
  The composition root passes B's and C's implementations. A runs every cleaner before it deletes its own row, and
  never imports B or C. *(codex declares it in the spaces handler, `app/router/handlers/spaces/spaces.go`, and the
  media, projects, wiki and jobs handlers each provide one. In a module, the same interface goes in the module.)*
- **A lib that owns tables takes an opaque scope.** A `libs/` package with its own tables (a document store, a blob
  store) knows no tenant. Each of its rows carries a scope string that the calling module builds, and the module
  puts the tenant in it (codex: `"<tenant_id>:<space_id>"` for `libs/docstore`). Build the scope in one place
  (codex: `internal/blobscope`), so no caller forgets the tenant.

## Hierarchies (tag trees, folders)

Keep a tree in a closure table with [`go-bumbu/closure-tree`](https://github.com/go-bumbu/closure-tree), in the
module that owns what the tree organises: listing the items under a node is then one join to the closure table,
subtree included, and no query crosses a module. *(reference-app: `internal/notes/tagstore.go`; codex:
`internal/model/media/tags.go`)*

- Embed `closuretree.Node` in a private row type and use the tenant id (or a scope built from it) as the library's
  tenant, so every tree call is scoped.
- Bind a tree to the transaction for every write (`closuretree.New(tx, row{})`): its writes become savepoints and
  commit with the module's own rows.
- Create with `GetOrAdd` when names are unique among siblings: the find and the insert run under the library's
  per-tenant lock. Moves are checked first (not under itself or a descendant, the parent in the tree) so the
  caller learns which rule broke.
- Expose the tree as nested nodes (`{id, parentId, name, children}`); take a move as `parentId` and a place among
  siblings as `afterId` (0 = first), the library's own convention.
- Respace sibling order when `NeedsRenormalize` says so, after a reorder.

## Caches of derived data

Cache a derived result (counts, a snapshot) only after measuring the query it replaces. Then keep the cache honest
*(codex: `internal/model/media/facetcache.go`)*:

- Key it by tenant, and count every write to the tables it reads with a **generation counter** (a GORM hook). File
  an entry under the generation read *before* loading it, so a write during the load makes it stale at once.
- Bound it (rows or entries, LRU) and give it an off switch in config.
- Keep the uncached SQL path as the **oracle**, and add a test that asserts the cached answer equals it. Add another
  test that fails when the loader reads a table the hook does not watch.
- Write down its scope: a cache in one process goes stale when another process writes to the same database.

## Database selection (`github.com/go-bumbu/db/gormdb`)

- `gormdb.Open(Cfg, Drivers)` returns a `*gorm.DB` for `sqlite` or `postgres`, and `gormdb.Close` releases it.
  `Validate` lets the config loader fail early. Pass only the drivers the app supports; the binary links only those.
- Use the pure-Go `gormdb/sqlite` driver so the binary needs no CGO; `gormdb/sqlitecgo` only when you need SQLite
  extensions. Give the db file its own directory, because of the WAL files.
- SQLite already runs as one write connection plus a read-only pool, routed by `dbresolver`, so writers queue in Go
  instead of running out of the busy timeout. Keep to its rules: only `tx` inside a transaction, no tuning of
  `db.DB()`, and tests on `gormdbtest.Sqlite` (a temp file with the same setup), never `:memory:`.
- Postgres defaults to `sslmode=require`. Pass the password as an env var, and log the host and db name, never the
  DSN.
- In the composition root, route GORM's logs through the app logger (`gormlogger.NewSlogLogger`, `Warn`, 200 ms slow
  threshold, record-not-found ignored).
- A driver `gormdb` does not ship belongs in `go-bumbu/db`, not in the app. The app only maps its config section
  onto `gormdb.Cfg` in `app/cmd`. Modules do not change.
- WAL needs the database file on a local disk, not a network share. A backup copy needs the `-wal` file too.
- Prefer a pure-Go SQLite driver. codex uses the CGO one (`mattn/go-sqlite3`) and so ships linux/amd64 only:
  a CGO cross-compile needs a C cross-compiler for each target.

## Migrations

Each module runs `AutoMigrate` for its own rows in `New`. That adds tables, columns and indexes, but never renames
or drops them. For a rename, a type change or a data backfill, write an explicit step in the module's `migrate`, or
adopt a versioned migration tool. reference-app has not chosen one yet, so record the choice in your app's docs.

**State the compatibility policy in `AGENTS.md`.** Before the first real user, an app can say "no backwards
compatibility: no migration code, no compat shims, change the schema and drop the database". That keeps agents from
writing bridges that nobody needs. Remove the line when the first real deployment exists. *(aether)*

When a stored key names something in code (an app, a feature) and that thing is renamed, keep the old name next to
the new one (codex: `apps.App.FormerKeys`), and move the rows to the new key in the module's `migrate`.

## Tests

- In `TestMain`, call `testdbs.InitCustomDbs` with `SqliteNoCgo` as the default and `NewPostgres("")` as the extra
  target, and `testdbs.Clean()` after the run. `forEachDB` runs a test once per database, each with a fresh database.
- Postgres runs only with `TESTDBS_ALL=1` (`make test-alldbs`, needs docker). Run it whenever a store changes.
- Cover at least: create + read back equality, not-found, every validation rule, the maximum length fitting the
  column, list order, data surviving a new `Service` on the same db, a cancelled context, `New` without a db,
  tenant isolation, and fail-closed without a tenant.
- Inject a fixed clock through `Cfg.Now`. Never sleep to get distinct timestamps.
- Keep each module at 70% coverage or more: the gate is per package.
- **A lib with several backends gets one conformance suite** that every backend runs (codex: `libs/blobstore`,
  filesystem and S3). A new backend is done when it passes the suite.
- Golden-file tests (rendered images, generated output) keep the goldens local: they are gitignored, the test
  **skips** when a golden is absent, and `-update` writes a new one. CI never gates on them. *(aether:
  `libs/covergen`)*
