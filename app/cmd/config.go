package cmd

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/go-bumbu/config"
	"github.com/go-bumbu/db/gormdb"
	"github.com/go-bumbu/reference-app/app/router"
)

type AppCfg struct {
	Server serverCfg
	Obs    obsCfg `config:"Observability"`
	DB     dbCfg
	Auth   authCfg
	Env    Env

	// Msgs collects the config loader output; it is logged once the logger exists.
	Msgs []Msg
}

type Env struct {
	LogLevel string
	// Production masks API error bodies: no internal detail reaches the client.
	Production bool
}

// serverCfg configures the main HTTP server.
type serverCfg struct {
	BindIp string
	Port   int
}

func (c serverCfg) Addr() string {
	return listenAddr(c.BindIp, c.Port)
}

// obsCfg configures the observability server (metrics, health, ready).
// Enabled false means the server is never started at all.
type obsCfg struct {
	Enabled bool
	BindIp  string
	Port    int
}

func (c obsCfg) Addr() string {
	return listenAddr(c.BindIp, c.Port)
}

// authCfg selects how requests are authenticated; see router.AuthCfg.
type authCfg struct {
	// Method: "none" (default; one fixed tenant and user, for demos), "proxy"
	// (identity headers of a trusted reverse proxy) or "oidc" (not implemented yet).
	Method      string
	ProxyHeader proxyHeaderCfg
}

// proxyHeaderCfg configures Method "proxy".
type proxyHeaderCfg struct {
	// UserHeader carries the authenticated user (Authelia: Remote-User,
	// oauth2-proxy: X-Forwarded-User).
	UserHeader string
	// TenantHeader carries the user's tenant; empty puts every user in the
	// default tenant.
	TenantHeader string
	// TrustedProxies are the CIDRs or bare IPs allowed to assert the headers;
	// empty trusts every peer.
	TrustedProxies []string
}

// routerCfg maps the config file section onto the router's auth config. The
// loader already validated TrustedProxies, so the error only guards a config
// that bypassed getAppCfg. (The parsed prefixes cannot be cached on the struct:
// go-bumbu/config walks every field, unexported ones included.)
func (c authCfg) routerCfg() (router.AuthCfg, error) {
	trusted, err := parseTrustedProxies(c.ProxyHeader.TrustedProxies)
	if err != nil {
		return router.AuthCfg{}, err
	}
	return router.AuthCfg{
		Method: c.Method,
		Proxy: router.ProxyAuthCfg{
			UserHeader:     c.ProxyHeader.UserHeader,
			TenantHeader:   c.ProxyHeader.TenantHeader,
			TrustedProxies: trusted,
		},
	}, nil
}

// dbCfg selects the database: Type "sqlite" (default, a local file) or
// "postgres". Only the section matching Type is used.
type dbCfg struct {
	Type     string
	Sqlite   sqliteCfg
	Postgres postgresCfg
}

type sqliteCfg struct {
	// Path of the database file; relative paths are resolved against the
	// working directory at startup.
	Path string
}

type postgresCfg struct {
	Host     string
	Port     int
	User     string
	Password string // prefer the env var REFAPP_DB_POSTGRES_PASSWORD over the file
	DbName   string
	SslMode  string // disable, require (default), verify-full, ...
}

// gormCfg maps the config file section onto the gormdb library's config, so the
// file format stays ours even if the library changes.
func (c dbCfg) gormCfg() gormdb.Cfg {
	return gormdb.Cfg{
		Driver: c.Type,
		Sqlite: gormdb.SqliteCfg{Path: c.Sqlite.Path},
		Postgres: gormdb.PostgresCfg{
			Host:     c.Postgres.Host,
			Port:     c.Postgres.Port,
			User:     c.Postgres.User,
			Password: c.Postgres.Password,
			DbName:   c.Postgres.DbName,
			SslMode:  c.Postgres.SslMode,
		},
	}
}

func listenAddr(bindIP string, port int) string {
	if bindIP == "" {
		return ":" + strconv.Itoa(port)
	}
	return bindIP + ":" + strconv.Itoa(port)
}

type Msg struct {
	Level string
	Msg   string
}

// EnvVarPrefix is the prefix of the env vars that override config values,
// use like: REFAPP_ENV_LOGLEVEL=debug
const EnvVarPrefix = "REFAPP" // TODO replace the env prefix

// configSearchPaths is where the config file is looked up when no -c flag is
// given, in order of precedence: the working directory (development), then the
// packaged location (e.g. a deb + systemd unit). Missing files are skipped; if
// none exist the built-in defaults apply.
var configSearchPaths = []string{
	"./config.yaml",
	"/etc/" + appName + "/config.yaml",
}

