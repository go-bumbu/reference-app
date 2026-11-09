package log

import (
	"log/slog"
	"net/http"
	"time"
)

// LoggingMiddleware logs every request using the provided logger
func LoggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		timeStart := time.Now()
		respWriter := responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		next.ServeHTTP(&respWriter, r)

		level := slog.LevelVar{}
		if httpIsErr(respWriter.statusCode) {
			level.Set(slog.LevelError)
		} else {
			// regular requests are logged, change to debug if we do not want that
			level.Set(slog.LevelInfo)
		}
		logger.LogAttrs(
			r.Context(),
			level.Level(),
			"http request/response",
			slog.String("method", r.Method),
			slog.String("host", r.Host),
			slog.String("path", r.URL.Path),
			slog.String("remote-address", r.RemoteAddr),
			//slog.String("user-agent", r.UserAgent()),
			slog.String("x-correlation-id", r.Header.Get("x-correlation-id")),
			slog.Duration("duration", time.Since(timeStart)),
			slog.Int("response-code", respWriter.statusCode),
		)
	})
}

// responseWriter takes extends a http response writer, to be able to extract the response status code
// before writing to the output.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader overlays the original method of http.ResponseWriter
// it reads the status code written to the request and stores in the intermediate struct for later usage
func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// httpIsErr determines if the status code is considered an error, and returns a bool accordingly
func httpIsErr(statusCode int) bool {
	return statusCode < 600 && statusCode >= 500
}
