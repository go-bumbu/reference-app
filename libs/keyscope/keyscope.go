// Package keyscope is the scope model of API keys: which routes a key may use,
// and what a key grants.
//
// An app declares a Rule for each of its routes where it registers the route:
// closed to keys (the zero value, so a route nobody declared fails closed), open
// to any valid key, or open to an area (a name such as "notes") at the level the
// HTTP method implies (GET and HEAD read, everything else writes). A Registry
// records the declarations, and the areas they name are the vocabulary a key can
// be granted: "<area>:read" or "<area>:write", where write includes read.
//
// A Grant is what one key may do. It is made from the scope strings a client
// asks for, or the ones stored with a key, by Registry.Parse, which fails closed
// on anything the registry does not know. Check decides whether a Grant may use
// a route.
//
// The package knows nothing about tokens, storage, users or HTTP routing: it
// imports the standard library only.
package keyscope

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Level is what a key may do in one area. Write includes Read.
type Level int

const (
	// None is no access, the level of every area a grant does not name.
	None Level = iota
	// Read allows the GET and HEAD routes of the area.
	Read
	// Write allows every route of the area.
	Write
)

// String is the level as it appears in a scope: "read", "write" or "none".
func (l Level) String() string {
	switch l {
	case Read:
		return "read"
	case Write:
		return "write"
	}
	return "none"
}

func parseLevel(s string) (Level, bool) {
	switch s {
	case "read":
		return Read, true
	case "write":
		return Write, true
	}
	return None, false
}

// MethodLevel is the level a request method needs: Read for GET, HEAD and
// OPTIONS, Write for everything else.
func MethodLevel(method string) Level {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return Read
	}
	return Write
}

type ruleKind int

const (
	kindClosed ruleKind = iota
	kindAnyKey
	kindArea
)

// Rule is what a route declares about keys. The zero value is closed.
type Rule struct {
	kind  ruleKind
	area  string
	level Level // None: the level comes from the request method
}

// Closed is the rule of a route no key may use. It is the zero Rule.
func Closed() Rule { return Rule{} }

// AnyKey is the rule of a route every valid key may use, whatever it grants
// (e.g. "who am I").
func AnyKey() Rule { return Rule{kind: kindAnyKey} }

// Area is the rule of a route of the area, at the level its method implies
// (MethodLevel).
func Area(area string) Rule { return Rule{kind: kindArea, area: area} }

// AreaAt is the rule of a route of the area at an explicit level, for a route
// whose method says the wrong thing (a POST that only reads, say).
func AreaAt(area string, level Level) Rule { return Rule{kind: kindArea, area: area, level: level} }

// IsClosed reports whether no key may use the route.
func (r Rule) IsClosed() bool { return r.kind == kindClosed }

// Scope is the scope a key needs for the route when requested with method:
// "<area>:<level>", "" for AnyKey and for a closed rule.
func (r Rule) Scope(method string) string {
	if r.kind != kindArea {
		return ""
	}
	return r.area + ":" + r.need(method).String()
}

func (r Rule) need(method string) Level {
	if r.level != None {
		return r.level
	}
	return MethodLevel(method)
}

// areaPattern is what an area may look like: lower case, starting with a
// letter, so that "<area>:<level>" splits on its one colon.
var areaPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// Registry collects the rules an app declares. It is not safe for concurrent
// use while rules are registered; register them all at startup, then only read.
type Registry struct {
	areas map[string]struct{}
	errs  []error
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{areas: map[string]struct{}{}}
}

// Register records rule and returns it, so a declaration reads inline where the
// route is registered. A malformed rule is remembered and reported by Err.
func (g *Registry) Register(rule Rule) Rule {
	if rule.kind != kindArea {
		return rule
	}
	if !areaPattern.MatchString(rule.area) {
		g.errs = append(g.errs, fmt.Errorf("keyscope: invalid area %q: want lower case letters, digits and '-', starting with a letter", rule.area))
		return rule
	}
	if rule.level != None && rule.level != Read && rule.level != Write {
		g.errs = append(g.errs, fmt.Errorf("keyscope: invalid level %d for area %q", rule.level, rule.area))
		return rule
	}
	g.areas[rule.area] = struct{}{}
	return rule
}

// Err reports every malformed rule registered so far, nil when there is none.
// Check it once all the routes are registered, so a bad declaration fails the
// start instead of closing a route without a word.
func (g *Registry) Err() error { return errors.Join(g.errs...) }

// Areas returns the areas the registered rules name, sorted: what a key can be
// granted.
func (g *Registry) Areas() []string {
	out := make([]string, 0, len(g.areas))
	for a := range g.areas {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// ParseError is a set of scopes that cannot be granted. Its message is fit to
// show to the client that asked for them.
type ParseError struct{ Msg string }

func (e *ParseError) Error() string { return e.Msg }

// Parse turns scope strings into a Grant. It fails closed: every scope must be
// "<area>:read" or "<area>:write" of a registered area, an area may appear
// once, and at least one must be given. Use it on what a client asks for, and
// on what is stored with a key, so a key whose area the app no longer has is
// refused whole rather than half honoured.
func (g *Registry) Parse(scopes []string) (Grant, error) {
	if len(scopes) == 0 {
		return Grant{}, &ParseError{Msg: "at least one scope is required"}
	}
	levels := make(map[string]Level, len(scopes))
	for _, s := range scopes {
		area, lvl, ok := strings.Cut(s, ":")
		level, okLevel := parseLevel(lvl)
		_, known := g.areas[area]
		if !ok || !okLevel || !known {
			return Grant{}, &ParseError{Msg: "unknown scope " + strconv.Quote(s)}
		}
		if _, dup := levels[area]; dup {
			return Grant{}, &ParseError{Msg: "area " + strconv.Quote(area) + " is listed more than once"}
		}
		levels[area] = level
	}
	return Grant{levels: levels}, nil
}

// Grant is what one key may do: a level per area. The zero Grant allows nothing.
type Grant struct {
	levels map[string]Level
}

// Level is the level the grant has in area, None when it does not name it.
func (g Grant) Level(area string) Level { return g.levels[area] }

// Scopes returns the grant as scope strings, sorted: the form to store and to
// show, which Parse reads back.
func (g Grant) Scopes() []string {
	out := make([]string, 0, len(g.levels))
	for area, l := range g.levels {
		out = append(out, area+":"+l.String())
	}
	sort.Strings(out)
	return out
}

// ErrClosed is returned by Check for a route no key may use.
var ErrClosed = errors.New("keyscope: route is closed to API keys")

// DeniedError is returned by Check when the grant lacks the level the route
// needs in its area. Have is None when the grant does not name the area at all.
type DeniedError struct {
	Area string
	Need Level
	Have Level
}

func (e *DeniedError) Error() string {
	if e.Have == None {
		return "keyscope: no access to " + e.Area
	}
	return "keyscope: " + e.Area + " is " + e.Have.String() + "-only"
}

// Check reports whether a key with grant may use a route declared with rule,
// requested with method: nil, ErrClosed, or a *DeniedError.
func Check(rule Rule, grant Grant, method string) error {
	switch rule.kind {
	case kindAnyKey:
		return nil
	case kindArea:
		need, have := rule.need(method), grant.Level(rule.area)
		if have >= need {
			return nil
		}
		return &DeniedError{Area: rule.area, Need: need, Have: have}
	}
	return ErrClosed
}
