# Database

`libs/gormdb` opens a `*gorm.DB` for the driver named in the config's `DB.Type`; nothing above it knows which one it is.

| `DB.Type`  | Settings                                                    | Notes                                                       |
|------------|-------------------------------------------------------------|-------------------------------------------------------------|
| `sqlite`   | `DB.Sqlite.Path`                                            | default; the directory is created, WAL, busy timeout, FKs on |
| `postgres` | `DB.Postgres.{Host,Port,User,Password,DbName,SslMode}`      | `SslMode` defaults to `require`; pass the password by env var |

* The config loader resolves the SQLite path to an absolute one and validates the section, so a typo fails at load.
* Each module owns its tables: it takes the `*gorm.DB` in `New`, migrates its own schema and keeps its GORM row types
  private (`internal/notes/store.go`). GORM's errors are translated at the store (e.g. to `notes.ErrNotFound`) and
  never reach the handlers.
* Module tests run on every database through [go-bumbu/testdbs](https://github.com/go-bumbu/testdbs):
  SQLite always, Postgres in a container (needs docker) with `make test-alldbs`.
