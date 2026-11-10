package toupee

import (
	"math"
	"testing"
)

func TestScalarString(t *testing.T) {
	for _, tt := range []struct {
		name string
		v    Value
		want string
		ok   bool
	}{
		{"string", Value{Kind: KindString, String: "a<b"}, "a<b", true},
		{"empty string", Value{Kind: KindString}, "", true},
		{"true", Value{Kind: KindBoolean, Boolean: true}, "true", true},
		{"false", Value{Kind: KindBoolean}, "false", true},
		{"zero", Value{Kind: KindInteger}, "0", true},
		{"negative", Value{Kind: KindInteger, Integer: -42}, "-42", true},
		{"max int64", Value{Kind: KindInteger, Integer: math.MaxInt64}, "9223372036854775807", true},
		{"min int64", Value{Kind: KindInteger, Integer: math.MinInt64}, "-9223372036854775808", true},
		{"float", Value{Kind: KindFloat, Float: 3.14}, "3.14", true},
		{"whole float", Value{Kind: KindFloat, Float: 5.0}, "5", true},
		{"negative float", Value{Kind: KindFloat, Float: -0.5}, "-0.5", true},
		{"float zero", Value{Kind: KindFloat}, "0", true},
		{"large float", Value{Kind: KindFloat, Float: 1e20}, "100000000000000000000", true},
		{"small float", Value{Kind: KindFloat, Float: 0.000001}, "0.000001", true},
		{"nil", Value{}, "", false},
		{"empty list", Value{Kind: KindList}, "", false},
		{"list", Value{Kind: KindList, List: []Value{{Kind: KindString, String: "a"}}}, "", false},
		{"empty map", Value{Kind: KindMap, Map: map[string]Value{}}, "", false},
		{"map", Value{Kind: KindMap, Map: map[string]Value{"a": {}}}, "", false},
		{"irrelevant payload on string", Value{Kind: KindString, String: "s", Integer: 7, Boolean: true}, "s", true},
		{"irrelevant payload on integer", Value{Kind: KindInteger, Integer: 7, String: "s", Float: 2}, "7", true},
		{"irrelevant payload on nil", Value{String: "s", Integer: 7, List: []Value{{}}}, "", false},
		{"irrelevant payload on map", Value{Kind: KindMap, String: "s"}, "", false},
	} {
		got, ok := scalarString(tt.v)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: scalarString = %q, %v; want %q, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}
