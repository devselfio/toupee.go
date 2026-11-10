package toupee_test

import (
	"errors"
	"testing"

	"github.com/devselfio/toupee.go"
)

// TestRenderTransforms covers the transform= attribute on t-var and t-raw
// through Render: chain order, the fallback rules and escaping after the
// chain, as Toupee v0.4.0's renderVariable applies them.
func TestRenderTransforms(t *testing.T) {
	data := map[string]toupee.Value{
		"s":     str("hello world"),
		"html":  str(`<b>"Tom" & 'Jerry'</b>`),
		"empty": str(""),
		"no":    boolean(false),
		"zero":  integer(0),
		"num":   integer(-12),
		"nil":   {},
		"list":  {Kind: toupee.KindList, List: []toupee.Value{str("a"), str("b")}},
		"map":   {Kind: toupee.KindMap, Map: map[string]toupee.Value{"k": str("v")}},
	}
	runRenderCases(t, []renderCase{
		{"single", `<t-var name="s" transform="upper" />`, data, `HELLO WORLD`},
		{"left to right", `<t-var name="s" transform="truncate:5|upper" />`, data, `HELLO`},
		{"left to right, other order", `<t-var name="s" transform="upper|replace:L:_|truncate:4" />`, data, `HE__`},
		{"empty pipe steps skipped", `<t-var name="s" transform="|upper||" />`, data, `HELLO WORLD`},
		{"empty attribute is no chain", `<t-var name="s" transform="" />`, data, `hello world`},
		{"empty argument kept", `<t-var name="s" transform="replace:o:" />`, data, `hell wrld`},
		{"argument spaces kept", `<t-var name="s" transform="replace: :_" />`, data, `hello_world`},
		{"integer is text", `<t-var name="num" transform="replace:-:minus " />`, data, `minus 12`},

		// Escaping comes after the chain: t-var escapes, t-raw does not.
		{"t-var escapes the result", `<t-var name="s" transform="replace:o:<&>" />`, data, `hell&lt;&amp;&gt; w&lt;&amp;&gt;rld`},
		{"t-raw is verbatim", `<t-raw name="s" transform="replace:o:<&>" />`, data, `hell<&> w<&>rld`},
		{"escape on t-raw", `<t-raw name="html" transform="escape" />`, data, `&lt;b&gt;&quot;Tom&quot; &amp; &#x27;Jerry&#x27;&lt;/b&gt;`},
		{"escape on t-var escapes twice", `<t-var name="html" transform="escape" />`, data, `&amp;lt;b&amp;gt;&amp;quot;Tom&amp;quot; &amp;amp; &amp;#x27;Jerry&amp;#x27;&amp;lt;/b&amp;gt;`},
		{"length sees the unescaped value", `<t-var name="html" transform="length" />`, data, `22`},

		// A present scalar is used even when empty, false or zero.
		{"empty is present", `[<t-var name="empty" transform="upper" />]`, data, `[]`},
		{"empty takes default", `<t-var name="empty" transform="default:none" />`, data, `none`},
		{"false is present", `<t-var name="no" transform="default:x|upper" />`, data, `FALSE`},
		{"zero is present", `<t-var name="zero" transform="default:x" />`, data, `0`},
		{"present beats body", `<t-var name="s" transform="first:2">body</t-var>`, data, `he`},

		// A body takes precedence over a default step, and is transformed.
		{"body is transformed", `<t-var name="absent" transform="upper">fall <i>back</i></t-var>`, data, `FALL &lt;I&gt;BACK&lt;/I&gt;`},
		{"raw body is transformed", `<t-raw name="absent" transform="upper">fall <i>back</i></t-raw>`, data, `FALL <I>BACK</I>`},
		{"body beats default", `<t-var name="absent" transform="default:d">body</t-var>`, data, `body`},
		{"empty body then default", `<t-var name="absent" transform="default:d"></t-var>`, data, `d`},
		{"transformed body keeps inner escaping", `<t-raw name="absent" transform="upper"><t-var name="html" /></t-raw>`, data, `&LT;B&GT;&QUOT;TOM&QUOT; &AMP; &#X27;JERRY&#X27;&LT;/B&GT;`},
		{"inner transform in body", `<t-raw name="absent" transform="replace:O:0"><t-raw name="s" transform="upper" /></t-raw>`, data, `HELL0 W0RLD`},

		// A default step anywhere in the chain exempts a missing value,
		// which then enters the chain as "".
		{"missing with default", `<t-var name="absent" transform="default:n/a" />`, data, `n/a`},
		{"late default", `<t-var name="absent" transform="upper|default:n/a" />`, data, `n/a`},
		{"default then more", `<t-var name="absent" transform="default:n/a|upper" />`, data, `N/A`},
		{"default without argument", `[<t-var name="absent" transform="default" />]`, data, `[]`},
		{"missing path with default", `<t-var name="a.b.c" transform="default:x" />`, data, `x`},
		{"nil with default", `<t-var name="nil" transform="default:x" />`, data, `x`},
		{"list with default", `<t-var name="list" transform="default:x" />`, data, `x`},
		{"map with default", `<t-raw name="map" transform="length|default:x" />`, data, `0`},
	})
}

