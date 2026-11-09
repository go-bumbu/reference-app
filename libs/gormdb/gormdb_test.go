package gormdb

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenSqlite(t *testing.T) {
	// a nested, missing directory: Open must create it
	path := filepath.Join(t.TempDir(), "data", "sqlite", "test.db")
	db, err := Open(Cfg{Driver: DriverSqlite, Sqlite: SqliteCfg{Path: path, BusyTimeout: 2 * time.Second}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := Close(db); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file not created: %v", err)
	}

	// the pragmas are set from the DSN, so every pooled connection has them
	pragmas := map[string]string{
		"busy_timeout": "2000",
		"journal_mode": "wal",
		"foreign_keys": "1",
	}
	for pragma, want := range pragmas {
		var got string
		if err := db.Raw("PRAGMA " + pragma).Scan(&got).Error; err != nil {
			t.Fatalf("read %s: %v", pragma, err)
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", pragma, got, want)
		}
	}
}

func TestOpenSqliteUnwritableDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// the parent "directory" is a regular file
	_, err := Open(Cfg{Driver: DriverSqlite, Sqlite: SqliteCfg{Path: filepath.Join(file, "test.db")}})
	if err == nil {
		t.Fatal("expected an error when the directory cannot be created")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Cfg
		wantErr string
	}{
		{name: "sqlite", cfg: Cfg{Driver: DriverSqlite, Sqlite: SqliteCfg{Path: "x.db"}}},
		{name: "postgres", cfg: Cfg{Driver: DriverPostgres, Postgres: PostgresCfg{Host: "h", DbName: "d"}}},
		{name: "unknown driver", cfg: Cfg{Driver: "mysql"}, wantErr: "unsupported driver"},
		{name: "empty driver", cfg: Cfg{}, wantErr: "unsupported driver"},
		{name: "sqlite without path", cfg: Cfg{Driver: DriverSqlite}, wantErr: "path"},
		{name: "postgres without host", cfg: Cfg{Driver: DriverPostgres, Postgres: PostgresCfg{DbName: "d"}}, wantErr: "host"},
		{name: "postgres without db", cfg: Cfg{Driver: DriverPostgres, Postgres: PostgresCfg{Host: "h"}}, wantErr: "database name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got %v, want an error containing %q", err, tc.wantErr)
			}
			// Open refuses the same config before touching any driver
			if _, openErr := Open(tc.cfg); openErr == nil {
				t.Error("Open accepted an invalid config")
			}
		})
	}
}

func TestPostgresDSN(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		got := PostgresCfg{Host: "db", User: "app", Password: "pw", DbName: "app"}.DSN()
		want := "postgres://app:pw@db:5432/app?sslmode=require" //nolint:gosec // G101: fake test credentials
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("special characters are escaped", func(t *testing.T) {
		cfg := PostgresCfg{Host: "db", Port: 6543, User: "app", Password: "p@ss:w/rd?#", DbName: "app", SslMode: "disable"} //nolint:gosec // G101: fake test credentials
		u, err := url.Parse(cfg.DSN())
		if err != nil {
			t.Fatalf("DSN is not a valid URL: %v", err)
		}
		if pw, _ := u.User.Password(); pw != cfg.Password {
			t.Errorf("password: got %q, want %q", pw, cfg.Password)
		}
		if u.Port() != "6543" {
			t.Errorf("port: got %q, want 6543", u.Port())
		}
		if got := u.Query().Get("sslmode"); got != "disable" {
			t.Errorf("sslmode: got %q, want disable", got)
		}
	})

	t.Run("ipv6 host without user", func(t *testing.T) {
		got := PostgresCfg{Host: "::1", DbName: "app"}.DSN()
		want := "postgres://[::1]:5432/app?sslmode=require"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
