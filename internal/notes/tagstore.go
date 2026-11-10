package notes

import (
	"context"
	"errors"
	"fmt"

	closuretree "github.com/go-bumbu/closure-tree"
	"github.com/go-bumbu/db/gormdb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// tagRow is a node of the tag tree: closure-tree owns its identity, tenant
// scope and sibling order (the embedded Node), this adds the payload. The
// library keeps the tree's structure in tables named after this one
// (closure_tree_note_tags, plus a meta and a lock table).
type tagRow struct {
	closuretree.Node
	Name string `gorm:"size:100;not null"` // keep in sync with MaxTagNameLen

	// Children is filled by TreeDescendants only, which links the rows it reads
	// into a tree through this field.
	Children []*tagRow `gorm:"-"`
}

// TableName pins the table name, so renaming the Go type does not move the data.
func (tagRow) TableName() string { return "note_tags" }

func (r *tagRow) toTag() Tag {
	return Tag{ID: r.Id(), ParentID: r.Parent(), Name: r.Name, Children: toTags(r.Children)}
}

func toTags(rows []*tagRow) []Tag {
	out := make([]Tag, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toTag())
	}
	return out
}

// noteTagRow gives a note a tag. The primary key leads with the tenant, like
// every index of the module, and serves the lookups by note; the second index
// serves the ones by tag (the tag filter, deleting a tag).
type noteTagRow struct {
	TenantID string `gorm:"primaryKey;size:16;index:idx_note_tag_links_tenant_tag,priority:1"`
	NoteID   string `gorm:"primaryKey;size:16"`
	TagID    uint   `gorm:"primaryKey;autoIncrement:false;index:idx_note_tag_links_tenant_tag,priority:2"`
}

func (noteTagRow) TableName() string { return "note_tag_links" }

// newTree returns the tag tree bound to db. Bound to a transaction, every write
// of the tree is a savepoint of it, so a change of the tree and of the module's
// own tables commit together. A tree bound to the store's handle and used inside
// a transaction would reach for a second connection instead: on SQLite the one
// writer is taken by the transaction, and it would wait forever.
func newTree(db *gorm.DB) (*closuretree.Tree, error) {
	tree, err := closuretree.New(db, tagRow{})
	if err != nil {
		return nil, fmt.Errorf("tag tree: %w", err)
	}
	return tree, nil
}

// mapTreeErr translates closure-tree's errors into the module's.
func mapTreeErr(err error) error {
	switch {
	case errors.Is(err, closuretree.ErrNodeNotFound):
		return ErrTagNotFound
	case errors.Is(err, closuretree.ErrParentNotFound):
		return &ValidationError{Field: "parentId", Msg: "is not a tag"}
	case errors.Is(err, closuretree.ErrInvalidMove):
		return &ValidationError{Field: "parentId", Msg: "cannot hold the tag"}
	case errors.Is(err, closuretree.ErrInvalidAfterNode):
		return &ValidationError{Field: "afterId", Msg: "is not a tag under the parent"}
	case errors.Is(err, closuretree.ErrAfterNodeIsSelf):
		return &ValidationError{Field: "afterId", Msg: "must not be the tag itself"}
	}
	return err
}

// readTree returns the tag tree for reads outside a transaction. Its queries
// are recursive CTEs (WITH ...), which the SQLite resolver would send to the one
// write connection; the Read clause sends them to the read pool, so reading the
// tree never waits behind a writer. The Session makes the handle reusable.
func readTree(db *gorm.DB) (*closuretree.Tree, error) {
	return newTree(db.Clauses(gormdb.Read).Session(&gorm.Session{}))
}

// tagTree reads the tenant's whole tree.
func (s *store) tagTree(ctx context.Context, tenantID string) ([]Tag, error) {
	var roots []*tagRow
	if err := s.tree.TreeDescendants(ctx, 0, 0, tenantID, &roots); err != nil {
		return nil, fmt.Errorf("select tags: %w", err)
	}
	return toTags(roots), nil
}

// tagTx runs fn in a transaction with the tag tree bound to it.
func (s *store) tagTx(ctx context.Context, fn func(tx *gorm.DB, tree *closuretree.Tree) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tree, err := newTree(tx)
		if err != nil {
			return err
		}
		return fn(tx, tree)
	})
}

// createTag adds the tag under parentID, after afterID.
//
// GetOrAdd looks for a sibling of that name and adds the tag under the tree's
// per-tenant write lock, so two requests for the same new name cannot both
// create it. It places the tag first; the reorder that follows runs in the same
// transaction, so a bad afterID leaves nothing behind.
func (s *store) createTag(ctx context.Context, tenantID, name string, parentID uint, afterID *uint) (Tag, error) {
	row := tagRow{Name: name}
	err := s.tagTx(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		var count int64
		if err := tx.Model(&tagRow{}).Where("tenant = ?", tenantID).Count(&count).Error; err != nil {
			return fmt.Errorf("count tags: %w", err)
		}
		if count >= MaxTagsPerTenant {
			return ErrTooManyTags
		}
		created, err := tree.GetOrAdd(ctx, &row, parentID, tenantID, []string{"Name"})
		if err != nil {
			return mapTreeErr(err)
		}
		if !created {
			return fmt.Errorf("%w: %q", ErrTagExists, name)
		}
		if afterID == nil || *afterID == 0 {
			return nil
		}
		if err := tree.Update(ctx, row.Id(), nil, nil, afterID, tenantID); err != nil {
			return mapTreeErr(err)
		}
		return renormalizeIfNeeded(ctx, tree, parentID, tenantID)
	})
	if err != nil {
		return Tag{}, err
	}
	return Tag{ID: row.Id(), ParentID: parentID, Name: name, Children: []Tag{}}, nil
}

