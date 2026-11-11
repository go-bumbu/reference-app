package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/go-bumbu/reference-app/internal/tenants"
)

const (
	userHeader   = "Remote-User"
	tenantHeader = "Remote-Tenant"
)

// request builds an API request with the given identity headers; remote is the
// peer address ("" keeps httptest's 192.0.2.1).
func request(method, path, body, remote string, headers map[string]string) *http.Request {
	req := httptest.NewRequest(method, apiV0MountPrefix+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if remote != "" {
		req.RemoteAddr = remote
	}
	return req
}

type meBody struct {
	UserID   string `json:"userId"`
	Subject  string `json:"subject"`
	TenantID string `json:"tenantId"`
	Tenant   string `json:"tenant"`
}

func getMe(t *testing.T, h http.Handler, req *http.Request) (int, meBody) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var me meBody
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
			t.Fatalf("decode /me: %v", err)
		}
	}
	return rec.Code, me
}

func TestAuthNoneIsTheDefaultIdentity(t *testing.T) {
	for _, method := range []string{"", AuthNone} {
		t.Run("method="+method, func(t *testing.T) {
			r := newTestRouter(t, newDeps(t), false, AuthCfg{Method: method})
			// identity headers mean nothing in none mode
			code, me := getMe(t, r, request(http.MethodGet, "/me", "", "", map[string]string{userHeader: "mallory"}))
			if code != http.StatusOK {
				t.Fatalf("status: got %d, want 200", code)
			}
			if me.Subject != DefaultUser || me.Tenant != DefaultTenant || me.UserID == "" || me.TenantID == "" {
				t.Errorf("got %+v, want %s of %s with ids", me, DefaultUser, DefaultTenant)
			}
		})
	}
}

func TestAuthProxy(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name       string
		cfg        ProxyAuthCfg
		remote     string
		headers    map[string]string
		wantStatus int
		wantUser   string
		wantTenant string
	}{
		{
			name: "no identity header", cfg: ProxyAuthCfg{UserHeader: userHeader},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "user only, default tenant", cfg: ProxyAuthCfg{UserHeader: userHeader},
			headers:    map[string]string{userHeader: "alice", tenantHeader: "ignored"},
			wantStatus: http.StatusOK, wantUser: "alice", wantTenant: DefaultTenant,
		},
		{
			name: "user and tenant", cfg: ProxyAuthCfg{UserHeader: userHeader, TenantHeader: tenantHeader},
			headers:    map[string]string{userHeader: "alice", tenantHeader: "acme"},
			wantStatus: http.StatusOK, wantUser: "alice", wantTenant: "acme",
		},
		{
			name: "tenant header configured but missing", cfg: ProxyAuthCfg{UserHeader: userHeader, TenantHeader: tenantHeader},
			headers:    map[string]string{userHeader: "alice"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "trusted peer", cfg: ProxyAuthCfg{UserHeader: userHeader, TrustedProxies: trusted},
			remote: "10.1.2.3:4000", headers: map[string]string{userHeader: "alice"},
			wantStatus: http.StatusOK, wantUser: "alice", wantTenant: DefaultTenant,
		},
		{
			name: "untrusted peer", cfg: ProxyAuthCfg{UserHeader: userHeader, TrustedProxies: trusted},
			remote: "192.168.1.1:4000", headers: map[string]string{userHeader: "alice"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "identity the app cannot hold", cfg: ProxyAuthCfg{UserHeader: userHeader},
			headers:    map[string]string{userHeader: strings.Repeat("x", tenants.MaxSubjectLen+1)},
			wantStatus: http.StatusUnauthorized,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRouter(t, newDeps(t), false, AuthCfg{Method: AuthProxy, Proxy: tc.cfg})
			code, me := getMe(t, r, request(http.MethodGet, "/me", "", tc.remote, tc.headers))
			if code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", code, tc.wantStatus)
			}
			if code == http.StatusOK && (me.Subject != tc.wantUser || me.Tenant != tc.wantTenant) {
				t.Errorf("got %s of %s, want %s of %s", me.Subject, me.Tenant, tc.wantUser, tc.wantTenant)
			}
		})
	}
}

