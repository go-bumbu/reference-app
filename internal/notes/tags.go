package notes

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Tags organise the notes of a tenant in one tree: a tag sits at the root or
// under another tag, and a note carries any number of tags. Selecting a tag
// selects the notes that carry it or any tag below it.
//
// The tree is stored as a closure table (github.com/go-bumbu/closure-tree, see
// tagstore.go), scoped to the tenant like every other row of the module.

// MaxTagNameLen is the longest tag name, in characters.
const MaxTagNameLen = 100

// MaxTagsPerTenant bounds a tenant's tree, so the whole of it can be read in
// one response without pagination.
const MaxTagsPerTenant = 1000

// maxTagID is the largest tag id: the ids are the database's signed 64-bit
// integers, and a larger value cannot reach the database at all.
const maxTagID = math.MaxInt64

// ErrTagNotFound is returned when a tag does not exist in the actor's tenant.
var ErrTagNotFound = errors.New("tag not found")

// ErrTagExists is returned when a tag would get the name of one of its
// siblings: names are unique among the children of one parent, compared as
// written (case-sensitive).
var ErrTagExists = errors.New("a sibling tag already has this name")

// ErrTooManyTags is returned when a new tag would take the tenant past
// MaxTagsPerTenant.
var ErrTooManyTags = fmt.Errorf("a tenant has at most %d tags", MaxTagsPerTenant)

// Tag is a node of the tenant's tag tree. ParentID is 0 for a tag at the root.
// Children are the tags directly below it, in the order the user gave them,
// each with its own children; never nil.
type Tag struct {
	ID       uint
	ParentID uint
	Name     string
	Children []Tag
}

// TagInput holds the fields of a new tag. ParentID 0 puts it at the root.
// AfterID is the sibling to place it after; nil or 0 places it first.
type TagInput struct {
	Name     string
	ParentID uint
	AfterID  *uint
}

// TagPatch changes a tag; a nil field is left as it is. ParentID moves the tag,
// with its subtree, under another tag (0 for the root). AfterID places it after
// that sibling under the parent it ends up in, 0 meaning first; a move without
// AfterID puts the tag last among its new siblings.
type TagPatch struct {
	Name     *string
	ParentID *uint
	AfterID  *uint
}

// ListOptions narrows List. The zero value lists every note.
type ListOptions struct {
	// TagID keeps the notes that carry the tag or a tag below it. A tag that does
	// not exist in the tenant matches no note.
	TagID uint
}

// Tags returns the tenant's tag tree: the tags at the root, each with its
// subtree, in tree order. A tenant without tags has an empty tree.
func (s *Service) Tags(ctx context.Context, a Actor) ([]Tag, error) {
	if a.TenantID == "" {
		return nil, ErrNoTenant
	}
	return s.store.tagTree(ctx, a.TenantID)
}

// CreateTag adds a tag to the tenant's tree. The name is trimmed, must not be
// empty, must fit MaxTagNameLen and must not be a sibling's (ErrTagExists). A
// parent or an after-sibling that is not in the tenant's tree is a
// *ValidationError; a tenant that has MaxTagsPerTenant tags already gets
// ErrTooManyTags.
func (s *Service) CreateTag(ctx context.Context, a Actor, in TagInput) (Tag, error) {
	if a.TenantID == "" {
		return Tag{}, ErrNoTenant
	}
	name, err := normalizeTagName(in.Name)
	if err != nil {
		return Tag{}, err
	}
	if err := checkTagRef("parentId", &in.ParentID); err != nil {
		return Tag{}, err
	}
	if err := checkTagRef("afterId", in.AfterID); err != nil {
		return Tag{}, err
	}
	return s.store.createTag(ctx, a.TenantID, name, in.ParentID, in.AfterID)
}

// UpdateTag renames, moves and/or reorders a tag of the tenant and returns it
// as it is now, its subtree included. Nothing is written unless the whole patch
// is valid. A tag that does not exist is ErrTagNotFound; a name already used
// among the siblings it ends up with is ErrTagExists; a parent that is the tag
// itself, one of its descendants or not in the tree, or an after-sibling that is
// not under the target parent, is a *ValidationError. An empty patch changes
// nothing.
func (s *Service) UpdateTag(ctx context.Context, a Actor, id uint, p TagPatch) (Tag, error) {
	if a.TenantID == "" {
		return Tag{}, ErrNoTenant
	}
	if !validTagID(id) {
		return Tag{}, ErrTagNotFound
	}
	if err := checkTagRef("parentId", p.ParentID); err != nil {
		return Tag{}, err
	}
	if err := checkTagRef("afterId", p.AfterID); err != nil {
		return Tag{}, err
	}
	if p.Name != nil {
		name, err := normalizeTagName(*p.Name)
		if err != nil {
			return Tag{}, err
		}
		p.Name = &name
	}
	return s.store.updateTag(ctx, a.TenantID, id, p)
}

// DeleteTag deletes a tag of the tenant together with every tag below it, and
// takes them off the notes that carried them; the notes stay. A tag that does
// not exist is ErrTagNotFound.
func (s *Service) DeleteTag(ctx context.Context, a Actor, id uint) error {
	if a.TenantID == "" {
		return ErrNoTenant
	}
	// 0 is closure-tree's virtual root, above every tag: deleting it would take
	// the whole tree. It is no tag, so it is not found.
	if !validTagID(id) {
		return ErrTagNotFound
	}
	return s.store.deleteTag(ctx, a.TenantID, id)
}

// AddNoteTag gives a note of the tenant a tag of the tenant and returns the
// note. Adding a tag the note already carries changes nothing. A missing note
// is ErrNotFound, a missing tag ErrTagNotFound.
func (s *Service) AddNoteTag(ctx context.Context, a Actor, noteID string, tagID uint) (Note, error) {
	if a.TenantID == "" {
		return Note{}, ErrNoTenant
	}
	if !validTagID(tagID) {
		return Note{}, ErrTagNotFound
	}
	return s.store.setNoteTag(ctx, a.TenantID, noteID, tagID, true)
}

// RemoveNoteTag takes a tag off a note of the tenant and returns the note.
// Removing a tag the note does not carry changes nothing. A missing note is
// ErrNotFound, a missing tag ErrTagNotFound.
func (s *Service) RemoveNoteTag(ctx context.Context, a Actor, noteID string, tagID uint) (Note, error) {
	if a.TenantID == "" {
		return Note{}, ErrNoTenant
	}
	if !validTagID(tagID) {
		return Note{}, ErrTagNotFound
	}
	return s.store.setNoteTag(ctx, a.TenantID, noteID, tagID, false)
}

// validTagID reports whether id can name a tag: 0 is closure-tree's virtual
// root, and ids past maxTagID cannot be stored.
func validTagID(id uint) bool {
	return id != 0 && uint64(id) <= maxTagID
}

// checkTagRef checks a parent or after-sibling reference of a patch or an
// input: nil (absent) and 0 (the root, or first) are fine, and so is any id
// the database can hold; whether that tag exists is the store's to check.
func checkTagRef(field string, id *uint) error {
	if id != nil && uint64(*id) > maxTagID {
		return &ValidationError{Field: field, Msg: "is not a tag"}
	}
	return nil
}

// normalizeTagName trims a tag name and checks it.
func normalizeTagName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", &ValidationError{Field: "name", Msg: "must not be empty"}
	case utf8.RuneCountInString(name) > MaxTagNameLen:
		return "", &ValidationError{Field: "name", Msg: fmt.Sprintf("must be at most %d characters", MaxTagNameLen)}
	}
	return name, nil
}
