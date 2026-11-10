package toupee

import (
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"
)

func vs(s string) Value           { return Value{Kind: KindString, String: s} }
func vl(items ...Value) Value     { return Value{Kind: KindList, List: items} }
func vm(m map[string]Value) Value { return Value{Kind: KindMap, Map: m} }

type controlCase struct {
	name  string
	input string
	ctx   Context
	want  string
}

func runControlCases(t *testing.T, cases []controlCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.input, tt.ctx, nil)
			if err != nil {
				t.Fatalf("Render(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Render(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func data(m map[string]Value) Context { return Context{Data: m} }

// Test Plan 2: presence versus contents, glob and unselected branches.
func TestControlConditions(t *testing.T) {
	present := data(map[string]Value{
		"nil":   {},
		"list":  vl(vs("a")),
		"map":   vm(map[string]Value{"k": vs("v")}),
		"empty": vs(""),
		"zero":  {Kind: KindInteger},
		"f":     {Kind: KindBoolean},
		"path":  vs("a/b/c.txt"),
		"brk":   vs("[x]"),
		"star":  vs("a*b"),
	})
	cond := func(attrs string) string { return `<t-if ` + attrs + `>T<t-else />F</t-if>` }
	runControlCases(t, []controlCase{
		{"present nil equals empty", cond(`var="nil" equals=""`), present, "T"},
		{"present list equals empty", cond(`var="list" equals=""`), present, "T"},
		{"present map equals empty", cond(`var="map" equals=""`), present, "T"},
		{"missing never equals empty", cond(`var="gone" equals=""`), present, "F"},
		{"missing not-equals anything", cond(`var="gone" not-equals=""`), present, "T"},
		{"present nil not-equals empty is false", cond(`var="nil" not-equals=""`), present, "F"},
		{"missing contains nothing", cond(`var="gone" contains=""`), present, "F"},
		{"missing matches nothing", cond(`var="gone" matches="*"`), present, "F"},
		{"present nil exists", cond(`var="nil"`), present, "T"},
		{"present nil is not truthy", cond(`var="nil" truthy`), present, "F"},
		{"present nil not-truthy", cond(`var="nil" not-truthy`), present, "T"},
		{"empty string exists", cond(`var="empty" exists`), present, "T"},
		{"zero exists, not truthy", cond(`var="zero" truthy`), present, "F"},
		{"false exists", cond(`var="f" not-exists`), present, "F"},
		{"missing not-exists", cond(`var="gone" not-exists`), present, "T"},
		{"missing not-truthy", cond(`var="gone" not-truthy`), present, "T"},
		{"nonempty list truthy", cond(`var="list" truthy`), present, "T"},
		{"no source tests the root", cond(`condition="x"`), present, "T"},
		{"not-equals is not read as equals", cond(`var="empty" not-equals=""`), present, "F"},
		{"not-truthy is not read as truthy", cond(`var="list" not-truthy`), present, "F"},
		{"glob star crosses slashes", cond(`var="path" matches="a*txt"`), present, "T"},
		{"glob question mark is one byte", cond(`var="path" matches="a?b?c.txt"`), present, "T"},
		{"glob is whole-string", cond(`var="path" matches="a/b"`), present, "F"},
		{"brackets are literal", cond(`var="brk" matches="[x]"`), present, "T"},
		{"brackets are not a class", cond(`var="brk" matches="[xy]"`), present, "F"},
		{"star in text matches literally", cond(`var="star" matches="a*b"`), present, "T"},
		{"elif after false if", `<t-if var="gone">1<t-elif var="f" equals="false" />2<t-else />3</t-if>`, present, "2"},
	})

	// An unselected branch is never rendered, so its undefined variable is
	// not an error; a selected one is.
	got, err := Render(`<t-if var="x">ok<t-else /><t-var name="undefined" /></t-if>`, data(map[string]Value{"x": vs("1")}), nil)
	if got != "ok" || err != nil {
		t.Errorf("unselected branch: got %q, %v; want \"ok\", nil", got, err)
	}
	_, err = Render(`<t-if var="x"><t-var name="undefined" /></t-if>`, data(map[string]Value{"x": vs("1")}), nil)
	var re *RenderError
	if !errors.As(err, &re) || re.Kind != UndefinedVariable || re.Message != "undefined" {
		t.Errorf("selected branch: got %v; want UndefinedVariable undefined", err)
	}

	// Attrs: presence for exists, nonempty for truthy. Slots: presence,
	// even of an empty body, for everything but the negations.
	ac := Context{Attrs: map[string]string{"e": "", "v": "x"}, Slots: map[string]string{"s": ""}}
	runControlCases(t, []controlCase{
		{"empty attr exists", cond(`attr="e"`), ac, "T"},
		{"empty attr is not truthy", cond(`attr="e" truthy`), ac, "F"},
		{"absent attr not-truthy", cond(`attr="none" not-truthy`), ac, "T"},
		{"attr equals", cond(`attr="v" equals="x"`), ac, "T"},
		{"absent attr not-equals", cond(`attr="none" not-equals="x"`), ac, "T"},
		{"empty slot exists", cond(`slot="s"`), ac, "T"},
		{"empty slot counts as truthy", cond(`slot="s" truthy`), ac, "T"},
		{"present slot equals anything", cond(`slot="s" equals="nope"`), ac, "T"},
		{"absent slot not-exists", cond(`slot="none" not-exists`), ac, "T"},
		{"absent slot equals nothing", cond(`slot="none" equals=""`), ac, "F"},
		{"var wins over attr", cond(`attr="v" var="gone"`), ac, "F"},
	})
}

func TestGlobMatch(t *testing.T) {
	for _, tt := range []struct {
		text, pattern string
		want          bool
	}{
		{"", "", true}, {"", "*", true}, {"", "?", false}, {"a", "", false},
		{"abc", "a*c", true}, {"abc", "*b*", true}, {"abc", "*d*", false},
		{"aXbXc", "a*c", true}, {"aaa", "a*a*a", true}, {"ab", "a*b*c", false},
		{"é", "?", false}, {"é", "??", true}, // bytes, not runes
		{"x/y", "*", true}, {`a\b`, `a\b`, true}, {"a.b", "a?b", true},
	} {
		if got := globMatch(tt.text, tt.pattern); got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.text, tt.pattern, got, tt.want)
		}
	}
}

