package handlers

import (
	"errors"
	"net/http"

	"github.com/go-bumbu/http/problemjson"
	"github.com/go-bumbu/reference-app/app/reqctx"
)

// errNoIdentity means a protected handler was mounted without the auth guard.
var errNoIdentity = errors.New("no identity on the request context: route is not behind the auth guard")

// meResponse is the API representation of the caller (schema Me in
// docs/openapi/api-v0.yaml).
type meResponse struct {
	UserID     string `json:"userId"`
	Subject    string `json:"subject"`
	TenantID   string `json:"tenantId"`
	TenantSlug string `json:"tenant"`
}

// MeHandler handles GET /me: who the authenticated caller is.
func MeHandler(problems *problemjson.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := reqctx.IdentityFrom(r.Context())
		if !ok {
			problems.Write(w, r, http.StatusInternalServerError, problemjson.SlugInternal, "unexpected error")
			return
		}
		writeJSON(w, http.StatusOK, meResponse{
			UserID: id.UserID, Subject: id.Subject, TenantID: id.TenantID, TenantSlug: id.TenantSlug,
		})
	})
}
