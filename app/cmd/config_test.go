package cmd

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-bumbu/reference-app/app/router"
)

// TestResolveConfigFile covers the packaged-vs-development lookup: an explicit
// -c is mandatory so typos fail loudly, while the implicit search falls through
// the working directory to /etc and finally to defaults-only.
func TestResolveConfigFile(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "config.yaml")
	etc := filepath.Join(dir, "etc-config.yaml")
	if err := os.WriteFile(etc, []byte("Env:\n  LogLevel: debug\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		flag          string
		paths         []string
		wantPath      string
		wantMandatory bool
	}{
		{
			name:          "explicit flag wins and is mandatory",
			flag:          "/nonexistent/custom.yaml",
			paths:         []string{etc},
			wantPath:      "/nonexistent/custom.yaml",
			wantMandatory: true,
		},
		{
			name:          "first existing search path is used",
			paths:         []string{local, etc},
			wantPath:      etc,
			wantMandatory: false,
		},
		{
			name:          "no config found means defaults only",
			paths:         []string{local, filepath.Join(dir, "missing.yaml")},
			wantPath:      "",
			wantMandatory: false,
		},
		{
			name:          "a directory in the search path is skipped",
			paths:         []string{dir, etc},
			wantPath:      etc,
			wantMandatory: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, mandatory := resolveConfigFile(tc.flag, tc.paths)
			if path != tc.wantPath {
				t.Errorf("path: got %q, want %q", path, tc.wantPath)
			}
			if mandatory != tc.wantMandatory {
				t.Errorf("mandatory: got %v, want %v", mandatory, tc.wantMandatory)
			}
		})
	}
}

// TestGetAppCfgMissingMandatoryFile ensures a bad -c path is an error rather
// than a silent fall-through to defaults.
func TestGetAppCfgMissingMandatoryFile(t *testing.T) {
	if _, err := getAppCfg(filepath.Join(t.TempDir(), "nope.yaml"), true); err == nil {
		t.Fatal("expected an error for a missing mandatory config file")
	}
}

// TestGetAppCfgNoFileUsesDefaults verifies the empty-path case yields the built-in defaults.
func TestGetAppCfgNoFileUsesDefaults(t *testing.T) {
	cfg, err := getAppCfg("", false)
	if err != nil {
		t.Fatalf("getAppCfg: %v", err)
	}
	if cfg.Server.Port != defaultCfg.Server.Port {
		t.Errorf("port: got %d, want %d", cfg.Server.Port, defaultCfg.Server.Port)
	}
	if cfg.Obs.Enabled {
		t.Error("observability server should be disabled by default")
	}
}

// TestGetAppCfgFileAndEnv verifies the precedence: config file over defaults, env vars over the file.
func TestGetAppCfgFileAndEnv(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	content := "Server:\n  Port: 7000\nObservability:\n  Enabled: true\nEnv:\n  LogLevel: warn\n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVarPrefix+"_ENV_LOGLEVEL", "debug")

	cfg, err := getAppCfg(file, true)
	if err != nil {
		t.Fatalf("getAppCfg: %v", err)
	}
	if cfg.Server.Port != 7000 {
		t.Errorf("port: got %d, want 7000", cfg.Server.Port)
	}
	if !cfg.Obs.Enabled {
		t.Error("observability server should be enabled by the config file")
	}
	if cfg.Env.LogLevel != "debug" {
		t.Errorf("log level: got %q, want the env var value %q", cfg.Env.LogLevel, "debug")
	}
}

