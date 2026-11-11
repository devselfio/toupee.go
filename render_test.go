package toupee_test

import (
	"errors"
	"testing"

	"github.com/devselfio/toupee.go"
)

func str(s string) toupee.Value    { return toupee.Value{Kind: toupee.KindString, String: s} }
func integer(n int64) toupee.Value { return toupee.Value{Kind: toupee.KindInteger, Integer: n} }
func boolean(b bool) toupee.Value  { return toupee.Value{Kind: toupee.KindBoolean, Boolean: b} }

type renderCase struct {
	name  string
	input string
	data  map[string]toupee.Value
	want  string
}

func runRenderCases(t *testing.T, cases []renderCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toupee.Render(tt.input, toupee.Context{Data: tt.data}, nil)
			if err != nil {
				t.Fatalf("Render(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Render(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderEscaping(t *testing.T) {
	v := map[string]toupee.Value{"v": str(`a&b<c>d"e'f é ✓`)}
	runRenderCases(t, []renderCase{
		{"all five characters and unicode", `<t-var name="v" />`, v, `a&amp;b&lt;c&gt;d&quot;e&#x27;f é ✓`},
		{"raw passes through", `<t-raw name="v" />`, v, `a&b<c>d"e'f é ✓`},
		{"literal text is not escaped", `<p class='x'>&amp; é</p>`, nil, `<p class='x'>&amp; é</p>`},
		{"already escaped value is escaped again", `<t-var name="e" />`, map[string]toupee.Value{"e": str("&amp;")}, `&amp;amp;`},
		{"escaped fallback", `<t-var name="m"><b>&</b></t-var>`, nil, `&lt;b&gt;&amp;&lt;/b&gt;`},
		{"raw fallback", `<t-raw name="m"><b>&</b></t-raw>`, nil, `<b>&</b>`},
		{"nested t-var default is escaped twice", `<t-var name="m"><t-var name="v" /></t-var>`, map[string]toupee.Value{"v": str("<")}, `&amp;lt;`},
		{"nested t-var default inside t-raw", `<t-raw name="m"><t-var name="v" /></t-raw>`, map[string]toupee.Value{"v": str("<")}, `&lt;`},
	})
}

func TestRenderFallbackSelection(t *testing.T) {
	data := map[string]toupee.Value{
		"empty": str(""),
		"no":    boolean(false),
		"zero":  integer(0),
		"fzero": {Kind: toupee.KindFloat, Float: 0},
		"nil":   {},
		"list":  {Kind: toupee.KindList, List: []toupee.Value{str("a")}},
		"elist": {Kind: toupee.KindList},
		"map":   {Kind: toupee.KindMap, Map: map[string]toupee.Value{"k": str("v")}},
		"emap":  {Kind: toupee.KindMap, Map: map[string]toupee.Value{}},
	}
	cases := []renderCase{
		{"present empty string beats body", `[<t-var name="empty">d</t-var>]`, data, `[]`},
		{"false beats body", `<t-var name="no">d</t-var>`, data, `false`},
		{"zero beats body", `<t-var name="zero">d</t-var>`, data, `0`},
		{"float zero beats body", `<t-var name="fzero">d</t-var>`, data, `0`},
		{"missing uses body", `<t-var name="absent">d</t-var>`, data, `d`},
		{"missing with empty body", `[<t-var name="absent"></t-var>]`, data, `[]`},
	}
	for _, k := range []string{"nil", "list", "elist", "map", "emap"} {
		cases = append(cases,
			renderCase{k + " uses body", `<t-var name="` + k + `">d</t-var>`, data, `d`},
			renderCase{k + " with empty body", `[<t-raw name="` + k + `"></t-raw>]`, data, `[]`},
		)
	}
	runRenderCases(t, cases)

	for _, k := range []string{"absent", "nil", "list", "elist", "map", "emap"} {
		for _, input := range []string{`<t-var name="` + k + `" />`, `<t-raw name="` + k + `"/>`} {
			got, err := toupee.Render(input, toupee.Context{Data: data}, nil)
			var re *toupee.RenderError
			if got != "" || !errors.As(err, &re) || re.Kind != toupee.UndefinedVariable || re.Message != k {
				t.Errorf("Render(%q) = %q, %v; want \"\" and UndefinedVariable %q", input, got, err, k)
			}
		}
	}
}

func TestRenderParsing(t *testing.T) {
	data := map[string]toupee.Value{"x": str("X"), "y": str("Y"), "a": str(`"q"`)}
	runRenderCases(t, []renderCase{
		{"quoted > in the tag", `<t-var name="m" title="a>b">d</t-var>`, data, `d`},
		{"attribute name boundary", `<t-var xname="y" name="x" />`, data, `X`},
		{"name must follow a space", `<t-var data-name="y" name="x" />`, data, `X`},
		{"empty name is present", `<t-var name="">d</t-var>`, data, `d`},
		{"ordinary html and entities", `<div class="c">&lt;&nbsp;<!-- <b> --></div>`, data, `<div class="c">&lt;&nbsp;<!-- <b> --></div>`},
		{"variable inside an html attribute", `<a title="<t-var name="a" />">`, data, `<a title="&quot;q&quot;">`},
		{"adjacent nodes", `<t-var name="x" /><t-var name="y" /><t-raw name="x"/>`, data, `XYX`},
		{"t-var with bare self-close", `<t-var name="x"/>`, data, `X`},
		{"unknown elements are literal", `<t-varx name="x" /><t-unknown />`, data, `<t-varx name="x" /><t-unknown />`},
		{"opener is case-sensitive", `<T-VAR name="x" />`, data, `<T-VAR name="x" />`},
		{"self-closing default inside a block", `<t-var name="m"><t-var name="x" /></t-var>`, data, `X`},
		{"surrounding newlines kept", "a\n<t-comment>c</t-comment>\nb\r\n", data, "a\n\nb\r\n"},
		{"self-closing comment", `a<t-comment />b`, data, `ab`},
		{"comment hides undefined variable", `<t-comment><t-var name="nope" /></t-comment>ok`, data, `ok`},
		{"comment hides malformed directive", `<t-comment><t-var name="x"</t-comment>ok`, data, `ok`},
		{"comment hides future directive", `<t-comment><t-if condition="x">no</t-if><t-include template="z" /></t-comment><t-var name="x" />`, data, `X`},
		{"comment hides transform", `<t-comment><t-var name="x" transform="upper" /></t-comment><t-var name="x" />`, data, `X`},
		{"html comment is literal", `<!-- <t-var name="x" /> -->`, data, `<!-- X -->`},
		// Toupee v0.4.0 ends a block at the first same-kind closing tag.
		{"first close ends a block", `<t-var name="m">a</t-var>b</t-var>`, data, `ab</t-var>`},
	})

}

func TestRenderMalformedUnusedFallback(t *testing.T) {
	data := map[string]toupee.Value{"x": str("X")}
	got, err := toupee.Render(`<t-var name="x"><t-raw name="y"></t-var>`, toupee.Context{Data: data}, nil)
	var re *toupee.RenderError
	if got != "" || !errors.As(err, &re) || re.Kind != toupee.MalformedElement || re.Message != "missing closing tag" {
		t.Fatalf("Render = %q, %v; want \"\" and MalformedElement missing closing tag", got, err)
	}
	if re.Line != 1 || re.Column != 17 {
		t.Errorf("error at %d:%d, want 1:17", re.Line, re.Column)
	}
}

func TestRenderErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		input     string
		kind      toupee.ErrorKind
		message   string
		line, col int
	}{
		{"unclosed var", `<t-var name="x"`, toupee.MalformedElement, "unclosed variable tag", 1, 1},
		{"unclosed raw after text", `ab<t-raw name="x" `, toupee.MalformedElement, "unclosed variable tag", 1, 3},
		{"quote hides the end", `<t-var name="x>`, toupee.MalformedElement, "unclosed variable tag", 1, 1},
		{"missing name", `x<t-var />`, toupee.MalformedElement, "missing 'name' attribute", 1, 2},
		{"single-quoted name", `<t-raw name='x' />`, toupee.MalformedElement, "missing 'name' attribute", 1, 1},
		{"missing var close", "\n  <t-var name=\"x\">d", toupee.MalformedElement, "missing closing tag", 2, 3},
		{"raw closed by var", `<t-raw name="x">d</t-var>`, toupee.MalformedElement, "missing closing tag", 1, 1},
		{"unclosed comment tag", `ok <t-comment`, toupee.MalformedElement, "unclosed <t-comment> tag", 1, 4},
		{"missing comment close", "a\nb<t-comment>x", toupee.MalformedElement, "missing closing </t-comment> tag", 2, 2},
		{"undefined", `<p><t-var name="user.name" /></p>`, toupee.UndefinedVariable, "user.name", 1, 4},
		{"undefined dynamic path keeps its spelling", `<t-var name="a.$(b)" />`, toupee.UndefinedVariable, "a.$(b)", 1, 1},
		{"utf-8 bytes count", "é✓<t-var name=\"m\" />", toupee.UndefinedVariable, "m", 1, 6},
		{"crlf counts the cr", "a\r\nb\r\n  <t-var name=\"m\" />", toupee.UndefinedVariable, "m", 3, 3},
		{"nested fallback offset", "x\n<t-var name=\"a\">\n  é<t-raw name=\"b\" />\n</t-var>", toupee.UndefinedVariable, "b", 3, 5},
		{"nested malformed offset", "<t-raw name=\"a\">\n\t<t-var name=\"b\"></t-raw>", toupee.MalformedElement, "missing closing tag", 2, 2},
		{"malformed before an unclosed if", `<t-var name="x"><t-if condition="y">`, toupee.MalformedElement, "missing closing tag", 1, 1},
		{"same-kind nested block is cut at the first close", `<t-var name="m">a<t-var name="n">b</t-var>c</t-var>`, toupee.MalformedElement, "missing closing tag", 1, 18},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toupee.Render(tt.input, toupee.Context{}, nil)
			if got != "" {
				t.Errorf("partial output %q", got)
			}
			var re *toupee.RenderError
			if !errors.As(err, &re) {
				t.Fatalf("error %v (%T), want a *RenderError", err, err)
			}
			if re.Kind != tt.kind || re.Message != tt.message || re.Line != tt.line || re.Column != tt.col {
				t.Errorf("got %s %q at %d:%d, want %s %q at %d:%d",
					re.Kind, re.Message, re.Line, re.Column, tt.kind, tt.message, tt.line, tt.col)
			}
		})
	}
}

