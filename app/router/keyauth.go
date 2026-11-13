package router

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/app/reqctx"
	"github.com/go-bumbu/reference-app/app/router/handlers"
	"github.com/go-bumbu/reference-app/internal/tenants"
	"github.com/go-bumbu/reference-app/libs/keyscope"
	"github.com/go-bumbu/userauth/service/pat"
)

// APIKeys is the token service the API keys live in (go-bumbu/userauth's
// pat.Service): the handler manages a user's keys with it, and the key guard
// verifies the key a request presents.
type APIKeys interface {
	handlers.APIKeysService
	Verify(presented string) (pat.TokenInfo, bool, error)
}

// KeyOwners finds the owner of an API key: an existing user, by its ID, with
// its tenant (internal/tenants). It never provisions.
type KeyOwners interface {
	PrincipalByUserID(ctx context.Context, userID string) (tenants.Principal, error)
}

// msgKeyInvalid is the one answer to every bad key, so a caller learns nothing
// about which part of it was wrong.
const msgKeyInvalid = "a valid API key is required"

// keyGuard authenticates the requests of the key-only API mount by the API key
// in their Authorization header, and applies each route's key rule. It never
// reads the proxy's identity headers: the proxy lets this mount through without
// a login, so anything such a header says on it is the client's own claim.
type keyGuard struct {
	keys     APIKeys
	owners   KeyOwners
	scopes   *keyscope.Registry
	problems *problemjson.Writer
	logger   *slog.Logger
}

// protect wraps a handler of the key-only mount: the key must be valid (else
// 401), and must grant what rule asks for the request's method (else 403).
// The handler then sees the key's owner as the Identity, and the key itself.
func (g *keyGuard) protect(rule keyscope.Rule, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, owner, ok := g.authenticate(w, r)
		if !ok {
			return
		}
		if err := keyscope.Check(rule, key.Grant, r.Method); err != nil {
			g.problems.Write(w, r, http.StatusForbidden, problemjson.SlugForbidden, forbiddenDetail(err))
			return
		}
		ctx := reqctx.WithIdentity(r.Context(), reqctx.Identity{
			TenantID: owner.TenantID, TenantSlug: owner.TenantSlug, UserID: owner.UserID, Subject: owner.Subject,
		})
		next.ServeHTTP(w, r.WithContext(reqctx.WithKey(ctx, key)))
	})
}

// authenticate resolves the request's key and its owner. On failure it has
// answered: 401 for every way a key can be bad (missing, malformed, unknown,
// wrong secret, expired, revoked, a scope the app no longer knows, its owner
// gone), 500 for a failed lookup.
func (g *keyGuard) authenticate(w http.ResponseWriter, r *http.Request) (reqctx.Key, tenants.Principal, bool) {
	token, ok := bearerToken(r)
	if !ok {
		g.unauthorized(w, r)
		return reqctx.Key{}, tenants.Principal{}, false
	}
	info, ok, err := g.keys.Verify(token)
	if err != nil {
		g.internal(w, r, "verify API key", err)
		return reqctx.Key{}, tenants.Principal{}, false
	}
	if !ok {
		g.unauthorized(w, r)
		return reqctx.Key{}, tenants.Principal{}, false
	}
	grant, err := g.scopes.Parse(info.Scopes)
	if err != nil {
		g.unauthorized(w, r)
		return reqctx.Key{}, tenants.Principal{}, false
	}
	owner, err := g.owners.PrincipalByUserID(r.Context(), info.UserID)
	if errors.Is(err, tenants.ErrUserNotFound) {
		g.unauthorized(w, r)
		return reqctx.Key{}, tenants.Principal{}, false
	}
	if err != nil {
		g.internal(w, r, "resolve API key owner", err)
		return reqctx.Key{}, tenants.Principal{}, false
	}
	return reqctx.Key{ID: info.TokenID, Name: info.Name, Grant: grant}, owner, true
}

// bearerToken is the token of an "Authorization: Bearer <token>" header; the
// scheme is case-insensitive (RFC 9110).
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

// forbiddenDetail says what the key cannot do.
func forbiddenDetail(err error) string {
	var denied *keyscope.DeniedError
	if errors.As(err, &denied) {
		if denied.Have == keyscope.None {
			return "this API key has no access to " + denied.Area
		}
		return "this API key is read-only for " + denied.Area
	}
	return "API keys can't use this endpoint"
}

func (g *keyGuard) unauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
	g.problems.Write(w, r, http.StatusUnauthorized, problemjson.SlugUnauthorized, msgKeyInvalid)
}

func (g *keyGuard) internal(w http.ResponseWriter, r *http.Request, what string, err error) {
	g.logger.Error(what, slog.String("path", r.URL.Path), slog.String("error", err.Error()))
	g.problems.Write(w, r, http.StatusInternalServerError, problemjson.SlugInternal, "unexpected error")
}
