// Package notes is a prototype business module: it owns one business concern
// (short text notes), its data and a small public API.
//
// It shows the shape every module in internal/ follows:
//   - it knows nothing about HTTP, JSON or the app: app/ translates requests
//     into calls to this API and maps the results back,
//   - it owns its data: nothing outside this package reads or writes the store,
//   - it reports failures as typed errors (ErrNotFound, *ValidationError) that
//     the transport layer maps to status codes,
//   - every call is scoped to the caller's tenant (Actor): a note of another
//     tenant does not exist for it, and the store applies the filter itself, so
//     no caller can forget it,
//   - shared runtime dependencies (db, logger, clock) are passed in by app/,
//     never instantiated here.
//
// Persistence goes through GORM (store.go): the module receives a *gorm.DB
// and migrates its own tables, so it runs unchanged on SQLite or Postgres.
package notes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

const (
	MaxTitleLen = 200
	MaxBodyLen  = 10_000
)

// ErrNotFound is returned when a note does not exist.
var ErrNotFound = errors.New("note not found")

// ValidationError reports an input that breaks a business rule.
// Field is the name of the offending input field.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Msg)
}

// Actor is who a call is made for: the tenant whose notes it may touch and the
// user acting. The IDs come from the tenants module, resolved by app/ from the
// authenticated request; this module only uses them as opaque scope keys.
type Actor struct {
	TenantID string
	UserID   string
}

// ErrNoTenant is returned when a call has no tenant to scope it to. It is a
// wiring bug, never a user error: fail closed instead of reading every tenant.
var ErrNoTenant = errors.New("notes: actor has no tenant")

// Note is a single note. Notes are shared by every user of their tenant;
// CreatedBy is the id of the user who wrote it.
type Note struct {
	ID        string
	TenantID  string
	CreatedBy string
	Title     string
	Body      string
	CreatedAt time.Time
}

// Input holds the user-provided fields of a note.
type Input struct {
	Title string
	Body  string
}

// Cfg holds the dependencies of the Service; zero values get sensible defaults.
type Cfg struct {
	// Now returns the current time; defaults to time.Now. Inject a fixed clock in tests.
	Now func() time.Time
}

// Service manages notes.
type Service struct {
	store *store
	now   func() time.Time
}

// New returns a notes Service that stores its data in db, creating or
// updating the module's tables first. The caller owns db and closes it.
func New(ctx context.Context, db *gorm.DB, cfg Cfg) (*Service, error) {
	if db == nil {
		return nil, errors.New("notes: db is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Service{store: &store{db: db}, now: cfg.Now}
	if err := s.store.migrate(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Create validates the input and stores a new note in the actor's tenant.
func (s *Service) Create(ctx context.Context, a Actor, in Input) (Note, error) {
	if a.TenantID == "" {
		return Note{}, ErrNoTenant
	}
	in.Title = strings.TrimSpace(in.Title)
	if err := validate(in); err != nil {
		return Note{}, err
	}
	id, err := newID()
	if err != nil {
		return Note{}, fmt.Errorf("generate note id: %w", err)
	}
	n := Note{
		ID:        id,
		TenantID:  a.TenantID,
		CreatedBy: a.UserID,
		Title:     in.Title,
		Body:      in.Body,
		// microseconds: the finest precision every supported database keeps,
		// so the returned note equals the one read back later
		CreatedAt: s.now().UTC().Truncate(time.Microsecond),
	}
	if err := s.store.create(ctx, n); err != nil {
		return Note{}, err
	}
	return n, nil
}

// Get returns the note with the given id in the actor's tenant, or
// ErrNotFound; a note of another tenant is not found either.
func (s *Service) Get(ctx context.Context, a Actor, id string) (Note, error) {
	if a.TenantID == "" {
		return Note{}, ErrNoTenant
	}
	return s.store.get(ctx, a.TenantID, id)
}

// Update replaces the title and body of the note with the given id in the
// actor's tenant and returns the updated note, or ErrNotFound. Any user of the
// tenant may edit any of its notes: notes are shared, and the author and
// creation time stay as they were.
func (s *Service) Update(ctx context.Context, a Actor, id string, in Input) (Note, error) {
	if a.TenantID == "" {
		return Note{}, ErrNoTenant
	}
	in.Title = strings.TrimSpace(in.Title)
	if err := validate(in); err != nil {
		return Note{}, err
	}
	return s.store.update(ctx, a.TenantID, id, in)
}

// Delete removes the note with the given id from the actor's tenant, or
// returns ErrNotFound. Like Update, any user of the tenant may delete it.
func (s *Service) Delete(ctx context.Context, a Actor, id string) error {
	if a.TenantID == "" {
		return ErrNoTenant
	}
	return s.store.delete(ctx, a.TenantID, id)
}

// List returns the notes of the actor's tenant, newest first.
func (s *Service) List(ctx context.Context, a Actor) ([]Note, error) {
	if a.TenantID == "" {
		return nil, ErrNoTenant
	}
	return s.store.list(ctx, a.TenantID)
}

func validate(in Input) error {
	switch {
	case in.Title == "":
		return &ValidationError{Field: "title", Msg: "must not be empty"}
	case utf8.RuneCountInString(in.Title) > MaxTitleLen:
		return &ValidationError{Field: "title", Msg: fmt.Sprintf("must be at most %d characters", MaxTitleLen)}
	case utf8.RuneCountInString(in.Body) > MaxBodyLen:
		return &ValidationError{Field: "body", Msg: fmt.Sprintf("must be at most %d characters", MaxBodyLen)}
	}
	return nil
}

// newID returns a random 16 hex characters id.
func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
