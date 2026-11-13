package client

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// APIKey is one of the caller's API keys. Usable is false for a key whose
// scopes the server no longer knows: it authenticates nothing.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	Usable     bool       `json:"usable"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
}

// CreatedAPIKey is a key just created, with its token: the only time the
// server shows it.
type CreatedAPIKey struct {
	APIKey
	Token string `json:"token"`
}

// APIKeyInput holds the fields of a new key. Scopes are "<area>:read" or
// "<area>:write"; a nil ExpiresAt means one year from now.
type APIKeyInput struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// APIKeyScope is an area a key can be granted, and its levels.
type APIKeyScope struct {
	Area   string   `json:"area"`
	Levels []string `json:"levels"`
}

// APIKeys returns the caller's API keys, oldest first. Like every API key
// operation it needs a signed-in user: with an API key it is ErrForbidden.
func (c *Client) APIKeys(ctx context.Context) ([]APIKey, error) {
	var out struct {
		Items []APIKey `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/api-keys", nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// CreateAPIKey creates an API key of the caller; bad input is ErrValidation,
// a caller with as many keys as allowed ErrConflict.
func (c *Client) CreateAPIKey(ctx context.Context, in APIKeyInput) (CreatedAPIKey, error) {
	var out CreatedAPIKey
	err := c.do(ctx, http.MethodPost, "/api-keys", in, http.StatusCreated, &out)
	return out, err
}

// APIKeyScopes returns every area a key can be granted.
func (c *Client) APIKeyScopes(ctx context.Context) ([]APIKeyScope, error) {
	var out struct {
		Items []APIKeyScope `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/api-keys/scopes", nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// RevokeAPIKey revokes one of the caller's keys; a missing one is ErrNotFound.
func (c *Client) RevokeAPIKey(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api-keys/"+url.PathEscape(id), nil, http.StatusNoContent, nil)
}
