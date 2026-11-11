package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-bumbu/db/gormdb"
	"github.com/go-bumbu/db/gormdb/postgres"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"github.com/go-bumbu/reference-app/app/logger"
	"github.com/go-bumbu/reference-app/app/metainfo"
	"github.com/go-bumbu/reference-app/app/router"
	"github.com/go-bumbu/reference-app/app/spa"
	"github.com/go-bumbu/reference-app/internal/jobs"
	"github.com/go-bumbu/reference-app/internal/notes"
	"github.com/go-bumbu/reference-app/internal/tenants"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func serverCmd() *cobra.Command {
	var configFile string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start the web server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(configFile)
		},
	}
	cmd.Flags().StringVarP(&configFile, "config", "c", "",
		fmt.Sprintf("config file; default: %s then %s", configSearchPaths[0], configSearchPaths[1]))
	return cmd
}

// runServer is the composition root: it loads the config, instantiates the
// shared resources (logger, db, ...), wires them into the handlers and hands
// the servers to serveWithGracefulShutdown. Every step returns an error instead
// of panicking, so a failure ends the command with a clear message.
func runServer(configFile string) (err error) {
	path, mandatory := resolveConfigFile(configFile, configSearchPaths)
	cfg, err := getAppCfg(path, mandatory)
	if err != nil {
		return err
	}

	// setup the logger
	l, err := logger.GetDefault(logger.GetLogLevel(cfg.Env.LogLevel))
	if err != nil {
		return err
	}

	l.Info("App startup",
		slog.String("component", "startup"),
		slog.String("version", metainfo.Version),
		slog.String("Build Date", metainfo.BuildTime),
		slog.String("commit", metainfo.ShaVer),
	)
	// print config messages delayed, the logger did not exist while loading the config
	for _, m := range cfg.Msgs {
		if m.Level == "info" {
			l.Info(m.Msg, slog.String("component", "config"))
		} else {
			l.Debug(m.Msg, slog.String("component", "config"))
		}
	}

	// instantiate shared resources here (db, stores, business modules, background workers)
	// and pass the references to the router.
	db, err := openDB(cfg.DB, l)
	if err != nil {
		return err
	}
	// closed on every return: after a failed startup step as well as after shutdown
	defer func() {
		if cerr := gormdb.Close(db); cerr != nil {
			l.Error("close database", slog.String("component", "db"), slog.String("error", cerr.Error()))
			err = errors.Join(err, cerr)
		}
	}()

	mods, err := newModules(cfg, db, l)
	if err != nil {
		return err
	}
	mainHandler, err := newMainHandler(cfg, mods, l)
	if err != nil {
		return err
	}

	servers := []namedServer{{
		name: "main",
		srv: &http.Server{
			Addr:              cfg.Server.Addr(),
			Handler:           mainHandler,
			ReadHeaderTimeout: 5 * time.Second, // gosec G112: Potential Slowloris Attack because ReadHeaderTimeout
		},
	}}

	// The observability server (metrics, health, ready) is opt-in.
	if cfg.Obs.Enabled {
		servers = append(servers, namedServer{
			name: "observability",
			srv: &http.Server{
				Addr:              cfg.Obs.Addr(),
				Handler:           router.Observability(),
				ReadHeaderTimeout: 5 * time.Second,
			},
		})
	} else {
		l.Info("observability server disabled", slog.String("component", "observability"))
	}

	// Background workers start last, once nothing else can fail before serving,
	// and stop in the shutdown hook, which runs on every exit of the servers.
	// The db is closed by the deferred call above, after everything that uses it stopped.
	if err := mods.jobs.Start(context.Background()); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), appShutdownTimeout)
		defer cancel()
		return errors.Join(fmt.Errorf("start jobs: %w", err), mods.jobs.ShutDown(stopCtx))
	}
	l.Info("jobs started", slog.String("component", "jobs"), slog.String("logDir", cfg.Jobs.LogDir))
	shutdown := func(ctx context.Context) error {
		// the scheduler stops first (no new fires), then the runner drains
		return mods.jobs.ShutDown(ctx)
	}

	return serveWithGracefulShutdown(l, servers, shutdown)
}

