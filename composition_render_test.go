package toupee_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/devselfio/toupee.go"
)

// mapLoader returns a Loader over templates that counts its calls by name.
func mapLoader(templates map[string]string, calls map[string]int) toupee.Loader {
	return func(name string) (string, error) {
		if calls != nil {
			calls[name]++
		}
		src, ok := templates[name]
		if !ok {
			return "", &toupee.RenderError{Kind: toupee.TemplateNotFound, Message: name}
		}
		return src, nil
	}
}

func s(v string) toupee.Value { return toupee.Value{Kind: toupee.KindString, String: v} }

// wantRenderError checks Render's result is "" and a *RenderError equal to
// want.
func wantRenderError(t *testing.T, got string, err error, want toupee.RenderError) {
	t.Helper()
	var re *toupee.RenderError
	if got != "" || !errors.As(err, &re) || *re != want {
		t.Fatalf("Render = %q, %v; want \"\" and %+v", got, err, want)
	}
}

func TestCompositionLoaders(t *testing.T) {
	const input = "<p>\n  <t-include template=\"x\" />\n</p>"
	t.Run("empty template", func(t *testing.T) {
		calls := map[string]int{}
		got, err := toupee.Render(input, toupee.Context{}, mapLoader(map[string]string{"x": ""}, calls))
		if got != "<p>\n\n</p>" || err != nil || calls["x"] != 1 {
			t.Fatalf("Render = %q, %v (calls %v)", got, err, calls)
		}
	})
	missing := toupee.RenderError{Line: 2, Column: 3, Kind: toupee.TemplateNotFound, Message: "x"}
	t.Run("nil loader", func(t *testing.T) {
		got, err := toupee.Render(input, toupee.Context{}, nil)
		wantRenderError(t, got, err, missing)
	})
	t.Run("missing name", func(t *testing.T) {
		got, err := toupee.Render(input, toupee.Context{}, mapLoader(nil, nil))
		wantRenderError(t, got, err, missing)
	})
	t.Run("wrapped missing error is not modified", func(t *testing.T) {
		inner := &toupee.RenderError{Line: 9, Column: 9, Kind: toupee.TemplateNotFound, Message: "elsewhere"}
		loader := func(string) (string, error) { return "", fmt.Errorf("lookup: %w", inner) }
		got, err := toupee.Render(input, toupee.Context{}, loader)
		wantRenderError(t, got, err, missing)
		if *inner != (toupee.RenderError{Line: 9, Column: 9, Kind: toupee.TemplateNotFound, Message: "elsewhere"}) {
			t.Errorf("loader's error was modified: %+v", *inner)
		}
	})
	t.Run("other loader errors are returned unchanged", func(t *testing.T) {
		errDisk := errors.New("disk on fire")
		other := &toupee.RenderError{Kind: toupee.MalformedElement, Message: "loader's own"}
		for _, want := range []error{errDisk, fmt.Errorf("read: %w", errDisk), other} {
			loader := func(string) (string, error) { return "partial", want }
			got, err := toupee.Render(input, toupee.Context{}, loader)
			if got != "" || err != want {
				t.Errorf("Render = %q, %v; want \"\" and the loader's error %v itself", got, err, want)
			}
		}
		if *other != (toupee.RenderError{Kind: toupee.MalformedElement, Message: "loader's own"}) {
			t.Errorf("loader's error was modified: %+v", *other)
		}
	})
	t.Run("extend parent missing", func(t *testing.T) {
		loader := mapLoader(map[string]string{"page": "\n  <t-extend template=\"base\">"}, nil)
		got, err := toupee.Render(`<t-extend template="page">`, toupee.Context{}, loader)
		wantRenderError(t, got, err, toupee.RenderError{Line: 2, Column: 3, Kind: toupee.TemplateNotFound, Message: "base"})
	})
	t.Run("extend of an empty template", func(t *testing.T) {
		got, err := toupee.Render(`<t-extend template="e"><t-define>x</t-define>`, toupee.Context{}, mapLoader(map[string]string{"e": " "}, nil))
		if got != " " || err != nil {
			t.Fatalf("Render = %q, %v; want the empty parent's text", got, err)
		}
		got, err = toupee.Render(`<t-extend template="e"><t-define>x</t-define>`, toupee.Context{}, mapLoader(map[string]string{"e": ""}, nil))
		if got != "" || err != nil {
			t.Fatalf("Render = %q, %v; want \"\", nil", got, err)
		}
	})
}

