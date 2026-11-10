package toupee

import (
	"bytes"
	"errors"
)

// maxDepth bounds nested includes, and separately nested slot fills, as
// upstream's default max_depth does.
const maxDepth = 50

// errDeferred reports that a template loaded or a slot fill parsed during
// rendering uses a construct this port does not implement yet. Render turns
// it into the bootstrap result for the whole call.
var errDeferred = errors.New("toupee: deferred construct")

// renderer renders the parsed nodes of one template. source is the text
// those nodes were parsed from, so errors are located in it; templateName
// is its name, "" for the input. ctx belongs to this template: it is a copy
// the caller's maps are never reached through. depth counts the includes
// above it and fills the slot fills.
type renderer struct {
	source       string
	ctx          Context
	loader       Loader
	templateName string
	depth        int
	fills        int
}

// renderNodes renders nodes in order.
func (r *renderer) renderNodes(nodes []node) (string, error) {
	var b bytes.Buffer
	for _, n := range nodes {
		var s string
		var err error
		switch n.kind {
		case textNode:
			s = n.text
		case variableNode:
			s, err = r.renderVariable(n.variable)
		case attrOutputNode:
			s = r.renderAttrOutput(n.attrOutput)
		case boundTagNode:
			s, err = r.renderBoundTag(n.boundTag)
		case extendNode:
			s, err = r.renderExtend(n.extension)
		case slotNode:
			err = r.renderSlot(n.slot, &b)
		case includeNode:
			err = r.renderInclude(n.include, &b)
		}
		if err != nil {
			return "", err
		}
		b.WriteString(s)
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