// updateTag applies the patch in one transaction and reads the tag back.
func (s *store) updateTag(ctx context.Context, tenantID string, id uint, p TagPatch) (Tag, error) {
	var out Tag
	err := s.tagTx(ctx, func(_ *gorm.DB, tree *closuretree.Tree) error {
		var cur tagRow
		if err := tree.GetNode(ctx, id, tenantID, &cur); err != nil {
			return mapTreeErr(err)
		}
		u, err := planTagUpdate(ctx, tree, tenantID, &cur, p)
		if err != nil {
			return err
		}
		if u.item != nil || u.moveTo != nil || u.afterID != nil {
			if err := tree.Update(ctx, id, u.item, u.moveTo, u.afterID, tenantID); err != nil {
				return mapTreeErr(err)
			}
			if err := renormalizeIfNeeded(ctx, tree, u.parentID, tenantID); err != nil {
				return err
			}
		}
		return readTag(ctx, tree, tenantID, id, &out)
	})
	if err != nil {
		return Tag{}, err
	}
	return out, nil
}

// tagUpdate is what a patch writes, in closure-tree's terms: a nil field
// leaves that aspect alone. parentID is the parent the tag ends up under.
type tagUpdate struct {
	// item stays a nil interface unless the name changes: a typed nil pointer
	// in it would tell closure-tree there is a payload to write.
	item     any
	moveTo   *uint
	afterID  *uint
	parentID uint
}

// planTagUpdate checks the patch against the tag as it is and returns what to
// write. A name or parent equal to the current one is no change.
//
// The sibling-name check is a check, not a constraint: the parent is not a
// column the database could hold unique together with the name. On SQLite,
// which runs one writer at a time, the transaction keeps it honest; on Postgres
// two concurrent renames could still both pass it.
func planTagUpdate(ctx context.Context, tree *closuretree.Tree, tenantID string, cur *tagRow, p TagPatch) (tagUpdate, error) {
	u := tagUpdate{parentID: cur.Parent(), afterID: p.AfterID}
	if p.ParentID != nil && *p.ParentID != u.parentID {
		if err := checkMove(ctx, tree, tenantID, cur.Id(), *p.ParentID); err != nil {
			return u, err
		}
		u.moveTo, u.parentID = p.ParentID, *p.ParentID
	}
	name := cur.Name
	if p.Name != nil && *p.Name != cur.Name {
		name = *p.Name
		u.item = &tagRow{Name: name}
	}
	if u.item != nil || u.moveTo != nil {
		if err := checkSiblingName(ctx, tree, tenantID, u.parentID, name, cur.Id()); err != nil {
			return u, err
		}
	}
	return u, nil
}

// checkMove refuses a move of tag id under to when to is the tag itself, one of
// its descendants, or not in the tree. closure-tree refuses the first two as
// well, but cannot tell the caller which rule was broken.
func checkMove(ctx context.Context, tree *closuretree.Tree, tenantID string, id, to uint) error {
	if to == 0 {
		return nil
	}
	if to == id {
		return &ValidationError{Field: "parentId", Msg: "must not be the tag itself"}
	}
	var parent tagRow
	if err := tree.GetNode(ctx, to, tenantID, &parent); err != nil {
		if errors.Is(err, closuretree.ErrNodeNotFound) {
			return &ValidationError{Field: "parentId", Msg: "is not a tag"}
		}
		return err
	}
	below, err := tree.IsDescendant(ctx, id, to, tenantID)
	if err != nil {
		return fmt.Errorf("check tag move: %w", err)
	}
	if below {
		return &ValidationError{Field: "parentId", Msg: "must not be below the tag itself"}
	}
	return nil
}

// checkSiblingName returns ErrTagExists when a child of parentID other than
// except already has the name.
func checkSiblingName(ctx context.Context, tree *closuretree.Tree, tenantID string, parentID uint, name string, except uint) error {
	var twin tagRow
	found, err := tree.FindChild(ctx, parentID, tenantID, &tagRow{Name: name}, []string{"Name"}, &twin)
	if err != nil {
		return fmt.Errorf("check tag name: %w", err)
	}
	if found && twin.Id() != except {
		return fmt.Errorf("%w: %q", ErrTagExists, name)
	}
	return nil
}