// TestGetAppCfgDB covers the DB section: SQLite by default with an absolute
// path, Postgres selected via the file with the password from an env var, and
// a broken DB config failing the load instead of the startup.
func TestGetAppCfgDB(t *testing.T) {
	t.Run("defaults to sqlite with an absolute path", func(t *testing.T) {
		cfg, err := getAppCfg("", false)
		if err != nil {
			t.Fatalf("getAppCfg: %v", err)
		}
		if cfg.DB.Type != "sqlite" {
			t.Errorf("type: got %q, want sqlite", cfg.DB.Type)
		}
		if !filepath.IsAbs(cfg.DB.Sqlite.Path) {
			t.Errorf("sqlite path: got %q, want an absolute path", cfg.DB.Sqlite.Path)
		}
	})

	t.Run("postgres from file and env", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "config.yaml")
		content := "DB:\n  Type: postgres\n  Postgres:\n    Host: db.local\n    User: app\n    DbName: app\n"
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvVarPrefix+"_DB_POSTGRES_PASSWORD", "s3cret")

		cfg, err := getAppCfg(file, true)
		if err != nil {
			t.Fatalf("getAppCfg: %v", err)
		}
		pg := cfg.DB.gormCfg().Postgres
		if pg.Host != "db.local" || pg.DbName != "app" || pg.Password != "s3cret" {
			t.Errorf("postgres cfg: got %+v", pg)
		}
		if pg.Port != 5432 || pg.SslMode != "require" {
			t.Errorf("postgres defaults lost: got port %d, sslmode %q", pg.Port, pg.SslMode)
		}
	})

	for name, content := range map[string]string{
		"unknown type":          "DB:\n  Type: oracle\n",
		"postgres without host": "DB:\n  Type: postgres\n  Postgres:\n    DbName: app\n",
		"sqlite without path":   "DB:\n  Sqlite:\n    Path: \"\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := getAppCfg(file, true); err == nil {
				t.Fatal("expected an invalid DB config to fail the load")
			}
		})
	}
}

// TestGetAppCfgAuth covers the Auth section: none by default, proxy with its
// headers and trusted proxies from the file, and invalid setups failing the load.
func TestGetAppCfgAuth(t *testing.T) {
	writeCfg := func(t *testing.T, content string) string {
		t.Helper()
		file := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return file
	}

	t.Run("defaults to none", func(t *testing.T) {
		cfg, err := getAppCfg("", false)
		if err != nil {
			t.Fatalf("getAppCfg: %v", err)
		}
		if cfg.Auth.Method != router.AuthNone {
			t.Errorf("method: got %q, want %q", cfg.Auth.Method, router.AuthNone)
		}
	})

	t.Run("proxy from file", func(t *testing.T) {
		file := writeCfg(t, "Auth:\n  Method: \" Proxy \"\n  ProxyHeader:\n"+
			"    UserHeader: X-Forwarded-User\n    TenantHeader: \" X-Tenant \"\n"+
			"    TrustedProxies:\n      - 127.0.0.1\n      - 10.0.0.0/8\n")
		cfg, err := getAppCfg(file, true)
		if err != nil {
			t.Fatalf("getAppCfg: %v", err)
		}
		rc, err := cfg.Auth.routerCfg()
		if err != nil {
			t.Fatalf("routerCfg: %v", err)
		}
		want := router.AuthCfg{
			Method: router.AuthProxy,
			Proxy: router.ProxyAuthCfg{
				UserHeader:   "X-Forwarded-User",
				TenantHeader: "X-Tenant",
				TrustedProxies: []netip.Prefix{
					netip.MustParsePrefix("127.0.0.1/32"),
					netip.MustParsePrefix("10.0.0.0/8"),
				},
			},
		}
		if !reflect.DeepEqual(rc, want) {
			t.Errorf("got %+v, want %+v", rc, want)
		}
	})

	for name, content := range map[string]string{
		"unknown method":            "Auth:\n  Method: basic\n",
		"proxy without user header": "Auth:\n  Method: proxy\n  ProxyHeader:\n    UserHeader: \" \"\n",
		"bad trusted proxy":         "Auth:\n  Method: proxy\n  ProxyHeader:\n    TrustedProxies: [\"10.0.0.0/33\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := getAppCfg(writeCfg(t, content), true); err == nil {
				t.Fatal("expected an invalid Auth config to fail the load")
			}
		})
	}
}

// TestRunServerWiresTheRouter runs the real composition root up to the router.
// With "oidc" router.New refuses to start; it checks its dependencies first, so
// getting ErrOIDCNotImplemented proves runServer passed all of them in.
func TestRunServerWiresTheRouter(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	content := "Auth:\n  Method: oidc\nDB:\n  Sqlite:\n    Path: " + filepath.Join(t.TempDir(), "app.db") + "\n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runServer(file)
	if !errors.Is(err, router.ErrOIDCNotImplemented) {
		t.Fatalf("got %v, want ErrOIDCNotImplemented", err)
	}
}
