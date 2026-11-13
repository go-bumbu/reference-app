package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-bumbu/reference-app/internal/tenants"
	"github.com/go-bumbu/userauth"
	"github.com/go-bumbu/userauth/service/pat"
	"github.com/go-bumbu/userauth/service/pat/store/memory"
)

// testKeyOwners is the token service's view of the users (app/cmd has the real
// one): a user of internal/tenants, found by its ID.
type testKeyOwners struct{ tenants *tenants.Service }

func (o testKeyOwners) GetUser(id string) (userauth.User, error) {
	p, err := o.tenants.PrincipalByUserID(context.Background(), id)
	if errors.Is(err, tenants.ErrUserNotFound) {
		return userauth.User{}, userauth.ErrUserNotFound
	}
	if err != nil {
		return userauth.User{}, err
	}
	return userauth.User{ID: p.UserID, LoginID: p.Subject, Enabled: true}, nil
}

func (testKeyOwners) GetUserByLogin(string) (userauth.User, error) {
	return userauth.User{}, userauth.ErrUserNotFound
}

const testKeyPrefix = "refapp"

var keyProxyCfg = AuthCfg{Method: AuthProxy, Proxy: ProxyAuthCfg{UserHeader: userHeader, TenantHeader: tenantHeader}}

// newKeyService is a token service on an in-memory store over the deps' users.
func newKeyService(t *testing.T, d testDeps) *pat.Service {
	t.Helper()
	svc, err := pat.NewService(memory.New(), testKeyOwners{tenants: d.tenants}, pat.Opts{Prefix: testKeyPrefix})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// newKeyRouter is the router in proxy mode with API keys on.
func newKeyRouter(t *testing.T, d testDeps, keys *pat.Service, production bool) http.Handler {
	t.Helper()
	h, err := New(Cfg{
		Notes: d.notes, Tags: d.notes, Jobs: d.jobs, Tenants: d.tenants, Production: production,
		Auth: keyProxyCfg, APIKeys: keys, KeyOwners: d.tenants,
	})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return h
}

func as(tenant, user string) map[string]string {
	return map[string]string{userHeader: user, tenantHeader: tenant}
}

// keyRequest builds a request to the key-only mount; token "" sends no
// Authorization header.
func keyRequest(method, path, body, token string) *http.Request {
	req := httptest.NewRequest(method, keyMountPrefix+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type keyBody struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	Usable     bool       `json:"usable"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	Token      string     `json:"token"`
}

// createKey creates a key through the session API, as the user of the tenant.
func createKey(t *testing.T, h http.Handler, tenant, user, body string) keyBody {
	t.Helper()
	rec := do(h, request(http.MethodPost, "/api-keys", body, "", as(tenant, user)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: got %d: %s", rec.Code, rec.Body.String())
	}
	var k keyBody
	if err := json.Unmarshal(rec.Body.Bytes(), &k); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.Header().Get("Location"), apiV0MountPrefix+"/api-keys/"+k.ID; got != want {
		t.Errorf("Location: got %q, want %q", got, want)
	}
	return k
}

func TestAPIKeyLifecycle(t *testing.T) {
	d := newDeps(t)
	h := newKeyRouter(t, d, newKeyService(t, d), false)

	k := createKey(t, h, "acme", "alice", `{"name":" ci bot ","scopes":["notes:write"]}`)
	if !strings.HasPrefix(k.Token, testKeyPrefix+"_") || k.Name != "ci bot" || !k.Usable ||
		len(k.Scopes) != 1 || k.Scopes[0] != "notes:write" {
		t.Fatalf("created key: %+v", k)
	}

	// the key acts as its owner, in the owner's tenant
	rec := do(h, keyRequest(http.MethodGet, "/me", "", k.Token))
	if rec.Code != http.StatusOK {
		t.Fatalf("/me with key: got %d: %s", rec.Code, rec.Body.String())
	}
	var me struct {
		meBody
		APIKey *struct {
			ID     string   `json:"id"`
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		} `json:"apiKey"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Subject != "alice" || me.Tenant != "acme" || me.APIKey == nil || me.APIKey.ID != k.ID {
		t.Errorf("/me with key: %s", rec.Body.String())
	}
	// a session /me carries no key
	if rec := do(h, request(http.MethodGet, "/me", "", "", as("acme", "alice"))); strings.Contains(rec.Body.String(), "apiKey") {
		t.Errorf("session /me shows a key: %s", rec.Body.String())
	}

	if rec := do(h, keyRequest(http.MethodPost, "/notes", `{"title":"from a script"}`, k.Token)); rec.Code != http.StatusCreated {
		t.Fatalf("create note with key: got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, request(http.MethodGet, "/notes", "", "", as("acme", "bob"))); !strings.Contains(rec.Body.String(), "from a script") {
		t.Errorf("a colleague does not see the key's note: %s", rec.Body.String())
	}

	list := do(h, request(http.MethodGet, "/api-keys", "", "", as("acme", "alice")))
	var keys struct{ Items []keyBody }
	if err := json.Unmarshal(list.Body.Bytes(), &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys.Items) != 1 || keys.Items[0].ID != k.ID || keys.Items[0].Token != "" || keys.Items[0].LastUsedAt == nil {
		t.Errorf("list: %s", list.Body.String())
	}
	// another user sees none of alice's keys and cannot revoke them
	if rec := do(h, request(http.MethodGet, "/api-keys", "", "", as("acme", "bob"))); !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("bob's list: %s", rec.Body.String())
	}
	if rec := do(h, request(http.MethodDelete, "/api-keys/"+k.ID, "", "", as("acme", "bob"))); rec.Code != http.StatusNotFound {
		t.Errorf("revoke another user's key: got %d, want 404", rec.Code)
	}

	if rec := do(h, request(http.MethodDelete, "/api-keys/"+k.ID, "", "", as("acme", "alice"))); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, keyRequest(http.MethodGet, "/me", "", k.Token)); rec.Code != http.StatusUnauthorized {
		t.Errorf("revoked key: got %d, want 401", rec.Code)
	}
	if rec := do(h, request(http.MethodDelete, "/api-keys/"+k.ID, "", "", as("acme", "alice"))); rec.Code != http.StatusNotFound {
		t.Errorf("revoke twice: got %d, want 404", rec.Code)
	}
}

// TestAPIKeyBadKeysLookAlike checks every way a key can be bad gets the same 401.
func TestAPIKeyBadKeysLookAlike(t *testing.T) {
	d := newDeps(t)
	keys := newKeyService(t, d)
	h := newKeyRouter(t, d, keys, false)
	good := createKey(t, h, "acme", "alice", `{"name":"k","scopes":["notes:read"]}`)
	alice := d.actorFor(t, "acme", "alice")

	gone, _, err := keys.Mint("0000000000000000", "orphan", []string{"notes:read"}, nil, pat.HashOnly)
	if err != nil {
		t.Fatal(err)
	}
	unknownScope, _, err := keys.Mint(alice.UserID, "old", []string{"photos:read"}, nil, pat.HashOnly)
	if err != nil {
		t.Fatal(err)
	}
	noScope, _, err := keys.Mint(alice.UserID, "empty", nil, nil, pat.HashOnly)
	if err != nil {
		t.Fatal(err)
	}
	soon := time.Now().Add(50 * time.Millisecond)
	expired, _, err := keys.Mint(alice.UserID, "short", []string{"notes:read"}, &soon, pat.HashOnly)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(soon) + 10*time.Millisecond)
	wrongSecret := good.Token[:len(good.Token)-1] + "x"
	if strings.HasSuffix(good.Token, "x") {
		wrongSecret = good.Token[:len(good.Token)-1] + "y"
	}

	tests := map[string]func(*http.Request){
		"no header":      func(*http.Request) {},
		"basic scheme":   func(r *http.Request) { r.Header.Set("Authorization", "Basic "+good.Token) },
		"bearer, empty":  func(r *http.Request) { r.Header.Set("Authorization", "Bearer   ") },
		"malformed":      func(r *http.Request) { r.Header.Set("Authorization", "Bearer nonsense") },
		"other prefix":   func(r *http.Request) { r.Header.Set("Authorization", "Bearer pat_abc_def") },
		"wrong secret":   func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+wrongSecret) },
		"owner gone":     func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+gone) },
		"unknown scope":  func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+unknownScope) },
		"no scope":       func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+noScope) },
		"expired":        func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+expired) },
		"proxy identity": func(r *http.Request) { r.Header.Set(userHeader, "alice"); r.Header.Set(tenantHeader, "acme") },
	}
	for name, set := range tests {
		t.Run(name, func(t *testing.T) {
			req := keyRequest(http.MethodGet, "/notes", "", "")
			set(req)
			rec := do(h, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401: %s", rec.Code, rec.Body.String())
			}
			var p struct{ Detail string }
			_ = json.Unmarshal(rec.Body.Bytes(), &p)
			if p.Detail != msgKeyInvalid {
				t.Errorf("detail: got %q, want %q", p.Detail, msgKeyInvalid)
			}
		})
	}
	// the scheme is case-insensitive
	req := keyRequest(http.MethodGet, "/notes", "", "")
	req.Header.Set("Authorization", "bEaReR "+good.Token)
	if rec := do(h, req); rec.Code != http.StatusOK {
		t.Errorf("lower-case scheme: got %d, want 200", rec.Code)
	}
}

