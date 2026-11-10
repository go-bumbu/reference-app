package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-bumbu/db/gormdb/gormdbtest"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"github.com/go-bumbu/reference-app/internal/notes"
	"github.com/go-bumbu/reference-app/internal/tenants"
	"github.com/gorilla/mux"
)

// These tests keep docs/openapi/api-v0.yaml and the router honest with each other:
//   - TestOpenAPICoversAllRoutes: every mounted (method, path) is documented and
//     every documented one is mounted.
//   - TestOpenAPIResponseContract: real responses are validated against the
//     schema the spec declares for that operation, status and content type, so a
//     renamed field, a dropped `required` or an undocumented status fails here.

var specDoc = sync.OnceValues(func() (*openapi3.T, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errors.New("could not resolve this test file's location")
	}
	// resolved relative to this file, so it does not depend on go test's working directory
	specPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "docs", "openapi", "api-v0.yaml")
	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", specPath, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("%s failed OpenAPI validation: %w", specPath, err)
	}
	return doc, nil
})

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := specDoc()
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// testDeps are the modules behind the router, on one fresh SQLite file; the
// modules' own tests cover the other databases.
type testDeps struct {
	notes   *notes.Service
	tenants *tenants.Service
}

func newDeps(t *testing.T) testDeps {
	t.Helper()
	// closed by the helper when the test ends; GORM's logger is discarded, as a
	// miss is a normal outcome here, not log noise
	db := gormdbtest.Sqlite(t, sqlite.Driver{})
	notesSvc, err := notes.New(context.Background(), db, notes.Cfg{})
	if err != nil {
		t.Fatalf("notes.New: %v", err)
	}
	tenantsSvc, err := tenants.New(context.Background(), db, tenants.Cfg{})
	if err != nil {
		t.Fatalf("tenants.New: %v", err)
	}
	return testDeps{notes: notesSvc, tenants: tenantsSvc}
}

// actorFor provisions the identity and returns it as a notes actor, to seed
// data the way a request of that identity would.
func (d testDeps) actorFor(t *testing.T, tenant, subject string) notes.Actor {
	t.Helper()
	p, err := d.tenants.Provision(context.Background(), tenant, subject)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	return notes.Actor{TenantID: p.TenantID, UserID: p.UserID}
}

func newTestRouter(t *testing.T, d testDeps, production bool, auth AuthCfg) *mux.Router {
	t.Helper()
	h, err := New(Cfg{Notes: d.notes, Tags: d.notes, Tenants: d.tenants, Production: production, Auth: auth})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	r, ok := h.(*mux.Router)
	if !ok {
		t.Fatalf("router.New returned %T, want *mux.Router", h)
	}
	return r
}

