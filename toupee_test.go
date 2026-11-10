package toupee_test

import (
	"errors"
	"fmt"
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
	}{
		{name: "empty everything", input: "", ctx: toupee.Context{}, loader: nil},
		{name: "text with nil loader", input: "<p>hi</p>", ctx: toupee.Context{}, loader: nil},
		{
			name:  "populated context and loader",
			input: `<t-var name="x" />`,
			ctx: toupee.Context{
				Data:  map[string]toupee.Value{"x": {Kind: toupee.KindString, String: "y"}},
				Attrs: map[string]string{"": "a"},
				Slots: map[string]string{"": "s"},
			},
			loader: loader,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toupee.Render(tt.input, tt.ctx, tt.loader)
			if got != "" || err != nil {
				t.Fatalf("Render = %q, %v; want \"\", nil from the bootstrap stub", got, err)
			}
		})
	}
	if calls != 0 {
		t.Errorf("the stub called the loader %d times, want 0", calls)
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
// Rendering semantics are forthcoming: the bootstrap Render returns "".
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
	out, err := toupee.Render(`<t-include template="card" />`, ctx, loader)
	fmt.Printf("%q %v\n", out, err)

	_, err = loader("absent")
	var re *toupee.RenderError
	fmt.Println(errors.As(err, &re), re.Kind)
	// Output:
	// "" <nil>
	// true TemplateNotFound
}
