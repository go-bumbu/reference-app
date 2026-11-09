package handlers

import (
	"net/http"
	"runtime"

	"github.com/go-bumbu/reference-app/app/metainfo"
)

// infoResponse is the body of GET /info (schema Info in docs/openapi/api-v0.yaml).
type infoResponse struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
	Commit    string `json:"commit"`
	GoVersion string `json:"goVersion"`
}

// InfoHandler reports the build information baked into the binary; the values
// come from -ldflags at release time.
func InfoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, infoResponse{
			Version:   metainfo.Version,
			BuildTime: metainfo.BuildTime,
			Commit:    metainfo.ShaVer,
			GoVersion: runtime.Version(),
		})
	})
}
