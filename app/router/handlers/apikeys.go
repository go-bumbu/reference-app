package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/app/reqctx"
	"github.com/go-bumbu/reference-app/libs/keyscope"
	"github.com/go-bumbu/userauth/service/pat"
	"github.com/gorilla/mux"
)

// MaxAPIKeyNameLen bounds an API key's name; it is the limit of the token service.
const MaxAPIKeyNameLen = 100

// DefaultAPIKeyLifetime is how long a key lives when its creator names no
// expiry. Keys outlive the identity provider (just-in-time provisioning never
// learns that a user left), so a key never lives forever by default.
const DefaultAPIKeyLifetime = 365 * 24 * time.Hour

// APIKeysService is what the API keys handler needs from the token service
// (go-bumbu/userauth's pat.Service): the caller's keys, by owner user id.
type APIKeysService interface {
	Mint(userID, name string, scopes []string, expiresAt *time.Time, storage pat.Storage) (string, pat.TokenRecord, error)
	List(userID string) ([]pat.TokenRecord, error)
	Revoke(userID, tokenID string) error
}

// APIKeysHandler serves /api-keys: a signed-in user lists, creates and revokes
// their own API keys, and reads which scopes a key can be granted. A request
// made with an API key is refused (keys never manage keys), which is the second
// lock behind the route policy that already closes these routes to keys.
type APIKeysHandler struct {
	Keys APIKeysService
	// Scopes is the vocabulary the routes declared; it validates what a new key
	// asks for, and decodes what stored keys grant.
	Scopes   *keyscope.Registry
	Problems *problemjson.Writer
	Logger   *slog.Logger
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
}

// apiKeyResponse is a key as the API shows it (schema ApiKey). Usable is false
// for a key whose stored scopes the app no longer knows: it opens nothing and
// lists no scopes, but is shown so its owner can revoke it.
type apiKeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	Usable     bool       `json:"usable"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
}

// apiKeyCreatedResponse is a key just created (schema ApiKeyCreated): the only
// time its token is shown.
type apiKeyCreatedResponse struct {
	apiKeyResponse
	Token string `json:"token"`
}

type apiKeyListResponse struct {
	Items []apiKeyResponse `json:"items"`
}

// apiKeyRequest is the body of POST /api-keys (schema ApiKeyInput).
type apiKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// apiKeyScopeResponse is one area a key can be granted (schema ApiKeyScope).
type apiKeyScopeResponse struct {
	Area   string   `json:"area"`
	Levels []string `json:"levels"`
}

type apiKeyScopeListResponse struct {
	Items []apiKeyScopeResponse `json:"items"`
}

func (h *APIKeysHandler) toResponse(rec pat.TokenRecord) apiKeyResponse {
	out := apiKeyResponse{
		ID: rec.TokenID, Name: rec.Name, Scopes: []string{},
		CreatedAt: rec.CreatedAt, LastUsedAt: rec.LastUsedAt, ExpiresAt: rec.ExpiresAt,
	}
	if grant, err := h.Scopes.Parse(rec.Scopes); err == nil {
		out.Scopes, out.Usable = grant.Scopes(), true
	}
	return out
}

// List handles GET /api-keys: the caller's keys, oldest first.
func (h *APIKeysHandler) List(w http.ResponseWriter, r *http.Request) {
	id, ok := h.caller(w, r)
	if !ok {
		return
	}
	recs, err := h.Keys.List(id.UserID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	out := apiKeyListResponse{Items: make([]apiKeyResponse, 0, len(recs))}
	for _, rec := range recs {
		out.Items = append(out.Items, h.toResponse(rec))
	}
	writeJSON(w, http.StatusOK, out)
}

// ListScopes handles GET /api-keys/scopes: every area a key can be granted, each
// at read or write.
func (h *APIKeysHandler) ListScopes(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.caller(w, r); !ok {
		return
	}
	areas := h.Scopes.Areas()
	out := apiKeyScopeListResponse{Items: make([]apiKeyScopeResponse, 0, len(areas))}
	for _, a := range areas {
		out.Items = append(out.Items, apiKeyScopeResponse{
			Area: a, Levels: []string{keyscope.Read.String(), keyscope.Write.String()},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// Create handles POST /api-keys. Every field is validated before the token
// service is asked, so a bad request is told what is wrong with it.
func (h *APIKeysHandler) Create(w http.ResponseWriter, r *http.Request) {
	id, ok := h.caller(w, r)
	if !ok {
		return
	}
	var req apiKeyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.Problems.Write(w, r, http.StatusBadRequest, SlugBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxAPIKeyNameLen {
		h.invalid(w, r, "/name", "must be 1 to 100 characters")
		return
	}
	grant, err := h.Scopes.Parse(req.Scopes)
	if err != nil {
		h.invalid(w, r, "/scopes", err.Error())
		return
	}
	now := h.now()
	expires := req.ExpiresAt
	if expires == nil {
		e := now.Add(DefaultAPIKeyLifetime).UTC()
		expires = &e
	} else if !expires.After(now) {
		h.invalid(w, r, "/expiresAt", "must be in the future")
		return
	}
	token, rec, err := h.Keys.Mint(id.UserID, name, grant.Scopes(), expires, pat.HashOnly)
	switch {
	case errors.Is(err, pat.ErrTooManyTokens):
		h.Problems.Write(w, r, http.StatusConflict, problemjson.SlugConflict, "too many API keys: revoke one first")
		return
	case errors.Is(err, pat.ErrInvalidExpiry):
		h.invalid(w, r, "/expiresAt", "must be in the future")
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	w.Header().Set("Location", r.URL.Path+"/"+rec.TokenID)
	writeJSON(w, http.StatusCreated, apiKeyCreatedResponse{apiKeyResponse: h.toResponse(rec), Token: token})
}

// Revoke handles DELETE /api-keys/{keyId}; another user's key is a 404, like a
// missing one.
func (h *APIKeysHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id, ok := h.caller(w, r)
	if !ok {
		return
	}
	keyID := mux.Vars(r)["keyId"]
	err := h.Keys.Revoke(id.UserID, keyID)
	switch {
	case errors.Is(err, pat.ErrTokenNotFound):
		h.Problems.Write(w, r, http.StatusNotFound, problemjson.SlugNotFound, "API key "+keyID+" not found")
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// caller returns the signed-in user, and refuses a request made with an API key.
func (h *APIKeysHandler) caller(w http.ResponseWriter, r *http.Request) (reqctx.Identity, bool) {
	if _, keyed := reqctx.KeyFrom(r.Context()); keyed {
		h.Problems.Write(w, r, http.StatusForbidden, problemjson.SlugForbidden, "API keys can't use this endpoint")
		return reqctx.Identity{}, false
	}
	id, ok := reqctx.IdentityFrom(r.Context())
	if !ok {
		h.internalError(w, r, errNoIdentity)
	}
	return id, ok
}

func (h *APIKeysHandler) invalid(w http.ResponseWriter, r *http.Request, pointer, msg string) {
	h.Problems.WriteValidation(w, r, strings.TrimPrefix(pointer, "/")+" "+msg,
		problemjson.FieldError{Pointer: pointer, Detail: msg})
}

func (h *APIKeysHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.Logger.Error("api keys", slog.String("path", r.URL.Path), slog.String("error", err.Error()))
	h.Problems.Write(w, r, http.StatusInternalServerError, problemjson.SlugInternal, "unexpected error")
}

func (h *APIKeysHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}
