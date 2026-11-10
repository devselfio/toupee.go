package toupee

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
