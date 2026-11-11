package engine

import (
	"reflect"
	"testing"
)

func lookupContext() Context {
	s := func(v string) Value { return Value{Kind: KindString, String: v} }
	return Context{Data: map[string]Value{
		"user": {Kind: KindMap, Map: map[string]Value{"name": s("Ada")}},
		"items": {Kind: KindList, List: []Value{
			s("zero"),
			{Kind: KindMap, Map: map[string]Value{"id": s("one")}},
		}},
		"":       {Kind: KindMap, Map: map[string]Value{"": s("empty-empty"), "k": s("empty-k")}},
		"a":      {Kind: KindMap, Map: map[string]Value{"": s("a-empty")}},
		"key":    s("name"),
		"idx":    {Kind: KindInteger, Integer: 1},
		"dotted": s("x.y"),
		"m": {Kind: KindMap, Map: map[string]Value{
			"x.y":   s("dotted-key"),
			"true":  s("bool-key"),
			"1.5":   s("float-key"),
			"name":  s("m-name"),
			"inner": s("name"),
		}},
		"f":     {Kind: KindFloat, Float: 1.5},
		"t":     {Kind: KindBoolean, Boolean: true},
		"nil":   {},
		"list":  {Kind: KindList},
		"which": s("inner"),
	}}
}

func TestResolveStatic(t *testing.T) {
	ctx := lookupContext()
	for _, tt := range []struct {
		path string
		want string
		ok   bool
	}{
		{"user.name", "Ada", true},
		{"items.0", "zero", true},
		{"items.1.id", "one", true},
		{"items.2", "", false},
		{"items.-1", "", false},
		{"items.x", "", false},
		{"items.+1", "", false},
		{"user.missing", "", false},
		{"missing.name", "", false},
		{"user.name.more", "", false},
		{"items.0.x", "", false},
		{".k", "empty-k", true},
		{".", "", true},  // the map under the empty key, as upstream
		{"a.", "", true}, // an empty remaining path is the map itself
	} {
		got, ok := resolve(ctx, tt.path)
		if ok != tt.ok {
			t.Errorf("resolve(%q) ok = %v, want %v", tt.path, ok, tt.ok)
			continue
		}
		if tt.want != "" && (got.Kind != KindString || got.String != tt.want) {
			t.Errorf("resolve(%q) = %+v, want %q", tt.path, got, tt.want)
		}
	}
	if got, ok := resolve(ctx, ""); !ok || got.Kind != KindMap || !reflect.DeepEqual(got.Map, ctx.Data) {
		t.Errorf("resolve(\"\") = %+v, %v; want the root map", got, ok)
	}
	if got, _ := resolve(ctx, "."); got.Kind != KindMap || got.Map["k"].String != "empty-k" {
		t.Errorf("resolve(\".\") = %+v, want the map under the empty key", got)
	}
	if got, _ := resolve(ctx, "a."); got.Kind != KindMap {
		t.Errorf("resolve(\"a.\") = %+v, want the map a", got)
	}
}

func TestResolveDynamic(t *testing.T) {
	ctx := lookupContext()
	for _, tt := range []struct {
		path string
		want string
		ok   bool
	}{
		{"user.$(key)", "Ada", true},
		{"items.$(idx).id", "one", true},
		{"m.$(dotted)", "dotted-key", true},
		{"m.$(t)", "bool-key", true},
		{"m.$(f)", "float-key", true},
		{"m.$(m.$(which))", "m-name", true},
		{"user.$(m.$(which))", "Ada", true},
		{"user.$(missing)", "", false},
		{"user.$(nil)", "", false},
		{"user.$(list)", "", false},
		{"user.$(user)", "", false},
		{"user..$(key)", "Ada", true}, // dynamic lookup skips empty segments
		{"$(", "", false},
	} {
		got, ok := resolve(ctx, tt.path)
		if ok != tt.ok {
			t.Errorf("resolve(%q) ok = %v, want %v", tt.path, ok, tt.ok)
			continue
		}
		if tt.ok && (got.Kind != KindString || got.String != tt.want) {
			t.Errorf("resolve(%q) = %+v, want %q", tt.path, got, tt.want)
		}
	}
}

func TestResolveNextPathSegment(t *testing.T) {
	for _, tt := range []struct{ path, seg, rest string }{
		{"a.b.c", "a", "b.c"},
		{"a", "a", ""},
		{".a", "", "a"},
		{"$(a.b).c", "$(a.b)", "c"},
		{"$(a.$(b.c)).d", "$(a.$(b.c))", "d"},
		{"$(a)", "$(a)", ""},
		{"$(a.b", "$(a.b", ""},
	} {
		seg, rest := nextPathSegment(tt.path)
		if seg != tt.seg || rest != tt.rest {
			t.Errorf("nextPathSegment(%q) = %q, %q; want %q, %q", tt.path, seg, rest, tt.seg, tt.rest)
		}
	}
}
