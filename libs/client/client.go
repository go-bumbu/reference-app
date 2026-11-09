// Package client is a Go client for the /api/v0 HTTP API described in
// docs/openapi/api-v0.yaml.
//
// It is written by hand against the spec and depends on the standard library
// only: it shares no types with the server, so the wire format is the one
// contract between them (app/router's client test runs it against the real
// router to keep both honest).
//
// Errors returned by the API are *Problem values (RFC 9457); compare them with
// errors.Is against ErrNotFound, ErrUnauthorized and ErrValidation, or use
// errors.As for the details.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// apiPrefix is the mount point of the API, the spec's `servers` entry.
const apiPrefix = "/api/v0"

// maxResponseBytes caps how much of a response body is read.
const maxResponseBytes = 10 << 20

// Client calls the API. It is safe for concurrent use.
type Client struct {
	base    string // escaped URL of the API mount, without a trailing slash
	http    *http.Client
	headers http.Header
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the http.Client used for requests; the default is a
// client with a 30 seconds timeout.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// WithHeader adds a header to every request, e.g. the identity headers a
// trusted proxy would set when talking to the server directly in development.
func WithHeader(key, value string) Option {
	return func(c *Client) { c.headers.Add(key, value) }
}

// New returns a Client for the server at baseURL (scheme and host, optionally
// a path prefix the server is mounted under), e.g. "http://localhost:8080".
func New(baseURL string, opts ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("client: invalid base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("client: base URL %q must use http or https", baseURL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("client: base URL %q has no host", baseURL)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("client: base URL %q must not have a query or fragment", baseURL)
	}
	c := &Client{
		base:    strings.TrimSuffix(u.String(), "/") + apiPrefix,
		http:    &http.Client{Timeout: 30 * time.Second},
		headers: http.Header{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Info is the server's build information.
type Info struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
	Commit    string `json:"commit"`
	GoVersion string `json:"goVersion"`
}

// Me is the authenticated caller.
type Me struct {
	UserID   string `json:"userId"`
	Subject  string `json:"subject"`
	TenantID string `json:"tenantId"`
	Tenant   string `json:"tenant"`
}

// Note is a note of the caller's tenant.
type Note struct {
	ID        string    `json:"id"`
	CreatedBy string    `json:"createdBy"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// NoteInput holds the fields of a new note.
type NoteInput struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
}

// Info returns the server's build information; it needs no authentication.
func (c *Client) Info(ctx context.Context) (Info, error) {
	var out Info
	err := c.do(ctx, http.MethodGet, "/info", nil, http.StatusOK, &out)
	return out, err
}

// Me returns the authenticated caller.
func (c *Client) Me(ctx context.Context) (Me, error) {
	var out Me
	err := c.do(ctx, http.MethodGet, "/me", nil, http.StatusOK, &out)
	return out, err
}

// ListNotes returns the notes of the caller's tenant, newest first.
func (c *Client) ListNotes(ctx context.Context) ([]Note, error) {
	var out struct {
		Items []Note `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/notes", nil, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// GetNote returns a note of the caller's tenant; a missing note, or one of
// another tenant, is ErrNotFound.
func (c *Client) GetNote(ctx context.Context, id string) (Note, error) {
	var out Note
	err := c.do(ctx, http.MethodGet, "/notes/"+url.PathEscape(id), nil, http.StatusOK, &out)
	return out, err
}

// CreateNote creates a note in the caller's tenant; input breaking a rule is
// ErrValidation, with the failing fields in Problem.Errors.
func (c *Client) CreateNote(ctx context.Context, in NoteInput) (Note, error) {
	var out Note
	err := c.do(ctx, http.MethodPost, "/notes", in, http.StatusCreated, &out)
	return out, err
}

// UpdateNote replaces the title and body of a note of the caller's tenant;
// a missing note is ErrNotFound, input breaking a rule is ErrValidation.
func (c *Client) UpdateNote(ctx context.Context, id string, in NoteInput) (Note, error) {
	var out Note
	err := c.do(ctx, http.MethodPut, "/notes/"+url.PathEscape(id), in, http.StatusOK, &out)
	return out, err
}

// DeleteNote deletes a note of the caller's tenant; a missing note is ErrNotFound.
func (c *Client) DeleteNote(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/notes/"+url.PathEscape(id), nil, http.StatusNoContent, nil)
}

// do sends the request and decodes a wantStatus response into out; any other
// status becomes a *Problem. path must already be escaped.
func (c *Client) do(ctx context.Context, method, path string, body any, wantStatus int, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("client: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reqBody)
	if err != nil {
		return fmt.Errorf("client: build request: %w", err)
	}
	for k, v := range c.headers {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "application/json, application/problem+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("client: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("client: %s %s: read response: %w", method, path, err)
	}

	if resp.StatusCode != wantStatus {
		return newProblem(resp, data)
	}
	if out == nil { // a response without a body, e.g. 204
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("client: %s %s: decode response: %w", method, path, err)
	}
	return nil
}

// Sentinel errors a *Problem matches with errors.Is, by status code.
var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrNotFound     = errors.New("not found")
	ErrValidation   = errors.New("validation failed")
)

// Problem is an error response of the API: an RFC 9457 problem details body,
// or, when the response is not one (e.g. a proxy's error page), the status
// with the start of the body as Detail.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Reference string       `json:"reference,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

// FieldError is one failing field of a validation problem.
type FieldError struct {
	// Pointer is a JSON pointer to the field, e.g. "/title".
	Pointer string `json:"pointer"`
	Detail  string `json:"detail"`
}

func (p *Problem) Error() string {
	msg := fmt.Sprintf("api error %d", p.Status)
	if p.Title != "" {
		msg += " " + p.Title
	}
	if p.Detail != "" {
		msg += ": " + p.Detail
	}
	return msg
}

// Is maps the status code onto the package's sentinel errors.
func (p *Problem) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return p.Status == http.StatusUnauthorized
	case ErrNotFound:
		return p.Status == http.StatusNotFound
	case ErrValidation:
		return p.Status == http.StatusUnprocessableEntity
	}
	return false
}

// Slug is the stable last path segment of Type, e.g. "not_found".
func (p *Problem) Slug() string {
	return p.Type[strings.LastIndex(p.Type, "/")+1:]
}

// maxDetailLen bounds the Detail taken from a body that is not problem+json.
const maxDetailLen = 512

func newProblem(resp *http.Response, body []byte) *Problem {
	p := &Problem{}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json") {
		_ = json.Unmarshal(body, p) // a broken body still yields the status below
	} else {
		p.Detail = strings.TrimSpace(string(body))
		if len(p.Detail) > maxDetailLen {
			p.Detail = strings.ToValidUTF8(p.Detail[:maxDetailLen], "")
		}
	}
	p.Status = resp.StatusCode
	if p.Title == "" {
		p.Title = http.StatusText(resp.StatusCode)
	}
	return p
}
