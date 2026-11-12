package toupee_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	toupee "github.com/devselfio/toupee.go"
)

// notFound is a loader that knows no template, answering as the Loader
// contract says.
func notFound(name string) (string, error) {
	return "", &toupee.RenderError{Kind: toupee.TemplateNotFound, Message: name}
}

func TestValidateMissingInclude(t *testing.T) {
	got, err := toupee.Validate(map[string]string{"page.html": `<t-include template="missing.html" />`}, nil)
	want := []toupee.Diagnostic{
		{Template: "page.html", Line: 1, Column: 1, Kind: toupee.TemplateNotFound, Message: "missing include 'missing.html'"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
}

func TestValidateMissingExtend(t *testing.T) {
	got, err := toupee.Validate(map[string]string{
		"child.html": `<t-extend template="ghost.html"><t-define slot="main">hi</t-define></t-extend>`,
	}, nil)
	want := []toupee.Diagnostic{
		{Template: "child.html", Line: 1, Column: 1, Kind: toupee.TemplateNotFound, Message: "missing extends 'ghost.html'"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
}

func TestValidateResolvesInSet(t *testing.T) {
	got, err := toupee.Validate(map[string]string{
		"base.html": `<t-slot name="main" />`,
		"page.html": `<t-extend template="base.html"><t-define slot="main">content</t-define></t-extend>`,
	}, nil)
	if err != nil || len(got) != 0 {
		t.Errorf("Validate = %#v, %v; want none, nil", got, err)
	}
}

func TestValidateResolvesThroughLoader(t *testing.T) {
	loader := func(name string) (string, error) {
		if name == "nav.html" {
			return "<nav>links</nav>", nil
		}
		return notFound(name)
	}
	got, err := toupee.Validate(map[string]string{
		"page.html": `<t-include template="nav.html" /><t-include template="gone.html" />`,
	}, loader)
	want := []toupee.Diagnostic{
		{Template: "page.html", Line: 1, Column: 34, Kind: toupee.TemplateNotFound, Message: "missing include 'gone.html'"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
}

func TestValidateUnrenderedBranch(t *testing.T) {
	source := `<t-if var="show"><t-include template="deep.html" /></t-if>`
	got, err := toupee.Validate(map[string]string{"page.html": source}, nil)
	want := []toupee.Diagnostic{
		{Template: "page.html", Line: 1, Column: 18, Kind: toupee.TemplateNotFound, Message: "missing include 'deep.html'"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
	out, err := toupee.Render(source, toupee.Context{}, nil)
	if out != "" || err != nil {
		t.Errorf("Render = %q, %v; want \"\", nil", out, err)
	}
}

func TestValidateLineAndColumn(t *testing.T) {
	got, err := toupee.Validate(map[string]string{
		"page.html": "<header>nav</header>\n<t-include template=\"sidebar.html\" />\n<footer />",
	}, nil)
	want := []toupee.Diagnostic{
		{Template: "page.html", Line: 2, Column: 1, Kind: toupee.TemplateNotFound, Message: "missing include 'sidebar.html'"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
}

func TestValidateEmptySet(t *testing.T) {
	got, err := toupee.Validate(nil, nil)
	if err != nil || len(got) != 0 {
		t.Errorf("Validate(nil, nil) = %#v, %v; want none, nil", got, err)
	}
	got, err = toupee.Validate(map[string]string{}, notFound)
	if err != nil || len(got) != 0 {
		t.Errorf("Validate(empty, notFound) = %#v, %v; want none, nil", got, err)
	}
}

func TestValidateWalksEveryBody(t *testing.T) {
	every := `<t-if var="a">a<t-elif var="b" /><t-include template="m/elif.html" /><t-else /><t-include template="m/else.html" /></t-if>
<t-for item in items><t-include template="m/for.html" /><t-else /><t-include template="m/for-else.html" /></t-for>
<t-slot name="side"><t-include template="m/slot.html" /></t-slot>
<t-var name="title"><t-include template="m/var.html" /></t-var>
<t-raw name="body"><t-include template="m/raw.html" /></t-raw>
<t-let name="x"><t-include template="m/let.html" /></t-let>
<div>
  <t-include template="card.html">
    <t-define name="head">
      <t-include template="m/define.html" />
    </t-define>
    <t-include template="m/anonymous.html" />
  </t-include>
</div>
`
	got, err := toupee.Validate(map[string]string{
		"card.html": `<t-slot name="head" /><t-slot />`,
		"page.html": every,
	}, nil)
	missing := func(line, column int, name string) toupee.Diagnostic {
		return toupee.Diagnostic{Template: "page.html", Line: line, Column: column, Kind: toupee.TemplateNotFound, Message: "missing include '" + name + "'"}
	}
	want := []toupee.Diagnostic{
		missing(1, 34, "m/elif.html"),
		missing(1, 80, "m/else.html"),
		missing(2, 22, "m/for.html"),
		missing(2, 67, "m/for-else.html"),
		missing(3, 21, "m/slot.html"),
		missing(4, 21, "m/var.html"),
		missing(5, 20, "m/raw.html"),
		missing(6, 17, "m/let.html"),
		missing(10, 7, "m/define.html"),
		missing(12, 5, "m/anonymous.html"),
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
}

func TestValidateWalksExtendDefinitions(t *testing.T) {
	got, err := toupee.Validate(map[string]string{
		"base.html": "",
		"post.html": "<t-extend template=\"base.html\">\n<t-define name=\"content\">\n  <t-include template=\"m/extend-define.html\" />\n</t-define>",
	}, nil)
	want := []toupee.Diagnostic{
		{Template: "post.html", Line: 3, Column: 3, Kind: toupee.TemplateNotFound, Message: "missing include 'm/extend-define.html'"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
}

func TestValidateReportsSyntax(t *testing.T) {
	got, err := toupee.Validate(map[string]string{
		"bad.html":  "<p>\n  <t-if var=\"x\">open",
		"dup.html":  "<t-extend template=\"bad.html\">\n<t-define name=\"t\">1</t-define>\n  <t-define slot=\"t\">2</t-define>",
		"good.html": `<t-include template="bad.html" /><t-include template="gone.html" />`,
	}, nil)
	want := []toupee.Diagnostic{
		{Template: "bad.html", Line: 2, Column: 3, Kind: toupee.MalformedElement, Message: "missing closing </t-if> tag"},
		{Template: "dup.html", Line: 3, Column: 3, Kind: toupee.DuplicateSlotDefinition, Message: "t"},
		{Template: "good.html", Line: 1, Column: 34, Kind: toupee.TemplateNotFound, Message: "missing include 'gone.html'"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
}

func TestValidateLoaderFailure(t *testing.T) {
	boom := errors.New("disk on fire")
	set := map[string]string{"page.html": `<t-include template="nav.html" />`}
	got, err := toupee.Validate(set, func(string) (string, error) { return "", boom })
	if got != nil || !errors.Is(err, boom) {
		t.Fatalf("Validate = %#v, %v; want nil, an error wrapping %v", got, err, boom)
	}
	if msg := err.Error(); msg != "toupee: validate page.html: load nav.html: disk on fire" {
		t.Errorf("error text = %q", msg)
	}

	got, err = toupee.Validate(set, notFound)
	want := []toupee.Diagnostic{
		{Template: "page.html", Line: 1, Column: 1, Kind: toupee.TemplateNotFound, Message: "missing include 'nav.html'"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Validate = %#v, %v\nwant %#v, nil", got, err, want)
	}
}

func ExampleValidate() {
	diags, err := toupee.Validate(map[string]string{
		"page.html": "<main>\n<t-include template=\"nav.html\" />\n</main>",
	}, nil)
	if err != nil {
		panic(err)
	}
	for _, d := range diags {
		fmt.Printf("%s:%d:%d: %s\n", d.Template, d.Line, d.Column, d.Message)
	}
	// Output: page.html:2:1: missing include 'nav.html'
}
