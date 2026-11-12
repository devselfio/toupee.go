package engine

// Composition payloads of node. Every pos is the absolute byte offset of the
// element's opening '<' in the source the node was parsed from.

// attrOutput is a <t-attr name="..." /> element.
type attrOutput struct {
	name string
	pos  int
}

// slot is a <t-slot> element. name "" is the anonymous slot; body is the
// default rendered when the slot is not filled.
type slot struct {
	name string
	body []node
	pos  int
}

// contextBinding copies the caller's data at path to key in an include's
// data.
type contextBinding struct{ path, key string }

// definition is a <t-define> fill for the slot name.
type definition struct {
	name, source string
	body         []node // parsed for syntax; source is retained for later slot rendering
	pos          int
}

// include is a <t-include> element.
type include struct {
	template        string
	attrs           map[string]string
	defines         []definition
	anonymousSource string
	isolated        bool
	bindings        []contextBinding
	pos             int
	anonymousBody   []node // the parsed anonymous fill; never rendered
	origin          int    // pos mapped to the template's original source
}

// extension is a <t-extend> template: the parent's name and the fills it
// supplies.
type extension struct {
	template string
	defines  []definition
	pos      int
}

// boundTag is an ordinary HTML opening tag with t-var: or t-attr: bindings.
type boundTag struct {
	segments []tagSegment
	pos      int
}

// tagSegment is literal tag text or one binding.
type tagSegment struct {
	literal string
	binding *attributeBinding // nil means literal, including an empty literal
}

// attributeBinding renders htmlAttr from the data path refName (variable)
// or from the include attribute refName.
type attributeBinding struct {
	htmlAttr, refName string
	variable          bool
}

// defineResult is the parsed body of an include or extend.
type defineResult struct {
	defines         []definition
	anonymousSource string
	anonymousBody   []node
}