// modules are the business modules, built once on the shared db.
type modules struct {
	tenants *tenants.Service
	notes   *notes.Service
	jobs    *jobs.Service
}

// newModules instantiates the business modules on db; each migrates its own
// tables. A module that needs another gets it here, through its interface.
func newModules(cfg AppCfg, db *gorm.DB, l *slog.Logger) (modules, error) {
	tenantsSvc, err := tenants.New(context.Background(), db, tenants.Cfg{})
	if err != nil {
		return modules{}, fmt.Errorf("init tenants: %w", err)
	}
	notesSvc, err := notes.New(context.Background(), db, notes.Cfg{})
	if err != nil {
		return modules{}, fmt.Errorf("init notes: %w", err)
	}
	jobsSvc, err := jobs.New(db, jobs.Cfg{
		Notes:       notesSvc,
		LogDir:      cfg.Jobs.LogDir,
		Parallelism: cfg.Jobs.Parallelism,
		QueueSize:   cfg.Jobs.QueueSize,
		HistorySize: cfg.Jobs.HistorySize,
		Logger:      l,
	})
	if err != nil {
		return modules{}, fmt.Errorf("init jobs: %w", err)
	}
	return modules{tenants: tenantsSvc, notes: notesSvc, jobs: jobsSvc}, nil
}

// newMainHandler wires the modules, the auth method and the web UI into the
// main handler.
func newMainHandler(cfg AppCfg, mods modules, l *slog.Logger) (http.Handler, error) {
	authCfg, err := cfg.Auth.routerCfg()
	if err != nil {
		return nil, fmt.Errorf("config Auth: %w", err)
	}
	logAuthMode(l, authCfg)
	ui, err := spa.Handler()
	if err != nil {
		return nil, fmt.Errorf("init web UI: %w", err)
	}
	h, err := router.New(router.Cfg{
		Logger:     l,
		Notes:      mods.notes,
		Tags:       mods.notes,
		Jobs:       mods.jobs,
		Tenants:    mods.tenants,
		Auth:       authCfg,
		UI:         ui,
		Production: cfg.Env.Production,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize main app handler: %w", err)
	}
	return h, nil
}

// logAuthMode reports the auth method, and warns about the setups that trust
// more than they should outside a demo.
func logAuthMode(l *slog.Logger, c router.AuthCfg) {
	attrs := []any{slog.String("component", "auth"), slog.String("method", c.Method)}
	switch c.Method {
	case router.AuthNone:
		l.Warn("auth method none: every request acts as user "+router.DefaultUser+
			" of tenant "+router.DefaultTenant+"; use it for demos only", attrs...)
	case router.AuthProxy:
		attrs = append(attrs, slog.String("userHeader", c.Proxy.UserHeader),
			slog.String("tenantHeader", c.Proxy.TenantHeader))
		if len(c.Proxy.TrustedProxies) == 0 {
			l.Warn("auth method proxy without TrustedProxies: every peer may assert identity headers", attrs...)
			return
		}
		l.Info("auth method proxy", attrs...)
	default:
		l.Info("auth method "+c.Method, attrs...)
	}
}

// openDB connects to the configured database, logging GORM's output through
// the app logger.
func openDB(cfg dbCfg, l *slog.Logger) (*gorm.DB, error) {
	gcfg := cfg.gormCfg()
	gcfg.Logger = gormlogger.NewSlogLogger(
		l.With(slog.String("component", "gorm")),
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			IgnoreRecordNotFoundError: true, // a miss is a normal outcome, mapped to ErrNotFound
			LogLevel:                  gormlogger.Warn,
		},
	)
	db, err := gormdb.Open(gcfg, gormdb.Drivers{Sqlite: sqlite.Driver{}, Postgres: postgres.Driver{}})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	where := cfg.Sqlite.Path
	if cfg.Type == gormdb.DriverPostgres {
		where = cfg.Postgres.Host + "/" + cfg.Postgres.DbName // never the DSN: it holds the password
	}
	l.Info("database ready", slog.String("component", "db"), slog.String("type", cfg.Type), slog.String("location", where))
	return db, nil
}
