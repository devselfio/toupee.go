package toupee

import (
	"fmt"
	"strconv"
	"strings"
)

// transformStep is one step of a transform= chain: a name and its literal
// positional arguments.
type transformStep struct {
	name string
	args []string
}

// transformFn is a built-in transform. Like Toupee v0.4.0's, every
// transform maps a string and its arguments to a string. An error is the
// detail of a MalformedElement.
type transformFn func(value string, args []string) (string, error)

// transformRegistry maps a transform's name to its function.
type transformRegistry map[string]transformFn

// builtinTransforms is the private registry of Toupee's built-in transforms,
// keyed by their upstream names (src/transform.zig registerBuiltins). It is
// filled at initialisation and only read afterwards. abs, floor, ceil, int,
// float, decimal, bool and date are not implemented yet (issue #14's second
// slice), so naming one is an unknown transform for now.
var builtinTransforms = transformRegistry{
	"upper":      transformUpper,
	"lower":      transformLower,
	"capitalize": transformCapitalize,
	"trim":       transformTrim,
	"slugify":    transformSlugify,
	"truncate":   transformTruncate,
	"replace":    transformReplace,
	"default":    transformDefault,
	"length":     transformLength,
	"escape":     transformEscape,
	"url_encode": transformURLEncode,
	"url_decode": transformURLDecode,
	"js_escape":  transformJSEscape,
	"join":       transformJoin,
	"split":      transformSplit,
	"first":      transformFirst,
	"last":       transformLast,
}

// parseTransformSpec parses a transform= attribute value as Toupee's
// Parser.zig parseTransformSpec does: split on every '|', skip empty steps,
// split each step on every ':' into a name and arguments. Nothing is trimmed
// or case-folded, empty arguments are kept, and there is no quoting. Names
// are not checked here: an unknown name fails only when its step runs.
func parseTransformSpec(spec string) []transformStep {
	var steps []transformStep
	for _, part := range strings.Split(spec, "|") {
		if part == "" {
			continue
		}
		fields := strings.Split(part, ":")
		step := transformStep{name: fields[0]}
		if len(fields) > 1 {
			step.args = fields[1:]
		}
		steps = append(steps, step)
	}
	return steps
}

// hasDefaultTransform reports whether any step is named exactly default.
func hasDefaultTransform(steps []transformStep) bool {
	for _, s := range steps {
		if s.name == "default" {
			return true
		}
	}
	return false
}

// applyTransforms runs steps on value left to right, each on the previous
// result, and stops at the first failure. The error is a MalformedElement
// *RenderError without a position; the caller places it.
func applyTransforms(value string, steps []transformStep) (string, error) {
	for _, s := range steps {
		fn, ok := builtinTransforms[s.name]
		if !ok {
			return "", &RenderError{Kind: MalformedElement, Message: fmt.Sprintf("unknown transform %q", s.name)}
		}
		out, err := fn(value, s.args)
		if err != nil {
			return "", &RenderError{Kind: MalformedElement, Message: s.name + ": " + err.Error()}
		}
		value = out
	}
	return value, nil
}

// parseCount parses an unsigned 64-bit decimal count as Zig's
// std.fmt.parseInt(usize, s, 10) does: an optional '+' or '-' sign (a
// negative count is valid only when it is zero), then digits, with '_'
// allowed between them.
func parseCount(s string) (uint64, bool) {
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" || s[0] == '_' || s[len(s)-1] == '_' {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		d := uint64(c - '0')
		if n > (^uint64(0)-d)/10 {
			return 0, false
		}
		n = n*10 + d
	}
	if neg && n != 0 {
		return 0, false
	}
	return n, true
}

// countArg returns args[0] as a count, or def when there is no argument.
func countArg(args []string, def uint64, required bool) (uint64, error) {
	if len(args) == 0 {
		if required {
			return 0, fmt.Errorf("missing count")
		}
		return def, nil
	}
	n, ok := parseCount(args[0])
	if !ok {
		return 0, fmt.Errorf("invalid count %q", args[0])
	}
	return n, nil
}

func isASCIIAlpha(c byte) bool { return c|0x20 >= 'a' && c|0x20 <= 'z' }

func isASCIIAlnum(c byte) bool { return isASCIIAlpha(c) || c >= '0' && c <= '9' }

func asciiUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

func asciiLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

// mapBytes applies f to every byte of value.
func mapBytes(value string, f func(byte) byte) string {
	b := []byte(value)
	for i, c := range b {
		b[i] = f(c)
	}
	return string(b)
}

// transformUpper uppercases ASCII letters only.
func transformUpper(value string, _ []string) (string, error) {
	return mapBytes(value, asciiUpper), nil
}

// transformLower lowercases ASCII letters only.
func transformLower(value string, _ []string) (string, error) {
	return mapBytes(value, asciiLower), nil
}