func TestAPIKeyScopePolicy(t *testing.T) {
	d := newDeps(t)
	h := newKeyRouter(t, d, newKeyService(t, d), false)
	read := createKey(t, h, "acme", "alice", `{"name":"reader","scopes":["notes:read"]}`)
	write := createKey(t, h, "acme", "alice", `{"name":"writer","scopes":["notes:write"]}`)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		wantStatus int
		wantDetail string
	}{
		{name: "read key lists notes", method: http.MethodGet, path: "/notes", token: read.Token, wantStatus: http.StatusOK},
		{name: "read key cannot create", method: http.MethodPost, path: "/notes", body: `{"title":"x"}`, token: read.Token,
			wantStatus: http.StatusForbidden, wantDetail: "this API key is read-only for notes"},
		{name: "write key creates", method: http.MethodPost, path: "/notes", body: `{"title":"x"}`, token: write.Token,
			wantStatus: http.StatusCreated},
		{name: "any key asks who it is", method: http.MethodGet, path: "/me", token: read.Token, wantStatus: http.StatusOK},
		{name: "tags are closed", method: http.MethodGet, path: "/tags", token: write.Token,
			wantStatus: http.StatusForbidden, wantDetail: "API keys can't use this endpoint"},
		{name: "jobs are closed", method: http.MethodGet, path: "/jobs", token: write.Token, wantStatus: http.StatusForbidden},
		{name: "keys never list keys", method: http.MethodGet, path: "/api-keys", token: write.Token, wantStatus: http.StatusForbidden},
		{name: "keys never mint keys", method: http.MethodPost, path: "/api-keys",
			body: `{"name":"n","scopes":["notes:write"]}`, token: write.Token, wantStatus: http.StatusForbidden},
		{name: "keys never revoke keys", method: http.MethodDelete, path: "/api-keys/" + read.ID, token: write.Token,
			wantStatus: http.StatusForbidden},
		{name: "info stays public", method: http.MethodGet, path: "/info", wantStatus: http.StatusOK},
		{name: "a bad key on a closed route is 401", method: http.MethodGet, path: "/tags", token: "nonsense",
			wantStatus: http.StatusUnauthorized},
		{name: "unknown path is a problem 404", method: http.MethodGet, path: "/nope", token: write.Token,
			wantStatus: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, keyRequest(tc.method, tc.path, tc.body, tc.token))
			if rec.Code != tc.wantStatus {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantDetail != "" && !strings.Contains(rec.Body.String(), tc.wantDetail) {
				t.Errorf("body %s does not say %q", rec.Body.String(), tc.wantDetail)
			}
		})
	}
}