// Test Plan 3: ownership of nested separators, and parse errors.
func TestControlNestingAndErrors(t *testing.T) {
	d := data(map[string]Value{"yes": vs("1"), "xs": vl(vs("a"), vs("b")), "none": vl()})
	runControlCases(t, []controlCase{
		{"loop else inside if body", `<t-if var="yes"><t-for x in none>L<t-else />E</t-for><t-else />N</t-if>`, d, "E"},
		{"if else inside loop body", `<t-for x in xs><t-if var="gone">Y<t-else /><t-var name="x" /></t-if><t-else />E</t-for>`, d, "ab"},
		{"nested if keeps its elif", `<t-if var="gone">A<t-else /><t-if var="gone">B<t-elif var="yes" />C</t-if></t-if>`, d, "C"},
		{"comment hides stray separators", `a<t-comment><t-else /><t-elif x="y"></t-comment>b`, d, "ab"},
		{"comment inside branch", `<t-if var="yes">a<t-comment>no</t-comment>b</t-if>`, d, "ab"},
		{"bare t-if tests the root", `<t-if>x</t-if>`, d, "x"},
	})

	for _, tt := range []struct {
		name, input, message string
		line, col            int
	}{
		{"malformed unselected branch", `<t-if var="yes">ok<t-else /><t-var></t-if>`, "missing 'name' attribute", 1, 29},
		{"stray else", "a\n  <t-else />", "stray <t-else> or <t-elif> outside conditional or loop", 2, 3},
		{"stray elif", `<t-elif var="x">`, "stray <t-else> or <t-elif> outside conditional or loop", 1, 1},
		{"elif in a loop is stray", `<t-for x in xs>a<t-elif var="x" />b</t-for>`, "stray <t-else> or <t-elif> outside conditional or loop", 1, 17},
		{"unclosed if tag", `<t-if var="x"`, "unclosed <t-if> tag", 1, 1},
		{"missing if close", "x\n <t-if var=\"x\">a", "missing closing </t-if> tag", 2, 2},
		{"unclosed for tag", `<t-for x in xs`, "unclosed <t-for> tag", 1, 1},
		{"missing for close", `<t-for x in xs>a`, "missing closing </t-for> tag", 1, 1},
		{"missing in", `<t-for x xs></t-for>`, "missing 'in' keyword in <t-for>", 1, 1},
		{"missing item", `<t-for x></t-for>`, "missing item variable in <t-for>", 1, 1},
		{"letters in limit", `<t-for x in xs limit="two"></t-for>`, "invalid integer value for attribute", 1, 1},
		{"empty offset", `<t-for x in xs offset=""></t-for>`, "invalid integer value for attribute", 1, 1},
		{"negative limit", `<t-for x in xs limit="-1"></t-for>`, "invalid integer value for attribute", 1, 1},
		{"overflowing offset", `<t-for x in xs offset="18446744073709551616"></t-for>`, "invalid integer value for attribute", 1, 1},
		{"multiline malformed loop", "ok\n\t<t-for x in xs\n  limit=\"x\">\n</t-for>", "invalid integer value for attribute", 2, 2},
		{"let without name", `<t-let>x</t-let>`, "missing 'name' attribute on <t-let>", 1, 1},
		{"let without close", "\n<t-let name=\"x\">x", "missing closing </t-let> tag", 2, 1},
		{"unclosed let tag", `<t-let name="x"`, "unclosed <t-let> tag", 1, 1},
		{"error inside nested body keeps source offset", "<t-for x in xs>\n<t-if var=\"x\">\n  <t-var /></t-if></t-for>", "missing 'name' attribute", 3, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.input, d, nil)
			var re *RenderError
			if got != "" || !errors.As(err, &re) {
				t.Fatalf("Render = %q, %v; want \"\" and a RenderError", got, err)
			}
			if re.Kind != MalformedElement || re.Message != tt.message || re.Line != tt.line || re.Column != tt.col {
				t.Errorf("got %s %q at %d:%d, want MalformedElement %q at %d:%d", re.Kind, re.Message, re.Line, re.Column, tt.message, tt.line, tt.col)
			}
		})
	}
}

