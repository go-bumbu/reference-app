package notes

import (
	"context"
	"errors"
	"fmt"
	"time"

	closuretree "github.com/go-bumbu/closure-tree"
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

// toNote maps the row onto a note without tags; withTags adds them.
func (r noteRow) toNote() Note {
	// UTC: Postgres hands times back in the session's time zone
	return Note{
		ID: r.ID, TenantID: r.TenantID, CreatedBy: r.CreatedBy,
		Title: r.Title, Body: r.Body, CreatedAt: r.CreatedAt.UTC(),
		TagIDs: []uint{},
	}
}

// store is the only gateway to the module's tables. It speaks GORM, the
// Service speaks the domain: GORM's errors are translated here and never
// cross into the rest of the module.
type store struct {
	db *gorm.DB
	// tree is the tag tree bound to db's read pool (readTree); writes bind one
	// to their transaction (see newTree).
	tree *closuretree.Tree
}

func newStore(db *gorm.DB) (*store, error) {
	tree, err := readTree(db)
	if err != nil {
		return nil, err
	}
	return &store{db: db, tree: tree}, nil
}

// migrate creates or updates the module's tables, the tag tree's included.
func (s *store) migrate(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(&noteRow{}, &noteTagRow{}); err != nil {
		return fmt.Errorf("migrate notes: %w", err)
	}
	// closure-tree's Migrate takes no context: bind the tree to one instead.
	migrator, err := newTree(s.db.WithContext(ctx))
	if err != nil {
		return err
	}
	if err := migrator.Migrate(); err != nil {
		return fmt.Errorf("migrate note tags: %w", err)
	}
	return nil
}

func (s *store) create(ctx context.Context, n Note) error {
	row := noteRow{
		ID: n.ID, TenantID: n.TenantID, CreatedBy: n.CreatedBy,
		Title: n.Title, Body: n.Body, CreatedAt: n.CreatedAt,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert note: %w", err)
	}
	return nil
}

// takeNote loads the note with the id into row, only if it belongs to the tenant.
func takeNote(db *gorm.DB, tenantID, id string, row *noteRow) error {
	// Take, not First: First adds an ORDER BY the primary key, useless for a lookup by it
	err := db.Where("tenant_id = ? AND id = ?", tenantID, id).Take(row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("select note: %w", err)
	}
	return nil
}

// get returns the note with the id and its tags, only if it belongs to the
// tenant. Two reads without a transaction, which on SQLite would take the one
// writer: a tag changed between them is at worst one request stale.
func (s *store) get(ctx context.Context, tenantID, id string) (Note, error) {
	db := s.db.WithContext(ctx)
	var row noteRow
	if err := takeNote(db, tenantID, id, &row); err != nil {
		return Note{}, err
	}
	out, err := withTags(db, tenantID, []noteRow{row}, []string{id})
	if err != nil {
		return Note{}, err
	}
	return out[0], nil
}

// update sets the title and body of the note with the id, only if it belongs
// to the tenant, and returns it as stored. The write and the read back share a
// transaction, so the note returned is the one this call wrote.
func (s *store) update(ctx context.Context, tenantID, id string, in Input) (Note, error) {
	var out []Note
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&noteRow{}).
			Where("tenant_id = ? AND id = ?", tenantID, id).
			Updates(map[string]any{"title": in.Title, "body": in.Body})
		if res.Error != nil {
			return fmt.Errorf("update note: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		var row noteRow
		if err := takeNote(tx, tenantID, id, &row); err != nil {
			return err
		}
		var err error
		out, err = withTags(tx, tenantID, []noteRow{row}, []string{id})
		return err
	})
	if err != nil {
		return Note{}, err
	}
	return out[0], nil
}

// delete removes the note with the id and its tag links, only if it belongs to
// the tenant.
func (s *store) delete(ctx context.Context, tenantID, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&noteRow{})
		if res.Error != nil {
			return fmt.Errorf("delete note: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Where("tenant_id = ? AND note_id = ?", tenantID, id).Delete(&noteTagRow{}).Error; err != nil {
			return fmt.Errorf("delete note tag links: %w", err)
		}
		return nil
	})
}

// list returns the tenant's notes with their tags, newest first, narrowed to
// the notes under a tag when opts names one; the id breaks ties so the order is
// stable. Like get, it reads without a transaction.
// noteIDs returns the ids of the tenant's notes in the order of list.
func (s *store) noteIDs(ctx context.Context, tenantID string) ([]string, error) {
	ids := []string{}
	err := s.db.WithContext(ctx).Model(&noteRow{}).Where("tenant_id = ?", tenantID).
		Order("created_at DESC, id ASC").Pluck("id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("select note ids: %w", err)
	}
	return ids, nil
}

func (s *store) list(ctx context.Context, tenantID string, opts ListOptions) ([]Note, error) {
	db := s.db.WithContext(ctx)
	notesQuery := func() *gorm.DB {
		q := db.Model(&noteRow{}).Where("tenant_id = ?", tenantID)
		if opts.TagID != 0 {
			q = q.Where("id IN (?)", taggedNoteIDs(db, s.tree.ClosureTableName(), tenantID, opts.TagID))
		}
		return q
	}
	var rows []noteRow
	if err := notesQuery().Order("created_at DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("select notes: %w", err)
	}
	return withTags(db, tenantID, rows, notesQuery().Select("id"))
}