// TestRenderTransformErrors checks that a failing or unknown step is a
// MalformedElement at the element, that a default step does not hide it or
// a missing value without one, and that nothing is output.
func TestRenderTransformErrors(t *testing.T) {
	data := map[string]toupee.Value{"s": str("hello"), "nil": {}}
	for _, tt := range []struct {
		name      string
		input     string
		kind      toupee.ErrorKind
		message   string
		line, col int
	}{
		{"unknown name", `<t-var name="s" transform="bogus" />`, toupee.MalformedElement, `unknown transform "bogus"`, 1, 1},
		{"names are case-sensitive", "ab\n <t-raw name=\"s\" transform=\"Upper\" />", toupee.MalformedElement, `unknown transform "Upper"`, 2, 2},
		{"names are not trimmed", `<t-var name="s" transform=" upper" />`, toupee.MalformedElement, `unknown transform " upper"`, 1, 1},
		{"empty name", `<t-var name="s" transform=":x" />`, toupee.MalformedElement, `unknown transform ""`, 1, 1},
		{"unknown after a good step", `x<t-var name="s" transform="upper|bogus" />`, toupee.MalformedElement, `unknown transform "bogus"`, 1, 2},
		{"slice-two name is unknown", `<t-var name="s" transform="abs" />`, toupee.MalformedElement, `unknown transform "abs"`, 1, 1},
		{"bad argument", `<t-var name="s" transform="truncate:x" />`, toupee.MalformedElement, `truncate: invalid count "x"`, 1, 1},
		{"missing argument", `<t-var name="s" transform="truncate" />`, toupee.MalformedElement, `truncate: missing count`, 1, 1},
		{"default does not hide an earlier failing step", `<t-var name="absent" transform="truncate:x|default:d" />`, toupee.MalformedElement, `truncate: invalid count "x"`, 1, 1},
		{"default does not hide an unknown step", `<t-var name="absent" transform="bogus|default:d" />`, toupee.MalformedElement, `unknown transform "bogus"`, 1, 1},
		{"error in a used body", `<t-var name="absent">a<t-raw name="s" transform="bogus" /></t-var>`, toupee.MalformedElement, `unknown transform "bogus"`, 1, 23},
		{"missing without default is undefined", `<t-var name="absent" transform="upper" />`, toupee.UndefinedVariable, "absent", 1, 1},
		{"nil without default is undefined", `<t-raw name="nil" transform="trim|Default" />`, toupee.UndefinedVariable, "nil", 1, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toupee.Render(tt.input, toupee.Context{Data: data}, nil)
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

	// Steps run only when their element renders: an unknown name in an
	// unused body or in a comment is not an error.
	runRenderCases(t, []renderCase{
		{"unused body", `<t-var name="s"><t-var name="s" transform="bogus" /></t-var>`, data, `hello`},
		{"comment", `<t-comment><t-var name="s" transform="bogus" /></t-comment>ok`, data, `ok`},
	})
}
