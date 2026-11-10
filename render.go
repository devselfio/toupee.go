package toupee

import (
	"errors"
	"strings"
)

// renderer renders parsed nodes for one Render call.
type renderer struct {
	source string
}

// renderContext is the scope nodes render in. data is owned by the render:
// Render starts from a shallow copy of the caller's Data, each loop
// iteration from a shallow copy of its parent's, and t-let writes into the
// scope it runs in. attrs and slots are only read.
type renderContext struct {
	data  map[string]Value
	attrs map[string]string
	slots map[string]string
}

// newRenderContext returns a render-owned scope for the caller's ctx. The
// caller's maps are never written.
func newRenderContext(ctx Context) *renderContext {
	return &renderContext{data: cloneData(ctx.Data), attrs: ctx.Attrs, slots: ctx.Slots}
}

// cloneRenderContext returns a child scope: a shallow copy of parent's
// top-level data, sharing its attrs and slots.
func cloneRenderContext(parent *renderContext) *renderContext {
	return &renderContext{data: cloneData(parent.data), attrs: parent.attrs, slots: parent.slots}
}

func cloneData(data map[string]Value) map[string]Value {
	out := make(map[string]Value, len(data)+2)
	for k, v := range data {
		out[k] = v
	}
	return out
}

// publicContext is the read-only view resolve takes.
func (c *renderContext) publicContext() Context {
	return Context{Data: c.data, Attrs: c.attrs, Slots: c.slots}
}

// lookup resolves path in c's data, as resolve does.
func (c *renderContext) lookup(path string) (Value, bool) {
	return resolve(c.publicContext(), path)
}

// renderNodes renders nodes in order in ctx.
func (r *renderer) renderNodes(nodes []node, ctx *renderContext) (string, error) {
	var b strings.Builder
	for _, n := range nodes {
		var s string
		var err error
		switch n.kind {
		case textNode:
			s = n.text
		case variableNode:
			s, err = r.renderVariable(n.variable, ctx)
		case conditionalNode:
			s, err = r.renderConditional(n.conditional, ctx)
		case loopNode:
			s, err = r.renderLoop(n.loop, ctx)
		case letNode:
			err = r.renderLet(n.let, ctx)
		}
		if err != nil {
			return "", err
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

// renderVariable renders v's scalar value, even an empty or zero one, or else
// its explicit body, even an empty one. With neither, a default step in its
// transform chain stands in for the value with ""; without one it is an
// UndefinedVariable error whose message is the requested path. The chain
// then runs on whichever was chosen, and a t-var escapes its result; a t-raw
// does not. A failing step is a MalformedElement at the element.
func (r *renderer) renderVariable(v variable, ctx *renderContext) (string, error) {
	var value string
	val, ok := ctx.lookup(v.name)
	if ok {
		value, ok = scalarString(val)
	}
	if !ok {
		switch {
		case v.hasBody:
			var err error
			if value, err = r.renderNodes(v.body, ctx); err != nil {
				return "", err
			}
		case hasDefaultTransform(v.transforms):
		default:
			return "", errorAt(r.source, v.pos, UndefinedVariable, v.name)
		}
	}
	value, err := r.applyTransformsAt(value, v.transforms, v.pos)
	if err != nil {
		return "", err
	}
	if v.raw {
		return value, nil
	}
	return escapeHTML(value), nil
}

// applyTransformsAt runs steps on value and places a failure at the element
// whose opening '<' is at byte offset pos.
func (r *renderer) applyTransformsAt(value string, steps []transformStep, pos int) (string, error) {
	out, err := applyTransforms(value, steps)
	if err != nil {
		var re *RenderError
		if errors.As(err, &re) {
			return "", errorAt(r.source, pos, re.Kind, re.Message)
		}
		return "", err
	}
	return out, nil
}
