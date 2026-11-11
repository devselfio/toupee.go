package toupee

import (
	"errors"
	"reflect"
	"testing"
)

// TestParseDuplicateDefinitionLocation checks that a repeated definition
// is reported at the second opener, in the original source, even inside an
// include body whose indentation is stripped before parsing.
func TestParseDuplicateDefinitionLocation(t *testing.T) {
	for _, tt := range []struct {
		name, input, slot string
		line, col         int
	}{
		{"extend", "<t-extend template=\"b\">\n<t-define name=\"t\">1</t-define>\n  <t-define slot=\"t\">2</t-define>", "t", 3, 3},
		{"include stripped body", "<div>\n  <t-include template=\"c\">\n      <t-define>a</t-define>\n      x <t-define name=\"\">b</t-define>\n  </t-include>", "", 4, 9},
		{"include one line", `<t-include template="c"><t-define name="a">1</t-define><t-define name="a">2</t-define></t-include>`, "a", 1, 56},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(tt.input)
			var re *RenderError
			if !errors.As(err, &re) {
				t.Fatalf("parse error = %v, want a *RenderError", err)
			}
			want := RenderError{Line: tt.line, Column: tt.col, Kind: DuplicateSlotDefinition, Message: tt.slot}
			if *re != want {
				t.Errorf("error = %+v, want %+v", *re, want)
			}
		})
	}
}

// TestParseCompositionErrors checks the MalformedElement details and their
// locations, including errors inside stripped definition bodies.
func TestParseCompositionErrors(t *testing.T) {
	for _, tt := range []struct {
		input, message string
		line, col      int
	}{
		{`<t-attr name="a" >`, "unclosed <t-attr> tag", 1, 1},
		{`<t-attr label="a" />`, "missing 'name' attribute on <t-attr>", 1, 1},
		{`x<t-slot name="a"`, "unclosed <t-slot> tag", 1, 2},
		{`<t-slot name="a">default`, "missing closing </t-slot> tag", 1, 1},
		{`<t-include name="a" />`, "missing 'template' attribute on <t-include>", 1, 1},
		{`<t-include template="a"`, "unclosed <t-include> tag", 1, 1},
		{"\n<t-include template=\"a\">x", "missing closing </t-include> tag", 2, 1},
		{` <t-extend name="a">`, "missing 'template' attribute on <t-extend>", 1, 2},
		{"<t-extend template=\"a\">\n<t-define name=\"b\">x", "missing closing </t-define> tag", 2, 1},
		{"<t-extend template=\"a\">\n<t-define name=\"b\"", "unclosed <t-define> tag", 2, 1},
		{"<t-include template=\"a\">\n    <t-define name=\"b\">\n        <p><t-var name=\"x\"</p>\n    </t-define>\n</t-include>", "missing closing tag", 3, 12},
		{"<t-include template=\"a\">\n    <p>\n    <t-var name=\"x\">\n</t-include>", "missing closing tag", 3, 5},
		// #12's control-flow errors inside stripped bodies are located in
		// the original source too.
		{"<t-include template=\"a\">\n    <t-define name=\"b\">\n        <p><t-if var=\"x\">y</p>\n    </t-define>\n</t-include>", "missing closing </t-if> tag", 3, 12},
		{"<t-include template=\"a\">\n      x <t-for item=\"i\">y\n</t-include>", "missing item variable in <t-for>", 2, 9},
		{"<t-extend template=\"a\">\n<t-define name=\"b\">\n  x <t-let name=\"n\">1\n</t-define>", "missing closing </t-let> tag", 3, 5},
		// An unclosed t-debug in an indentation-stripped definition is
		// located at its '<' in the original source.
		{"<t-include template=\"card\">\n    <t-define name=\"body\">\n        <t-debug\n    </t-define>\n</t-include>", "unclosed <t-debug> tag", 3, 9},
		{"<t-include template=\"card\">\n  a\n  <t-debug\n</t-include>", "unclosed <t-debug> tag", 3, 3},
	} {
		_, err := parse(tt.input)
		var re *RenderError
		if !errors.As(err, &re) {
			t.Errorf("parse(%q) error = %v, want a *RenderError", tt.input, err)
			continue
		}
		want := RenderError{Line: tt.line, Column: tt.col, Kind: MalformedElement, Message: tt.message}
		if *re != want {
			t.Errorf("parse(%q) error = %+v, want %+v", tt.input, *re, want)
		}
	}
}

