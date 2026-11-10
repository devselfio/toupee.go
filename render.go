package toupee

import (
	"errors"
	"strings"
)

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
// its explicit body, even an empty one. With neither, a default step in its
// transform chain stands in for the value with ""; without one it is an
// UndefinedVariable error whose message is the requested path. The chain
// then runs on whichever was chosen, and a t-var escapes its result; a t-raw
// does not. A failing step is a MalformedElement at the element.
func (r *renderer) renderVariable(v variable) (string, error) {
	var value string
	val, ok := resolve(r.ctx, v.name)
	if ok {
		value, ok = scalarString(val)
	}
	if !ok {
		switch {
		case v.hasBody:
			var err error
			if value, err = r.renderNodes(v.body); err != nil {
				return "", err
			}
		case hasDefaultTransform(v.transforms):
		default:
			return "", errorAt(r.source, v.pos, UndefinedVariable, v.name)
		}
	}
	value, err := applyTransforms(value, v.transforms)
	if err != nil {
		var re *RenderError
		if errors.As(err, &re) {
			return "", errorAt(r.source, v.pos, re.Kind, re.Message)
		}
		return "", err
	}
	if v.raw {
		return value, nil
	}
	return escapeHTML(value), nil
}