// resolveConfigFile decides which config file to load. An explicit flag value
// is mandatory — a typo must fail loudly rather than silently fall back to
// defaults. Without a flag the search paths are probed and the first existing
// one wins; an empty return means "no config file, use defaults only".
func resolveConfigFile(flagValue string, searchPaths []string) (path string, mandatory bool) {
	if flagValue != "" {
		return flagValue, true
	}
	for _, p := range searchPaths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, false
		}
	}
	return "", false
}

// defaultCfg represents the basic set of sensible defaults
var defaultCfg = AppCfg{
	Server: serverCfg{
		BindIp: "",
		Port:   8080,
	},
	Obs: obsCfg{
		Enabled: false,
		BindIp:  "",
		Port:    9090,
	},
	DB: dbCfg{
		Type: gormdb.DriverSqlite,
		// in a directory of its own: SQLite keeps -wal and -shm files next to the db
		Sqlite: sqliteCfg{Path: "data/sqlite/" + appName + ".db"},
		Postgres: postgresCfg{
			Port:    5432,
			SslMode: "require",
		},
	},
	Auth: authCfg{
		Method:      router.AuthNone,
		ProxyHeader: proxyHeaderCfg{UserHeader: "Remote-User"},
	},
	Env: Env{
		LogLevel: "info",
	},
}

// getAppCfg loads the configuration, in increasing order of precedence:
// defaults → .env file → config file → env vars.
// An empty file means "defaults + env only"; mandatory makes a missing file an
// error — used when the path came from -c.
//
// Validation and normalization of the loaded values (absolute paths, trimming
// secrets read from "@<path>" files, cross-field rules) belongs here too, so
// runServer only ever sees a valid config.
func getAppCfg(file string, mandatory bool) (AppCfg, error) {
	configMsg := []Msg{}
	cfg := AppCfg{}
	opts := []any{
		config.Defaults{Item: defaultCfg},              // load default config
		config.EnvFile{Path: ".env", Mandatory: false}, // optional .env in the working directory
	}
	if file != "" {
		opts = append(opts, config.CfgFile{Path: file, Mandatory: mandatory})
	}
	opts = append(opts,
		config.EnvVar{Prefix: EnvVarPrefix}, // load config using ENVs with prefix
		config.Unmarshal{Item: &cfg},        // unmarshal config into cfg struct
		config.Writer{Fn: func(level, msg string) { // store messages about config loading, the logger does not exist yet
			if level == config.InfoLevel {
				configMsg = append(configMsg, Msg{Level: "info", Msg: msg})
			}
			if level == config.DebugLevel {
				configMsg = append(configMsg, Msg{Level: "debug", Msg: msg})
			}
		}},
	)
	_, err := config.Load(opts...)
	cfg.Msgs = configMsg
	if err != nil {
		return cfg, fmt.Errorf("load config: %w", err)
	}
	if err := normalizeDBCfg(&cfg.DB); err != nil {
		return cfg, fmt.Errorf("config DB: %w", err)
	}
	if err := normalizeAuthCfg(&cfg.Auth); err != nil {
		return cfg, fmt.Errorf("config Auth: %w", err)
	}
	return cfg, nil
}

// normalizeAuthCfg lower-cases the method, rejects an unknown one, and for
// "proxy" trims the header names, requires a UserHeader and parses TrustedProxies.
func normalizeAuthCfg(c *authCfg) error {
	c.Method = strings.ToLower(strings.TrimSpace(c.Method))
	if c.Method == "" {
		c.Method = router.AuthNone
	}
	if !slices.Contains(router.AuthMethods, c.Method) {
		return fmt.Errorf("unknown method %q: must be one of %s", c.Method, strings.Join(router.AuthMethods, ", "))
	}
	if c.Method != router.AuthProxy {
		return nil
	}
	ph := &c.ProxyHeader
	ph.UserHeader = strings.TrimSpace(ph.UserHeader)
	ph.TenantHeader = strings.TrimSpace(ph.TenantHeader)
	if ph.UserHeader == "" {
		return fmt.Errorf("method %q requires ProxyHeader.UserHeader", router.AuthProxy)
	}
	_, err := parseTrustedProxies(ph.TrustedProxies)
	return err
}

// parseTrustedProxies parses CIDRs; a bare IP is a single-address prefix
// (10.0.0.5 == 10.0.0.5/32).
func parseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.Contains(e, "/") {
			addr, err := netip.ParseAddr(e)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy %q: %w", e, err)
			}
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(e)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", e, err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// normalizeDBCfg makes the sqlite path absolute, so a later chdir cannot move
// the database, and rejects a config gormdb.Open could not work with.
func normalizeDBCfg(c *dbCfg) error {
	if c.Type == gormdb.DriverSqlite && c.Sqlite.Path != "" {
		abs, err := filepath.Abs(c.Sqlite.Path)
		if err != nil {
			return fmt.Errorf("resolve sqlite path: %w", err)
		}
		c.Sqlite.Path = abs
	}
	return c.gormCfg().Validate()
}
