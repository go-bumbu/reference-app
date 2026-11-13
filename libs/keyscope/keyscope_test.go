package keyscope

import (
	"errors"
	"net/http"
	"slices"
	"testing"
)

func registry(t *testing.T, rules ...Rule) *Registry {
	t.Helper()
	g := NewRegistry()
	for _, r := range rules {
		g.Register(r)
	}
	if err := g.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return g
}

func TestZeroRuleIsClosed(t *testing.T) {
	var r Rule
	if !r.IsClosed() || !Closed().IsClosed() {
		t.Fatal("the zero Rule and Closed() must be closed")
	}
	g := registry(t, Area("notes"))
	grant, err := g.Parse([]string{"notes:write"})
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(r, grant, http.MethodGet); !errors.Is(err, ErrClosed) {
		t.Errorf("got %v, want ErrClosed", err)
	}
}

func TestRegisterCollectsAreas(t *testing.T) {
	g := registry(t, Area("notes"), AreaAt("jobs", Read), Area("notes"), AnyKey(), Closed())
	if got, want := g.Areas(), []string{"jobs", "notes"}; !slices.Equal(got, want) {
		t.Errorf("Areas: got %v, want %v", got, want)
	}
}

func TestRegisterRejectsMalformed(t *testing.T) {
	for _, area := range []string{"", "Notes", "no:tes", "1notes", "no tes", "notes!"} {
		g := NewRegistry()
		g.Register(Area(area))
		if g.Err() == nil {
			t.Errorf("area %q: want an error", area)
		}
		if len(g.Areas()) != 0 {
			t.Errorf("area %q: a malformed area must not be grantable", area)
		}
	}
	g := NewRegistry()
	g.Register(AreaAt("notes", Level(7)))
	if g.Err() == nil {
		t.Error("an invalid level must be an error")
	}
}

func TestParse(t *testing.T) {
	g := registry(t, Area("notes"), Area("jobs"))
	tests := []struct {
		name    string
		scopes  []string
		want    []string
		wantErr bool
	}{
		{name: "one", scopes: []string{"notes:read"}, want: []string{"notes:read"}},
		{name: "sorted", scopes: []string{"notes:write", "jobs:read"}, want: []string{"jobs:read", "notes:write"}},
		{name: "none", scopes: nil, wantErr: true},
		{name: "unknown area", scopes: []string{"tags:read"}, wantErr: true},
		{name: "unknown level", scopes: []string{"notes:admin"}, wantErr: true},
		{name: "no level", scopes: []string{"notes"}, wantErr: true},
		{name: "none level", scopes: []string{"notes:none"}, wantErr: true},
		{name: "area twice", scopes: []string{"notes:read", "notes:write"}, wantErr: true},
		{name: "one bad spoils all", scopes: []string{"notes:read", "gone:read"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			grant, err := g.Parse(tc.scopes)
			if tc.wantErr {
				var pe *ParseError
				if !errors.As(err, &pe) || pe.Error() == "" {
					t.Fatalf("got %v, want a *ParseError", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := grant.Scopes(); !slices.Equal(got, tc.want) {
				t.Errorf("Scopes: got %v, want %v", got, tc.want)
			}
			back, err := g.Parse(grant.Scopes())
			if err != nil || !slices.Equal(back.Scopes(), tc.want) {
				t.Errorf("round trip: got %v, %v", back.Scopes(), err)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	g := registry(t, Area("notes"), Area("jobs"), AreaAt("search", Read))
	read, _ := g.Parse([]string{"notes:read"})
	write, _ := g.Parse([]string{"notes:write"})
	tests := []struct {
		name     string
		rule     Rule
		grant    Grant
		method   string
		wantErr  error
		wantHave Level
	}{
		{name: "read key reads", rule: Area("notes"), grant: read, method: http.MethodGet},
		{name: "read key heads", rule: Area("notes"), grant: read, method: http.MethodHead},
		{name: "write key reads", rule: Area("notes"), grant: write, method: http.MethodGet},
		{name: "write key writes", rule: Area("notes"), grant: write, method: http.MethodDelete},
		{name: "read key cannot write", rule: Area("notes"), grant: read, method: http.MethodPost,
			wantErr: &DeniedError{}, wantHave: Read},
		{name: "other area", rule: Area("jobs"), grant: write, method: http.MethodGet,
			wantErr: &DeniedError{}, wantHave: None},
		{name: "explicit level beats the method", rule: AreaAt("notes", Read), grant: read, method: http.MethodPost},
		{name: "any key", rule: AnyKey(), grant: Grant{}, method: http.MethodGet},
		{name: "closed", rule: Closed(), grant: write, method: http.MethodGet, wantErr: ErrClosed},
		{name: "zero grant", rule: Area("notes"), grant: Grant{}, method: http.MethodGet,
			wantErr: &DeniedError{}, wantHave: None},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Check(tc.rule, tc.grant, tc.method)
			switch want := tc.wantErr.(type) {
			case nil:
				if err != nil {
					t.Fatalf("got %v, want nil", err)
				}
			case *DeniedError:
				var de *DeniedError
				if !errors.As(err, &de) {
					t.Fatalf("got %v, want a *DeniedError", err)
				}
				if de.Have != tc.wantHave || de.Area == "" || de.Error() == "" {
					t.Errorf("got %+v, want Have %v", de, tc.wantHave)
				}
			default:
				if !errors.Is(err, want) {
					t.Fatalf("got %v, want %v", err, want)
				}
			}
		})
	}
}

func TestRuleScope(t *testing.T) {
	tests := []struct {
		rule   Rule
		method string
		want   string
	}{
		{Area("notes"), http.MethodGet, "notes:read"},
		{Area("notes"), http.MethodPut, "notes:write"},
		{AreaAt("notes", Read), http.MethodPost, "notes:read"},
		{AnyKey(), http.MethodGet, ""},
		{Closed(), http.MethodGet, ""},
	}
	for _, tc := range tests {
		if got := tc.rule.Scope(tc.method); got != tc.want {
			t.Errorf("%+v %s: got %q, want %q", tc.rule, tc.method, got, tc.want)
		}
	}
}

func TestLevelString(t *testing.T) {
	for l, want := range map[Level]string{None: "none", Read: "read", Write: "write", Level(9): "none"} {
		if got := l.String(); got != want {
			t.Errorf("%d: got %q, want %q", l, got, want)
		}
	}
}

func TestMethodLevel(t *testing.T) {
	for m, want := range map[string]Level{
		http.MethodGet: Read, http.MethodHead: Read, http.MethodOptions: Read,
		http.MethodPost: Write, http.MethodPut: Write, http.MethodPatch: Write, http.MethodDelete: Write,
	} {
		if got := MethodLevel(m); got != want {
			t.Errorf("%s: got %v, want %v", m, got, want)
		}
	}
}
