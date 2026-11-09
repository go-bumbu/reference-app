package log

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name               string
		method             string
		path               string
		correlationID      string
		handlerStatusCode  int
		expectedLogLevel   string
		expectedStatusCode int
	}{
		{
			name:               "successful GET request",
			method:             http.MethodGet,
			path:               "/api/v1/test",
			correlationID:      "test-correlation-id-123",
			handlerStatusCode:  http.StatusOK,
			expectedLogLevel:   "INFO",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "successful POST request",
			method:             http.MethodPost,
			path:               "/api/v1/create",
			correlationID:      "test-correlation-id-456",
			handlerStatusCode:  http.StatusCreated,
			expectedLogLevel:   "INFO",
			expectedStatusCode: http.StatusCreated,
		},
		{
			name:               "internal server error request",
			method:             http.MethodGet,
			path:               "/api/v1/error",
			correlationID:      "test-correlation-id-789",
			handlerStatusCode:  http.StatusInternalServerError,
			expectedLogLevel:   "ERROR",
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:               "bad gateway error",
			method:             http.MethodGet,
			path:               "/api/v1/gateway",
			correlationID:      "test-correlation-id-bad-gateway",
			handlerStatusCode:  http.StatusBadGateway,
			expectedLogLevel:   "ERROR",
			expectedStatusCode: http.StatusBadGateway,
		},
		{
			name:               "service unavailable error",
			method:             http.MethodGet,
			path:               "/api/v1/unavailable",
			correlationID:      "test-correlation-id-unavailable",
			handlerStatusCode:  http.StatusServiceUnavailable,
			expectedLogLevel:   "ERROR",
			expectedStatusCode: http.StatusServiceUnavailable,
		},
		{
			name:               "client error 4xx should log as INFO",
			method:             http.MethodGet,
			path:               "/api/v1/notfound",
			correlationID:      "test-correlation-id-notfound",
			handlerStatusCode:  http.StatusNotFound,
			expectedLogLevel:   "INFO",
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:               "no correlation ID header",
			method:             http.MethodGet,
			path:               "/api/v1/test",
			correlationID:      "",
			handlerStatusCode:  http.StatusOK,
			expectedLogLevel:   "INFO",
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			// Create a test handler that returns the specified status code
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.handlerStatusCode != http.StatusOK {
					w.WriteHeader(tt.handlerStatusCode)
				}
				_, _ = w.Write([]byte("test response"))
			})

			// Wrap the handler with logging middleware
			handler := LoggingMiddleware(testHandler, logger)

			// Create a test request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"
			if tt.correlationID != "" {
				req.Header.Set("x-correlation-id", tt.correlationID)
			}

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Serve the request
			handler.ServeHTTP(rr, req)

			// Verify response status code
			if rr.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rr.Code)
			}

			// Verify log output
			logOutput := logBuffer.String()

			// Check log level
			if !strings.Contains(logOutput, `"level":"`+tt.expectedLogLevel+`"`) {
				t.Errorf("expected log level %s, log output: %s", tt.expectedLogLevel, logOutput)
			}

			// Check method is logged
			if !strings.Contains(logOutput, `"method":"`+tt.method+`"`) {
				t.Errorf("expected method %s in log, got: %s", tt.method, logOutput)
			}

			// Check path is logged
			if !strings.Contains(logOutput, `"path":"`+tt.path+`"`) {
				t.Errorf("expected path %s in log, got: %s", tt.path, logOutput)
			}

			// Check host is logged
			if !strings.Contains(logOutput, `"host":"example.com"`) {
				t.Errorf("expected host in log, got: %s", logOutput)
			}

			// Check correlation ID is logged
			if tt.correlationID != "" {
				if !strings.Contains(logOutput, `"x-correlation-id":"`+tt.correlationID+`"`) {
					t.Errorf("expected correlation ID %s in log, got: %s", tt.correlationID, logOutput)
				}
			}

			// Check response code is logged
			if !strings.Contains(logOutput, `"response-code":`+strconv.Itoa(tt.expectedStatusCode)) {
				t.Errorf("expected response code %d in log, got: %s", tt.expectedStatusCode, logOutput)
			}

			// Check duration is logged
			if !strings.Contains(logOutput, `"duration"`) {
				t.Errorf("expected duration in log, got: %s", logOutput)
			}
		})
	}
}