// transformCapitalize uppercases an ASCII letter at the start or after a
// space, tab or LF, and leaves every other byte alone.
func transformCapitalize(value string, _ []string) (string, error) {
	b := []byte(value)
	prevSpace := true
	for i, c := range b {
		if prevSpace && isASCIIAlpha(c) {
			b[i] = asciiUpper(c)
		}
		prevSpace = c == ' ' || c == '\t' || c == '\n'
	}
	return string(b), nil
}

// toupeeSpace is the set Toupee trims: space, tab, LF and CR.
const toupeeSpace = " \t\n\r"

func transformTrim(value string, _ []string) (string, error) {
	return strings.Trim(value, toupeeSpace), nil
}

// transformSlugify keeps ASCII letters and digits, lowercased, and turns
// every run of other bytes into one hyphen, with none at either end.
func transformSlugify(value string, _ []string) (string, error) {
	var b strings.Builder
	prevHyphen := true
	for i := 0; i < len(value); i++ {
		c := value[i]
		if isASCIIAlnum(c) {
			b.WriteByte(asciiLower(c))
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.TrimSuffix(b.String(), "-"), nil
}

// transformTruncate keeps the first count bytes, with no ellipsis.
func transformTruncate(value string, args []string) (string, error) {
	n, err := countArg(args, 0, true)
	if err != nil {
		return "", err
	}
	if uint64(len(value)) <= n {
		return value, nil
	}
	return value[:n], nil
}

// transformReplace replaces every nonoverlapping occurrence of args[0],
// left to right, with args[1] or "". An empty old string replaces nothing.
func transformReplace(value string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing search string")
	}
	old, repl := args[0], ""
	if len(args) > 1 {
		repl = args[1]
	}
	if old == "" {
		return value, nil
	}
	return strings.ReplaceAll(value, old, repl), nil
}

// transformDefault returns args[0], or "", for an empty value.
func transformDefault(value string, args []string) (string, error) {
	if value != "" {
		return value, nil
	}
	if len(args) > 0 {
		return args[0], nil
	}
	return "", nil
}

// transformLength is the value's length in bytes.
func transformLength(value string, _ []string) (string, error) {
	return strconv.Itoa(len(value)), nil
}

func transformEscape(value string, _ []string) (string, error) {
	return escapeHTML(value), nil
}

const upperHex = "0123456789ABCDEF"

// transformURLEncode keeps ASCII letters, digits and -._~ and writes every
// other byte as %XX in uppercase hex.
func transformURLEncode(value string, _ []string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if isASCIIAlnum(c) || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0x0f])
		}
	}
	return b.String(), nil
}

func hexValue(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// transformURLDecode decodes each % followed by two hex digits and keeps
// every other byte, '+' and malformed sequences included.
func transformURLDecode(value string, _ []string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '%' && i+2 < len(value) {
			hi, ok1 := hexValue(value[i+1])
			lo, ok2 := hexValue(value[i+2])
			if ok1 && ok2 {
				b.WriteByte(hi<<4 | lo)
				i += 2
				continue
			}
		}
		b.WriteByte(value[i])
	}
	return b.String(), nil
}

// transformJSEscape backslash-escapes backslash, both quotes, LF, CR and
// tab, and writes the UTF-8 bytes of U+2028 and U+2029 as the six-character
// escapes backslash-u-2028 and backslash-u-2029. It adds no quotes and does
// no HTML escaping.
func transformJSEscape(value string, _ []string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		switch c := value[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\'':
			b.WriteString(`\'`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c == 0xE2 && i+2 < len(value) && value[i+1] == 0x80 && (value[i+2] == 0xA8 || value[i+2] == 0xA9) {
				if value[i+2] == 0xA8 {
					b.WriteString("\\u2028")
				} else {
					b.WriteString("\\u2029")
				}
				i += 2
				continue
			}
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// transformJoin replaces each LF with args[0], or one space.
func transformJoin(value string, args []string) (string, error) {
	sep := " "
	if len(args) > 0 {
		sep = args[0]
	}
	return strings.ReplaceAll(value, "\n", sep), nil
}

// transformSplit replaces each occurrence of the separator args[0] with LF,
// except one that starts the value, which is dropped. An empty separator
// leaves the value unchanged.
func transformSplit(value string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing separator")
	}
	sep := args[0]
	if sep == "" {
		return value, nil
	}
	if strings.HasPrefix(value, sep) {
		value = value[len(sep):]
	}
	return strings.ReplaceAll(value, sep, "\n"), nil
}

// transformFirst keeps the first count bytes (default 1).
func transformFirst(value string, args []string) (string, error) {
	n, err := countArg(args, 1, false)
	if err != nil {
		return "", err
	}
	if n >= uint64(len(value)) {
		return value, nil
	}
	return value[:n], nil
}

// transformLast keeps the last count bytes (default 1).
func transformLast(value string, args []string) (string, error) {
	n, err := countArg(args, 1, false)
	if err != nil {
		return "", err
	}
	if n >= uint64(len(value)) {
		return value, nil
	}
	return value[uint64(len(value))-n:], nil
}
