package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	// serverShutdownTimeout is the grace period for in-flight requests of every server.
	serverShutdownTimeout = 10 * time.Second
	// appShutdownTimeout bounds the shutdown hook (background workers, db, ...).
	appShutdownTimeout = 30 * time.Second
)

type namedServer struct {
	name string
	srv  *http.Server
}

// serveWithGracefulShutdown runs all servers until SIGINT/SIGTERM or until one of
// them fails (e.g. the port is already in use), then shuts every server down and
// runs the shutdown hook within a bounded window.
//
// The servers share an errgroup context: a failing server cancels it, which
// stops the others too, and its error is returned to the caller.
func serveWithGracefulShutdown(l *slog.Logger, servers []namedServer, shutdown func(ctx context.Context) error) error {
	rootCtx, rootCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer rootCancel()

	g, gctx := errgroup.WithContext(rootCtx)
	for _, s := range servers {
		g.Go(func() error { return serveHTTP(gctx, s.srv, l, s.name) })
	}
	g.Go(func() error {
		<-gctx.Done()
		if rootCtx.Err() != nil {
			l.Info("shutdown signal received", slog.String("component", "shutdown"))
		}
		// restore the default signal handling: a second Ctrl-C kills the process
		// instead of waiting for the graceful shutdown to finish.
		rootCancel()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), appShutdownTimeout)
		defer cancel()
		return shutdown(shutdownCtx)
	})

	err := g.Wait()
	if err != nil {
		return err
	}
	l.Info("shutdown complete", slog.String("component", "shutdown"))
	return nil
}

// serveHTTP binds the listener first, so a bind error (port in use, bad address)
// is returned synchronously instead of being lost in a goroutine. It serves until
// ctx is canceled and then shuts the server down.
func serveHTTP(ctx context.Context, srv *http.Server, l *slog.Logger, name string) error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("%s server listen: %w", name, err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	l.Info(name+" server started", slog.String("component", name), slog.String("addr", ln.Addr().String()))

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		// Serve failed on its own, before any shutdown was requested.
		return fmt.Errorf("%s server: %w", name, err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()
	// Shutdown drains idle connections but only *waits* for in-flight handlers:
	// it never cancels their request context, so a slow handler runs to completion
	// regardless of the grace period. Close drops the remaining connections, which
	// is what actually cancels the request context and lets the process exit.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		l.Warn(name+" server shutdown error, closing remaining connections",
			slog.String("component", name), slog.String("error", err.Error()))
		if cerr := srv.Close(); cerr != nil {
			l.Warn(name+" server close error", slog.String("component", name), slog.String("error", cerr.Error()))
		}
	}
	l.Info(name+" server stopped", slog.String("component", name))
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