// TestRenderDebug shows t-debug rendering as nothing, as Toupee renders it
// with its debug option off, the default. Only the opening tag is consumed.
func TestRenderDebug(t *testing.T) {
	x := map[string]toupee.Value{"x": str("X")}
	runRenderCases(t, []renderCase{
		{"self-closing", `before<t-debug />after`, nil, `beforeafter`},
		{"self-closing no space", `before<t-debug/>after`, nil, `beforeafter`},
		{"opening tag", `before<t-debug>after`, nil, `beforeafter`},
		{"only debug", `<t-debug />`, nil, ``},
		{"body and closing tag are text", `a<t-debug>body</t-debug>b`, nil, `abody</t-debug>b`},
		{"quoted > in the tag", `a<t-debug note=">" />b`, nil, `ab`},
		{"no name boundary", `a<t-debugger>b`, nil, `ab`},
		{"upper case is literal", `a<T-debug>b`, nil, `a<T-debug>b`},
		{"adjacent", `a<t-debug /><t-debug><t-debug/>b`, nil, `ab`},
		{"whitespace and newlines kept", "a \n<t-debug />\n b", nil, "a \n\n b"},
		{"malformed debug in a comment", `a<t-comment><t-debug</t-comment>b`, nil, `ab`},
		{"variables on both sides", `<t-var name="x" /><t-debug /><t-var name="x" />`, x, `XX`},
		{"in a conditional", `<t-var name="x" /><t-if var="x">y<t-debug /></t-if>`, x, `Xy`},
		{"after a transform", `<t-var name="x" transform="lower" /><t-debug />`, x, `x`},
		{"after an upper transform", `<t-var name="x" transform="upper" /><t-debug />`, x, `X`},
		{"in a loop body", `<t-for item in xs>[<t-var name="item" /><t-debug />]</t-for>`,
			map[string]toupee.Value{"xs": {Kind: toupee.KindList, List: []toupee.Value{str("a"), str("b")}}}, `[a][b]`},
		{"in a loop over nothing", `<t-for item in xs>a<t-debug /><t-else />none</t-for>`, nil, `none`},
		{"in a bound tag's body", `<a t-var:href="x"><t-debug />l</a>`, x, `<a href="X">l</a>`},
		{"in a slot default", `<t-slot><t-let name="y" transform="upper">1</t-let><t-debug />d</t-slot>`, nil, `d`},
		{"in a variable fallback", `<t-var name="m">a<t-debug />b</t-var>`, nil, `ab`},
	})
}

