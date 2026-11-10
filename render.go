package toupee

import "strings"

// renderer renders parsed nodes against one call's Context. It only reads
// the Context.
type renderer struct {
	source string
	ctx    Context
}

// renderNodes renders nodes in order.
func (r *renderer) renderNodes(nodes []node) (string, error) {
	var b strings.Builder
	for _, n := range nodes {
		switch n.kind {
		case textNode:
			b.WriteString(n.text)
		case variableNode:
			s, err := r.renderVariable(n.variable)
			if err != nil {
				return "", err
			}
			b.WriteString(s)
		}
	}
	return b.String(), nil
}

// renderVariable renders v's scalar value, even an empty or zero one, or else
// its explicit body, even an empty one. With neither it is an
// UndefinedVariable error whose message is the requested path. A t-var
// escapes whichever it renders; a t-raw does not.
func (r *renderer) renderVariable(v variable) (string, error) {
	var value string
	val, ok := resolve(r.ctx, v.name)
	if ok {
		value, ok = scalarString(val)
	}
	if !ok {
		if !v.hasBody {
			return "", errorAt(r.source, v.pos, UndefinedVariable, v.name)
		}
		var err error
		if value, err = r.renderNodes(v.body); err != nil {
			return "", err
		}
	}
	if v.raw {
		return value, nil
	}
	return escapeHTML(value), nil
}
