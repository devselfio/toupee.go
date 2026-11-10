package toupee

import (
	"errors"
	"reflect"
	"sort"
	"testing"
)

// sliceOneTransforms are the built-ins issue #14's first slice registers.
// The numeric, boolean and date names (abs, floor, ceil, int, float,
// decimal, bool, date) belong to its second slice.
var sliceOneTransforms = []string{
	"capitalize", "default", "escape", "first", "join", "js_escape", "last",
	"length", "lower", "replace", "slugify", "split", "trim", "truncate",
	"upper", "url_decode", "url_encode",
}

func TestBuiltinTransformNames(t *testing.T) {
	var got []string
	for name := range builtinTransforms {
		got = append(got, name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, sliceOneTransforms) {
		t.Errorf("builtinTransforms = %q, want %q", got, sliceOneTransforms)
	}
}

// TestTransformBuiltins checks each built-in's exact bytes, as Toupee
// v0.4.0's src/transform.zig produces them, and its argument errors.
func TestTransformBuiltins(t *testing.T) {
	a := func(args ...string) []string { return args }
	for _, tt := range []struct {
		name    string
		value   string
		args    []string
		want    string
		wantErr bool
	}{
		// upper/lower: ASCII letters only.
		{"upper", "hello", nil, "HELLO", false},
		{"upper", "héllo ß1", nil, "HéLLO ß1", false},
		{"lower", "HeLLO É", nil, "hello É", false},
		{"upper", "", nil, "", false},

		// capitalize: a letter after start, space, tab or LF; never lowercases.
		{"capitalize", "hello wORLD", nil, "Hello WORLD", false},
		{"capitalize", "a\tb\nc\rd-e", nil, "A\tB\nC\rd-e", false},
		{"capitalize", "  x", nil, "  X", false},
		{"capitalize", "1a éa", nil, "1a éa", false},

		// trim: exactly space, tab, LF, CR.
		{"trim", " \t\r\nx y\n \r", nil, "x y", false},
		{"trim", "\vx\f", nil, "\vx\f", false},
		{"trim", " x", nil, " x", false},

		// slugify: ASCII alphanumerics lowered, other runs one hyphen.
		{"slugify", "Hello World", nil, "hello-world", false},
		{"slugify", "  --Hello,  World!!  ", nil, "hello-world", false},
		{"slugify", "Café au lait", nil, "caf-au-lait", false},
		{"slugify", "!!!", nil, "", false},
		{"slugify", "a_b", nil, "a-b", false},

		// truncate: byte count, no ellipsis, count required.
		{"truncate", "hello", a("3"), "hel", false},
		{"truncate", "hello", a("5"), "hello", false},
		{"truncate", "hello", a("99"), "hello", false},
		{"truncate", "hello", a("0"), "", false},
		{"truncate", "é", a("1"), "\xc3", false},
		{"truncate", "hello", a("+2"), "he", false},
		{"truncate", "hello", a("-0"), "", false},
		{"truncate", "hello", a("1_0"), "hello", false},
		{"truncate", "hello", a("3", "x"), "hel", false},
		{"truncate", "hello", nil, "", true},
		{"truncate", "hello", a(""), "", true},
		{"truncate", "hello", a("x"), "", true},
		{"truncate", "hello", a("-1"), "", true},
		{"truncate", "hello", a(" 3"), "", true},
		{"truncate", "hello", a("_3"), "", true},
		{"truncate", "hello", a("18446744073709551616"), "", true},

		// replace: nonoverlapping, left to right; empty old is a no-op.
		{"replace", "foo boo", a("o", "0"), "f00 b00", false},
		{"replace", "aaa", a("aa", "b"), "ba", false},
		{"replace", "foo", a("o"), "f", false},
		{"replace", "foo", a("o", ""), "f", false},
		{"replace", "foo", a("", "x"), "foo", false},
		{"replace", "foo", a("x", "y"), "foo", false},
		{"replace", "foo", nil, "", true},

		// default: only for the empty string.
		{"default", "", a("n/a"), "n/a", false},
		{"default", "x", a("n/a"), "x", false},
		{"default", " ", a("n/a"), " ", false},
		{"default", "", nil, "", false},

		// length: bytes, in decimal.
		{"length", "hello", nil, "5", false},
		{"length", "", nil, "0", false},
		{"length", "é✓", nil, "5", false},

		// escape: Toupee's five entities, nothing else.
		{"escape", `<b>"hi"</b>`, nil, "&lt;b&gt;&quot;hi&quot;&lt;/b&gt;", false},
		{"escape", `a&b'c é`, nil, "a&amp;b&#x27;c é", false},
		{"escape", "&amp;", nil, "&amp;amp;", false},

		// url_encode: unreserved kept, every other byte %XX uppercase.
		{"url_encode", "hello world&foo=bar", nil, "hello%20world%26foo%3Dbar", false},
		{"url_encode", "-._~aZ09", nil, "-._~aZ09", false},
		{"url_encode", "é+/", nil, "%C3%A9%2B%2F", false},

		// url_decode: valid pairs only; '+' and bad sequences are kept.
		{"url_decode", "hello%20world%26foo", nil, "hello world&foo", false},
		{"url_decode", "a+b", nil, "a+b", false},
		{"url_decode", "%c3%A9", nil, "é", false},
		{"url_decode", "%zz%4", nil, "%zz%4", false},
		{"url_decode", "100%", nil, "100%", false},
		{"url_decode", "%41", nil, "A", false},
		{"url_decode", "%41x", nil, "Ax", false},
		{"url_decode", "%%41x", nil, "%Ax", false},

		// js_escape: backslash, quotes, LF, CR, tab, U+2028, U+2029.
		{"js_escape", `a\b`, nil, `a\\b`, false},
		{"js_escape", `say "hi" it's`, nil, `say \"hi\" it\'s`, false},
		{"js_escape", "l1\nl2\rl3\tc", nil, `l1\nl2\rl3\tc`, false},
		{"js_escape", "a\xe2\x80\xa8b\xe2\x80\xa9c", nil, `a\u2028b\u2029c`, false},
		{"js_escape", "<x>&é", nil, "<x>&é", false},
		{"js_escape", "a\xe2\x80\xa8", nil, `a\u2028`, false},
		{"js_escape", "\xe2\x80", nil, "\xe2\x80", false},

		// join: LF boundaries to the separator, default one space.
		{"join", "a\nb\nc", a(", "), "a, b, c", false},
		{"join", "a\nb", nil, "a b", false},
		{"join", "a\n\nb\n", a("-"), "a--b-", false},
		{"join", "", a("-"), "", false},

		// split: separator to LF, except a separator at the very start.
		{"split", "a,b,c", a(","), "a\nb\nc", false},
		{"split", ",a,,b,", a(","), "a\n\nb\n", false},
		{"split", ",,a", a(","), "\na", false},
		{"split", "a::b", a("::"), "a\nb", false},
		{"split", "a,b", a(""), "a,b", false},
		{"split", "a,b", nil, "", true},

		// first/last: byte windows, default 1, clamped.
		{"first", "hello", nil, "h", false},
		{"first", "hello", a("3"), "hel", false},
		{"first", "hello", a("9"), "hello", false},
		{"first", "hello", a("0"), "", false},
		{"first", "é", nil, "\xc3", false},
		{"first", "hello", a("x"), "", true},
		{"first", "hello", a(""), "", true},
		{"last", "hello", nil, "o", false},
		{"last", "hello", a("3"), "llo", false},
		{"last", "hello", a("9"), "hello", false},
		{"last", "hello", a("0"), "", false},
		{"last", "é", nil, "\xa9", false},
		{"last", "hello", a("-2"), "", true},
	} {
		fn, ok := builtinTransforms[tt.name]
		if !ok {
			t.Errorf("%s is not registered", tt.name)
			continue
		}
		got, err := fn(tt.value, tt.args)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s(%q, %q) = %q, want an error", tt.name, tt.value, tt.args, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s(%q, %q) = %q, %v; want %q", tt.name, tt.value, tt.args, got, err, tt.want)
		}
	}
}