func TestParseInclude(t *testing.T) {
	input := "<t-include template=\"card\" isolated context=\"post.author as writer, , item\" label=\"a&amp;b\" wide>\n" +
		"    <t-define name=\"title\">\n        <h2>T</h2>\n    </t-define>\n    <p>body</p>\n    <p>more</p>\n</t-include>"
	got, err := parse(input)
	if err != nil || len(got.nodes) != 1 || got.nodes[0].kind != includeNode {
		t.Fatalf("parse = %+v, %v; want one include node", got, err)
	}
	inc := got.nodes[0].include
	if inc.template != "card" || !inc.isolated || inc.pos != 0 {
		t.Errorf("include = %q isolated=%v pos=%d", inc.template, inc.isolated, inc.pos)
	}
	if want := map[string]string{"label": "a&amp;b", "wide": ""}; !reflect.DeepEqual(inc.attrs, want) {
		t.Errorf("attrs = %v, want %v", inc.attrs, want)
	}
	if want := []contextBinding{{"post.author", "writer"}, {"item", "item"}}; !reflect.DeepEqual(inc.bindings, want) {
		t.Errorf("bindings = %v, want %v", inc.bindings, want)
	}
	if len(inc.defines) != 1 || inc.defines[0].name != "title" || inc.defines[0].source != "<h2>T</h2>" {
		t.Errorf("defines = %+v", inc.defines)
	}
	if inc.anonymousSource != "<p>body</p>\n<p>more</p>" {
		t.Errorf("anonymous source = %q", inc.anonymousSource)
	}
	if inc, _, _ := (&parser{source: `<t-include template="c" isolatedx="1" />`}).parseInclude(0, 39); inc.isolated {
		t.Error("isolatedx counts as isolated")
	}
}

func TestParseContextBindings(t *testing.T) {
	for _, tt := range []struct {
		spec string
		want []contextBinding
	}{
		{"", nil},
		{" , ", nil},
		{"post", []contextBinding{{"post", "post"}}},
		{"a.b.c", []contextBinding{{"a.b.c", "c"}}},
		{" a.b as  x , c ", []contextBinding{{"a.b", "x"}, {"c", "c"}}},
		{"a.$(k) as v", []contextBinding{{"a.$(k)", "v"}}},
	} {
		if got := parseContextBindings(tt.spec); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseContextBindings(%q) = %v, want %v", tt.spec, got, tt.want)
		}
	}
}

func TestParseBoundTag(t *testing.T) {
	lit := func(s string) tagSegment { return tagSegment{literal: s} }
	bind := func(attr, ref string, variable bool) tagSegment {
		return tagSegment{binding: &attributeBinding{htmlAttr: attr, refName: ref, variable: variable}}
	}
	input := `x<a href="/" t-var:title="t.$(k)" data-t-var:y="z" t-attr:class="c">`
	got, err := parse(input)
	if err != nil || len(got.nodes) != 2 || got.nodes[1].kind != boundTagNode {
		t.Fatalf("parse = %+v, %v", got, err)
	}
	want := boundTag{pos: 1, segments: []tagSegment{
		lit(`<a href="/"`), bind("title", "t.$(k)", true), lit(` data-t-var:y="z"`), bind("class", "c", false), lit(">"),
	}}
	if !reflect.DeepEqual(got.nodes[1].boundTag, want) {
		t.Errorf("bound tag = %+v, want %+v", got.nodes[1].boundTag, want)
	}
	// A truncated binding does not panic.
	for _, in := range []string{`<a t-var:>`, `<a t-var:x=>`, `<a t-attr:x="y>`} {
		if _, err := Render(in, Context{}, nil); err != nil {
			t.Errorf("Render(%q) error: %v", in, err)
		}
	}
}

func TestParseSlot(t *testing.T) {
	got, err := parse("<t-slot name=\"a\">d<t-var name=\"x\" /></t-slot>\r\n<t-slot/>\nz")
	if err != nil || len(got.nodes) != 3 {
		t.Fatalf("parse = %+v, %v", got, err)
	}
	a, b := got.nodes[0].slot, got.nodes[1].slot
	if a.name != "a" || a.pos != 0 || len(a.body) != 2 || a.body[1].variable.pos != 18 {
		t.Errorf("slot a = %+v", a)
	}
	if b.name != "" || b.pos != 47 || b.body != nil {
		t.Errorf("slot b = %+v", b)
	}
	if got.nodes[2].text != "z" {
		t.Errorf("trailing text = %q, want z", got.nodes[2].text)
	}
}

func TestParseExtendStart(t *testing.T) {
	got, err := parse("\r\n\t <t-extend template=\"base\">\nignored\n<t-define>a</t-define>\n<t-define slot=\"s\">b</t-define>")
	if err != nil || len(got.nodes) != 1 || got.nodes[0].kind != extendNode {
		t.Fatalf("parse = %+v, %v", got, err)
	}
	ext := got.nodes[0].extension
	if ext.template != "base" || ext.pos != 4 || len(ext.defines) != 2 || ext.defines[0].name != "" || ext.defines[1].name != "s" {
		t.Errorf("extension = %+v", ext)
	}
}