func TestParseUnsigned(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"0", 0, true}, {"7", 7, true}, {"+7", 7, true}, {"-0", 0, true}, {"1_000", 1000, true},
		{"18446744073709551615", math.MaxUint64, true},
		{"18446744073709551616", 0, false}, {"-1", 0, false}, {"", 0, false}, {"+", 0, false},
		{"_1", 0, false}, {"1_", 0, false}, {"0x10", 0, false}, {" 1", 0, false}, {"1.0", 0, false},
	} {
		got, ok := parseUnsigned(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseUnsigned(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

// Test Plan 4: windows, sorting, metadata and unchanged inputs.
func TestControlLoops(t *testing.T) {
	item := func(title string, extra map[string]Value) Value {
		m := map[string]Value{"title": vs(title)}
		for k, v := range extra {
			m[k] = v
		}
		return vm(m)
	}
	build := func() Context {
		return data(map[string]Value{
			"posts": vl(
				item("B", map[string]Value{"date": vs("2024-02")}),
				item("A", map[string]Value{"date": vs("2024-03")}),
				item("C", map[string]Value{"date": {Kind: KindInteger, Integer: 9}}),
				item("D", nil),
				item("E", map[string]Value{"date": vs("2024-01")}),
			),
			"box":  vm(map[string]Value{"items": vl(vs("x"), vs("y"))}),
			"bad":  vm(map[string]Value{"items": vs("x")}),
			"text": vs("abc"),
		})
	}
	ctx := build()
	const each = `<t-var name="p.title" />`
	runControlCases(t, []controlCase{
		{"zero limit renders else", `<t-for p in posts limit="0">x<t-else />empty</t-for>`, ctx, "empty"},
		{"offset at end renders else", `<t-for p in posts offset="5">x<t-else />empty</t-for>`, ctx, "empty"},
		{"offset past end renders else", `<t-for p in posts offset="18446744073709551615">x<t-else />empty</t-for>`, ctx, "empty"},
		{"huge limit keeps all", `<t-for p in posts limit="18446744073709551615">` + each + `</t-for>`, ctx, "BACDE"},
		{"map with items list", `<t-for i in box><t-var name="i" /></t-for>`, ctx, "xy"},
		{"map with non-list items", `<t-for i in bad>x<t-else />E</t-for>`, ctx, "E"},
		{"string collection", `<t-for i in text>x<t-else />E</t-for>`, ctx, "E"},
		{"missing collection", `<t-for i in gone>x<t-else />E</t-for>`, ctx, "E"},
		{"missing collection without else", `a<t-for i in gone>x</t-for>b`, ctx, "ab"},
		{"missing and non-string keys sort first, stably", `<t-for p in posts sort="date">` + each + `</t-for>`, ctx, "CDEBA"},
		{"descending", `<t-for p in posts sort="date" order="desc">` + each + `</t-for>`, ctx, "ABECD"},
		{"order other than desc ascends", `<t-for p in posts sort="title" order="DESC">` + each + `</t-for>`, ctx, "ABCDE"},
		{"empty sort field sorts string items", `<t-for i in list sort="">` + `<t-var name="i" /></t-for>`, data(map[string]Value{"list": vl(vs("b"), vs("a"))}), "ab"},
		{
			"sort, then offset, then limit; metadata describes the window",
			`<t-for p in posts sort="title" order="desc" offset="1" limit="3" as loop>` +
				`<t-var name="loop.index" />/<t-var name="loop.number" />/<t-var name="loop.length" />` +
				`<t-if var="loop.first">F</t-if><t-if var="loop.last">L</t-if><t-if var="loop.first" equals="false">!</t-if>` + each + `,</t-for>`,
			ctx, "0/1/3FD,1/2/3C,2/3/3LB,",
		},
		{"alias overrides item of the same name", `<t-for p in posts limit="1" as p><t-var name="p.number" /></t-for>`, ctx, "1"},
	})

	want := build()
	for i := 0; i < 2; i++ {
		got, err := Render(`<t-for p in posts sort="title">`+each+`</t-for>`, ctx, nil)
		if got != "ABCDE" || err != nil {
			t.Fatalf("run %d: got %q, %v", i, got, err)
		}
	}
	if !reflect.DeepEqual(ctx, want) {
		t.Errorf("sorting changed the caller's collection:\n got %#v\nwant %#v", ctx, want)
	}
}

func TestLoopWindow(t *testing.T) {
	items := []Value{vs("a"), vs("b"), vs("c")}
	u := func(n uint64) *uint64 { return &n }
	for _, tt := range []struct {
		limit, offset *uint64
		want          int
	}{
		{nil, nil, 3}, {u(0), nil, 0}, {nil, u(0), 3}, {u(2), u(2), 1},
		{u(math.MaxUint64), u(math.MaxUint64), 0}, {u(math.MaxUint64), u(1), 2},
	} {
		if got := loopWindow(items, tt.limit, tt.offset); len(got) != tt.want {
			t.Errorf("loopWindow(limit %v, offset %v) has %d items, want %d", tt.limit, tt.offset, len(got), tt.want)
		}
	}
}

// Test Plan 5: capture and scope.
func TestControlLetAndScope(t *testing.T) {
	d := func() Context {
		return Context{Data: map[string]Value{
			"x":     vs("outer"),
			"items": vl(vs("1"), vs("2")),
			"html":  vs("<b>"),
		}}
	}
	runControlCases(t, []controlCase{
		{"let in a selected branch stays visible", `<t-if var="x"><t-let name="y">set</t-let></t-if><t-var name="y" />`, d(), "set"},
		{"let emits nothing and overwrites", `<t-let name="x">a</t-let>[<t-var name="x" />]<t-let name="x">b</t-let><t-var name="x" />`, d(), "[a]b"},
		{"let reads the scope it runs in", `<t-let name="y">[<t-var name="x" />]</t-let><t-var name="y" />`, d(), "[outer]"},
		{"let name is a literal key", `<t-let name="a.b">v</t-let><t-var name="a.b">missing</t-var>`, d(), "missing"},
		{
			"loop-local let does not reach the next iteration or the parent",
			`<t-for i in items><t-var name="y">-</t-var><t-let name="y"><t-var name="i" /></t-let><t-var name="y" />,</t-for><t-var name="y">gone</t-var>`,
			d(), "-1,-2,gone",
		},
		{"loop let does not overwrite parent binding", `<t-for i in items><t-let name="x">in</t-let></t-for><t-var name="x" />`, d(), "outer"},
		{
			"nested loop rebinds then restores the outer item",
			`<t-for x in items>[<t-for x in items><t-var name="x" /></t-for><t-var name="x" />]</t-for><t-var name="x" />`,
			d(), "[121][122]outer",
		},
		{
			"nested aliases",
			`<t-for a in items as loop><t-for b in items as loop><t-var name="loop.number" /></t-for>:<t-var name="loop.number" />,</t-for>`,
			d(), "12:1,12:2,",
		},
		{"captured markup escapes through t-var", `<t-let name="c"><i><t-raw name="html" /></i></t-let><t-var name="c" />`, d(), "&lt;i&gt;&lt;b&gt;&lt;/i&gt;"},
		{"captured markup is verbatim through t-raw", `<t-let name="c"><i><t-var name="html" /></i></t-let><t-raw name="c" />`, d(), "<i>&lt;b&gt;</i>"},
	})

	for _, input := range []string{
		`<t-let name="x">changed</t-let><t-let name="new">n</t-let><t-for i in items as m><t-let name="items">z</t-let></t-for>`,
		`<t-let name="x">changed</t-let><t-var name="undefined" />`,
	} {
		ctx, want := d(), d()
		_, _ = Render(input, ctx, nil)
		if !reflect.DeepEqual(ctx, want) {
			t.Errorf("Render(%q) changed the caller's Context:\n got %#v\nwant %#v", input, ctx, want)
		}
	}
	var nilCtx Context
	if got, err := Render(`<t-let name="x">v</t-let><t-var name="x" />`, nilCtx, nil); got != "v" || err != nil || nilCtx.Data != nil {
		t.Errorf("let on a zero Context: got %q, %v, data %v", got, err, nilCtx.Data)
	}
}

func TestLoopMetadata(t *testing.T) {
	got := loopMetadata(0, 1)
	want := vm(map[string]Value{
		"index": vs("0"), "number": vs("1"), "length": vs("1"),
		"first": vs("true"), "last": vs("true"),
	})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loopMetadata(0, 1) = %#v", got)
	}
	mid := loopMetadata(1, 3)
	if _, ok := mid.Map["first"]; ok {
		t.Error("middle iteration has a first entry")
	}
	if _, ok := mid.Map["last"]; ok {
		t.Error("middle iteration has a last entry")
	}
	if !reflect.DeepEqual(mid.Map["index"], vs(strconv.Itoa(1))) {
		t.Errorf("index = %#v", mid.Map["index"])
	}
}
