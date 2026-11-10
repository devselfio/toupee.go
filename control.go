package toupee

// conditionSource is where a condition reads its subject from.
type conditionSource uint8

const (
	sourceVariable conditionSource = iota
	sourceAttr
	sourceSlot
)

// comparisonKind is the test a condition applies to its subject.
type comparisonKind uint8

const (
	compareExists comparisonKind = iota
	compareNotExists
	compareTruthy
	compareNotTruthy
	compareEquals
	compareNotEquals
	compareContains
	compareStartsWith
	compareEndsWith
	compareMatches
)

// condition is the test on one t-if or t-elif tag. operand is meaningful
// only for the comparisons that take a value.
type condition struct {
	source     conditionSource
	name       string
	comparison comparisonKind
	operand    string
}

// conditionalBranch is one t-if or t-elif condition and its body.
type conditionalBranch struct {
	condition condition
	body      []node
}

// conditional is a t-if element: ordered branches, the first true of which
// renders, and an optional t-else body.
type conditional struct {
	pos      int
	branches []conditionalBranch
	elseBody []node
}

// loop is a t-for element. A nil limit or offset is absent; a pointer to
// zero is a supplied zero. alias is "" when absent.
type loop struct {
	pos        int
	item       string
	collection string
	alias      string
	sorted     bool // a sort= attribute is present, even an empty one
	sortField  string
	descending bool
	limit      *uint64
	offset     *uint64
	body       []node
	elseBody   []node
}

// letBinding is a t-let element: its body, rendered, is stored as a string
// under the literal top-level name.
type letBinding struct {
	pos  int
	name string
	body []node
	// transforms is the parsed transform= chain, nil when absent or empty.
	transforms []transformStep
}
