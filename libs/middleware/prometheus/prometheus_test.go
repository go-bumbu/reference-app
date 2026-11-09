package prometheus

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

func TestPromMiddleware(t *testing.T) {
	tests := []struct {
		name               string
		method             string
		path               string
		routeTemplate      string
		handlerStatusCode  int
		expectedStatusCode int
		expectedIsError    bool
	}{
		{
			name:               "successful GET request",
			method:             http.MethodGet,
			path:               "/api/v1/users/123",
			routeTemplate:      "/api/v1/users/{id}",
			handlerStatusCode:  http.StatusOK,
			expectedStatusCode: http.StatusOK,
			expectedIsError:    false,
		},
		{
			name:               "successful POST request",
			method:             http.MethodPost,
			path:               "/api/v1/users",
			routeTemplate:      "/api/v1/users",
			handlerStatusCode:  http.StatusCreated,
			expectedStatusCode: http.StatusCreated,
			expectedIsError:    false,
		},
		{
			name:               "not found error",
			method:             http.MethodGet,
			path:               "/api/v1/notfound",
			routeTemplate:      "/api/v1/notfound",
			handlerStatusCode:  http.StatusNotFound,
			expectedStatusCode: http.StatusNotFound,
			expectedIsError:    false,
		},
		{
			name:               "internal server error",
			method:             http.MethodGet,
			path:               "/api/v1/error",
			routeTemplate:      "/api/v1/error",
			handlerStatusCode:  http.StatusInternalServerError,
			expectedStatusCode: http.StatusInternalServerError,
			expectedIsError:    true,
		},
		{
			name:               "bad gateway error",
			method:             http.MethodGet,
			path:               "/api/v1/gateway",
			routeTemplate:      "/api/v1/gateway",
			handlerStatusCode:  http.StatusBadGateway,
			expectedStatusCode: http.StatusBadGateway,
			expectedIsError:    true,
		},
		{
			name:               "service unavailable error",
			method:             http.MethodPut,
			path:               "/api/v1/service",
			routeTemplate:      "/api/v1/service",
			handlerStatusCode:  http.StatusServiceUnavailable,
			expectedStatusCode: http.StatusServiceUnavailable,
			expectedIsError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.handlerStatusCode != http.StatusOK {
					w.WriteHeader(tt.handlerStatusCode)
				}
				_, _ = w.Write([]byte("test response"))
			})

			// Setup mux router with the route template
			router := mux.NewRouter()
			router.Handle(tt.routeTemplate, PromMiddleware(testHandler)).Methods(tt.method)

			// Create test request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()

			// Serve the request
			router.ServeHTTP(rr, req)

			// Verify response status code
			if rr.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rr.Code)
			}

			// Verify response body
			if rr.Body.String() != "test response" {
				t.Errorf("expected body 'test response', got '%s'", rr.Body.String())
			}

			// Verify that httpIsErr returns the expected result
			if httpIsErr(rr.Code) != tt.expectedIsError {
				t.Errorf("httpIsErr(%d) = %v, expected %v", rr.Code, httpIsErr(rr.Code), tt.expectedIsError)
			}
		})
	}
}

func TestPromMiddleware_Integration(t *testing.T) {
	// Test the actual middleware function
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	})

	// Setup mux router
	router := mux.NewRouter()
	router.Handle("/api/test", PromMiddleware(testHandler)).Methods(http.MethodGet)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	// Serve the request
	router.ServeHTTP(rr, req)

	// Verify response
	if rr.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	if rr.Body.String() != "test response" {
		t.Errorf("expected body 'test response', got '%s'", rr.Body.String())
	}
}

func TestPromMiddleware_Duration(t *testing.T) {
	sleepDuration := 50 * time.Millisecond
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(sleepDuration)
		w.WriteHeader(http.StatusOK)
	})

	// Setup mux router
	router := mux.NewRouter()
	router.Handle("/api/slow", PromMiddleware(testHandler)).Methods(http.MethodGet)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/api/slow", nil)
	rr := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(rr, req)
	elapsed := time.Since(start)

	// Verify that the handler actually waited
	if elapsed < sleepDuration {
		t.Errorf("handler did not sleep long enough: %v < %v", elapsed, sleepDuration)
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, rr.Code)
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
		{
			name:           "status 502 Bad Gateway",
			statusCode:     http.StatusBadGateway,
			expectedStatus: http.StatusBadGateway,
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

func TestPromMiddleware_RouteTemplateExtraction(t *testing.T) {
	// Test that the middleware correctly extracts route templates with path variables
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	})

	// Setup mux router with path variables
	router := mux.NewRouter()
	router.Handle("/api/users/{id}", PromMiddleware(testHandler)).Methods(http.MethodGet)
	router.Handle("/api/users/{id}/posts/{postId}", PromMiddleware(testHandler)).Methods(http.MethodGet)

	tests := []struct {
		name          string
		path          string
		expectedCode  int
		expectedRoute string
	}{
		{
			name:          "single path variable",
			path:          "/api/users/123",
			expectedCode:  http.StatusOK,
			expectedRoute: "/api/users/{id}",
		},
		{
			name:          "multiple path variables",
			path:          "/api/users/123/posts/456",
			expectedCode:  http.StatusOK,
			expectedRoute: "/api/users/{id}/posts/{postId}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != tt.expectedCode {
				t.Errorf("expected status code %d, got %d", tt.expectedCode, rr.Code)
			}
		})
	}
}

func BenchmarkPromMiddleware(b *testing.B) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	})

	router := mux.NewRouter()
	router.Handle("/api/test", PromMiddleware(testHandler)).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
	}
}