// TestRenderDebugErrors shows an unclosed t-debug is a MalformedElement at
// its '<', and that t-debug never hides a later error.
func TestRenderDebugErrors(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		kind        toupee.ErrorKind
		message     string
		line, col   int
	}{
		{"unclosed", "\n  <t-debug", toupee.MalformedElement, "unclosed <t-debug> tag", 2, 3},
		{"unmatched double quote", `ab<t-debug note=">`, toupee.MalformedElement, "unclosed <t-debug> tag", 1, 3},
		{"after output", `<t-var name="x" />a<t-debug`, toupee.MalformedElement, "unclosed <t-debug> tag", 1, 20},
		{"in a body", "<t-if var=\"x\">\n<t-debug</t-if>", toupee.MalformedElement, "unclosed <t-debug> tag", 2, 1},
		{"undefined after debug", `a<t-debug /><t-var name="missing" />`, toupee.UndefinedVariable, "missing", 1, 13},
		{"malformed after debug", `a<t-debug /><t-var name="x"`, toupee.MalformedElement, "unclosed variable tag", 1, 13},
		{"undefined before debug", `<t-var name="x" /><t-var name="undefined" /><t-debug />`, toupee.UndefinedVariable, "undefined", 1, 19},
		{"include with no loader", `<t-include template="c"><t-if var="x"><t-debug /></t-if></t-include>`, toupee.TemplateNotFound, "c", 1, 1},
		{"extend with no loader", `<t-extend template="c"><t-define name="a"><t-debug /></t-define>`, toupee.TemplateNotFound, "c", 1, 1},
		{"include in a loop with no loader", `<t-for item in xs><t-include template="c" /></t-for><t-debug />`, toupee.TemplateNotFound, "c", 1, 19},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]toupee.Value{"x": str("X"), "xs": {Kind: toupee.KindList, List: []toupee.Value{str("a")}}}
			got, err := toupee.Render(tt.input, toupee.Context{Data: data}, nil)
			if got != "" {
				t.Errorf("partial output %q", got)
			}
			var re *toupee.RenderError
			if !errors.As(err, &re) {
				t.Fatalf("error %v (%T), want a *RenderError", err, err)
			}
			want := toupee.RenderError{Line: tt.line, Column: tt.col, Kind: tt.kind, Message: tt.message}
			if *re != want {
				t.Errorf("got %+v, want %+v", *re, want)
			}
		})
	}
}

