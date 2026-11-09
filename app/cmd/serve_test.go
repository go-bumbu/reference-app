package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// freeAddr reserves a free local port and releases it, serveHTTP listens on srv.Addr itself.
func freeAddr(t *testing.T) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := probe.Addr().String()
	if cerr := probe.Close(); cerr != nil {
		t.Fatalf("release port: %v", cerr)
	}
	return addr
}

// TestServeHTTPForceClosesStuckHandler asserts serveHTTP returns even when a
// handler outlives the shutdown grace period.
//
// http.Server.Shutdown waits for in-flight handlers but never cancels their
// request context, so a slow handler is unaffected by it. Without the
// srv.Close() that follows an expired grace period, serveHTTP blocks for as long
// as the handler runs and Ctrl-C appears to hang.
func TestServeHTTPForceClosesStuckHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the full shutdown grace period")
	}
	// Long enough that a serveHTTP which waits for it fails the test rather than
	// passing slowly.
	const handlerBlock = serverShutdownTimeout + 20*time.Second

	entered := make(chan struct{})
	handlerCtxErr := make(chan error, 1)
	srv := &http.Server{
		Addr: freeAddr(t),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			select {
			case <-r.Context().Done():
				handlerCtxErr <- r.Context().Err()
			case <-time.After(handlerBlock):
				handlerCtxErr <- nil
			}
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	served := make(chan error, 1)
	go func() {
		served <- serveHTTP(ctx, srv, silentLogger(), "test")
	}()

	// A raw connection, so the request stays in-flight while the client waits
	// rather than being torn down by a client-side timeout.
	conn, err := dialWithRetry(t, srv.Addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := fmt.Fprint(conn, "GET /slow HTTP/1.1\r\nHost: test\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never ran")
	}

	cancel()

	// allow margin for the grace period to expire and Close to land.
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serveHTTP returned error: %v", err)
		}
	case <-time.After(handlerBlock - 5*time.Second):
		t.Fatal("serveHTTP did not return after the shutdown grace period expired")
	}

	// The force-close is what cancels the stuck handler's context.
	select {
	case err := <-handlerCtxErr:
		if err == nil {
			t.Fatal("handler ran to completion; its context was never canceled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler context was not canceled")
	}
}

// TestServeHTTPListenError asserts a bind failure is returned to the caller
// instead of panicking in a goroutine.
func TestServeHTTPListenError(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = busy.Close() }()

	srv := &http.Server{Addr: busy.Addr().String(), ReadHeaderTimeout: time.Second}
	err = serveHTTP(context.Background(), srv, silentLogger(), "test")
	if err == nil {
		t.Fatal("expected a listen error for a port in use")
	}
}

// TestServeWithGracefulShutdownStopsAllOnFailure asserts that one failing server
// stops the others and runs the shutdown hook, and that its error is returned.
func TestServeWithGracefulShutdownStopsAllOnFailure(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = busy.Close() }()

	servers := []namedServer{
		{name: "ok", srv: &http.Server{Addr: freeAddr(t), Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}},
		{name: "busy", srv: &http.Server{Addr: busy.Addr().String(), ReadHeaderTimeout: time.Second}},
	}
	hookCalled := false
	done := make(chan error, 1)
	go func() {
		done <- serveWithGracefulShutdown(silentLogger(), servers, func(context.Context) error {
			hookCalled = true
			return nil
		})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the listen error of the busy server")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveWithGracefulShutdown did not return after a server failed")
	}
	if !hookCalled {
		t.Error("shutdown hook was not called")
	}
}

// dialWithRetry waits for serveHTTP's listener to come up.
func dialWithRetry(t *testing.T, addr string) (net.Conn, error) {
	t.Helper()
	var lastErr error
	for range 50 {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	return nil, lastErr
}
