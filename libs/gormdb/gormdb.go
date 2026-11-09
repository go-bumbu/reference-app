// Package gormdb opens a GORM connection to the database selected by
// configuration, so the code above it stays driver-agnostic: business modules
// take a *gorm.DB and never know whether SQLite or Postgres is behind it.
//
// SQLite uses a pure-Go driver (github.com/glebarez/sqlite), so the binary
// builds without CGO.
package gormdb

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Supported drivers.
const (
	DriverSqlite   = "sqlite"
	DriverPostgres = "postgres"
)

const (
	defaultBusyTimeout  = 5 * time.Second
	defaultSqliteConns  = 10
	defaultPostgresPort = 5432
	defaultSslMode      = "require"
)

// Cfg selects and configures the database. Only the section matching Driver
// is used.
type Cfg struct {
	// Driver is DriverSqlite or DriverPostgres.
	Driver   string
	Sqlite   SqliteCfg
	Postgres PostgresCfg
	// Logger receives GORM's log output; nil keeps GORM's default logger.
	Logger gormlogger.Interface
}

// SqliteCfg configures a SQLite database file.
type SqliteCfg struct {
	// Path of the database file. Its directory is created when missing; keep
	// the file in a directory of its own, SQLite writes -wal and -shm files
	// next to it.
	Path string
	// BusyTimeout is how long a connection waits for a lock held by another
	// one before failing with SQLITE_BUSY; zero means 5s.
	BusyTimeout time.Duration
}

// PostgresCfg configures a Postgres connection.
type PostgresCfg struct {
	Host     string
	Port     int // zero means 5432
	User     string
	Password string
	DbName   string
	// SslMode is passed as libpq's sslmode (disable, require, verify-full, ...);
	// empty means "require".
	SslMode string
}

// DSN returns the connection URL. User, password and database name are
// escaped, so any character is safe in them.
func (c PostgresCfg) DSN() string {
	port := c.Port
	if port == 0 {
		port = defaultPostgresPort
	}
	sslMode := c.SslMode
	if sslMode == "" {
		sslMode = defaultSslMode
	}
	u := url.URL{
		Scheme:   "postgres",
		Host:     net.JoinHostPort(c.Host, strconv.Itoa(port)),
		Path:     "/" + c.DbName,
		RawQuery: url.Values{"sslmode": {sslMode}}.Encode(),
	}
	// without a user, libpq's defaults (PGUSER, the OS user) apply
	if c.User != "" {
		u.User = url.UserPassword(c.User, c.Password)
	}
	return u.String()
}

// Validate reports a configuration Open cannot work with. Call it while
// loading the config to fail before anything starts.
func (c Cfg) Validate() error {
	switch c.Driver {
	case DriverSqlite:
		if c.Sqlite.Path == "" {
			return errors.New("sqlite: path must not be empty")
		}
	case DriverPostgres:
		if c.Postgres.Host == "" {
			return errors.New("postgres: host must not be empty")
		}
		if c.Postgres.DbName == "" {
			return errors.New("postgres: database name must not be empty")
		}
	default:
		return fmt.Errorf("unsupported driver %q, want %q or %q", c.Driver, DriverSqlite, DriverPostgres)
	}
	return nil
}

// Open validates cfg and connects to the database. Close the returned handle
// with Close on shutdown.
func Open(cfg Cfg) (*gorm.DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	gormCfg := &gorm.Config{Logger: cfg.Logger}
	if cfg.Driver == DriverPostgres {
		db, err := gorm.Open(postgres.Open(cfg.Postgres.DSN()), gormCfg)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		return db, nil
	}
	return openSqlite(cfg.Sqlite, gormCfg)
}

func openSqlite(cfg SqliteCfg, gormCfg *gorm.Config) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o750); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}
	db, err := gorm.Open(sqlite.Open(sqliteDSN(cfg)), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", cfg.Path, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(defaultSqliteConns)
	return db, nil
}

// sqliteDSN sets the pragmas in the DSN rather than with db.Exec: most pragmas
// are per-connection state, and an Exec runs on a single pooled connection,
// leaving the others at their defaults.
//   - busy_timeout: wait for a lock instead of failing with SQLITE_BUSY at once
//     under write contention.
//   - journal_mode=WAL: readers do not block the writer and vice versa.
//   - foreign_keys: SQLite ignores foreign keys unless asked to; Postgres
//     always enforces them, so both drivers behave the same.
func sqliteDSN(cfg SqliteCfg) string {
	timeout := cfg.BusyTimeout
	if timeout <= 0 {
		timeout = defaultBusyTimeout
	}
	q := url.Values{"_pragma": {
		fmt.Sprintf("busy_timeout(%d)", timeout.Milliseconds()),
		"journal_mode(WAL)",
		"foreign_keys(1)",
	}}
	return cfg.Path + "?" + q.Encode()
}

// Close closes the connection pool behind db.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
