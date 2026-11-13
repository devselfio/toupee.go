package toupee_test

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	toupee "github.com/devselfio/toupee.go"
)

// prepareClean prepares templates with loader and fails the test unless the
// set is clean.
func prepareClean(t *testing.T, templates map[string]string, loader toupee.Loader) *toupee.Set {
	t.Helper()
	set, diags, err := toupee.Prepare(templates, loader)
	if set == nil || diags != nil || err != nil {
		t.Fatalf("Prepare = %v, %#v, %v; want a set, no diagnostics, nil", set, diags, err)
	}
	return set
}

// Oracle: upstream's unit test "engine addTemplate and renderTemplate"
// (src/root.zig).
func TestSetRendersSuccessiveContexts(t *testing.T) {
	set := prepareClean(t, map[string]string{"greeting.html": `Hello <t-var name="name" />!`}, nil)
	for _, name := range []string{"Alice", "Bob"} {
		got, err := set.Render("greeting.html", toupee.Context{Data: map[string]toupee.Value{"name": str(name)}})
		if want := "Hello " + name + "!"; got != want || err != nil {
			t.Errorf("Render for %s = %q, %v; want %q, nil", name, got, err, want)
		}
	}
}

func TestSetRendersIncludeAndExtendFromSet(t *testing.T) {
	set := prepareClean(t, map[string]string{
		"base.html": `<html><t-slot name="content" /><t-include template="foot.html" /></html>`,
		"mid.html":  `<t-extend template="base.html"><t-define name="content"><main><t-slot name="body" /></main></t-define>`,
		"page.html": `<t-extend template="mid.html"><t-define name="body"><t-var name="title" /></t-define>`,
		"foot.html": `<footer><t-var name="site" /></footer>`,
	}, func(string) (string, error) { return "", errors.New("loader called") })
	got, err := set.Render("page.html", toupee.Context{Data: map[string]toupee.Value{"title": str("T"), "site": str("S")}})
	if want := "<html><main>T</main><footer>S</footer></html>"; got != want || err != nil {
		t.Errorf("Render = %q, %v; want %q, nil", got, err, want)
	}
}

func TestSetOwnsItsSnapshot(t *testing.T) {
	templates := map[string]string{"a.html": `A<t-include template="b.html" />`, "b.html": "B"}
	set := prepareClean(t, templates, nil)
	templates["b.html"] = "CHANGED"
	delete(templates, "a.html")
	got, err := set.Render("a.html", toupee.Context{})
	if got != "AB" || err != nil {
		t.Errorf("Render = %q, %v; want \"AB\", nil", got, err)
	}
}

func TestSetFallsBackToLoader(t *testing.T) {
	var answer error
	loader := func(name string) (string, error) {
		if name != "b.html" {
			return notFound(name)
		}
		if answer != nil {
			return "", answer
		}
		return "B", nil
	}
	set := prepareClean(t, map[string]string{"a.html": `A<t-include template="b.html" />`}, loader)
	got, err := set.Render("a.html", toupee.Context{})
	if got != "AB" || err != nil {
		t.Fatalf("Render = %q, %v; want \"AB\", nil", got, err)
	}

	boom := errors.New("disk on fire")
	answer = boom
	got, err = set.Render("a.html", toupee.Context{})
	if got != "" || !errors.Is(err, boom) {
		t.Errorf("Render = %q, %v; want \"\", %v", got, err, boom)
	}

	answer = &toupee.RenderError{Kind: toupee.TemplateNotFound, Message: "b.html"}
	got, err = set.Render("a.html", toupee.Context{})
	wantRenderError(t, got, err, toupee.RenderError{Line: 1, Column: 2, Kind: toupee.TemplateNotFound, Message: "b.html"})
}

func TestSetRenderUnknownEntry(t *testing.T) {
	set := prepareClean(t, map[string]string{"a.html": "A"}, func(name string) (string, error) {
		if name == "nope.html" {
			return "N", nil
		}
		return notFound(name)
	})
	got, err := set.Render("nope.html", toupee.Context{})
	wantRenderError(t, got, err, toupee.RenderError{Line: 0, Column: 0, Kind: toupee.TemplateNotFound, Message: "nope.html"})

	empty := prepareClean(t, nil, nil)
	got, err = empty.Render("nope.html", toupee.Context{})
	wantRenderError(t, got, err, toupee.RenderError{Line: 0, Column: 0, Kind: toupee.TemplateNotFound, Message: "nope.html"})
}

