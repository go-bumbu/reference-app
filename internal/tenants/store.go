package tenants

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// tenantRow and userRow are the persistence models, private to the module:
// nothing outside this package reads or writes these tables.
type tenantRow struct {
	ID        string    `gorm:"primaryKey;size:16"`
	Slug      string    `gorm:"size:64;not null;uniqueIndex"` // keep in sync with MaxSlugLen
	CreatedAt time.Time `gorm:"not null"`
}

// TableName pins the table name, so renaming the Go type does not move the data.
func (tenantRow) TableName() string { return "tenants" }

type userRow struct {
	ID       string `gorm:"primaryKey;size:16"`
	TenantID string `gorm:"size:16;not null;uniqueIndex:idx_users_tenant_subject"`
	// Subject is unique per tenant, not globally: the same login in two tenants
	// is two users.
	Subject   string    `gorm:"size:255;not null;uniqueIndex:idx_users_tenant_subject"` // keep in sync with MaxSubjectLen
	CreatedAt time.Time `gorm:"not null"`
}

func (userRow) TableName() string { return "users" }

// store is the only gateway to the module's tables; GORM's errors are
// translated here and never leave the module.
type store struct {
	db *gorm.DB
}

func (s *store) migrate(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(&tenantRow{}, &userRow{}); err != nil {
		return fmt.Errorf("migrate tenants: %w", err)
	}
	return nil
}

// ensureTenant returns the id of the tenant with the slug, creating it first if
// needed. The insert ignores a unique conflict, so when two requests race to
// create the same tenant both read back the one row that won.
func (s *store) ensureTenant(ctx context.Context, slug string, genID func() (string, error), now time.Time) (string, error) {
	var row tenantRow
	found, err := s.take(ctx, &row, "slug = ?", slug)
	if err != nil || found {
		return row.ID, wrap("select tenant", err)
	}
	id, err := genID()
	if err != nil {
		return "", err
	}
	ins := tenantRow{ID: id, Slug: slug, CreatedAt: now}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&ins).Error; err != nil {
		return "", fmt.Errorf("insert tenant: %w", err)
	}
	if _, err := s.take(ctx, &row, "slug = ?", slug); err != nil {
		return "", fmt.Errorf("select tenant: %w", err)
	}
	return row.ID, nil
}

// ensureUser does for a user of the tenant what ensureTenant does for a tenant.
func (s *store) ensureUser(ctx context.Context, tenantID, subject string, genID func() (string, error), now time.Time) (string, error) {
	var row userRow
	found, err := s.take(ctx, &row, "tenant_id = ? AND subject = ?", tenantID, subject)
	if err != nil || found {
		return row.ID, wrap("select user", err)
	}
	id, err := genID()
	if err != nil {
		return "", err
	}
	ins := userRow{ID: id, TenantID: tenantID, Subject: subject, CreatedAt: now}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&ins).Error; err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}
	if _, err := s.take(ctx, &row, "tenant_id = ? AND subject = ?", tenantID, subject); err != nil {
		return "", fmt.Errorf("select user: %w", err)
	}
	return row.ID, nil
}

// take loads the single row matching the condition into dst; found is false
// when there is none.
func (s *store) take(ctx context.Context, dst any, query string, args ...any) (found bool, err error) {
	err = s.db.WithContext(ctx).Where(query, args...).Take(dst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}
