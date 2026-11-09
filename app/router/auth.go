package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/app/reqctx"
	"github.com/go-bumbu/reference-app/internal/tenants"
	"github.com/go-bumbu/userauth/auth/headerauth"
)

// Auth methods. Authentication (who the caller is) is delegated: the app never
// handles passwords or sessions. It only maps the asserted identity onto a
// tenant and a user, and isolates their data.
const (
	// AuthNone puts every request in DefaultTenant as DefaultUser. For demos and
	// local development only.
	AuthNone = "none"
	// AuthProxy trusts the identity headers of a reverse proxy that
	// authenticated the caller (Authelia, oauth2-proxy, ...).
	AuthProxy = "proxy"
	// AuthOIDC is reserved for OpenID Connect; it is not implemented yet and
	// router.New refuses it.
	AuthOIDC = "oidc"
)

// AuthMethods lists the accepted values of AuthCfg.Method.
var AuthMethods = []string{AuthNone, AuthProxy, AuthOIDC}

// ErrOIDCNotImplemented is returned by New for AuthOIDC.
var ErrOIDCNotImplemented = errors.New("auth method \"oidc\" is not implemented yet")

const (
	// DefaultTenant is the tenant of AuthNone, and of AuthProxy without a TenantHeader.
	DefaultTenant = "default"
	// DefaultUser is the user of AuthNone.
	DefaultUser = "admin"
)

// AuthCfg selects how requests are authenticated.
type AuthCfg struct {
	// Method is one of AuthMethods; empty means AuthNone.
	Method string
	// Proxy configures AuthProxy.
	Proxy ProxyAuthCfg
}

// ProxyAuthCfg configures AuthProxy. The deployment must guarantee the app is
// reachable only through the proxy, and that the proxy strips these headers
// from client requests; TrustedProxies is a second line of defense.
type ProxyAuthCfg struct {
	// UserHeader carries the authenticated user (Authelia: Remote-User,
	// oauth2-proxy: X-Forwarded-User); required.
	UserHeader string
	// TenantHeader carries the caller's tenant. Empty puts every user in
	// DefaultTenant; when set, a request without it is refused.
	TenantHeader string
	// TrustedProxies are the peers allowed to assert the headers; empty trusts
	// every peer.
	TrustedProxies []netip.Prefix
}

// TenantProvisioner resolves an asserted identity into tenant and user IDs,
// creating them on first sight (internal/tenants).
type TenantProvisioner interface {
	Provision(ctx context.Context, tenantSlug, subject string) (tenants.Principal, error)
}

// assertFunc extracts the identity the auth method vouches for; ok is false
// when the request is not authenticated.
type assertFunc func(w http.ResponseWriter, r *http.Request) (tenantSlug, subject string, ok bool)

// authGuard authenticates a request with the configured method, provisions the
// asserted identity and stores the result on the request context.
type authGuard struct {
	assert   assertFunc
	tenants  TenantProvisioner
	problems *problemjson.Writer
	logger   *slog.Logger
}

func newAuthGuard(cfg AuthCfg, provisioner TenantProvisioner, problems *problemjson.Writer, logger *slog.Logger) (*authGuard, error) {
	g := &authGuard{tenants: provisioner, problems: problems, logger: logger}
	switch cfg.Method {
	case AuthNone, "":
		g.assert = func(http.ResponseWriter, *http.Request) (string, string, bool) {
			return DefaultTenant, DefaultUser, true
		}
	case AuthProxy:
		assert, err := proxyAssert(cfg.Proxy, logger)
		if err != nil {
			return nil, err
		}
		g.assert = assert
	case AuthOIDC:
		return nil, ErrOIDCNotImplemented
	default:
		return nil, fmt.Errorf("unknown auth method %q: must be one of %s", cfg.Method, strings.Join(AuthMethods, ", "))
	}
	return g, nil
}

// proxyAssert reads the identity from the trusted proxy's headers, via
// go-bumbu/userauth's headerauth (which enforces TrustedProxies).
func proxyAssert(cfg ProxyAuthCfg, logger *slog.Logger) (assertFunc, error) {
	if cfg.UserHeader == "" {
		return nil, errors.New("auth method \"proxy\" requires a user header")
	}
	ha := headerauth.New(headerauth.Cfg{
		UserHeader:     cfg.UserHeader,
		TrustedProxies: cfg.TrustedProxies,
		Logger:         logger,
	})
	tenantHeader := cfg.TenantHeader
	return func(w http.ResponseWriter, r *http.Request) (string, string, bool) {
		if ok, _ := ha.HandleAuth(w, r); !ok {
			return "", "", false
		}
		data, err := headerauth.CtxGetRequestData(r)
		if err != nil {
			return "", "", false
		}
		if tenantHeader == "" {
			return DefaultTenant, data.UserName, true
		}
		// fail closed: a proxy configured to send the tenant that did not send it
		// must not drop the user into the default tenant
		tenant := strings.TrimSpace(r.Header.Get(tenantHeader))
		if tenant == "" {
			return "", "", false
		}
		return tenant, data.UserName, true
	}, nil
}

// protect wraps a handler that needs an authenticated caller.
func (g *authGuard) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantSlug, subject, ok := g.assert(w, r)
		if !ok {
			g.problems.Write(w, r, http.StatusUnauthorized, problemjson.SlugUnauthorized, "authentication required")
			return
		}
		p, err := g.tenants.Provision(r.Context(), tenantSlug, subject)
		if err != nil {
			var verr *tenants.ValidationError
			if errors.As(err, &verr) {
				// the authenticator vouched for an identity the app cannot hold
				g.problems.Write(w, r, http.StatusUnauthorized, problemjson.SlugUnauthorized, verr.Error())
				return
			}
			g.logger.Error("provision identity", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
			g.problems.Write(w, r, http.StatusInternalServerError, problemjson.SlugInternal, "unexpected error")
			return
		}
		ctx := reqctx.WithIdentity(r.Context(), reqctx.Identity{
			TenantID: p.TenantID, TenantSlug: p.TenantSlug, UserID: p.UserID, Subject: p.Subject,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
