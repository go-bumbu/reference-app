package notes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// noteRow is the persistence model of a Note. It is private to the module:
// nothing outside this package reads or writes the notes table, and the domain
// type Note stays free of storage tags, so the schema can change without
// touching the public API.
type noteRow struct {
	ID string `gorm:"primaryKey;size:16"`
	// TenantID scopes the row; the composite index serves List, which filters by
	// tenant and orders by creation time.
	TenantID  string    `gorm:"size:16;not null;index:idx_notes_tenant_created,priority:1"`
	CreatedBy string    `gorm:"size:16;not null"`
	Title     string    `gorm:"size:200;not null"` // keep in sync with MaxTitleLen
	Body      string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"not null;index:idx_notes_tenant_created,priority:2"`
}

// TableName pins the table name, so renaming the Go type does not move the data.
func (noteRow) TableName() string { return "notes" }

func (r noteRow) toNote() Note {
	// UTC: Postgres hands times back in the session's time zone
	return Note{
		ID: r.ID, TenantID: r.TenantID, CreatedBy: r.CreatedBy,
		Title: r.Title, Body: r.Body, CreatedAt: r.CreatedAt.UTC(),
	}
}

// store is the only gateway to the module's tables. It speaks GORM, the
// Service speaks the domain: GORM's errors are translated here and never
// cross into the rest of the module.
type store struct {
	db *gorm.DB
}

// migrate creates or updates the module's tables.
func (s *store) migrate(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(&noteRow{}); err != nil {
		return fmt.Errorf("migrate notes: %w", err)
	}
	return nil
}

func (s *store) create(ctx context.Context, n Note) error {
	//nolint:staticcheck // S1016: an explicit mapping, not a conversion, lets Note and noteRow diverge
	row := noteRow{
		ID: n.ID, TenantID: n.TenantID, CreatedBy: n.CreatedBy,
		Title: n.Title, Body: n.Body, CreatedAt: n.CreatedAt,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert note: %w", err)
	}
	return nil
}

// get returns the note with the id, only if it belongs to the tenant.
func (s *store) get(ctx context.Context, tenantID, id string) (Note, error) {
	var row noteRow
	// Take, not First: First adds an ORDER BY the primary key, useless for a lookup by it
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Note{}, ErrNotFound
	}
	if err != nil {
		return Note{}, fmt.Errorf("select note: %w", err)
	}
	return row.toNote(), nil
}

// list returns the tenant's notes, newest first; the id breaks ties so the
// order is stable.
func (s *store) list(ctx context.Context, tenantID string) ([]Note, error) {
	var rows []noteRow
	err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at DESC, id ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("select notes: %w", err)
	}
	out := make([]Note, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toNote())
	}
	return out, nil
}
