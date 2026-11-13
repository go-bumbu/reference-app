package cmd

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/go-bumbu/db/gormdb/gormdbtest"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"github.com/go-bumbu/reference-app/app/router"
	"github.com/go-bumbu/reference-app/internal/tenants"
	"github.com/go-bumbu/userauth"
	"github.com/go-bumbu/userauth/service/pat"
)

func TestNewAPIKeysOnlyBehindTheProxy(t *testing.T) {
	db := gormdbtest.Sqlite(t, sqlite.Driver{})
	ts, err := tenants.New(context.Background(), db, tenants.Cfg{})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"", router.AuthNone} {
		var cfg AppCfg
		cfg.Auth.Method = method
		svc, err := newAPIKeys(cfg, db, ts, slog.New(slog.DiscardHandler))
		if err != nil || svc != nil {
			t.Errorf("method %q: got %v, %v; want no key service", method, svc, err)
		}
	}

	var cfg AppCfg
	cfg.Auth.Method = router.AuthProxy
	svc, err := newAPIKeys(cfg, db, ts, slog.New(slog.DiscardHandler))
	if err != nil || svc == nil {
		t.Fatalf("proxy: got %v, %v", svc, err)
	}
	p, err := ts.Provision(context.Background(), "acme", "alice")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.Mint(p.UserID, "k", []string{"notes:read"}, nil, pat.HashOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, appName+"_") {
		t.Errorf("token %q does not start with the app name", token)
	}
	info, ok, err := svc.Verify(token)
	if err != nil || !ok || info.UserID != p.UserID || info.LoginID != "alice" {
		t.Errorf("verify: %+v %v %v", info, ok, err)
	}
}

func TestKeyOwners(t *testing.T) {
	db := gormdbtest.Sqlite(t, sqlite.Driver{})
	ts, err := tenants.New(context.Background(), db, tenants.Cfg{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := ts.Provision(context.Background(), "acme", "alice")
	if err != nil {
		t.Fatal(err)
	}
	o := keyOwners{tenants: ts}
	u, err := o.GetUser(p.UserID)
	if err != nil || u.ID != p.UserID || u.LoginID != "alice" || !u.Enabled {
		t.Errorf("GetUser: %+v, %v", u, err)
	}
	if _, err := o.GetUser("0000000000000000"); !errors.Is(err, userauth.ErrUserNotFound) {
		t.Errorf("missing user: got %v", err)
	}
	if _, err := o.GetUserByLogin("alice"); !errors.Is(err, userauth.ErrUserNotFound) {
		t.Errorf("GetUserByLogin: got %v", err)
	}
}
