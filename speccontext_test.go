package toupee_test

import (
	toupee "github.com/devselfio/toupee.go"

	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// decodeSpecContext turns a case's context JSON and named in: sections into
// the Context and Loader Render receives, as Toupee's test_runner.zig does:
// named sections first, then context.templates, which wins on a duplicate
// name. data is merged when an object; attrs, slots and templates are read
// when objects, a non-string entry becoming "". Other roots and members are
// ignored. Every map is new, so nothing is shared between cases.
func decodeSpecContext(src string, templates []specTemplate) (toupee.Context, toupee.Loader, error) {
	ctx := toupee.Context{Data: map[string]toupee.Value{}, Attrs: map[string]string{}, Slots: map[string]string{}}
	sources := map[string]string{}
	for _, t := range templates {
		sources[t.name] = t.source
	}
	loader := func(name string) (string, error) {
		src, ok := sources[name]
		if !ok {
			return "", &toupee.RenderError{Kind: toupee.TemplateNotFound, Message: name}
		}
		return src, nil
	}
	if src == "" {
		return ctx, loader, nil
	}

	dec := json.NewDecoder(strings.NewReader(src))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return toupee.Context{}, nil, fmt.Errorf("context: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return toupee.Context{}, nil, fmt.Errorf("context: trailing data after the JSON value")
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return ctx, loader, nil
	}
	if data, ok := obj["data"].(map[string]any); ok {
		for k, v := range data {
			val, err := jsonValue(v)
			if err != nil {
				return toupee.Context{}, nil, fmt.Errorf("context: data.%s: %w", k, err)
			}
			ctx.Data[k] = val
		}
	}
	for _, m := range []struct {
		key string
		dst map[string]string
	}{{"attrs", ctx.Attrs}, {"slots", ctx.Slots}, {"templates", sources}} {
		members, ok := obj[m.key].(map[string]any)
		if !ok {
			continue
		}
		for k, v := range members {
			s, _ := v.(string)
			m.dst[k] = s
		}
	}
	return ctx, loader, nil
}

// jsonValue converts a value decoded with UseNumber into a Value. A number
// without '.', 'e' or 'E' is an int64, parsed directly; any other number is a
// float64. A number out of range is an error, never a lossy coercion.
func jsonValue(v any) (toupee.Value, error) {
	switch v := v.(type) {
	case nil:
		return toupee.Value{}, nil
	case string:
		return toupee.Value{Kind: toupee.KindString, String: v}, nil
	case bool:
		return toupee.Value{Kind: toupee.KindBoolean, Boolean: v}, nil
	case json.Number:
		s := string(v)
		if strings.ContainsAny(s, ".eE") {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil || math.IsInf(f, 0) {
				return toupee.Value{}, fmt.Errorf("float %s out of range: %v", s, err)
			}
			return toupee.Value{Kind: toupee.KindFloat, Float: f}, nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return toupee.Value{}, fmt.Errorf("integer %s: %w", s, err)
		}
		return toupee.Value{Kind: toupee.KindInteger, Integer: n}, nil
	case []any:
		list := make([]toupee.Value, len(v))
		for i, item := range v {
			val, err := jsonValue(item)
			if err != nil {
				return toupee.Value{}, fmt.Errorf("[%d]: %w", i, err)
			}
			list[i] = val
		}
		return toupee.Value{Kind: toupee.KindList, List: list}, nil
	case map[string]any:
		m := make(map[string]toupee.Value, len(v))
		for k, item := range v {
			val, err := jsonValue(item)
			if err != nil {
				return toupee.Value{}, fmt.Errorf("%s: %w", k, err)
			}
			m[k] = val
		}
		return toupee.Value{Kind: toupee.KindMap, Map: m}, nil
	default:
		return toupee.Value{}, fmt.Errorf("unexpected JSON value of type %T", v)
	}
}

func TestDecodeSpecContext(t *testing.T) {
	str := func(s string) toupee.Value { return toupee.Value{Kind: toupee.KindString, String: s} }
	i64 := func(n int64) toupee.Value { return toupee.Value{Kind: toupee.KindInteger, Integer: n} }
	f64 := func(f float64) toupee.Value { return toupee.Value{Kind: toupee.KindFloat, Float: f} }
	empty := toupee.Context{Data: map[string]toupee.Value{}, Attrs: map[string]string{}, Slots: map[string]string{}}

	tests := []struct {
		name string
		src  string
		want toupee.Context
	}{
		{name: "empty", src: "", want: empty},
		{name: "non-object root ignored", src: `[1, 2]`, want: empty},
		{name: "string root ignored", src: `"data"`, want: empty},
		{name: "wrong-shaped members ignored", src: `{"data": [1], "attrs": "x", "slots": 3, "templates": null, "other": {}}`, want: empty},
		{
			name: "scalars",
			src: `{"data": {"s": "x", "t": true, "f": false, "n": null,
				"max": 9223372036854775807, "min": -9223372036854775808,
				"int": 5, "five": 5.0, "exp": 1e3, "negexp": 25E-1, "neg0": -0}}`,
			want: toupee.Context{
				Data: map[string]toupee.Value{
					"s": str("x"), "t": {Kind: toupee.KindBoolean, Boolean: true}, "f": {Kind: toupee.KindBoolean}, "n": {},
					"max": i64(math.MaxInt64), "min": i64(math.MinInt64),
					"int": i64(5), "five": f64(5), "exp": f64(1000), "negexp": f64(2.5), "neg0": i64(0),
				},
				Attrs: map[string]string{}, Slots: map[string]string{},
			},
		},
		{
			name: "nested lists and maps",
			src:  `{"data": {"l": [1, "a", [], {"k": [null, 2.5]}], "m": {"inner": {"x": false}}, "e": {}}}`,
			want: toupee.Context{
				Data: map[string]toupee.Value{
					"l": {Kind: toupee.KindList, List: []toupee.Value{i64(1), str("a"), {Kind: toupee.KindList, List: []toupee.Value{}},
						{Kind: toupee.KindMap, Map: map[string]toupee.Value{"k": {Kind: toupee.KindList, List: []toupee.Value{{}, f64(2.5)}}}}}},
					"m": {Kind: toupee.KindMap, Map: map[string]toupee.Value{"inner": {Kind: toupee.KindMap, Map: map[string]toupee.Value{"x": {Kind: toupee.KindBoolean}}}}},
					"e": {Kind: toupee.KindMap, Map: map[string]toupee.Value{}},
				},
				Attrs: map[string]string{}, Slots: map[string]string{},
			},
		},
		{
			name: "attrs and slots with empty names and non-strings",
			src:  `{"attrs": {"": "a", "class": "x", "n": 1, "b": true}, "slots": {"": "body", "side": null, "l": []}}`,
			want: toupee.Context{
				Data:  map[string]toupee.Value{},
				Attrs: map[string]string{"": "a", "class": "x", "n": "", "b": ""},
				Slots: map[string]string{"": "body", "side": "", "l": ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, loader, err := decodeSpecContext(tt.src, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
			if loader == nil {
				t.Error("loader is nil; the runner always supplies one")
			}
		})
	}
}

// An integer token never passes through float64: 2^53+1 is exact.
func TestDecodeSpecContextIntegerPrecision(t *testing.T) {
	ctx, _, err := decodeSpecContext(`{"data": {"n": 9007199254740993}}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ctx.Data["n"]; got.Kind != toupee.KindInteger || got.Integer != 9007199254740993 {
		t.Errorf("got %+v, want integer 9007199254740993", got)
	}
}

func TestDecodeSpecContextErrors(t *testing.T) {
	for _, src := range []string{
		`{`,
		`{"data": {}} x`,
		`{"data": {}} {}`,
		`{"data": {"n": 9223372036854775808}}`,
		`{"data": {"n": -9223372036854775809}}`,
		`{"data": {"f": 1e400}}`,
		`nul`,
	} {
		if _, _, err := decodeSpecContext(src, nil); err == nil {
			t.Errorf("decodeSpecContext(%q) accepted invalid or out-of-range JSON", src)
		}
	}
}

func TestDecodeSpecContextLoader(t *testing.T) {
	named := []specTemplate{
		{name: "card.html", source: "<div>named</div>"},
		{name: "blank.html", source: ""},
		{name: "dup.html", source: "first"},
		{name: "dup.html", source: "second"},
	}
	_, loader, err := decodeSpecContext(`{"templates": {"card.html": "<div>context</div>", "ctx.html": "c", "odd.html": 3}}`, named)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"card.html":  "<div>context</div>", // context.templates wins
		"blank.html": "",                   // present and empty is found
		"dup.html":   "second",
		"ctx.html":   "c",
		"odd.html":   "",
	} {
		got, err := loader(name)
		if err != nil || got != want {
			t.Errorf("loader(%q) = %q, %v; want %q, nil", name, got, err, want)
		}
	}
	_, err = loader("nope.html")
	var re *toupee.RenderError
	if !errors.As(err, &re) || re.Kind != toupee.TemplateNotFound || re.Message != "nope.html" {
		t.Errorf("loader(missing) error = %v, want *RenderError{Kind: TemplateNotFound, Message: \"nope.html\"}", err)
	}
}

// Each case gets fresh maps: nothing decoded for one case is visible in
// another, and changing one case's Context does not leak.
func TestDecodeSpecContextIsolation(t *testing.T) {
	ctxA, loadA, err := decodeSpecContext(`{"data": {"a": 1}, "attrs": {"x": "1"}, "templates": {"a.html": "A"}}`, []specTemplate{{"n.html", "N"}})
	if err != nil {
		t.Fatal(err)
	}
	ctxB, loadB, err := decodeSpecContext(``, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctxA.Data["leak"] = toupee.Value{}
	ctxA.Slots["leak"] = "x"
	if len(ctxB.Data) != 0 || len(ctxB.Attrs) != 0 || len(ctxB.Slots) != 0 {
		t.Errorf("second context not empty: %+v", ctxB)
	}
	for _, name := range []string{"a.html", "n.html"} {
		if _, err := loadA(name); err != nil {
			t.Errorf("first loader lost %s: %v", name, err)
		}
		if _, err := loadB(name); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("second loader sees %s from the first case", name)
		}
	}
}

func TestJSONValueRejectsUnknown(t *testing.T) {
	if _, err := jsonValue(struct{}{}); err == nil {
		t.Error("jsonValue accepted a non-JSON Go value")
	}
}