func TestPrepareReportsValidateDiagnostics(t *testing.T) {
	set, diags, err := toupee.Prepare(map[string]string{
		"bad.html":  "<p>\n  <t-if var=\"x\">open",
		"dup.html":  "<t-extend template=\"bad.html\">\n<t-define name=\"t\">1</t-define>\n  <t-define slot=\"t\">2</t-define>",
		"good.html": `<t-include template="bad.html" /><t-include template="gone.html" />`,
	}, nil)
	want := []toupee.Diagnostic{
		{Template: "bad.html", Line: 2, Column: 3, Kind: toupee.MalformedElement, Message: "missing closing </t-if> tag"},
		{Template: "dup.html", Line: 3, Column: 3, Kind: toupee.DuplicateSlotDefinition, Message: "t"},
		{Template: "good.html", Line: 1, Column: 34, Kind: toupee.TemplateNotFound, Message: "missing include 'gone.html'"},
	}
	if set != nil || err != nil || !reflect.DeepEqual(diags, want) {
		t.Errorf("Prepare = %v, %#v, %v\nwant nil, %#v, nil", set, diags, err, want)
	}

	boom := errors.New("disk on fire")
	set, diags, err = toupee.Prepare(map[string]string{"page.html": `<t-include template="nav.html" />`},
		func(string) (string, error) { return "", boom })
	if set != nil || diags != nil || !errors.Is(err, boom) {
		t.Fatalf("Prepare = %v, %#v, %v; want nil, nil, an error wrapping %v", set, diags, err, boom)
	}
	if msg := err.Error(); msg != "toupee: validate page.html: load nav.html: disk on fire" {
		t.Errorf("error text = %q", msg)
	}
}

// Oracle: the prepared-set contract detects a fill re-entering itself
// through set templates only, since a set lookup is not a loader call.
func TestSetDetectsSlotCycleThroughExtend(t *testing.T) {
	set := prepareClean(t, map[string]string{
		"base.html":  `<t-slot name="a" />`,
		"entry.html": `<t-extend template="base.html"><t-define name="a"><t-extend template="base.html"></t-define>`,
	}, func(string) (string, error) { return "", errors.New("loader called") })
	got, err := set.Render("entry.html", toupee.Context{})
	wantRenderError(t, got, err, toupee.RenderError{Line: 1, Column: 1, Kind: toupee.CircularReference, Message: "a"})
}

func TestSetDetectsSlotCycleAfterInclude(t *testing.T) {
	set := prepareClean(t, map[string]string{
		"entry.html": `<t-slot name="x" />`,
		"step":       "",
	}, func(string) (string, error) { return "", errors.New("loader called") })
	got, err := set.Render("entry.html", toupee.Context{Slots: map[string]string{
		"x": `<t-include template="step" /><t-slot name="x" />`,
	}})
	wantRenderError(t, got, err, toupee.RenderError{Line: 1, Column: 30, Kind: toupee.CircularReference, Message: "x"})
}

func TestSetRuntimeErrors(t *testing.T) {
	set := prepareClean(t, map[string]string{
		"page.html": "<p>\n<t-include template=\"card.html\" /></p>",
		"card.html": "x\n  <t-var name=\"missing\" />",
		"a.html":    `<t-extend template="b.html">`,
		"b.html":    `<t-extend template="a.html">`,
		"deep.html": `<t-include template="deep.html" />`,
	}, nil)
	got, err := set.Render("page.html", toupee.Context{})
	wantRenderError(t, got, err, toupee.RenderError{Line: 2, Column: 3, Kind: toupee.UndefinedVariable, Message: "missing"})
	got, err = set.Render("a.html", toupee.Context{})
	wantRenderError(t, got, err, toupee.RenderError{Line: 1, Column: 1, Kind: toupee.CircularReference, Message: "b.html"})
	got, err = set.Render("deep.html", toupee.Context{})
	wantRenderError(t, got, err, toupee.RenderError{Line: 1, Column: 1, Kind: toupee.CircularReference, Message: "deep.html"})
}

