package prometheus

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_duration_seconds",
		Help: "Duration of HTTP requests for different paths, methods, status codes",
	},
		[]string{
			"type",
			"status",
			"method",
			"addr",
			"isError",
		},
	)
)

// PromMiddleware implements mux.MiddlewareFunc.
func PromMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		timeStart := time.Now()
		respWriter := responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		route := mux.CurrentRoute(r)
		path, _ := route.GetPathTemplate()

		next.ServeHTTP(&respWriter, r)

		isErrorStr := strconv.FormatBool(httpIsErr(respWriter.statusCode))
		statusCodeStr := strconv.Itoa(respWriter.statusCode)
		httpDuration.With(prometheus.Labels{
			"type":    r.Proto,
			"status":  statusCodeStr,
			"method":  r.Method,
			"addr":    path,
			"isError": isErrorStr,
		}).Observe(time.Since(timeStart).Seconds())
	})
}

// responseWriter takes extends a http response writer, to be able to extract the response status code
// before writing to the output.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader reads the status code written to the request and stores in the intermediate struct for later usage
func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// httpIsErr determines if the status code is considered an error, and returns a bool accordingly
func httpIsErr(statusCode int) bool {
	return statusCode < 600 && statusCode >= 500
}
