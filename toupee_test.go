package toupee_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/devselfio/toupee.go"
)

func TestRenderEmpty(t *testing.T) {
	calls := 0
	loader := func(name string) (string, error) {
		calls++
		return "", nil
	}
	for _, tt := range []struct {
		name   string
		input  string
		ctx    toupee.Context
		loader toupee.Loader
		want   string
	}{
		{name: "empty everything", input: "", ctx: toupee.Context{}, loader: nil, want: ""},
		{name: "text with nil loader", input: "<p>hi</p>", ctx: toupee.Context{}, loader: nil, want: "<p>hi</p>"},
		{
			name:  "populated context and loader",
			input: `<t-var name="x" />`,
			ctx: toupee.Context{
				Data:  map[string]toupee.Value{"x": {Kind: toupee.KindString, String: "y"}},
				Attrs: map[string]string{"": "a"},
				Slots: map[string]string{"": "s"},
			},
			loader: loader,
			want:   "y",
		},
		{
			name:   "debug is stripped",
			input:  `a<t-debug />b`,
			ctx:    toupee.Context{},
			loader: loader,
			want:   "ab",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toupee.Render(tt.input, tt.ctx, tt.loader)
			if got != tt.want || err != nil {
				t.Fatalf("Render = %q, %v; want %q, nil", got, err, tt.want)
			}
		})
	}
	if calls != 0 {
		t.Errorf("Render called the loader %d times, want 0", calls)
	}
}

// snapshotContext returns a populated, nested Context and an independent
// deep copy of it to compare against after rendering.
func snapshotContext() (toupee.Context, toupee.Context) {
	build := func() toupee.Context {
		return toupee.Context{
			Data: map[string]toupee.Value{
				"title": {Kind: toupee.KindString, String: "<Grid>"},
				"n":     {Kind: toupee.KindInteger, Integer: 3},
				"key":   {Kind: toupee.KindString, String: "b"},
				"list": {Kind: toupee.KindList, List: []toupee.Value{
					{Kind: toupee.KindMap, Map: map[string]toupee.Value{
						"a": {Kind: toupee.KindFloat, Float: 1.5},
						"b": {Kind: toupee.KindBoolean, Boolean: true},
					}},
					{},
				}},
				"empty": {Kind: toupee.KindMap, Map: map[string]toupee.Value{}},
			},
			Attrs: map[string]string{"class": "wide"},
			Slots: map[string]string{"": "body"},
		}
	}
	return build(), build()
}

func TestRenderLeavesCallerStateAndLoaderAlone(t *testing.T) {
	ctx, want := snapshotContext()
	calls := 0
	loader := func(name string) (string, error) {
		calls++
		return "", nil
	}
	const ok = `<t-var name="title" /> <t-raw name="title" /> <t-var name="list.0.a" /> <t-var name="list.0.$(key)" /><t-var name="list.1">d</t-var>`
	const okWant = `&lt;Grid&gt; <Grid> 1.5 trued`
	for i := 0; i < 3; i++ {
		got, err := toupee.Render(ok, ctx, loader)
		if got != okWant || err != nil {
			t.Fatalf("run %d: Render = %q, %v; want %q, nil", i, got, err, okWant)
		}
		got, err = toupee.Render(`<t-var name="title" /><t-var name="empty" />`, ctx, loader)
		var re *toupee.RenderError
		if got != "" || !errors.As(err, &re) || re.Kind != toupee.UndefinedVariable || re.Message != "empty" {
			t.Fatalf("run %d: Render = %q, %v; want \"\" and UndefinedVariable empty", i, got, err)
		}
		got, err = toupee.Render(`<t-var name="title"`, ctx, loader)
		if got != "" || !errors.As(err, &re) || re.Kind != toupee.MalformedElement {
			t.Fatalf("run %d: Render = %q, %v; want \"\" and MalformedElement", i, got, err)
		}
		got, err = toupee.Render(`<t-var name="title" /><t-if var="n"><t-include template="x" /><t-debug /></t-if>`, ctx, loader)
		if got != "&lt;Grid&gt;" || err != nil {
			t.Fatalf("run %d: debug Render = %q, %v; want \"&lt;Grid&gt;\", nil", i, got, err)
		}
	}
	if !reflect.DeepEqual(ctx, want) {
		t.Errorf("Render modified the caller's Context:\n got %#v\nwant %#v", ctx, want)
	}
	if calls != 3 {
		t.Errorf("Render called the loader %d times, want 3: once per run, for the include", calls)
	}
}

func TestZeroValueIsNil(t *testing.T) {
	var v toupee.Value
	if v.Kind != toupee.KindNil {
		t.Errorf("Value{}.Kind = %v, want KindNil", v.Kind)
	}
}

// Example builds the seven Value forms, attrs, slots and a Loader honouring
// the missing-name contract, and renders through the single entry point.
// It renders two variables; the loader is not consulted by them.
func Example() {
	data := map[string]toupee.Value{
		"title":   {Kind: toupee.KindString, String: "Grid"},
		"visible": {Kind: toupee.KindBoolean, Boolean: true},
		"count":   {Kind: toupee.KindInteger, Integer: 3},
		"ratio":   {Kind: toupee.KindFloat, Float: 0.5},
		"tags": {Kind: toupee.KindList, List: []toupee.Value{
			{Kind: toupee.KindString, String: "a"},
			{Kind: toupee.KindString, String: "b"},
		}},
		"user": {Kind: toupee.KindMap, Map: map[string]toupee.Value{
			"name": {Kind: toupee.KindString, String: "Ada"},
		}},
		"missing": {}, // the zero Value is nil
	}
	templates := map[string]string{"card": `<div><t-slot /></div>`}
	loader := func(name string) (string, error) {
		src, ok := templates[name]
		if !ok {
			return "", &toupee.RenderError{Kind: toupee.TemplateNotFound, Message: name}
		}
		return src, nil
	}
	ctx := toupee.Context{
		Data:  data,
		Attrs: map[string]string{"class": "wide"},
		Slots: map[string]string{"": "body"},
	}
	out, err := toupee.Render(`<p><t-var name="title" />: <t-var name="count" /></p>`, ctx, loader)
	fmt.Printf("%q %v\n", out, err)

	_, err = loader("absent")
	var re *toupee.RenderError
	fmt.Println(errors.As(err, &re), re.Kind)
	// Output:
	// "<p>Grid: 3</p>" <nil>
	// true TemplateNotFound
}