// TestRenderLiteralSyntax shows words that merely look like syntax render
// as text, and t-define and t-extend outside an extend's start are literal
// text, as upstream.
func TestRenderLiteralSyntax(t *testing.T) {
	runRenderCases(t, []renderCase{
		{"text mentions t-if", `t-if and t-var: are words`, nil, `t-if and t-var: are words`},
		{"closing tag with t-var: text", `</a t-var:x>`, nil, `</a t-var:x>`},
		{"attribute value t-var:", `<p title="t-var:">hello</p>`, nil, `<p title="t-var:">hello</p>`},
		{"attribute value t-attr:", `<p title="t-attr:">hello</p>`, nil, `<p title="t-attr:">hello</p>`},
		{"attribute name ending t-var:", `<p data-t-var:example="x">hello</p>`, nil, `<p data-t-var:example="x">hello</p>`},
		{"literal tag beside a variable", `<p title="t-var:"><t-var name="x" /></p>`, map[string]toupee.Value{"x": str("a&b")}, `<p title="t-var:">a&amp;b</p>`},
		{"value mentions a tag", `<t-raw name="v" />`, map[string]toupee.Value{"v": str(`<t-if>`)}, `<t-if>`},
		{"stray t-define", `<t-define name="c"></t-define>`, nil, `<t-define name="c"></t-define>`},
		{"t-extend after content", `a<t-extend template="c">`, nil, `a<t-extend template="c">`},
		{"bound variable", `<a t-var:href="x">l</a>`, map[string]toupee.Value{"x": str("X")}, `<a href="X">l</a>`},
		{"bound attr without include", `<a t-attr:class="x">l</a>`, nil, `<a>l</a>`},
		{"t-attr without include", `[<t-attr name="c" />]`, nil, `[]`},
		{"unfilled slot", `[<t-slot />]`, nil, `[]`},
	})
}
