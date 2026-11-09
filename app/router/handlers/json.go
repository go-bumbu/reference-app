package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxBodyBytes caps the size of a JSON request body.
const maxBodyBytes = 1 << 20

// SlugBadRequest is the problem slug for a body that is not valid JSON or has
// the wrong shape. The generic slugs (not_found, validation_error, internal, ...)
// ship with problemjson.
const SlugBadRequest = "bad_request"

// writeJSON writes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSON decodes a single JSON object from the request body into dst.
// Unknown fields, trailing data and bodies over maxBodyBytes are rejected.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON body: must contain a single JSON object")
	}
	return nil
}
