package reqctx

import (
	"context"

	"github.com/go-bumbu/reference-app/libs/keyscope"
)

// Key is the API key a request was authenticated with: its id and name, and
// what it grants. A request carries one only on the key-only API mount; there
// the Identity is the key's owner.
type Key struct {
	ID    string
	Name  string
	Grant keyscope.Grant
}

type keyKey struct{}

// WithKey returns a copy of ctx carrying k.
func WithKey(ctx context.Context, k Key) context.Context {
	return context.WithValue(ctx, keyKey{}, k)
}

// KeyFrom returns the API key the request was made with; ok is false for a
// request a signed-in user made through the proxy.
func KeyFrom(ctx context.Context) (k Key, ok bool) {
	k, ok = ctx.Value(keyKey{}).(Key)
	return k, ok
}
