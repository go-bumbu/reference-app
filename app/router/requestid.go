package router

import (
	"context"
	"crypto/rand"
	"net/http"
)

// requestIDHeader carries the request id: read from the request when a proxy
// in front already assigned one, and always echoed on the response.
const requestIDHeader = "Request-Id"

// maxRequestIDLen bounds an incoming id; anything longer is replaced.
const maxRequestIDLen = 64

type requestIDKey struct{}

// withRequestID gives every request an id, stored in the context for
// requestID and echoed in the Request-Id response header. An incoming
// Request-Id is kept when it is short and made of [A-Za-z0-9._-], so a proxy's
// id correlates across hops; otherwise a random one is generated, so a client
// cannot write arbitrary text into the logs.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID(id) {
			id = rand.Text()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// requestID returns the id withRequestID assigned, or "" outside it.
func requestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for _, c := range id {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
		if !ok {
			return false
		}
	}
	return true
}
