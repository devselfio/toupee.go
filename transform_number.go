package toupee

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The numeric and boolean built-ins port Toupee v0.4.0's src/transform.zig:
// each reads its input as text, coerces it with parseNumber and prints the
// result as Zig's "{d}" would.

// parseNumber is Toupee's best-effort coercion: trim space, tab, LF and CR,
// parse a float as Zig's std.fmt.parseFloat does, and otherwise map exactly
// "true" to 1 and anything else to 0. It never fails.
func parseNumber(value string) float64 {
	trimmed := strings.Trim(value, toupeeSpace)
	if f, ok := parseZigFloat(trimmed); ok {
		return f
	}
	if trimmed == "true" {
		return 1
	}
	return 0
}

// parseZigFloat accepts Zig's float syntax: an optional sign, then inf,
// infinity or nan in any case, a 0x/0X hexadecimal float whose p exponent
// is optional, or a decimal float; underscores among the digits are
// skipped. An out-of-range value is an infinity or zero, not an error.
func parseZigFloat(s string) (float64, bool) {
	body := s
	neg := false
	if body != "" && (body[0] == '+' || body[0] == '-') {
		neg = body[0] == '-'
		body = body[1:]
	}
	if body == "" {
		return 0, false
	}
	switch strings.ToLower(body) {
	case "inf", "infinity":
		if neg {
			return math.Inf(-1), true
		}
		return math.Inf(1), true
	case "nan":
		return math.NaN(), true
	}
	body = strings.ReplaceAll(body, "_", "")
	if len(body) > 2 && body[0] == '0' && body[1]|0x20 == 'x' && !strings.ContainsAny(body, "pP") {
		body += "p0"
	}
	if neg {
		body = "-" + body
	}
	f, err := strconv.ParseFloat(body, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, false
	}
	return f, true
}

// floatToInt truncates f toward zero, maps NaN to 0 and clamps values at or
// beyond the signed 64-bit range, infinities included, to its endpoints.
func floatToInt(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// formatZigFloat prints f as Zig's "{d}" does: the shortest decimal that
// reads back as f, never with an exponent, and inf, -inf or nan.
func formatZigFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// formatMinimalFloat prints f with no trailing fractional zeros and no
// trailing point.
func formatMinimalFloat(f float64) string {
	s := formatZigFloat(f)
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}

// formatDecimal prints f with exactly places fractional digits by cutting
// or zero-padding the text of formatZigFloat: it never rounds. With zero
// places there is no point.
func formatDecimal(f float64, places uint64) string {
	s := formatZigFloat(f)
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		if places == 0 {
			return s
		}
		return s + "." + strings.Repeat("0", int(places))
	}
	frac := uint64(len(s) - dot - 1)
	if frac >= places {
		if places == 0 {
			return s[:dot]
		}
		return s[:uint64(dot)+1+places]
	}
	return s + strings.Repeat("0", int(places-frac))
}

// maxDecimalPlaces bounds decimal's padding. Upstream allocates whatever it
// is asked for and fails only when allocation does; this port refuses a
// count that large instead of exhausting memory.
const maxDecimalPlaces = 1 << 20

func transformAbs(value string, _ []string) (string, error) {
	return formatMinimalFloat(math.Abs(parseNumber(value))), nil
}

func transformFloor(value string, _ []string) (string, error) {
	return strconv.FormatInt(floatToInt(math.Floor(parseNumber(value))), 10), nil
}

func transformCeil(value string, _ []string) (string, error) {
	return strconv.FormatInt(floatToInt(math.Ceil(parseNumber(value))), 10), nil
}

// transformInt truncates toward zero.
func transformInt(value string, _ []string) (string, error) {
	return strconv.FormatInt(floatToInt(parseNumber(value)), 10), nil
}

func transformFloat(value string, _ []string) (string, error) {
	return formatMinimalFloat(parseNumber(value)), nil
}

// transformDecimal prints args[0] fractional digits, or two when the
// argument is missing or not an unsigned count.
func transformDecimal(value string, args []string) (string, error) {
	places := uint64(2)
	if len(args) > 0 {
		if n, ok := parseCount(args[0]); ok {
			places = n
		}
	}
	if places > maxDecimalPlaces {
		return "", fmt.Errorf("%d places is more than %d", places, maxDecimalPlaces)
	}
	return formatDecimal(parseNumber(value), places), nil
}

// transformBool is false for a value that, trimmed, is empty, exactly
// "false" or exactly "0", and true otherwise.
func transformBool(value string, _ []string) (string, error) {
	switch strings.Trim(value, toupeeSpace) {
	case "", "false", "0":
		return "false", nil
	}
	return "true", nil
}
