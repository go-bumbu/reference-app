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
  without touching the public API.
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

## Library-owned tables: the jobs of `internal/jobs`

`internal/jobs` owns two tables it never writes itself: tempo's `dbqueue` keeps the runs in `tempo_tasks` and
`dbschedule` the schedules in `tempo_schedules`, each migrated by its own `New` (fixed names, no `tenant_id` column).
The module reaches them only through tempo's runner and scheduler, and finds a run's or a schedule's tenant in its
JSON params (`{"tenant_id": ...}`), so the tenant filter is applied in Go on every read. Runs are mirrored, not
queried: the queue lives in memory and is recovered from `tempo_tasks` at startup, where a run that was running
when the process died is marked `failed`. Run logs are not in the database but in `Jobs.LogDir`, one JSON Lines file
per run, deleted with the run when the history is trimmed.

## Trees: the tags of `internal/notes`

The tag tree is a closure table kept by [`github.com/go-bumbu/closure-tree`](https://github.com/go-bumbu/closure-tree)
(`internal/notes/tagstore.go`), after codex's media tags:

- `tagRow` embeds `closuretree.Node` (id, tenant scope, sibling `sort_order`) and adds the name; its table is
  `note_tags`, and the library keeps `closure_tree_rel_note_tags`, `closure_tree_meta_note_tags` and
  `closure_tree_lock_note_tags` beside it. `store.migrate` runs `tree.Migrate()` after `AutoMigrate`.
- The library's tenant scope is the note tenant's id as it is: every tree call is scoped, so another tenant's tags
  do not exist. Tag ids are the library's auto-increment ids (the one exception to module-generated ids).
- `note_tag_links` (tenant, note, tag) links notes and tags. Listing a tag's notes joins it to the closure table
  (`ancestor_id` = the tag), so the subtree comes along without recursion; reading a note's tags joins `note_tags`, so
  a link a concurrent tag delete left behind is never returned.
- **Every tree write binds a tree to the transaction** (`newTree(tx)`, `store.tagTx`): the library's own writes then
  become savepoints of it, and the tree and the module's tables commit together. A tree bound to the store's `db`
  inside a transaction would wait forever for SQLite's one writer.
- Create uses `GetOrAdd` (find-or-insert under the library's per-tenant lock) so two creates of one name cannot both
  win; rename and move check sibling names in the transaction (a check, not a constraint).
- Pass `nil` (not a typed nil pointer) as the payload of `tree.Update` when the name does not change: a typed nil
  reads as a payload.
- After a reorder the store respaces the siblings' `sort_order` once `NeedsRenormalize` says the float64 halvings
  run low (`TestTagOrderSurvivesManyReorders`).
- The store's own tree (`readTree`) is bound to `db.Clauses(gormdb.Read).Session(...)`: closure-tree reads with
  recursive CTEs (`WITH ...`), which the SQLite resolver would otherwise send to the one writer. Migration binds a
  tree to `db.WithContext(ctx)`, as `Migrate` takes no context.
- **Id 0 is closure-tree's virtual root**, ancestor of every tag: `DeleteRecurse(0)` would delete the whole tree.
  The Service refuses 0 (and ids past int64, which no database binds) as `ErrTagNotFound` before the store
  (`TestTagIDZeroIsNoTag`).
- A tenant has at most `MaxTagsPerTenant` tags, counted in the create's transaction.
- The links have no foreign keys (closure-tree owns the tag table). On Postgres a link can be written right after a
  concurrent delete of its tag; reads skip it (`TestLinkToAMissingTagIsNotReturned`) and it stays until its note is
  deleted — accepted, as it is invisible and bounded by the notes.
- Concurrency tests that write from several goroutines run SQLite as the app opens it (`gormdbtest.Sqlite`, one
  writer that the others queue for): the bare testdbs database answers `SQLITE_BUSY` instead
  (`TestConcurrentCreateOfOneName`).

## Migrations — known debt

`New` runs `AutoMigrate` for the module's rows on every start. It adds tables, columns and indexes but never drops
or renames; a rename or a type change needs a hand-written step in the module's `migrate` until a versioned
migration tool is chosen. Nothing in `TODO.md` picks one yet — ask before introducing one.

## Tests

Module tests run on every database via [go-bumbu/testdbs](https://github.com/go-bumbu/testdbs): `TestMain` registers
`SqliteNoCgo` always and Postgres (in docker) only under `TESTDBS_ALL=1` (`make test-alldbs`); `forEachDB` gives
each subtest a fresh database. `app/` tests use one SQLite file in `t.TempDir()`.