type testContextKey string

func TestLoggingMiddleware_Context(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create a test handler that checks the context
	const contextKey testContextKey = "test-key"
	contextValue := "test-value"
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the context is passed through
		if val := r.Context().Value(contextKey); val != contextValue {
			t.Errorf("context not passed through correctly")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := LoggingMiddleware(testHandler, logger)

	// Create a request with a context value
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), contextKey, contextValue)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestLoggingMiddleware_Duration(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	sleepDuration := 100 * time.Millisecond
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(sleepDuration)
		w.WriteHeader(http.StatusOK)
	})

	handler := LoggingMiddleware(testHandler, logger)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(rr, req)
	elapsed := time.Since(start)

	// Verify that the handler actually waited
	if elapsed < sleepDuration {
		t.Errorf("handler did not sleep long enough: %v < %v", elapsed, sleepDuration)
	}

	// The log should contain a duration field
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, `"duration"`) {
		t.Errorf("expected duration in log output, got: %s", logOutput)
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedStatus int
	}{
		{
			name:           "status 200 OK",
			statusCode:     http.StatusOK,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "status 201 Created",
			statusCode:     http.StatusCreated,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "status 404 Not Found",
			statusCode:     http.StatusNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "status 500 Internal Server Error",
			statusCode:     http.StatusInternalServerError,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			rw := &responseWriter{
				ResponseWriter: rr,
				statusCode:     http.StatusOK,
			}

			rw.WriteHeader(tt.statusCode)

			if rw.statusCode != tt.expectedStatus {
				t.Errorf("expected status code %d, got %d", tt.expectedStatus, rw.statusCode)
			}

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected underlying response writer status code %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestResponseWriter_DefaultStatusCode(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: rr,
		statusCode:     http.StatusOK,
	}

	// When WriteHeader is not called explicitly, the default should remain
	if rw.statusCode != http.StatusOK {
		t.Errorf("expected default status code %d, got %d", http.StatusOK, rw.statusCode)
	}
}

func TestHttpIsErr(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{
			name:       "200 OK is not an error",
			statusCode: http.StatusOK,
			expected:   false,
		},
		{
			name:       "201 Created is not an error",
			statusCode: http.StatusCreated,
			expected:   false,
		},
		{
			name:       "204 No Content is not an error",
			statusCode: http.StatusNoContent,
			expected:   false,
		},
		{
			name:       "301 Moved Permanently is not an error",
			statusCode: http.StatusMovedPermanently,
			expected:   false,
		},
		{
			name:       "400 Bad Request is not an error",
			statusCode: http.StatusBadRequest,
			expected:   false,
		},
		{
			name:       "401 Unauthorized is not an error",
			statusCode: http.StatusUnauthorized,
			expected:   false,
		},
		{
			name:       "404 Not Found is not an error",
			statusCode: http.StatusNotFound,
			expected:   false,
		},
		{
			name:       "500 Internal Server Error is an error",
			statusCode: http.StatusInternalServerError,
			expected:   true,
		},
		{
			name:       "501 Not Implemented is an error",
			statusCode: http.StatusNotImplemented,
			expected:   true,
		},
		{
			name:       "502 Bad Gateway is an error",
			statusCode: http.StatusBadGateway,
			expected:   true,
		},
		{
			name:       "503 Service Unavailable is an error",
			statusCode: http.StatusServiceUnavailable,
			expected:   true,
		},
		{
			name:       "504 Gateway Timeout is an error",
			statusCode: http.StatusGatewayTimeout,
			expected:   true,
		},
		{
			name:       "599 is an error (upper boundary)",
			statusCode: 599,
			expected:   true,
		},
		{
			name:       "600 is not an error (beyond 5xx range)",
			statusCode: 600,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := httpIsErr(tt.statusCode)
			if result != tt.expected {
				t.Errorf("httpIsErr(%d) = %v, expected %v", tt.statusCode, result, tt.expected)
			}
		})
	}
}

func BenchmarkLoggingMiddleware(b *testing.B) {

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	})

	handler := LoggingMiddleware(testHandler, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Host = "example.com"
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("x-correlation-id", "benchmark-correlation-id")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logBuffer.Reset()
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}
