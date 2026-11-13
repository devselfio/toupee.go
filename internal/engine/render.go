package engine

import (
	"bytes"
	"errors"
)

// maxDepth bounds nested includes, as upstream's default max_depth does.
// Slot fills render at the depth of the slot that asks for them, as
// upstream renders them, and have no count limit of their own; see
// renderSlot for the repeats that are reported.
const maxDepth = 50

// renderer renders the parsed nodes of one template. source is the text
// those nodes were parsed from, so errors are located in it; templateName
// is its name, "" for the input. depth counts the includes above it. fills
// holds the slot fills being rendered around it, innermost last. loads
// counts the loader calls made so far by the whole Render call; every
// renderer of one call shares it. set, nil for Render, is the prepared set
// named templates are looked up in before the loader; a lookup there is not
// a loader call.
type renderer struct {
	source       string
	set          *Set
	loader       Loader
	loads        *int
	templateName string
	depth        int
	fills        []activeFill
}

// renderContext is the scope nodes render in. data is owned by the render:
// Render starts from a shallow copy of the caller's Data, each loop
// iteration, include and slot fill from a shallow copy of its parent's, and
// t-let writes into the scope it runs in. attrs and slots are only read; a
// scope that needs different ones gets new maps.
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
	var b bytes.Buffer
	for _, n := range nodes {
		var s string
		var err error
		switch n.kind {
		case textNode:
			s = n.text
		case variableNode:
			s, err = r.renderVariable(n.variable, ctx)
		case attrOutputNode:
			s = r.renderAttrOutput(n.attrOutput, ctx)
		case boundTagNode:
			s, err = r.renderBoundTag(n.boundTag, ctx)
		case extendNode:
			s, err = r.renderExtend(n.extension, ctx)
		case slotNode:
			err = r.renderSlot(n.slot, ctx, &b)
		case includeNode:
			err = r.renderInclude(n.include, ctx, &b)
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