// untaken is n conditionals on an absent variable: it renders "" and its
// parse allocates in proportion to n.
func untaken(n int) string {
	return strings.Repeat(`<t-if var="absent">x</t-if>`, n)
}

// retentionTemplates is the fixture of the three retention tests.
func retentionTemplates(n int) map[string]string {
	return map[string]string{
		"big.html": untaken(n),
		"inc.html": `<t-include template="big.html" />`,
		"ext.html": `<t-extend template="big.html"><t-define name="a">x</t-define>`,
	}
}

// checkRetained renders entry from a set at n = 20 and n = 2000 and from
// Render at n = 2000. A set that re-parses grows with n; one that retains
// its parses does not. Render, which always parses, shows the fixture is
// large enough to tell.
func checkRetained(t *testing.T, entry string) {
	t.Helper()
	setAllocs := func(n int) float64 {
		set := prepareClean(t, retentionTemplates(n), nil)
		return testing.AllocsPerRun(20, func() {
			if _, err := set.Render(entry, toupee.Context{}); err != nil {
				t.Fatal(err)
			}
		})
	}
	small, large := setAllocs(20), setAllocs(2000)
	if large > small+5 {
		t.Errorf("set allocations for %s: %v at n=20, %v at n=2000; want at most %v", entry, small, large, small+5)
	}
	templates := retentionTemplates(2000)
	loader := func(name string) (string, error) { return templates[name], nil }
	direct := testing.AllocsPerRun(20, func() {
		if _, err := toupee.Render(templates[entry], toupee.Context{}, loader); err != nil {
			t.Fatal(err)
		}
	})
	if direct < 2000 {
		t.Errorf("Render allocations for %s at n=2000 = %v; want at least 2000", entry, direct)
	}
}

func TestSetRetainsEntryParse(t *testing.T)         { checkRetained(t, "big.html") }
func TestSetRetainsIncludeTargetParse(t *testing.T) { checkRetained(t, "inc.html") }
func TestSetRetainsExtendParentParse(t *testing.T)  { checkRetained(t, "ext.html") }

func TestSetConcurrentRenders(t *testing.T) {
	set := prepareClean(t, map[string]string{
		"base.html": `<html><t-slot name="content" /><t-include template="foot.html" /></html>`,
		"foot.html": `<footer><t-var name="n" /></footer>`,
		"page.html": `<t-extend template="base.html"><t-define name="content"><t-let name="x"><t-var name="n" />!</t-let><t-for i in items><t-var name="i" /><t-var name="x" /></t-for></t-define>`,
	}, nil)
	var wg sync.WaitGroup
	for g := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := strconv.Itoa(g)
			ctx := toupee.Context{Data: map[string]toupee.Value{
				"n":     str(n),
				"items": {Kind: toupee.KindList, List: []toupee.Value{str("a" + n), str("b" + n)}},
			}}
			want := "<html>a" + n + n + "!b" + n + n + "!<footer>" + n + "</footer></html>"
			for range 200 {
				got, err := set.Render("page.html", ctx)
				if got != want || err != nil {
					t.Errorf("goroutine %d: Render = %q, %v; want %q, nil", g, got, err, want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func ExampleSet() {
	set, diags, err := toupee.Prepare(map[string]string{
		"layout.html":   `<main><t-include template="greeting.html" /></main>`,
		"greeting.html": `Hello <t-var name="name" />!`,
	}, nil)
	if err != nil || len(diags) > 0 {
		fmt.Println(diags, err)
		return
	}
	for _, name := range []string{"Alice", "Bob"} {
		ctx := toupee.Context{Data: map[string]toupee.Value{
			"name": {Kind: toupee.KindString, String: name},
		}}
		out, err := set.Render("layout.html", ctx)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(out)
	}
	// Output:
	// <main>Hello Alice!</main>
	// <main>Hello Bob!</main>
}