// renormalizeIfNeeded respaces the order of parentID's children once repeated
// inserts between the same two siblings have used up most of the float64
// precision their order is kept in.
func renormalizeIfNeeded(ctx context.Context, tree *closuretree.Tree, parentID uint, tenantID string) error {
	need, err := tree.NeedsRenormalize(ctx, parentID, tenantID, closuretree.DefaultHalvingsBuffer)
	if err != nil {
		return fmt.Errorf("check tag order: %w", err)
	}
	if !need {
		return nil
	}
	if err := tree.Renormalize(ctx, parentID, tenantID); err != nil {
		return fmt.Errorf("respace tag order: %w", err)
	}
	return nil
}

// readTag loads a tag with its subtree into out.
func readTag(ctx context.Context, tree *closuretree.Tree, tenantID string, id uint, out *Tag) error {
	var row tagRow
	if err := tree.GetNode(ctx, id, tenantID, &row); err != nil {
		return mapTreeErr(err)
	}
	var children []*tagRow
	if err := tree.TreeDescendants(ctx, id, 0, tenantID, &children); err != nil {
		return fmt.Errorf("select tag subtree: %w", err)
	}
	row.Children = children
	*out = row.toTag()
	return nil
}

// deleteTag deletes the tag, its subtree and every link to them in one
// transaction.
func (s *store) deleteTag(ctx context.Context, tenantID string, id uint) error {
	return s.tagTx(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		// The closure table lists a tag as its own descendant (depth 0), so the
		// subtree is every descendant of the tag.
		subtree := tx.Table(tree.ClosureTableName()).Select("descendant_id").
			Where("ancestor_id = ? AND tenant = ?", id, tenantID)
		if err := tx.Where("tenant_id = ? AND tag_id IN (?)", tenantID, subtree).Delete(&noteTagRow{}).Error; err != nil {
			return fmt.Errorf("delete tag links: %w", err)
		}
		if err := tree.DeleteRecurse(ctx, id, tenantID); err != nil {
			return mapTreeErr(err)
		}
		return nil
	})
}

// setNoteTag links (add) or unlinks the tag and the note, then reads the note
// back, all in one transaction.
func (s *store) setNoteTag(ctx context.Context, tenantID, noteID string, tagID uint, add bool) (Note, error) {
	var out Note
	err := s.tagTx(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		var row noteRow
		if err := takeNote(tx, tenantID, noteID, &row); err != nil {
			return err
		}
		var tag tagRow
		if err := tree.GetNode(ctx, tagID, tenantID, &tag); err != nil {
			return mapTreeErr(err)
		}
		link := noteTagRow{TenantID: tenantID, NoteID: noteID, TagID: tagID}
		if add {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
				return fmt.Errorf("insert tag link: %w", err)
			}
		} else if err := tx.Where("tenant_id = ? AND note_id = ? AND tag_id = ?", tenantID, noteID, tagID).
			Delete(&noteTagRow{}).Error; err != nil {
			return fmt.Errorf("delete tag link: %w", err)
		}
		notes, err := withTags(tx, tenantID, []noteRow{row}, []string{noteID})
		if err != nil {
			return err
		}
		out = notes[0]
		return nil
	})
	if err != nil {
		return Note{}, err
	}
	return out, nil
}

// taggedNoteIDs is the subquery for the notes of the tenant that carry the tag
// or one below it: a link's tag is a descendant (at any depth, itself
// included) of the selected tag in the closure table.
func taggedNoteIDs(db *gorm.DB, closureTbl, tenantID string, tagID uint) *gorm.DB {
	return db.Table("note_tag_links AS l").Select("l.note_id").
		Joins(fmt.Sprintf("JOIN %s AS ct ON ct.descendant_id = l.tag_id AND ct.tenant = l.tenant_id", closureTbl)).
		Where("l.tenant_id = ? AND ct.ancestor_id = ?", tenantID, tagID)
}

// withTags maps the rows onto notes with their tag ids, ascending. noteIDs
// selects the links to read: the rows' ids, or the subquery the rows were read
// with, which keeps a long list from turning into as many bind parameters.
//
// Only links to a tag that still exists count. The links have no foreign keys
// (closure-tree owns the tag table), and on Postgres a link can be written just
// after a concurrent delete of its tag committed; such a link is never returned,
// and stays as a harmless row until its note is deleted.
func withTags(db *gorm.DB, tenantID string, rows []noteRow, noteIDs any) ([]Note, error) {
	out := make([]Note, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	var links []noteTagRow
	err := db.Table("note_tag_links AS l").Select("l.note_id, l.tag_id").
		Joins("JOIN note_tags AS t ON t.node_id = l.tag_id AND t.tenant = l.tenant_id").
		Where("l.tenant_id = ? AND l.note_id IN (?)", tenantID, noteIDs).
		Order("l.tag_id ASC").
		Scan(&links).Error
	if err != nil {
		return nil, fmt.Errorf("select tag links: %w", err)
	}
	byNote := make(map[string][]uint, len(rows))
	for _, l := range links {
		byNote[l.NoteID] = append(byNote[l.NoteID], l.TagID)
	}
	for _, r := range rows {
		n := r.toNote()
		if tagIDs, ok := byNote[r.ID]; ok {
			n.TagIDs = tagIDs
		}
		out = append(out, n)
	}
	return out, nil
}
