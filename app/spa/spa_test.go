package spa

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func serve(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandlerServesTheBuiltUI(t *testing.T) {
	fsys := fstest.MapFS{
		"ui/index.html":    {Data: []byte("<html>app</html>")},
		"ui/assets/app.js": {Data: []byte("console.log(1)")},
		"ui/.gitkeep":      {Data: nil},
		"other/secret.txt": {Data: []byte("not served")},
	}
	h, err := handler(fsys, "ui")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path     string
		wantCode int
		wantBody string
	}{
		{path: "/", wantCode: http.StatusOK, wantBody: "app"},
		{path: "/assets/app.js", wantCode: http.StatusOK, wantBody: "console.log"},
		{path: "/notes/abc", wantCode: http.StatusOK, wantBody: "app"}, // deep link: index.html
		{path: "/.gitkeep", wantCode: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			rec := serve(t, h, tc.path)
			if rec.Code != tc.wantCode {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantCode)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body: got %q, want it to contain %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestHandlerWithoutABuild(t *testing.T) {
	h, err := handler(fstest.MapFS{"ui/.gitkeep": {}}, "ui")
	if err != nil {
		t.Fatal(err)
	}
	rec := serve(t, h, "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make package-ui") {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
}

// TestEmbeddedHandler builds the real handler, whether or not the UI was packaged.
func TestEmbeddedHandler(t *testing.T) {
	h, err := Handler()
	if err != nil {
		t.Fatal(err)
	}
	if code := serve(t, h, "/").Code; code != http.StatusOK && code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d", code)
	}
}