func TestCompositionChildErrors(t *testing.T) {
	loader := mapLoader(map[string]string{
		"var":    "<div>\n  <t-var name=\"missing\" /></div>",
		"syntax": "ok\n\t<t-slot name=\"a\">",
		"outer":  "x\n<t-include template=\"var\" />",
		"a":      "<t-extend template=\"b\">",
		"b":      "\n\n   <t-extend template=\"a\">",
		"fill":   "<t-slot />",
	}, nil)
	for _, tt := range []struct {
		input string
		want  toupee.RenderError
	}{
		{`text<t-include template="var" />`, toupee.RenderError{Line: 2, Column: 3, Kind: toupee.UndefinedVariable, Message: "missing"}},
		{`<t-include template="outer" />`, toupee.RenderError{Line: 2, Column: 3, Kind: toupee.UndefinedVariable, Message: "missing"}},
		{`<t-include template="syntax" />`, toupee.RenderError{Line: 2, Column: 2, Kind: toupee.MalformedElement, Message: "missing closing </t-slot> tag"}},
		{`<t-extend template="a">`, toupee.RenderError{Line: 3, Column: 4, Kind: toupee.CircularReference, Message: "a"}},
		// A fill is parsed as its own source.
		{"<t-include template=\"fill\">\n  ab\n  <t-var name=\"m\" />\n</t-include>", toupee.RenderError{Line: 2, Column: 1, Kind: toupee.UndefinedVariable, Message: "m"}},
	} {
		got, err := toupee.Render(tt.input, toupee.Context{}, loader)
		wantRenderError(t, got, err, tt.want)
	}
}

