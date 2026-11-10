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
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got.deferred {
				t.Fatal("unexpectedly deferred")
			}
			if !reflect.DeepEqual(got.nodes, tt.want) {
				t.Errorf("parse(%q) =\n%#v\nwant\n%#v", tt.input, got.nodes, tt.want)
			}
		})
	}
}

func TestParseDeferred(t *testing.T) {
	for _, input := range []string{
		`<t-for x in="y"></t-for>`,
		`a<t-var name="x">b<t-if var="y">c</t-if></t-var>`,
		`<t-var name="x" transform="upper" />`,
		`<t-include template="c"><t-define name="a"><t-let name="b">c</t-let></t-define></t-include>`,
		`<t-include template="c"><t-debug /></t-include>`,
		`<t-extend template="c"><t-define><t-else /></t-define>`,
		`<t-slot><t-for x in="y"></t-for></t-slot>`,
	} {
		got, err := parse(input)
		if err != nil || !got.deferred || got.nodes != nil {
			t.Errorf("parse(%q) = %+v, %v; want deferred with no nodes", input, got, err)
		}
	}
	for _, input := range []string{
		`<t-comment><t-for x in="y"></t-comment>`,
		`t-if t-var:x`,
		`<!-- t-var:x -->`,
		`<p title="t-var:">hello</p>`,
		`<p title="t-attr:">hello</p>`,
		`<p data-t-var:example="x">hello</p>`,
		`<p data-t-attr:example="x">hello</p>`,
		`a<t-var name="x">b<t-slot /></t-var>`,
		`<div t-attr:class="c"></div>`,
		`<a href="/" t-var:title="t">l</a>`,
		`<t-include template="c" /><t-attr name="a" />`,
		` <t-extend template="c">`,
	} {
		got, err := parse(input)
		if err != nil || got.deferred {
			t.Errorf("parse(%q) = %+v, %v; want not deferred", input, got, err)
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
