package toupee

// nodeKind selects which field of a node is meaningful.
type nodeKind uint8

const (
	textNode nodeKind = iota
	variableNode
	conditionalNode
	loopNode
	letNode
)

// node is one element of a parsed template: literal text, a variable, a
// conditional, a loop or a let capture. Comments produce no node. Only the
// field kind selects is meaningful.
type node struct {
	kind        nodeKind
	text        string
	variable    variable
	conditional *conditional
	loop        *loop
	let         *letBinding
}

// variable is a t-var (escaped) or t-raw (raw) element.
type variable struct {
	name    string
	body    []node
	hasBody bool // false for a self-closing tag; true for a block, even an empty one
	raw     bool
	pos     int // absolute opening '<' byte offset in the original source
}

// parsedTemplate is a parse result. deferred reports that the source uses a
// construct this port does not implement yet; see parse.
type parsedTemplate struct {
	nodes    []node
	deferred bool
}