func TestCompositionScope(t *testing.T) {
	loader := mapLoader(map[string]string{
		"show":   `[<t-var name="a">-</t-var>|<t-var name="b">-</t-var>|<t-attr name="k" />|<t-slot name="s">-</t-slot>]`,
		"wrap":   `<div><t-slot /></div>`,
		"layout": `<t-slot name="t">default</t-slot>|<t-slot>body default</t-slot>`,
		"page":   "<t-extend template=\"layout\">\n<t-define name=\"t\">page title</t-define>\n<t-define name=\"\">page body</t-define>",
	}, nil)
	data := map[string]toupee.Value{
		"a":   s("A"),
		"obj": {Kind: toupee.KindMap, Map: map[string]toupee.Value{"b": s("B&"), "key": s("b")}},
	}
	for _, tt := range []struct {
		name, input string
		ctx         toupee.Context
		want        string
	}{
		{"inherited data, own attrs and slots", `<t-include template="show" />`,
			toupee.Context{Data: data, Attrs: map[string]string{"k": "caller"}, Slots: map[string]string{"s": "caller"}}, "[A|-||-]"},
		{"inherited data with overlay", `<t-include template="show" context="obj.b, obj.$(obj.key) as a" k="v" />`,
			toupee.Context{Data: data}, "[B&amp;|B&amp;|v|-]"},
		{"isolated data with overlay", `<t-include template="show" isolated context="obj.b, missing as a" />`,
			toupee.Context{Data: data}, "[-|B&amp;||-]"},
		{"isolated without overlay", `<t-include template="show" isolated />`, toupee.Context{Data: data}, "[-|-||-]"},
		{"siblings and caller do not see child bindings", `<t-include template="show" context="obj.b" k="1" /><t-include template="show" /><t-var name="b">none</t-var><t-attr name="k" />`,
			toupee.Context{Data: data, Attrs: map[string]string{"k": "c"}}, "[A|B&amp;|1|-][A|-||-]nonec"},
		{"empty fill beats the default", `<t-include template="show"><t-define name="s"></t-define></t-include>`, toupee.Context{}, "[-|-||]"},
		{"empty context slot beats the default", `<t-extend template="layout">`, toupee.Context{Slots: map[string]string{"t": ""}}, "|body default"},
		{"fill renders in the child context", `<t-include template="wrap" label="L&" context="obj.b as v" isolated><t-attr name="label" /> <t-var name="v" /></t-include>`,
			toupee.Context{Data: data, Attrs: map[string]string{"label": "caller"}}, "<div>L&amp; B&amp;</div>"},
		{"include attrs are literal and escaped once", `<t-include template="show" k="&lt;&quot;" />`, toupee.Context{}, "[-|-|&amp;lt;&amp;quot;|-]"},
		{"derived definitions win", "<t-extend template=\"page\">\n<t-define name=\"t\">mine</t-define>", toupee.Context{}, "mine|page body"},
		{"parent definitions fill the rest", `<t-extend template="page">`, toupee.Context{Slots: map[string]string{"": "ctx body"}}, "page title|ctx body"},
		{"bindings", `<a t-var:href="obj.$(obj.key)" t-var:n="obj" t-attr:class="k" t-attr:id="none">`, toupee.Context{Data: data, Attrs: map[string]string{"k": `"q"`}}, `<a href="B&amp;" class="&quot;q&quot;">`},
		{"scalar bindings stay present", `<i t-var:a="f" t-var:b="z" t-var:c="e">`, toupee.Context{Data: map[string]toupee.Value{
			"f": {Kind: toupee.KindBoolean}, "z": {Kind: toupee.KindInteger}, "e": s(""),
		}}, `<i a="false" b="0" c="">`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toupee.Render(tt.input, tt.ctx, loader)
			if got != tt.want || err != nil {
				t.Fatalf("Render = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// TestCompositionReuse renders successes, errors and deferrals through
// includes and extends, then checks the caller's Context is unchanged and
// that the same Context and Loader render the same result again.
func TestCompositionReuse(t *testing.T) {
	ctx, want := snapshotContext()
	calls := map[string]int{}
	loader := mapLoader(map[string]string{
		"self":     `<t-include template="self" />`,
		"card":     `<b><t-attr name="class" /><t-slot /></b>`,
		"base":     `<html><t-slot /></html>`,
		"deferred": `ok<t-debug />`,
		"slotself": `<t-slot name="x" />`,
	}, calls)
	const good = `<t-include template="card" class="c" context="list.0 as title">x</t-include><t-attr name="class" /><t-var name="title" />`
	const goodWant = `<b>cx</b>wide&lt;Grid&gt;`
	for i := 0; i < 2; i++ {
		got, err := toupee.Render(good, ctx, loader)
		if got != goodWant || err != nil {
			t.Fatalf("run %d: Render = %q, %v; want %q", i, got, err, goodWant)
		}
		got, err = toupee.Render(`<t-extend template="base"><t-define>[<t-slot />]</t-define>`, ctx, loader)
		if got != "<html>[body]</html>" || err != nil {
			t.Fatalf("run %d: extend = %q, %v", i, got, err)
		}
		got, err = toupee.Render(`a<t-include template="self" />`, ctx, loader)
		wantRenderError(t, got, err, toupee.RenderError{Line: 1, Column: 1, Kind: toupee.CircularReference, Message: "self"})
		got, err = toupee.Render(`<t-include template="slotself"><t-define name="x">a<t-slot name="x" /></t-define></t-include>`, ctx, loader)
		var re *toupee.RenderError
		if got != "" || !errors.As(err, &re) || re.Kind != toupee.CircularReference || re.Message != "x" {
			t.Fatalf("run %d: self-filling slot = %q, %v; want CircularReference x", i, got, err)
		}
		got, err = toupee.Render(`<t-var name="title" /><t-include template="card"><t-var name="nope" /></t-include>`, ctx, loader)
		if got != "" || !errors.As(err, &re) || re.Kind != toupee.UndefinedVariable {
			t.Fatalf("run %d: failing fill = %q, %v; want \"\" and UndefinedVariable", i, got, err)
		}
		for _, in := range []string{
			`text<t-include template="deferred" />`,
			`<t-include template="card"><t-debug /></t-include>`,
			`<t-extend template="deferred">`,
		} {
			if got, err := toupee.Render(in, ctx, loader); got != "" || err != nil {
				t.Fatalf("run %d: Render(%q) = %q, %v; want the bootstrap \"\", nil", i, in, got, err)
			}
		}
	}
	if !reflect.DeepEqual(ctx, want) {
		t.Errorf("Render modified the caller's Context:\n got %#v\nwant %#v", ctx, want)
	}
	if calls["self"] != 2*51 || calls["card"] != 2*2 {
		t.Errorf("loader calls = %v; want self 102 and card 4", calls)
	}
}

// TestCompositionDeferredFill shows a context slot fill that uses a
// deferred construct keeping the bootstrap result for the whole render,
// even after output and inside an include.
func TestCompositionDeferredFill(t *testing.T) {
	ctx := toupee.Context{Slots: map[string]string{"": `<t-let name="a" transform="upper">b</t-let>`}}
	for _, in := range []string{`before<t-slot />`, `<t-extend template="base">`} {
		got, err := toupee.Render(in, ctx, mapLoader(map[string]string{"base": "x<t-slot />"}, nil))
		if got != "" || err != nil {
			t.Errorf("Render(%q) = %q, %v; want \"\", nil", in, got, err)
		}
	}
}