// TestTenantIsolationOverHTTP creates a note as one tenant and checks a user of
// another tenant can neither list nor read it, while a colleague can.
func TestTenantIsolationOverHTTP(t *testing.T) {
	r := newTestRouter(t, newDeps(t), false, AuthCfg{
		Method: AuthProxy,
		Proxy:  ProxyAuthCfg{UserHeader: userHeader, TenantHeader: tenantHeader},
	})
	as := func(tenant, user string) map[string]string {
		return map[string]string{userHeader: user, tenantHeader: tenant}
	}
	do := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	created := do(request(http.MethodPost, "/notes", `{"title":"acme plans"}`, "", as("acme", "alice")))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: got %d: %s", created.Code, created.Body.String())
	}
	var note struct {
		ID        string `json:"id"`
		CreatedBy string `json:"createdBy"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &note); err != nil {
		t.Fatal(err)
	}
	_, alice := getMe(t, r, request(http.MethodGet, "/me", "", "", as("acme", "alice")))
	if note.CreatedBy != alice.UserID {
		t.Errorf("createdBy: got %q, want alice's id %q", note.CreatedBy, alice.UserID)
	}

	if rec := do(request(http.MethodGet, "/notes/"+note.ID, "", "", as("globex", "alice"))); rec.Code != http.StatusNotFound {
		t.Errorf("get from another tenant: got %d, want 404", rec.Code)
	}
	list := do(request(http.MethodGet, "/notes", "", "", as("globex", "carol")))
	if !strings.Contains(list.Body.String(), `"items":[]`) {
		t.Errorf("another tenant's list is not empty: %s", list.Body.String())
	}
	if rec := do(request(http.MethodGet, "/notes/"+note.ID, "", "", as("acme", "bob"))); rec.Code != http.StatusOK {
		t.Errorf("get from a colleague: got %d, want 200", rec.Code)
	}
}

func TestAuthConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     AuthCfg
		wantErr error
	}{
		{name: "oidc", cfg: AuthCfg{Method: AuthOIDC}, wantErr: ErrOIDCNotImplemented},
		{name: "unknown method", cfg: AuthCfg{Method: "basic"}},
		{name: "proxy without user header", cfg: AuthCfg{Method: AuthProxy}},
	}
	deps := newDeps(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(Cfg{Notes: deps.notes, Tags: deps.notes, Jobs: deps.jobs, Tenants: deps.tenants, Auth: tc.cfg})
			if err == nil {
				t.Fatal("expected an error")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestRequiredDependencies(t *testing.T) {
	deps := newDeps(t)
	if _, err := New(Cfg{Tags: deps.notes, Jobs: deps.jobs, Tenants: deps.tenants}); err == nil {
		t.Error("expected an error without Notes")
	}
	if _, err := New(Cfg{Notes: deps.notes, Tenants: deps.tenants}); err == nil {
		t.Error("expected an error without Tags")
	}
	if _, err := New(Cfg{Notes: deps.notes, Tags: deps.notes, Jobs: deps.jobs}); err == nil {
		t.Error("expected an error without Tenants")
	}
	if _, err := New(Cfg{Notes: deps.notes, Tags: deps.notes, Tenants: deps.tenants}); err == nil {
		t.Error("expected an error without Jobs")
	}
}

// TestProvisionFailureIsInternal checks a failing tenants module is a 500
// without leaking the cause.
func TestProvisionFailureIsInternal(t *testing.T) {
	h, err := New(Cfg{Notes: failingNotes{}, Tags: failingNotes{}, Jobs: failingJobs{}, Tenants: failingTenants{}})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, request(http.MethodGet, "/me", "", "", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "db down") {
		t.Error("the internal error leaked to the client")
	}
}

type failingTenants struct{}

func (failingTenants) Provision(context.Context, string, string) (tenants.Principal, error) {
	return tenants.Principal{}, errors.New("db down")
}
