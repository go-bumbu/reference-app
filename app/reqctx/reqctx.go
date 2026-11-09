// Package reqctx carries the authenticated caller on the request context. It
// lives apart from the router so both the auth middleware (app/router) and the
// handlers (app/router/handlers) can use it without an import cycle.
package reqctx

import "context"

// Identity is the caller the auth guard resolved: the tenant and user IDs that
// every tenant-scoped module filters on, and the slug and subject they were
// resolved from (for display only, never as a scope key).
type Identity struct {
	TenantID   string
	TenantSlug string
	UserID     string
	Subject    string
}

type identityKey struct{}

// WithIdentity returns a copy of ctx carrying id.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFrom returns the Identity the auth guard stored. ok is false on a
// route the guard does not protect (e.g. /info).
func IdentityFrom(ctx context.Context) (id Identity, ok bool) {
	id, ok = ctx.Value(identityKey{}).(Identity)
	return id, ok
}
