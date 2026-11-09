// Package spa embeds the web UI (webui/) into the binary and serves it.
//
// `make package-ui` builds the Vue app and copies it into files/ui; without it
// the directory only holds a .gitkeep, and Handler serves a page that says so
// instead of failing, so a plain `go build` keeps working.
package spa

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"

	spah "github.com/go-bumbu/http/spa"
)

//go:embed all:files/ui
var uiFiles embed.FS

const uiDir = "files/ui"

// Handler serves the embedded UI: its assets by path, and index.html for every
// other path, so the client-side router handles deep links and reloads.
func Handler() (http.Handler, error) {
	return handler(uiFiles, uiDir)
}

func handler(fsys fs.FS, dir string) (http.Handler, error) {
	if _, err := fs.Stat(fsys, dir+"/index.html"); errors.Is(err, fs.ErrNotExist) {
		return notBuilt(), nil
	}
	return spah.New(spah.Cfg{FS: fsys, SubDir: dir})
}

func notBuilt() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("The web UI is not part of this build: run `make package-ui` and rebuild.\n"))
	})
}