// TestAPIKeyMountsStayApart checks each mount reads only its own credential: the
// key-only mount never trusts identity headers, the session mount never reads
// a key.
func TestAPIKeyMountsStayApart(t *testing.T) {
	d := newDeps(t)
	h := newKeyRouter(t, d, newKeyService(t, d), false)
	k := createKey(t, h, "acme", "alice", `{"name":"k","scopes":["notes:read"]}`)

	// a forged identity next to a key changes nothing: the key's owner it is
	req := keyRequest(http.MethodGet, "/me", "", k.Token)
	for name, v := range as("globex", "mallory") {
		req.Header.Set(name, v)
	}
	code, me := getMe(t, h, req)
	if code != http.StatusOK || me.Subject != "alice" || me.Tenant != "acme" {
		t.Errorf("key with forged headers: got %d %+v, want alice of acme", code, me)
	}

	// the session mount does not read keys
	sess := request(http.MethodGet, "/me", "", "", nil)
	sess.Header.Set("Authorization", "Bearer "+k.Token)
	if code, _ := getMe(t, h, sess); code != http.StatusUnauthorized {
		t.Errorf("key on the session mount: got %d, want 401", code)
	}
}

func TestAPIKeyTenantIsolation(t *testing.T) {
	d := newDeps(t)
	h := newKeyRouter(t, d, newKeyService(t, d), false)
	created := do(h, request(http.MethodPost, "/notes", `{"title":"globex plans"}`, "", as("globex", "carol")))
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	var note struct{ ID string }
	_ = json.Unmarshal(created.Body.Bytes(), &note)

	k := createKey(t, h, "acme", "alice", `{"name":"k","scopes":["notes:write"]}`)
	if rec := do(h, keyRequest(http.MethodGet, "/notes/"+note.ID, "", k.Token)); rec.Code != http.StatusNotFound {
		t.Errorf("another tenant's note: got %d, want 404", rec.Code)
	}
	if rec := do(h, keyRequest(http.MethodDelete, "/notes/"+note.ID, "", k.Token)); rec.Code != http.StatusNotFound {
		t.Errorf("delete another tenant's note: got %d, want 404", rec.Code)
	}
	if rec := do(h, keyRequest(http.MethodGet, "/notes", "", k.Token)); !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("list: %s", rec.Body.String())
	}
}

