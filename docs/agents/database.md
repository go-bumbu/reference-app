# Database — driver-agnostic GORM, module-owned tables

[`github.com/go-bumbu/db/gormdb`](https://github.com/go-bumbu/db) opens a `*gorm.DB` for the driver named in
`DB.Type`; nothing above it knows which one it is. Each
business module receives that handle, migrates its own tables and is the only code that touches them.
See [architecture.md](architecture.md) for module boundaries, [testing.md](testing.md) for the database matrix.

| `DB.Type` | Settings | Notes |
|---|---|---|
| `sqlite` (default) | `DB.Sqlite.Path` | pure Go (`gormdb/sqlite`, glebarez), no CGO; directory created; one writer + read pool |
| `postgres` | `DB.Postgres.{Host,Port,User,Password,DbName,SslMode}` | `SslMode` defaults to `require`; pass the password as `REFAPP_DB_POSTGRES_PASSWORD` |

## Config and connection

- The config loader (`normalizeDBCfg`) makes the SQLite path absolute, so a later chdir cannot move the database,
  and calls `gormdb.Cfg.Validate`, so a bad section fails at load, before anything starts.
- `app/cmd` maps its config section onto `gormdb.Cfg` (`dbCfg.gormCfg`): the file format stays the app's even if the
  library changes.
- `openDB` passes the drivers the app compiles in (`gormdb.Drivers{Sqlite: sqlite.Driver{}, Postgres:
  postgres.Driver{}}`); dropping one from the binary means dropping it here.
- SQLite runs in WAL mode with **one write connection and a pool of 10 read-only ones**, routed by GORM's
  `dbresolver`: writers queue in Go instead of failing with `SQLITE_BUSY`. Busy timeout 5 s, foreign keys on, on
  every connection. This brings rules of its own:
  - inside `db.Transaction` / `db.Connection` use only `tx`; a statement on `db` waits for the writer forever;
  - connection-dependent SQL (`last_insert_rowid()`, temp tables) needs a transaction; prefer `Create` or
    `RETURNING` for a new row's ID;
  - `WITH ...` queries go to the writer; force a statement with `db.Clauses(gormdb.Read)` / `gormdb.Write`;
  - never tune `db.DB()` (it is the writer) and always release through `gormdb.Close`.
- `gormdb.Open` turns on GORM's `TranslateError`: check `errors.Is(err, gorm.ErrDuplicatedKey)` /
  `gorm.ErrForeignKeyViolated`, the same on both databases.
- Keep the SQLite file in a directory of its own: WAL adds `-wal` and `-shm` files next to it.
- GORM logs through the app logger at `Warn`, slow threshold 200 ms, record-not-found ignored (a miss is normal).
- Log the DB location, never the Postgres DSN: it holds the password (`openDB`).
- The db is closed last, by a `defer` in `runServer`, after every server and worker stopped.

## Module persistence rules

Shown by `internal/notes/store.go` and `internal/tenants/store.go`:

- **Only a module's own store reads or writes its tables.** No joins or queries across modules, no shared
  persistence models. Need another module's data? Ask its API through an interface `app/` passes in.
- **Row types are private** (`noteRow`) and pinned with `TableName()`, so renaming the Go type does not move data.
  Domain types (`notes.Note`) carry no GORM tags; the store maps explicitly between them, so the schema can change
  without touching the public API. (The `//nolint:staticcheck // S1016` on that mapping is deliberate.)
- **GORM errors stop at the store**: `gorm.ErrRecordNotFound` becomes `notes.ErrNotFound`; everything else is wrapped
  with the operation (`"select note: %w"`). Handlers never see a GORM type.
- **Column sizes mirror the validation constants** (`size:200` ↔ `MaxTitleLen`); change both together, and keep the
  `TestMaxLengthFits` test that writes a maximal value on every database.
- **Tenant-scoped tables index `tenant_id` first** (`idx_notes_tenant_created`), matching the store's filter and
  order.
- Use `Take`, not `First`, for lookups by key: `First` adds an `ORDER BY` primary key.
- **Times are UTC, truncated to microseconds** before storing (the finest precision every supported db keeps) and
  converted back to UTC on read (Postgres returns the session's zone). Lists order with a tie-breaker
  (`created_at DESC, id ASC`) for a stable order.
- IDs are 16 random hex chars (`crypto/rand`), generated in the module, not by the database.
- Inject the clock through `Cfg.Now`; never call `time.Now` directly in module logic.

## Migrations — known debt

`New` runs `AutoMigrate` for the module's rows on every start. It adds tables, columns and indexes but never drops
or renames; a rename or a type change needs a hand-written step in the module's `migrate` until a versioned
migration tool is chosen. Nothing in `TODO.md` picks one yet — ask before introducing one.

## Tests

Module tests run on every database via [go-bumbu/testdbs](https://github.com/go-bumbu/testdbs): `TestMain` registers
`SqliteNoCgo` always and Postgres (in docker) only under `TESTDBS_ALL=1` (`make test-alldbs`); `forEachDB` gives
each subtest a fresh database. `app/` tests use one SQLite file in `t.TempDir()`.
