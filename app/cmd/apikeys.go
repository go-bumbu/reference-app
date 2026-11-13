package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-bumbu/reference-app/app/router"
	"github.com/go-bumbu/reference-app/internal/tenants"
	"github.com/go-bumbu/userauth"
	"github.com/go-bumbu/userauth/service/pat"
	patdb "github.com/go-bumbu/userauth/service/pat/store/db"
	"gorm.io/gorm"
)

// newAPIKeys builds the service that mints and verifies API keys: userauth's
// personal access tokens, kept hashed in the app's database (table user_pats)
// and prefixed with the app name, so a leaked key is recognisable. Keys exist
// only behind the auth proxy: for any other auth method it returns nil, the
// router then serves no key management and accepts no key.
func newAPIKeys(cfg AppCfg, db *gorm.DB, owners *tenants.Service, l *slog.Logger) (*pat.Service, error) {
	if cfg.Auth.Method != router.AuthProxy {
		return nil, nil
	}
	store, err := patdb.New(db)
	if err != nil {
		return nil, fmt.Errorf("init api keys store: %w", err)
	}
	svc, err := pat.NewService(store, keyOwners{tenants: owners}, pat.Opts{Prefix: appName, Logger: l})
	if err != nil {
		return nil, fmt.Errorf("init api keys: %w", err)
	}
	return svc, nil
}

// keyOwners answers the token service's questions about a key's owner: a user
// of internal/tenants, found by its ID. The app has no logins, so a lookup by
// login never finds anyone, and every user is enabled: disabling a user is the
// identity provider's business (which is why keys expire by default).
type keyOwners struct {
	tenants *tenants.Service
}

// GetUser finds the user whose ID owns a key.
func (o keyOwners) GetUser(id string) (userauth.User, error) {
	// pat's lookup carries no context; the request's own lookup of the owner
	// (router.KeyOwners) does.
	p, err := o.tenants.PrincipalByUserID(context.Background(), id)
	if errors.Is(err, tenants.ErrUserNotFound) {
		return userauth.User{}, userauth.ErrUserNotFound
	}
	if err != nil {
		return userauth.User{}, err
	}
	return userauth.User{ID: p.UserID, LoginID: p.Subject, Enabled: true}, nil
}

// GetUserByLogin never finds anyone; pat asks for it only for the user+token
// credential, which the app does not offer.
func (o keyOwners) GetUserByLogin(string) (userauth.User, error) {
	return userauth.User{}, userauth.ErrUserNotFound
}