func TestOpenAPICoversAllRoutes(t *testing.T) {
	doc := loadSpec(t)

	documented := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			documented[method+" "+path] = true
		}
	}

	mounted := map[string]bool{}
	err := newTestRouter(t, newDeps(t), false, AuthCfg{}).Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tpl, err := route.GetPathTemplate()
		if err != nil || !strings.HasPrefix(tpl, apiV0MountPrefix+"/") {
			return nil //nolint:nilerr // routes without a template (or outside the API) are not part of the contract
		}
		methods, err := route.GetMethods()
		if err != nil {
			return nil //nolint:nilerr // the mount-prefix subrouter has no methods
		}
		for _, m := range methods {
			mounted[m+" "+strings.TrimPrefix(tpl, apiV0MountPrefix)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if missing := diff(mounted, documented); len(missing) > 0 {
		t.Errorf("routes mounted but not documented in the spec: %v", missing)
	}
	if missing := diff(documented, mounted); len(missing) > 0 {
		t.Errorf("operations documented in the spec but not mounted: %v", missing)
	}
}

// diff returns the keys of a that are not in b, sorted.
func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

type contractCase struct {
	name       string
	method     string
	path       string // request path, relative to the mount prefix
	specPath   string // the spec's paths key for the operation
	body       string
	wantStatus int
}

func TestOpenAPIResponseContract(t *testing.T) {
	doc := loadSpec(t)

	for _, production := range []bool{false, true} {
		t.Run("production="+strconv.FormatBool(production), func(t *testing.T) {
			deps := newDeps(t)
			admin := deps.actorFor(t, DefaultTenant, DefaultUser)
			existing, err := deps.notes.Create(context.Background(), admin, notes.Input{Title: "existing", Body: "body"})
			if err != nil {
				t.Fatal(err)
			}
			other := deps.actorFor(t, "other", "someone")
			foreign, err := deps.notes.Create(context.Background(), other, notes.Input{Title: "foreign"})
			if err != nil {
				t.Fatal(err)
			}
			doomed, err := deps.notes.Create(context.Background(), admin, notes.Input{Title: "doomed"})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			work, err := deps.notes.CreateTag(ctx, admin, notes.TagInput{Name: "work"})
			if err != nil {
				t.Fatal(err)
			}
			reports, err := deps.notes.CreateTag(ctx, admin, notes.TagInput{Name: "reports", ParentID: work.ID})
			if err != nil {
				t.Fatal(err)
			}
			doomedTag, err := deps.notes.CreateTag(ctx, admin, notes.TagInput{Name: "doomed"})
			if err != nil {
				t.Fatal(err)
			}
			foreignTag, err := deps.notes.CreateTag(ctx, other, notes.TagInput{Name: "foreign"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := deps.notes.AddNoteTag(ctx, admin, existing.ID, reports.ID); err != nil {
				t.Fatal(err)
			}
			r := newTestRouter(t, deps, production, AuthCfg{Method: AuthNone})
			const tooBig = "9223372036854775808" // 1<<63: past every database's integer
			tagPath := func(id uint) string { return "/tags/" + strconv.FormatUint(uint64(id), 10) }
			noteTagPath := func(noteID string, tagID uint) string {
				return "/notes/" + noteID + "/tags/" + strconv.FormatUint(uint64(tagID), 10)
			}

			cases := []contractCase{
				{name: "info", method: http.MethodGet, path: "/info", specPath: "/info", wantStatus: http.StatusOK},
				{name: "me", method: http.MethodGet, path: "/me", specPath: "/me", wantStatus: http.StatusOK},
				{name: "list notes", method: http.MethodGet, path: "/notes", specPath: "/notes", wantStatus: http.StatusOK},
				{name: "create note", method: http.MethodPost, path: "/notes", specPath: "/notes",
					body: `{"title":"new","body":"text"}`, wantStatus: http.StatusCreated},
				{name: "create note without body field", method: http.MethodPost, path: "/notes", specPath: "/notes",
					body: `{"title":"new"}`, wantStatus: http.StatusCreated},
				{name: "create note malformed json", method: http.MethodPost, path: "/notes", specPath: "/notes",
					body: `{"title":`, wantStatus: http.StatusBadRequest},
				{name: "create note unknown field", method: http.MethodPost, path: "/notes", specPath: "/notes",
					body: `{"title":"x","color":"red"}`, wantStatus: http.StatusBadRequest},
				{name: "create note empty title", method: http.MethodPost, path: "/notes", specPath: "/notes",
					body: `{"title":"  "}`, wantStatus: http.StatusUnprocessableEntity},
				{name: "get note", method: http.MethodGet, path: "/notes/" + existing.ID, specPath: "/notes/{noteId}",
					wantStatus: http.StatusOK},
				{name: "get missing note", method: http.MethodGet, path: "/notes/missing", specPath: "/notes/{noteId}",
					wantStatus: http.StatusNotFound},
				{name: "get note of another tenant", method: http.MethodGet, path: "/notes/" + foreign.ID,
					specPath: "/notes/{noteId}", wantStatus: http.StatusNotFound},
				{name: "update note", method: http.MethodPut, path: "/notes/" + existing.ID, specPath: "/notes/{noteId}",
					body: `{"title":"edited","body":"new text"}`, wantStatus: http.StatusOK},
				{name: "update note malformed json", method: http.MethodPut, path: "/notes/" + existing.ID,
					specPath: "/notes/{noteId}", body: `{"title":`, wantStatus: http.StatusBadRequest},
				{name: "update note empty title", method: http.MethodPut, path: "/notes/" + existing.ID,
					specPath: "/notes/{noteId}", body: `{"title":"  "}`, wantStatus: http.StatusUnprocessableEntity},
				{name: "update missing note", method: http.MethodPut, path: "/notes/missing", specPath: "/notes/{noteId}",
					body: `{"title":"x"}`, wantStatus: http.StatusNotFound},
				{name: "update note of another tenant", method: http.MethodPut, path: "/notes/" + foreign.ID,
					specPath: "/notes/{noteId}", body: `{"title":"x"}`, wantStatus: http.StatusNotFound},
				{name: "delete note", method: http.MethodDelete, path: "/notes/" + doomed.ID, specPath: "/notes/{noteId}",
					wantStatus: http.StatusNoContent},
				{name: "delete missing note", method: http.MethodDelete, path: "/notes/missing",
					specPath: "/notes/{noteId}", wantStatus: http.StatusNotFound},
				{name: "delete note of another tenant", method: http.MethodDelete, path: "/notes/" + foreign.ID,
					specPath: "/notes/{noteId}", wantStatus: http.StatusNotFound},

				// the notes of a tag
				{name: "list notes of a tag", method: http.MethodGet, path: "/notes?tagId=" + strconv.FormatUint(uint64(work.ID), 10),
					specPath: "/notes", wantStatus: http.StatusOK},
				{name: "list notes of a missing tag", method: http.MethodGet, path: "/notes?tagId=99999", specPath: "/notes",
					wantStatus: http.StatusOK},
				{name: "list notes of a malformed tag", method: http.MethodGet, path: "/notes?tagId=abc", specPath: "/notes",
					wantStatus: http.StatusBadRequest},
				{name: "list notes of tag 0", method: http.MethodGet, path: "/notes?tagId=0", specPath: "/notes",
					wantStatus: http.StatusBadRequest},
				{name: "list notes of two tags", method: http.MethodGet, path: "/notes?tagId=1&tagId=2", specPath: "/notes",
					wantStatus: http.StatusBadRequest},
				{name: "list notes of a tag id past int64", method: http.MethodGet, path: "/notes?tagId=" + tooBig,
					specPath: "/notes", wantStatus: http.StatusBadRequest},
				{name: "tag a note", method: http.MethodPut, path: noteTagPath(existing.ID, work.ID),
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusOK},
				{name: "tag a note again", method: http.MethodPut, path: noteTagPath(existing.ID, work.ID),
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusOK},
				{name: "tag a missing note", method: http.MethodPut, path: noteTagPath("missing", work.ID),
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusNotFound},
				{name: "tag a note with a missing tag", method: http.MethodPut, path: noteTagPath(existing.ID, 99999),
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusNotFound},
				{name: "tag a note with a malformed tag", method: http.MethodPut, path: "/notes/" + existing.ID + "/tags/abc",
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusNotFound},
				{name: "tag a note with another tenant's tag", method: http.MethodPut,
					path: noteTagPath(existing.ID, foreignTag.ID), specPath: "/notes/{noteId}/tags/{tagId}",
					wantStatus: http.StatusNotFound},
				{name: "untag a note", method: http.MethodDelete, path: noteTagPath(existing.ID, work.ID),
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusOK},
				{name: "untag a missing note", method: http.MethodDelete, path: noteTagPath("missing", work.ID),
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusNotFound},
				{name: "untag a missing tag", method: http.MethodDelete, path: noteTagPath(existing.ID, 99999),
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusNotFound},
				{name: "untag a tag id past int64", method: http.MethodDelete, path: "/notes/" + existing.ID + "/tags/" + tooBig,
					specPath: "/notes/{noteId}/tags/{tagId}", wantStatus: http.StatusNotFound},

				// the tag tree
				{name: "list tags", method: http.MethodGet, path: "/tags", specPath: "/tags", wantStatus: http.StatusOK},
				{name: "create tag", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: `{"name":"home"}`, wantStatus: http.StatusCreated},
				{name: "create child tag after a sibling", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body:       fmt.Sprintf(`{"name":"q1","parentId":%d,"afterId":%d}`, work.ID, reports.ID),
					wantStatus: http.StatusCreated},
				{name: "create tag malformed json", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: `{"name":`, wantStatus: http.StatusBadRequest},
				{name: "create tag negative parent", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: `{"name":"x","parentId":-1}`, wantStatus: http.StatusBadRequest},
				{name: "create tag empty name", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: `{"name":" "}`, wantStatus: http.StatusUnprocessableEntity},
				{name: "create tag without a name", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: `{"parentId":0}`, wantStatus: http.StatusUnprocessableEntity},
				{name: "create tag under a parent id past int64", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: `{"name":"x","parentId":` + tooBig + `}`, wantStatus: http.StatusUnprocessableEntity},
				{name: "create tag after a tag of another parent", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: fmt.Sprintf(`{"name":"x","afterId":%d}`, reports.ID), wantStatus: http.StatusUnprocessableEntity},
				{name: "create tag under a missing parent", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: `{"name":"x","parentId":99999}`, wantStatus: http.StatusUnprocessableEntity},
				{name: "create tag with a sibling's name", method: http.MethodPost, path: "/tags", specPath: "/tags",
					body: `{"name":"work"}`, wantStatus: http.StatusConflict},
				{name: "rename tag", method: http.MethodPatch, path: tagPath(reports.ID), specPath: "/tags/{tagId}",
					body: `{"name":"papers"}`, wantStatus: http.StatusOK},
				{name: "move tag to the root", method: http.MethodPatch, path: tagPath(reports.ID), specPath: "/tags/{tagId}",
					body: `{"parentId":0,"afterId":0}`, wantStatus: http.StatusOK},
				{name: "empty tag patch", method: http.MethodPatch, path: tagPath(reports.ID), specPath: "/tags/{tagId}",
					body: `{}`, wantStatus: http.StatusOK},
				{name: "move tag under itself", method: http.MethodPatch, path: tagPath(work.ID), specPath: "/tags/{tagId}",
					body: fmt.Sprintf(`{"parentId":%d}`, work.ID), wantStatus: http.StatusUnprocessableEntity},
				{name: "move tag after a tag of another parent", method: http.MethodPatch, path: tagPath(work.ID),
					specPath: "/tags/{tagId}", body: fmt.Sprintf(`{"afterId":%d}`, childTag(t, deps, admin, work.ID)),
					wantStatus: http.StatusUnprocessableEntity},
				{name: "update tag id past int64", method: http.MethodPatch, path: "/tags/" + tooBig, specPath: "/tags/{tagId}",
					body: `{"name":"x"}`, wantStatus: http.StatusNotFound},
				{name: "rename tag to a sibling's name", method: http.MethodPatch, path: tagPath(reports.ID),
					specPath: "/tags/{tagId}", body: `{"name":"work"}`, wantStatus: http.StatusConflict},
				{name: "update tag malformed json", method: http.MethodPatch, path: tagPath(work.ID), specPath: "/tags/{tagId}",
					body: `{"name":`, wantStatus: http.StatusBadRequest},
				{name: "update missing tag", method: http.MethodPatch, path: tagPath(99999), specPath: "/tags/{tagId}",
					body: `{"name":"x"}`, wantStatus: http.StatusNotFound},
				{name: "update malformed tag id", method: http.MethodPatch, path: "/tags/abc", specPath: "/tags/{tagId}",
					body: `{"name":"x"}`, wantStatus: http.StatusNotFound},
				{name: "update tag of another tenant", method: http.MethodPatch, path: tagPath(foreignTag.ID),
					specPath: "/tags/{tagId}", body: `{"name":"x"}`, wantStatus: http.StatusNotFound},
				{name: "delete tag", method: http.MethodDelete, path: tagPath(doomedTag.ID), specPath: "/tags/{tagId}",
					wantStatus: http.StatusNoContent},
				{name: "delete missing tag", method: http.MethodDelete, path: tagPath(doomedTag.ID), specPath: "/tags/{tagId}",
					wantStatus: http.StatusNotFound},
				{name: "delete malformed tag id", method: http.MethodDelete, path: "/tags/abc", specPath: "/tags/{tagId}",
					wantStatus: http.StatusNotFound},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					rec := serve(r, tc.method, tc.path, tc.body)
					if rec.Code != tc.wantStatus {
						t.Fatalf("status: got %d, want %d; body: %s", rec.Code, tc.wantStatus, rec.Body.String())
					}
					validateResponse(t, doc, tc.method, tc.specPath, rec)
				})
			}
		})
	}
}

// childTag creates a tag under parent and returns its id: a tag that is not a
// sibling of the root tags, for an afterId the API must refuse.
func childTag(t *testing.T, d testDeps, a notes.Actor, parent uint) uint {
	t.Helper()
	tag, err := d.notes.CreateTag(context.Background(), a, notes.TagInput{Name: "child of " + strconv.Itoa(int(parent)), ParentID: parent})
	if err != nil {
		t.Fatal(err)
	}
	return tag.ID
}

// TestCreateTagLocation checks the Location the spec documents on 201 names
// the created tag.
func TestCreateTagLocation(t *testing.T) {
	r := newTestRouter(t, newDeps(t), false, AuthCfg{Method: AuthNone})
	rec := serve(r, http.MethodPost, "/tags", `{"name":"work"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var tag struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tag); err != nil {
		t.Fatal(err)
	}
	if want := apiV0MountPrefix + "/tags/" + strconv.FormatUint(uint64(tag.ID), 10); rec.Header().Get("Location") != want {
		t.Errorf("Location: got %q, want %q", rec.Header().Get("Location"), want)
	}
}

// TestOpenAPIInternalErrorContract covers the documented 500, which a healthy
// module never produces, with a failing fake.
func TestOpenAPIInternalErrorContract(t *testing.T) {
	doc := loadSpec(t)
	h, err := New(Cfg{Notes: failingNotes{}, Tags: failingNotes{}, Tenants: fixedTenants{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []contractCase{
		{method: http.MethodGet, path: "/notes", specPath: "/notes"},
		{method: http.MethodPut, path: "/notes/x/tags/1", specPath: "/notes/{noteId}/tags/{tagId}"},
		{method: http.MethodDelete, path: "/notes/x/tags/1", specPath: "/notes/{noteId}/tags/{tagId}"},
		{method: http.MethodGet, path: "/tags", specPath: "/tags"},
		{method: http.MethodPost, path: "/tags", specPath: "/tags", body: `{"name":"x"}`},
		{method: http.MethodPatch, path: "/tags/1", specPath: "/tags/{tagId}", body: `{"name":"x"}`},
		{method: http.MethodDelete, path: "/tags/1", specPath: "/tags/{tagId}"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := serve(h, tc.method, tc.path, tc.body)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status: got %d, want 500", rec.Code)
			}
			validateResponse(t, doc, tc.method, tc.specPath, rec)
			if strings.Contains(rec.Body.String(), "db down") {
				t.Error("the internal error leaked to the client")
			}
		})
	}
}

// TestUndocumentedRoutesAnswerProblemJSON checks the catch-all handlers of the API.
func TestUndocumentedRoutesAnswerProblemJSON(t *testing.T) {
	r := newTestRouter(t, newDeps(t), false, AuthCfg{})
	tests := []struct {
		method     string
		path       string
		wantStatus int
	}{
		{method: http.MethodGet, path: "/does-not-exist", wantStatus: http.StatusNotFound},
		{method: http.MethodDelete, path: "/notes", wantStatus: http.StatusMethodNotAllowed},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := serve(r, tc.method, tc.path, "")
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("content type: got %q, want application/problem+json", ct)
			}
		})
	}
}

func serve(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, apiV0MountPrefix+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// validateResponse checks that the spec documents the response status and
// content type for the operation, and that the body matches its schema.
func validateResponse(t *testing.T, doc *openapi3.T, method, specPath string, rec *httptest.ResponseRecorder) {
	t.Helper()
	item := doc.Paths.Value(specPath)
	if item == nil {
		t.Fatalf("spec has no path %s", specPath)
	}
	op := item.GetOperation(method)
	if op == nil {
		t.Fatalf("spec has no operation %s %s", method, specPath)
	}
	resp := op.Responses.Status(rec.Code)
	if resp == nil || resp.Value == nil {
		t.Fatalf("spec does not document status %d for %s %s", rec.Code, method, specPath)
	}
	// a response documented without content (204) must come without a body
	if len(resp.Value.Content) == 0 {
		if rec.Body.Len() != 0 {
			t.Errorf("%d %s %s: spec documents no body, got %q", rec.Code, method, specPath, rec.Body.String())
		}
		return
	}
	ct := rec.Header().Get("Content-Type")
	media := resp.Value.Content.Get(ct)
	if media == nil || media.Schema == nil {
		t.Fatalf("spec does not document content type %q for %d of %s %s", ct, rec.Code, method, specPath)
	}

	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v; body: %s", err, rec.Body.String())
	}
	if err := media.Schema.Value.VisitJSON(body, openapi3.EnableJSONSchema2020()); err != nil {
		t.Errorf("response does not match the schema of %d %s %s: %v\nbody: %s",
			rec.Code, method, specPath, err, rec.Body.String())
	}
}

// TestOpenAPIUnauthorizedContract covers the documented 401 of every
// protected operation: proxy mode, no identity header.
func TestOpenAPIUnauthorizedContract(t *testing.T) {
	doc := loadSpec(t)
	r := newTestRouter(t, newDeps(t), false, AuthCfg{Method: AuthProxy, Proxy: ProxyAuthCfg{UserHeader: "Remote-User"}})
	cases := []contractCase{
		{method: http.MethodGet, path: "/me", specPath: "/me"},
		{method: http.MethodGet, path: "/notes", specPath: "/notes"},
		{method: http.MethodPost, path: "/notes", specPath: "/notes", body: `{"title":"x"}`},
		{method: http.MethodGet, path: "/notes/x", specPath: "/notes/{noteId}"},
		{method: http.MethodPut, path: "/notes/x", specPath: "/notes/{noteId}", body: `{"title":"x"}`},
		{method: http.MethodDelete, path: "/notes/x", specPath: "/notes/{noteId}"},
		{method: http.MethodPut, path: "/notes/x/tags/1", specPath: "/notes/{noteId}/tags/{tagId}"},
		{method: http.MethodDelete, path: "/notes/x/tags/1", specPath: "/notes/{noteId}/tags/{tagId}"},
		{method: http.MethodGet, path: "/tags", specPath: "/tags"},
		{method: http.MethodPost, path: "/tags", specPath: "/tags", body: `{"name":"x"}`},
		{method: http.MethodPatch, path: "/tags/1", specPath: "/tags/{tagId}", body: `{"name":"x"}`},
		{method: http.MethodDelete, path: "/tags/1", specPath: "/tags/{tagId}"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := serve(r, tc.method, tc.path, tc.body)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d, want 401", rec.Code)
			}
			validateResponse(t, doc, tc.method, tc.specPath, rec)
		})
	}
}

type failingNotes struct{}

func (failingNotes) List(context.Context, notes.Actor, notes.ListOptions) ([]notes.Note, error) {
	return nil, errors.New("db down")
}

func (failingNotes) Get(context.Context, notes.Actor, string) (notes.Note, error) {
	return notes.Note{}, errors.New("db down")
}

func (failingNotes) Create(context.Context, notes.Actor, notes.Input) (notes.Note, error) {
	return notes.Note{}, errors.New("db down")
}

func (failingNotes) Update(context.Context, notes.Actor, string, notes.Input) (notes.Note, error) {
	return notes.Note{}, errors.New("db down")
}

func (failingNotes) Delete(context.Context, notes.Actor, string) error {
	return errors.New("db down")
}

func (failingNotes) AddNoteTag(context.Context, notes.Actor, string, uint) (notes.Note, error) {
	return notes.Note{}, errors.New("db down")
}

func (failingNotes) RemoveNoteTag(context.Context, notes.Actor, string, uint) (notes.Note, error) {
	return notes.Note{}, errors.New("db down")
}

func (failingNotes) Tags(context.Context, notes.Actor) ([]notes.Tag, error) {
	return nil, errors.New("db down")
}

func (failingNotes) CreateTag(context.Context, notes.Actor, notes.TagInput) (notes.Tag, error) {
	return notes.Tag{}, errors.New("db down")
}

func (failingNotes) UpdateTag(context.Context, notes.Actor, uint, notes.TagPatch) (notes.Tag, error) {
	return notes.Tag{}, errors.New("db down")
}

func (failingNotes) DeleteTag(context.Context, notes.Actor, uint) error {
	return errors.New("db down")
}

// fixedTenants resolves every identity to the same principal, without a db.
type fixedTenants struct{}

func (fixedTenants) Provision(_ context.Context, tenant, subject string) (tenants.Principal, error) {
	return tenants.Principal{TenantID: "t1", TenantSlug: tenant, UserID: "u1", Subject: subject}, nil
}
