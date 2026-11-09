package router

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricsEndpoint = "/metrics"
	healthEndpoint  = "/health"
	readyEndpoint   = "/ready"
)

// Observability builds the handler of the observability server: Prometheus
// metrics plus the health and readiness probes. It is served on its own port so
// it can stay off the public interface.
func Observability() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET "+metricsEndpoint, promhttp.Handler())

	// the ready endpoint should be used by kubernetes to determine startup ready state of the service
	// in the handler you can add some internal logic to switch to ready once startup is complete,
	// e.g. if you need to download a cache or similar
	mux.HandleFunc("GET "+readyEndpoint, textHandlerFunc("OK"))

	// the health endpoint should be used by kubernetes to determine the health of the system
	// in the handler you can add some internal logic to switch the health state if the
	// service is misbehaving.
	// NOTE: if you bind the health endpoint to e.g. the database connectivity, if you loose db network
	// all your containers will be unhealthy and restarted by kubernetes.
	mux.HandleFunc("GET "+healthEndpoint, textHandlerFunc("OK"))

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<a href="%[1]s">%[1]s</a><br><a href="%[2]s">%[2]s</a><br><a href="%[3]s">%[3]s</a>`,
			metricsEndpoint, healthEndpoint, readyEndpoint)
	})
	return mux
}

// textHandlerFunc writes a fixed plain-text body.
func textHandlerFunc(text string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, text)
	}
}
