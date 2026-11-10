package toupee

import "strconv"

// ValueKind selects which payload of a Value is meaningful.
type ValueKind uint8

// The seven value forms ADR 0008 fixes for the template boundary.
const (
	KindNil ValueKind = iota
	KindString
	KindBoolean
	KindInteger
	KindFloat
	KindList
	KindMap
)

// Value is a template datum. The zero Value is nil. Only the payload field
// selected by Kind is meaningful; the others are ignored, whatever they hold.
// Lists and maps contain Values recursively. Integer and Float match
// Toupee's signed 64-bit integer and 64-bit float.
type Value struct {
	Kind    ValueKind
	String  string
	Boolean bool
	Integer int64
	Float   float64
	List    []Value
	Map     map[string]Value
}

// scalarString returns v's text: a string unchanged, a boolean as true or
// false, an integer in signed decimal and a float in the shortest fixed
// decimal form, without an exponent. Nil, lists and maps, empty or not,
// have no text.
func scalarString(v Value) (string, bool) {
	switch v.Kind {
	case KindString:
		return v.String, true
	case KindBoolean:
		return strconv.FormatBool(v.Boolean), true
	case KindInteger:
		return strconv.FormatInt(v.Integer, 10), true
	case KindFloat:
		return strconv.FormatFloat(v.Float, 'f', -1, 64), true
	}
	return "", false
}
