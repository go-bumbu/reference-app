// Package tenants is the business module that owns the isolation root of the
// application: tenants, and the users that belong to them.
//
// The model is deliberately small:
//
//	Tenant   an isolated organization; every tenant-scoped row in other modules
//	         carries its ID
//	 └─ User a member of exactly one tenant; the same person in two tenants is
//	         two users
//
// Authentication is not done here. The auth layer in app/ establishes who the
// caller is (a tenant slug and a subject, e.g. from trusted proxy headers) and
// calls Provision, which turns that assertion into a Principal: the tenant and
// user IDs every other module scopes its data by. Unknown tenants and users are
// created on first sight (just-in-time provisioning), so onboarding stays with
// whatever authenticates the caller; this module never sees a password.
package tenants

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
	// MaxSlugLen bounds a tenant slug.
	MaxSlugLen = 64
	// MaxSubjectLen bounds a user subject (the login the auth layer asserts).
	MaxSubjectLen = 255
)

// ValidationError reports an identity assertion that breaks a rule, e.g. an
// empty or overlong subject. Field is "tenant" or "subject".
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Msg)
}

// ErrUserNotFound means no user has the given ID.
var ErrUserNotFound = errors.New("tenants: user not found")

// Principal is a resolved caller: the IDs other modules scope their data by,
// plus the human-readable values they were resolved from.
type Principal struct {
	TenantID   string
	TenantSlug string
	UserID     string
	Subject    string
}

// Cfg holds the dependencies of the Service; zero values get sensible defaults.
type Cfg struct {
	// Now returns the current time; defaults to time.Now. Inject a fixed clock in tests.
	Now func() time.Time
}

// Service manages tenants and users.
type Service struct {
	store *store
	now   func() time.Time
}

// New returns a tenants Service that stores its data in db, creating or
// updating the module's tables first. The caller owns db and closes it.
func New(ctx context.Context, db *gorm.DB, cfg Cfg) (*Service, error) {
	if db == nil {
		return nil, errors.New("tenants: db is required")
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

// Provision resolves the (tenant slug, subject) pair the auth layer asserted
// into a Principal, creating the tenant and the user when they do not exist yet.
// It is idempotent and safe under concurrent first logins of the same identity.
//
// Only call it with an identity the auth layer has verified: whoever can reach
// Provision can create tenants.
func (s *Service) Provision(ctx context.Context, tenantSlug, subject string) (Principal, error) {
	tenantSlug = strings.TrimSpace(tenantSlug)
	subject = strings.TrimSpace(subject)
	if err := validate(tenantSlug, subject); err != nil {
		return Principal{}, err
	}

	now := s.now().UTC().Truncate(time.Microsecond)
	tenantID, err := s.store.ensureTenant(ctx, tenantSlug, newID, now)
	if err != nil {
		return Principal{}, err
	}
	userID, err := s.store.ensureUser(ctx, tenantID, subject, newID, now)
	if err != nil {
		return Principal{}, err
	}
	return Principal{TenantID: tenantID, TenantSlug: tenantSlug, UserID: userID, Subject: subject}, nil
}

// PrincipalByUserID returns the Principal of an existing user, found by its ID,
// or ErrUserNotFound. It never creates anything: it resolves the owner of a
// credential the app issued itself (an API key), whose identity no
// authenticator asserted on this request.
func (s *Service) PrincipalByUserID(ctx context.Context, userID string) (Principal, error) {
	if userID == "" {
		return Principal{}, ErrUserNotFound
	}
	p, found, err := s.store.principalByUserID(ctx, userID)
	if err != nil {
		return Principal{}, err
	}
	if !found {
		return Principal{}, ErrUserNotFound
	}
	return p, nil
}

func validate(tenantSlug, subject string) error {
	switch {
	case tenantSlug == "":
		return &ValidationError{Field: "tenant", Msg: "must not be empty"}
	case utf8.RuneCountInString(tenantSlug) > MaxSlugLen:
		return &ValidationError{Field: "tenant", Msg: fmt.Sprintf("must be at most %d characters", MaxSlugLen)}
	case subject == "":
		return &ValidationError{Field: "subject", Msg: "must not be empty"}
	case utf8.RuneCountInString(subject) > MaxSubjectLen:
		return &ValidationError{Field: "subject", Msg: fmt.Sprintf("must be at most %d characters", MaxSubjectLen)}
	}
	return nil
}

// newID returns a random 16 hex characters id.
func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
