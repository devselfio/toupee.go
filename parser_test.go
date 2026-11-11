package toupee

import (
	"reflect"
	"testing"
)

func TestParseNodes(t *testing.T) {
	text := func(s string) node { return node{kind: textNode, text: s} }
	vr := func(v variable) node { return node{kind: variableNode, variable: v} }
	for _, tt := range []struct {
		name  string
		input string
		want  []node
	}{
		{"empty", "", nil},
		{"text only", "<p>&amp;</p>\n", []node{text("<p>&amp;</p>\n")}},
		{"self-closing", `a<t-var name="x" />b`, []node{
			text("a"), vr(variable{name: "x", pos: 1}), text("b"),
		}},
		{"empty block keeps hasBody", `<t-raw name="x"></t-raw>`, []node{
			vr(variable{name: "x", raw: true, hasBody: true, pos: 0}),
		}},
		{"body positions are absolute", `ab<t-var name="x">c<t-raw name="y"/></t-var>`, []node{
			text("ab"),
			vr(variable{name: "x", hasBody: true, pos: 2, body: []node{
				text("c"), vr(variable{name: "y", raw: true, pos: 19}),
			}}),
		}},
		{"comment yields no node", "a<t-comment>b</t-comment>c", []node{text("a"), text("c")}},
		{"quoted > inside tag", `<t-var name="x" t=">" />`, []node{vr(variable{name: "x", pos: 0})}},
		// transform= is split on '|' (empty steps skipped) and then ':'
		// (empty arguments kept), with no trimming or case folding.
		{"transform chain", `<t-var name="x" transform="||upper|replace:o:||" />`, []node{
			vr(variable{name: "x", pos: 0, transforms: []transformStep{
				{name: "upper"}, {name: "replace", args: []string{"o", ""}},
			}}),
		}},
		{"transform keeps spaces and case", `<t-raw name="x" transform=" Upper : a |trim">b</t-raw>`, []node{
			vr(variable{name: "x", raw: true, hasBody: true, pos: 0, body: []node{text("b")}, transforms: []transformStep{
				{name: " Upper ", args: []string{" a "}}, {name: "trim"},
			}}),
		}},
		{"unknown transform parses", `<t-var name="x" transform="bogus:1" />`, []node{
			vr(variable{name: "x", pos: 0, transforms: []transformStep{{name: "bogus", args: []string{"1"}}}}),
		}},
		{"empty transform attribute", `<t-var transform="" name="x" />`, []node{vr(variable{name: "x", pos: 0})}},
		{"transform in a body", `<t-var name="m"><t-raw name="x" transform="upper" /></t-var>`, []node{
			vr(variable{name: "m", hasBody: true, pos: 0, body: []node{
				vr(variable{name: "x", raw: true, pos: 16, transforms: []transformStep{{name: "upper"}}}),
			}}),
		}},
		// A t-let's transform= is parsed the same way onto its binding.
		{"let transform chain", `x<t-let name="s" transform="|date:%H:%M||Upper: ">a</t-let>`, []node{
			text("x"),
			{kind: letNode, let: &letBinding{pos: 1, name: "s", body: []node{text("a")}, transforms: []transformStep{
				{name: "date", args: []string{"%H", "%M"}}, {name: "Upper", args: []string{" "}},
			}}},
		}},
		{"let unknown transform parses", `<t-let name="s" transform="bogus"></t-let>`, []node{
			{kind: letNode, let: &letBinding{pos: 0, name: "s", transforms: []transformStep{{name: "bogus"}}}},
		}},
		{"let empty transform attribute", `<t-let transform="" name="s">a</t-let>`, []node{
			{kind: letNode, let: &letBinding{pos: 0, name: "s", body: []node{text("a")}}},
		}},
		// t-debug yields no node. It consumes only its opening tag, matched
		// by the prefix <t-debug with no name boundary, as upstream does,
		// and leaves the text on both sides, whitespace included.
		{"debug self-closing", "before<t-debug />after", []node{text("before"), text("after")}},
		{"debug self-closing no space", "before<t-debug/>after", []node{text("before"), text("after")}},
		{"debug opening tag", "before<t-debug>after", []node{text("before"), text("after")}},
		{"debug body and closer stay text", "a<t-debug>body</t-debug>b", []node{text("a"), text("body</t-debug>b")}},
		{"debug quoted >", `a<t-debug note=">" />b`, []node{text("a"), text("b")}},
		{"debug prefix without boundary", "a<t-debugger>b", []node{text("a"), text("b")}},
		{"debug upper case is literal", "a<T-debug>b", []node{text("a<T-debug>b")}},
		{"debug adjacent", "<t-debug /><t-debug>", nil},
		{"debug keeps whitespace", "a \n<t-debug />\n b", []node{text("a \n"), text("\n b")}},
		{"debug in a comment", "a<t-comment><t-debug</t-comment>b", []node{text("a"), text("b")}},
		{"debug in a body", `<t-var name="x">a<t-debug />b</t-var>`, []node{
			vr(variable{name: "x", hasBody: true, pos: 0, body: []node{text("a"), text("b")}}),
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.nodes, tt.want) {
				t.Errorf("parse(%q) =\n%#v\nwant\n%#v", tt.input, got.nodes, tt.want)
			}
		})
	}
}