func TestParseTransformSpec(t *testing.T) {
	step := func(name string, args ...string) transformStep {
		return transformStep{name: name, args: args}
	}
	for _, tt := range []struct {
		spec string
		want []transformStep
	}{
		{"", nil},
		{"|", nil},
		{"upper", []transformStep{step("upper")}},
		{"upper|lower", []transformStep{step("upper"), step("lower")}},
		{"||upper|||trim|", []transformStep{step("upper"), step("trim")}},
		{"replace:o:", []transformStep{step("replace", "o", "")}},
		{"replace:o", []transformStep{step("replace", "o")}},
		{"a::b:", []transformStep{step("a", "", "b", "")}},
		{":x", []transformStep{step("", "x")}},
		{" upper | Lower", []transformStep{step(" upper "), step(" Lower")}},
		{"replace: : ", []transformStep{step("replace", " ", " ")}},
		{"date:%H:%M", []transformStep{step("date", "%H", "%M")}},
		{"&amp;", []transformStep{step("&amp;")}},
	} {
		got := parseTransformSpec(tt.spec)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseTransformSpec(%q) = %#v, want %#v", tt.spec, got, tt.want)
		}
	}
}

func TestHasDefaultTransform(t *testing.T) {
	for _, tt := range []struct {
		spec string
		want bool
	}{
		{"", false},
		{"default", true},
		{"upper|default:x", true},
		{"default:x|upper", true},
		{"Default", false},
		{" default", false},
		{"defaults", false},
	} {
		if got := hasDefaultTransform(parseTransformSpec(tt.spec)); got != tt.want {
			t.Errorf("hasDefaultTransform(%q) = %v, want %v", tt.spec, got, tt.want)
		}
	}
}

func TestApplyTransforms(t *testing.T) {
	for _, tt := range []struct {
		value, spec, want string
	}{
		{"Hi", "", "Hi"},
		// Order matters: truncating first keeps the original's first bytes.
		{"hello world", "truncate:5|upper", "HELLO"},
		{"hello world", "upper|replace:L:_|truncate:4", "HE__"},
		{"a b", "replace: :\n|join:+", "a+b"},
		{"", "default:x|upper", "X"},
		{"", "upper|default:x", "x"},
	} {
		got, err := applyTransforms(tt.value, parseTransformSpec(tt.spec))
		if err != nil || got != tt.want {
			t.Errorf("applyTransforms(%q, %q) = %q, %v; want %q", tt.value, tt.spec, got, err, tt.want)
		}
	}
	for _, spec := range []string{
		"bogus", "upper|bogus", "Upper", " upper", ":x", "truncate|bogus",
		"bogus|default", "truncate:x|default:y", "abs", "date",
	} {
		got, err := applyTransforms("v", parseTransformSpec(spec))
		var re *RenderError
		if !errors.As(err, &re) || re.Kind != MalformedElement {
			t.Errorf("applyTransforms(%q) = %q, %v; want a MalformedElement error", spec, got, err)
		}
	}
}
