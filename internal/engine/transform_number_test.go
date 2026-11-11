package engine

import (
	"math"
	"testing"
)

// TestParseNumber ports Toupee v0.4.0's "parseNumber best-effort" test and
// pins the coercion edges: the four-byte trim set, true/false, unparseable
// text as 0 and Zig's float syntax.
func TestParseNumber(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want float64
	}{
		{"42", 42}, {"3.14", 3.14}, {"true", 1}, {"false", 0}, {"hello", 0},
		{"", 0}, {" -7 ", -7}, {"\t\r\n5\n", 5}, {"\v5", 0}, {"TRUE", 0},
		{" true ", 1}, {"1e3", 1000}, {"-.5", -0.5}, {"5.", 5}, {"+2", 2},
		{"1_000", 1000}, {"0x10", 16}, {"0X1.8", 1.5},
		{"0x1p-2", 0.25}, {"1e400", math.Inf(1)}, {"-inf", math.Inf(-1)},
		{"Infinity", math.Inf(1)}, {"1e", 0}, {".", 0}, {"1,5", 0},
		// Zig reads one optional sign; a second one is malformed.
		{"++1", 0}, {"+-1", 0}, {"-+1", 0}, {"--1", 0}, {"+-Inf", 0},
		{"-+inf", 0}, {"+-0x10", 0}, {"-", 0}, {"+", 0},
	} {
		if got := parseNumber(tt.in); got != tt.want {
			t.Errorf("parseNumber(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if got := parseNumber("nan"); !math.IsNaN(got) {
		t.Errorf("parseNumber(nan) = %v, want NaN", got)
	}
}

func TestFloatToInt(t *testing.T) {
	for _, tt := range []struct {
		in   float64
		want int64
	}{
		{29.99, 29}, {-29.99, -29}, {0, 0}, {math.NaN(), 0},
		{math.Inf(1), math.MaxInt64}, {math.Inf(-1), math.MinInt64},
		{9.3e18, math.MaxInt64}, {-9.3e18, math.MinInt64},
		{9.223372036854775e18, 9223372036854774784},
	} {
		if got := floatToInt(tt.in); got != tt.want {
			t.Errorf("floatToInt(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestFormatMinimalFloat(t *testing.T) {
	for _, tt := range []struct {
		in   float64
		want string
	}{
		{42, "42"}, {29.99, "29.99"}, {-3.7, "-3.7"}, {0.1, "0.1"},
		{1e21, "1000000000000000000000"}, {1e-7, "0.0000001"},
		{math.Copysign(0, -1), "-0"}, {math.Inf(1), "inf"},
		{math.Inf(-1), "-inf"}, {math.NaN(), "nan"},
	} {
		if got := formatMinimalFloat(tt.in); got != tt.want {
			t.Errorf("formatMinimalFloat(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatDecimal(t *testing.T) {
	for _, tt := range []struct {
		in     float64
		places uint64
		want   string
	}{
		{42, 2, "42.00"}, {29.999, 2, "29.99"}, {3.1, 4, "3.1000"},
		{-0.999, 1, "-0.9"}, {7.5, 0, "7"}, {7, 0, "7"}, {0.125, 3, "0.125"},
		{math.Inf(1), 2, "inf.00"}, {math.NaN(), 1, "nan.0"},
	} {
		if got := formatDecimal(tt.in, tt.places); got != tt.want {
			t.Errorf("formatDecimal(%v, %d) = %q, want %q", tt.in, tt.places, got, tt.want)
		}
	}
}

// TestNumericTransforms checks abs, floor, ceil, int, float, decimal and
// bool through the registry, with Toupee v0.4.0's in-source cases first.
func TestNumericTransforms(t *testing.T) {
	a := func(args ...string) []string { return args }
	for _, tt := range []struct {
		name  string
		value string
		args  []string
		want  string
	}{
		{"abs", "-42", nil, "42"}, {"abs", "7", nil, "7"},
		{"abs", "-3.7", nil, "3.7"}, {"abs", "true", nil, "1"},
		{"abs", "x", nil, "0"}, {"abs", "-0", nil, "0"}, {"abs", "-inf", nil, "inf"},
		{"floor", "3.7", nil, "3"}, {"floor", "-3.2", nil, "-4"},
		{"floor", "hello", nil, "0"}, {"floor", "1e300", nil, "9223372036854775807"},
		{"ceil", "3.2", nil, "4"}, {"ceil", "-3.7", nil, "-3"},
		{"ceil", "hello", nil, "0"}, {"ceil", "-1e300", nil, "-9223372036854775808"},
		{"int", "29.99", nil, "29"}, {"int", "true", nil, "1"},
		{"int", "hello", nil, "0"}, {"int", "-29.99", nil, "-29"},
		{"int", "nan", nil, "0"}, {"int", " 12 ", nil, "12"},
		{"float", "42", nil, "42"}, {"float", "29.990", nil, "29.99"},
		{"float", "1e2", nil, "100"}, {"float", "false", nil, "0"},
		{"decimal", "42", a("2"), "42.00"}, {"decimal", "29.999", a("2"), "29.99"},
		{"decimal", "3.1", a("4"), "3.1000"}, {"decimal", "3.14159", nil, "3.14"},
		{"decimal", "3.14159", a("x"), "3.14"}, {"decimal", "3.14159", a(""), "3.14"},
		{"decimal", "3.9", a("0"), "3"}, {"decimal", "3", a("+1"), "3.0"},
		{"decimal", "abc", a("1"), "0.0"},
		{"bool", "true", nil, "true"}, {"bool", "false", nil, "false"},
		{"bool", "0", nil, "false"}, {"bool", "hello", nil, "true"},
		{"bool", "", nil, "false"}, {"bool", " \t\n\r", nil, "false"},
		{"bool", " false\n", nil, "false"}, {"bool", "FALSE", nil, "true"},
		{"bool", "0.0", nil, "true"}, {"bool", "no", nil, "true"},
	} {
		fn, ok := builtinTransforms[tt.name]
		if !ok {
			t.Errorf("%s is not registered", tt.name)
			continue
		}
		got, err := fn(tt.value, tt.args)
		if err != nil || got != tt.want {
			t.Errorf("%s(%q, %q) = %q, %v; want %q", tt.name, tt.value, tt.args, got, err, tt.want)
		}
	}
}