func TestAPIKeyCreateValidation(t *testing.T) {
	d := newDeps(t)
	h := newKeyRouter(t, d, newKeyService(t, d), false)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	tests := []struct {
		name        string
		body        string
		wantStatus  int
		wantPointer string
	}{
		{name: "malformed json", body: `{"name":`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"name":"n","scopes":["notes:read"],"admin":true}`, wantStatus: http.StatusBadRequest},
		{name: "blank name", body: `{"name":"  ","scopes":["notes:read"]}`, wantStatus: http.StatusUnprocessableEntity, wantPointer: "/name"},
		{name: "long name", body: `{"name":"` + strings.Repeat("x", 101) + `","scopes":["notes:read"]}`,
			wantStatus: http.StatusUnprocessableEntity, wantPointer: "/name"},
		{name: "no scopes", body: `{"name":"n","scopes":[]}`, wantStatus: http.StatusUnprocessableEntity, wantPointer: "/scopes"},
		{name: "unknown scope", body: `{"name":"n","scopes":["tags:read"]}`, wantStatus: http.StatusUnprocessableEntity, wantPointer: "/scopes"},
		{name: "area twice", body: `{"name":"n","scopes":["notes:read","notes:write"]}`,
			wantStatus: http.StatusUnprocessableEntity, wantPointer: "/scopes"},
		{name: "past expiry", body: `{"name":"n","scopes":["notes:read"],"expiresAt":"` + past + `"}`,
			wantStatus: http.StatusUnprocessableEntity, wantPointer: "/expiresAt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, request(http.MethodPost, "/api-keys", tc.body, "", as("acme", "alice")))
			if rec.Code != tc.wantStatus {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantPointer != "" && !strings.Contains(rec.Body.String(), `"pointer":"`+tc.wantPointer+`"`) {
				t.Errorf("no pointer %s in %s", tc.wantPointer, rec.Body.String())
			}
		})
	}

	t.Run("default expiry is a year", func(t *testing.T) {
		k := createKey(t, h, "acme", "alice", `{"name":"n","scopes":["notes:read"]}`)
		want := time.Now().Add(365 * 24 * time.Hour)
		if k.ExpiresAt == nil || k.ExpiresAt.Sub(want).Abs() > time.Minute {
			t.Errorf("expiresAt: got %v, want about %v", k.ExpiresAt, want)
		}
	})
	t.Run("explicit expiry is kept", func(t *testing.T) {
		at := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
		k := createKey(t, h, "acme", "alice", `{"name":"n","scopes":["notes:read"],"expiresAt":"`+at.Format(time.RFC3339)+`"}`)
		if k.ExpiresAt == nil || !k.ExpiresAt.Equal(at) {
			t.Errorf("expiresAt: got %v, want %v", k.ExpiresAt, at)
		}
	})
}

func TestAPIKeyLimit(t *testing.T) {
	d := newDeps(t)
	h := newKeyRouter(t, d, newKeyService(t, d), false)
	for i := range 25 { // pat's default limit per user
		createKey(t, h, "acme", "alice", `{"name":"k`+strconv.Itoa(i)+`","scopes":["notes:read"]}`)
	}
	rec := do(h, request(http.MethodPost, "/api-keys", `{"name":"one too many","scopes":["notes:read"]}`, "", as("acme", "alice")))
	if rec.Code != http.StatusConflict {
		t.Errorf("got %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestAPIKeyUnusableKeyIsListed(t *testing.T) {
	d := newDeps(t)
	keys := newKeyService(t, d)
	h := newKeyRouter(t, d, keys, false)
	alice := d.actorFor(t, "acme", "alice")
	if _, _, err := keys.Mint(alice.UserID, "old", []string{"photos:read"}, nil, pat.HashOnly); err != nil {
		t.Fatal(err)
	}
	rec := do(h, request(http.MethodGet, "/api-keys", "", "", as("acme", "alice")))
	if !strings.Contains(rec.Body.String(), `"usable":false`) || !strings.Contains(rec.Body.String(), `"scopes":[]`) {
		t.Errorf("list: %s", rec.Body.String())
	}
}

func TestAPIKeyScopesEndpoint(t *testing.T) {
	d := newDeps(t)
	h := newKeyRouter(t, d, newKeyService(t, d), false)
	rec := do(h, request(http.MethodGet, "/api-keys/scopes", "", "", as("acme", "alice")))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"items":[{"area":"notes","levels":["read","write"]}]}`+"\n" {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPIKeysOffOutsideProxy(t *testing.T) {
	h := newTestRouter(t, newDeps(t), false, AuthCfg{Method: AuthNone})
	for _, path := range []string{keyMountPrefix + "/me", apiV0MountPrefix + "/api-keys"} {
		rec := do(h, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/problem+json") {
			t.Errorf("%s: got %d %s, want a problem 404", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

func TestAPIKeyConfigErrors(t *testing.T) {
	d := newDeps(t)
	keys := newKeyService(t, d)
	base := Cfg{Notes: d.notes, Tags: d.notes, Jobs: d.jobs, Tenants: d.tenants}

	none := base
	none.APIKeys, none.KeyOwners = keys, d.tenants
	if _, err := New(none); err == nil {
		t.Error("API keys without the proxy: want an error")
	}
	noOwners := base
	noOwners.Auth, noOwners.APIKeys = keyProxyCfg, keys
	if _, err := New(noOwners); err == nil {
		t.Error("API keys without KeyOwners: want an error")
	}
}

// failingKeys is a token service whose every call fails, for the 500s.
type failingKeys struct{}

var errKeysDown = errors.New("token store down")

func (failingKeys) Mint(string, string, []string, *time.Time, pat.Storage) (string, pat.TokenRecord, error) {
	return "", pat.TokenRecord{}, errKeysDown
}
func (failingKeys) List(string) ([]pat.TokenRecord, error) { return nil, errKeysDown }
func (failingKeys) Revoke(string, string) error            { return errKeysDown }
func (failingKeys) Verify(string) (pat.TokenInfo, bool, error) {
	return pat.TokenInfo{}, false, errKeysDown
}

func TestAPIKeyStoreFailuresAreInternal(t *testing.T) {
	d := newDeps(t)
	h, err := New(Cfg{
		Notes: d.notes, Tags: d.notes, Jobs: d.jobs, Tenants: d.tenants,
		Auth: keyProxyCfg, APIKeys: failingKeys{}, KeyOwners: d.tenants,
	})
	if err != nil {
		t.Fatal(err)
	}
	reqs := []*http.Request{
		request(http.MethodGet, "/api-keys", "", "", as("acme", "alice")),
		request(http.MethodPost, "/api-keys", `{"name":"n","scopes":["notes:read"]}`, "", as("acme", "alice")),
		request(http.MethodDelete, "/api-keys/abc", "", "", as("acme", "alice")),
		keyRequest(http.MethodGet, "/me", "", testKeyPrefix+"_abc_def"),
	}
	for _, req := range reqs {
		if rec := do(h, req); rec.Code != http.StatusInternalServerError {
			t.Errorf("%s %s: got %d, want 500", req.Method, req.URL.Path, rec.Code)
		}
	}
}

// TestOpenAPIKeyContract validates the responses of the API key operations, and
// of the key-only mount, against the spec.
func TestOpenAPIKeyContract(t *testing.T) {
	doc := loadSpec(t)
	for _, production := range []bool{false, true} {
		t.Run("production="+strconv.FormatBool(production), func(t *testing.T) {
			d := newDeps(t)
			h := newKeyRouter(t, d, newKeyService(t, d), production)
			read := createKey(t, h, "acme", "alice", `{"name":"reader","scopes":["notes:read"]}`)
			doomed := createKey(t, h, "acme", "alice", `{"name":"doomed","scopes":["notes:write"]}`)
			alice := as("acme", "alice")

			type kase struct {
				name       string
				req        *http.Request
				specPath   string
				wantStatus int
			}
			cases := []kase{
				{"list keys", request(http.MethodGet, "/api-keys", "", "", alice), "/api-keys", http.StatusOK},
				{"list keys anonymous", request(http.MethodGet, "/api-keys", "", "", nil), "/api-keys", http.StatusUnauthorized},
				{"create key", request(http.MethodPost, "/api-keys", `{"name":"n","scopes":["notes:read"],"expiresAt":null}`, "", alice),
					"/api-keys", http.StatusCreated},
				{"create key malformed", request(http.MethodPost, "/api-keys", `{"name":`, "", alice), "/api-keys", http.StatusBadRequest},
				{"create key unknown scope", request(http.MethodPost, "/api-keys", `{"name":"n","scopes":["x:read"]}`, "", alice),
					"/api-keys", http.StatusUnprocessableEntity},
				{"create key anonymous", request(http.MethodPost, "/api-keys", `{"name":"n","scopes":["notes:read"]}`, "", nil),
					"/api-keys", http.StatusUnauthorized},
				{"scopes", request(http.MethodGet, "/api-keys/scopes", "", "", alice), "/api-keys/scopes", http.StatusOK},
				{"scopes anonymous", request(http.MethodGet, "/api-keys/scopes", "", "", nil), "/api-keys/scopes", http.StatusUnauthorized},
				{"revoke key", request(http.MethodDelete, "/api-keys/"+doomed.ID, "", "", alice), "/api-keys/{keyId}", http.StatusNoContent},
				{"revoke missing key", request(http.MethodDelete, "/api-keys/"+doomed.ID, "", "", alice), "/api-keys/{keyId}", http.StatusNotFound},
				{"revoke key anonymous", request(http.MethodDelete, "/api-keys/x", "", "", nil), "/api-keys/{keyId}", http.StatusUnauthorized},

				{"key: me", keyRequest(http.MethodGet, "/me", "", read.Token), "/me", http.StatusOK},
				{"key: bad key", keyRequest(http.MethodGet, "/me", "", "nonsense"), "/me", http.StatusUnauthorized},
				{"key: list notes", keyRequest(http.MethodGet, "/notes", "", read.Token), "/notes", http.StatusOK},
				{"key: read-only", keyRequest(http.MethodPost, "/notes", `{"title":"x"}`, read.Token), "/notes", http.StatusForbidden},
				{"key: closed", keyRequest(http.MethodGet, "/tags", "", read.Token), "/tags", http.StatusForbidden},
				{"key: list keys", keyRequest(http.MethodGet, "/api-keys", "", read.Token), "/api-keys", http.StatusForbidden},
				{"key: revoke key", keyRequest(http.MethodDelete, "/api-keys/"+read.ID, "", read.Token), "/api-keys/{keyId}", http.StatusForbidden},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					rec := do(h, tc.req)
					if rec.Code != tc.wantStatus {
						t.Fatalf("status: got %d, want %d; body: %s", rec.Code, tc.wantStatus, rec.Body.String())
					}
					validateResponse(t, doc, tc.req.Method, tc.specPath, rec)
				})
			}

			t.Run("too many keys", func(t *testing.T) {
				for i := range 25 {
					rec := do(h, request(http.MethodPost, "/api-keys", `{"name":"k`+strconv.Itoa(i)+`","scopes":["notes:read"]}`, "", as("acme", "bob")))
					if rec.Code != http.StatusCreated {
						t.Fatalf("create %d: %d", i, rec.Code)
					}
				}
				rec := do(h, request(http.MethodPost, "/api-keys", `{"name":"n","scopes":["notes:read"]}`, "", as("acme", "bob")))
				if rec.Code != http.StatusConflict {
					t.Fatalf("status: got %d, want 409", rec.Code)
				}
				validateResponse(t, doc, http.MethodPost, "/api-keys", rec)
			})
		})
	}

	t.Run("internal", func(t *testing.T) {
		d := newDeps(t)
		h, err := New(Cfg{
			Notes: d.notes, Tags: d.notes, Jobs: d.jobs, Tenants: d.tenants,
			Auth: keyProxyCfg, APIKeys: failingKeys{}, KeyOwners: d.tenants,
		})
		if err != nil {
			t.Fatal(err)
		}
		alice := as("acme", "alice")
		for _, tc := range []struct {
			req      *http.Request
			specPath string
		}{
			{request(http.MethodGet, "/api-keys", "", "", alice), "/api-keys"},
			{request(http.MethodPost, "/api-keys", `{"name":"n","scopes":["notes:read"]}`, "", alice), "/api-keys"},
			{request(http.MethodDelete, "/api-keys/x", "", "", alice), "/api-keys/{keyId}"},
			{keyRequest(http.MethodGet, "/notes", "", testKeyPrefix+"_abc_def"), "/notes"},
		} {
			rec := do(h, tc.req)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("%s %s: got %d, want 500", tc.req.Method, tc.req.URL.Path, rec.Code)
			}
			validateResponse(t, doc, tc.req.Method, tc.specPath, rec)
			if strings.Contains(rec.Body.String(), errKeysDown.Error()) {
				t.Error("the internal error leaked to the client")
			}
		}
	})
}