// TestParseSupported parses t-debug nested in each enclosing construct: the
// enclosing node is kept, with the text around the debug tag in its body.
func TestParseSupported(t *testing.T) {
	for _, tt := range []struct {
		input string
		kind  nodeKind
	}{
		{`<t-for x in y><t-debug /></t-for>`, loopNode},
		{`<t-if var="y">a<t-elif var="z" /><t-debug /></t-if>`, conditionalNode},
		{`a<t-var name="x">b<t-debug /></t-var>`, variableNode},
		{`<t-include template="c"><t-debug /></t-include>`, includeNode},
		{`<t-extend template="c"><t-define><t-if var="x"><t-debug /></t-if></t-define>`, extendNode},
		{`<t-slot><t-for x in y><t-debug /></t-for></t-slot>`, slotNode},
	} {
		got, err := parse(tt.input)
		if err != nil || len(got.nodes) == 0 || got.nodes[len(got.nodes)-1].kind != tt.kind {
			t.Errorf("parse(%q) = %+v, %v; want a last node of kind %d", tt.input, got, err, tt.kind)
		}
	}
	got, err := parse(`<t-for x in y>a<t-debug />b</t-for>`)
	if err != nil || len(got.nodes) != 1 || got.nodes[0].kind != loopNode {
		t.Fatalf("parse loop = %+v, %v", got, err)
	}
	if body := got.nodes[0].loop.body; !reflect.DeepEqual(body, []node{{kind: textNode, text: "a"}, {kind: textNode, text: "b"}}) {
		t.Errorf("loop body = %#v; want text a, text b", body)
	}
}

// TestParseLiteralSyntax parses text that only looks like syntax, and
// constructs the port implements, without error.
func TestParseLiteralSyntax(t *testing.T) {
	for _, input := range []string{
		`<t-comment><t-for x in="y"></t-comment>`,
		`<t-if var="y">a</t-if><t-for x in y>b<t-else />c</t-for><t-let name="z">d</t-let>`,
		`t-if t-var:x`,
		`<!-- t-var:x -->`,
		`<p title="t-var:">hello</p>`,
		`<p title="t-attr:">hello</p>`,
		`<p data-t-var:example="x">hello</p>`,
		`<p data-t-attr:example="x">hello</p>`,
		`<t-let name="y" transform="upper">b</t-let>`,
		`<t-for x in y><t-let name="z" transform="bogus">1</t-let></t-for>`,
		`a<t-var name="x">b<t-slot /></t-var>`,
		`<div t-attr:class="c"></div>`,
		`<a href="/" t-var:title="t">l</a>`,
		`<t-include template="c" /><t-attr name="a" />`,
		` <t-extend template="c">`,
		`<t-include template="c"><t-define name="a"><t-let name="b">c</t-let></t-define></t-include>`,
		`<t-include template="c"><t-define name="a"><t-let name="b" transform="upper">c</t-let></t-define></t-include>`,
		`<t-slot><t-for x in y>a</t-for></t-slot>`,
	} {
		if _, err := parse(input); err != nil {
			t.Errorf("parse(%q): %v", input, err)
		}
	}
}

func TestParseHTMLHelpers(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want int
	}{
		{"<a>", 2}, {`<a b=">">`, 8}, {`<a b='>'>`, 6}, {`<a b=">`, -1}, {"<a", -1}, {"", -1},
	} {
		if got := findTagEnd(tt.in); got != tt.want {
			t.Errorf("findTagEnd(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
	for _, tt := range []struct {
		tag, name, want string
		ok              bool
	}{
		{`<t-var name="x" />`, "name", "x", true},
		{`<t-var name="" />`, "name", "", true},
		{`<t-var xname="y" name="x" />`, "name", "x", true},
		{`<t-var xname="y" />`, "name", "", false},
		{`<t-var name='x' />`, "name", "", false},
		{`<t-var name = "x" />`, "name", "", false},
		{`<t-var name="&amp;" />`, "name", "&amp;", true},
		{`<t-var name="x`, "name", "", false},
	} {
		got, ok := attrValue(tt.tag, tt.name)
		if got != tt.want || ok != tt.ok {
			t.Errorf("attrValue(%q, %q) = %q, %v; want %q, %v", tt.tag, tt.name, got, ok, tt.want, tt.ok)
		}
	}
	if got, want := escapeHTML(`&<>"'é`), "&amp;&lt;&gt;&quot;&#x27;é"; got != want {
		t.Errorf("escapeHTML = %q, want %q", got, want)
	}
}

func TestErrorAt(t *testing.T) {
	src := "ab\r\né\n<"
	for _, tt := range []struct{ pos, line, col int }{
		{0, 1, 1}, {2, 1, 3}, {3, 1, 4}, {4, 2, 1}, {6, 2, 3}, {7, 3, 1}, {100, 3, 2},
	} {
		e := errorAt(src, tt.pos, MalformedElement, "m")
		if e.Line != tt.line || e.Column != tt.col || e.Kind != MalformedElement || e.Message != "m" {
			t.Errorf("errorAt(pos %d) = %+v, want %d:%d", tt.pos, *e, tt.line, tt.col)
		}
	}
}
