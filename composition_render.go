package toupee

import (
	"bytes"
	"errors"
	"maps"
	"reflect"
)

// parkedSlot holds the anonymous fill an anonymous definition replaced, so
// the replacement's own anonymous slot can render it.
const parkedSlot = "__t_inner_body__"

// maxFillNesting is a backstop for slot fills nested inside one another
// that repeatsFill cannot prove endless: a fill that grows a t-let each
// time it fills itself again, or one that calls the loader on every pass.
// Upstream renders fills at the slot's depth with no bound, so it has no
// result for such a template; it overflows its stack, and without the
// backstop the Go process would die the same way. Whether a fill ends
// cannot be decided in general, so any backstop also rejects some finite
// chains: here, any that nests more than 1000 fills.
const maxFillNesting = 1000

// activeFill is a slot fill being rendered: the slot's name, the fill's
// source, the scope the slot was met in, the include depth it renders at
// and the number of loader calls made before it started. While the fill
// renders that scope is not written, so it can be compared with a later
// one.
type activeFill struct {
	name, source string
	ctx          *renderContext
	depth, loads int
}

// renderTemplate parses source, the template name, and renders it in ctx
// at depth. A source that uses a deferred construct is errDeferred.
func (r *renderer) renderTemplate(source, name string, ctx *renderContext, depth int) (string, error) {
	if depth > maxDepth {
		return "", errorAt(source, 0, CircularReference, name)
	}
	tmpl, err := parse(source)
	if err != nil {
		return "", err
	}
	if tmpl.deferred {
		return "", errDeferred
	}
	child := &renderer{source: source, loader: r.loader, loads: r.loads, templateName: name, depth: depth, fills: r.fills}
	return child.renderNodes(tmpl.nodes, ctx)
}

// loadTemplate returns the source of the template name, requested by the
// element at pos in r.source. A nil loader, or a loader error that is a
// TemplateNotFound RenderError, is a TemplateNotFound located at pos whose
// message is name; any other loader error is returned unchanged.
func (r *renderer) loadTemplate(name string, pos int) (string, error) {
	if r.loader == nil {
		return "", errorAt(r.source, pos, TemplateNotFound, name)
	}
	*r.loads++
	src, err := r.loader(name)
	if err != nil {
		var re *RenderError
		if errors.As(err, &re) && re.Kind == TemplateNotFound {
			return "", errorAt(r.source, pos, TemplateNotFound, name)
		}
		return "", err
	}
	return src, nil
}

// renderAttrOutput renders the escaped include attribute a names, or
// nothing when there is none.
func (r *renderer) renderAttrOutput(a attrOutput, ctx *renderContext) string {
	if v, ok := ctx.attrs[a.name]; ok {
		return escapeHTML(v)
	}
	return ""
}

// renderBoundTag renders tag's literal segments unchanged and each binding
// as ` ATTR="VALUE"` with VALUE escaped. A variable binding takes a scalar
// from the data; an attr binding takes an include attribute. A binding with
// no value is omitted with its leading space; it is never an error.
func (r *renderer) renderBoundTag(tag boundTag, ctx *renderContext) (string, error) {
	var b bytes.Buffer
	for _, seg := range tag.segments {
		if seg.binding == nil {
			b.WriteString(seg.literal)
			continue
		}
		var value string
		var ok bool
		if seg.binding.variable {
			var val Value
			if val, ok = ctx.lookup(seg.binding.refName); ok {
				value, ok = scalarString(val)
			}
		} else {
			value, ok = ctx.attrs[seg.binding.refName]
		}
		if !ok {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(seg.binding.htmlAttr)
		b.WriteString(`="`)
		b.WriteString(escapeHTML(value))
		b.WriteByte('"')
	}
	return b.String(), nil
}

// renderInclude renders the template inc names into out, indented to the
// insertion point. The template sees a copy of ctx's data, or no data when
// isolated, overlaid with the context bindings that resolve in ctx, and
// only the include's own attributes and fills.
func (r *renderer) renderInclude(inc include, ctx *renderContext, out *bytes.Buffer) error {
	src, err := r.loadTemplate(inc.template, inc.pos)
	if err != nil {
		return err
	}
	child := &renderContext{data: map[string]Value{}}
	if !inc.isolated {
		child.data = cloneData(ctx.data)
	}
	for _, b := range inc.bindings {
		if v, ok := ctx.lookup(b.path); ok {
			child.data[b.key] = v
		}
	}
	child.attrs = maps.Clone(inc.attrs)
	child.slots = map[string]string{}
	for _, d := range inc.defines {
		child.slots[d.name] = d.source
	}
	if inc.anonymousSource != "" {
		child.slots[""] = inc.anonymousSource
	}
	indent := insertionIndent(out)
	rendered, err := r.renderTemplate(src, inc.template, child, r.depth+1)
	if err != nil {
		return err
	}
	appendIndented(out, rendered, indent)
	return nil
}

// renderSlot renders the fill for s into out, indented to the insertion
// point: the fill's source parsed and rendered in a copy of ctx, or else
// s's default body in ctx, or else nothing. A present fill wins even when
// empty. An anonymous slot whose fill replaced a parked one sees the parked
// fill as its own anonymous slot.
//
// A fill renders at the slot's include depth, as upstream's does, and is
// not counted against maxDepth. Meeting the same slot with the same fill,
// at the same depth, in a scope equal to one a fill around it started from,
// with no loader call since, can only repeat forever; that is a
// CircularReference at the slot, with the slot name as its message. So is
// starting a fill inside maxFillNesting others, even if the chain would
// have ended: that backstop is what keeps an endless fill from overflowing
// the stack.
func (r *renderer) renderSlot(s slot, ctx *renderContext, out *bytes.Buffer) error {
	indent := insertionIndent(out)
	fill, ok := ctx.slots[s.name]
	if !ok {
		if len(s.body) == 0 {
			return nil
		}
		rendered, err := r.renderNodes(s.body, ctx)
		if err != nil {
			return err
		}
		appendIndented(out, rendered, indent)
		return nil
	}
	if r.repeatsFill(s.name, fill, ctx) || len(r.fills) >= maxFillNesting {
		return errorAt(r.source, s.pos, CircularReference, s.name)
	}
	tmpl, err := parse(fill)
	if err != nil {
		return err
	}
	if tmpl.deferred {
		return errDeferred
	}
	child := cloneRenderContext(ctx)
	if parked, ok := ctx.slots[parkedSlot]; ok && s.name == "" {
		child.slots = maps.Clone(ctx.slots)
		child.slots[""] = parked
		delete(child.slots, parkedSlot)
	}
	fills := append(r.fills[:len(r.fills):len(r.fills)], activeFill{name: s.name, source: fill, ctx: ctx, depth: r.depth, loads: *r.loads})
	fr := &renderer{source: fill, loader: r.loader, loads: r.loads, templateName: r.templateName, depth: r.depth, fills: fills}
	rendered, err := fr.renderNodes(tmpl.nodes, child)
	if err != nil {
		return err
	}
	appendIndented(out, rendered, indent)
	return nil
}

// repeatsFill reports whether a fill around r is rendering the slot name
// with fill, at r's include depth, from a scope equal to ctx, and no loader
// call has been made since that fill started. Rendering from that state
// reads nothing else, so it would meet this slot again in the same state
// forever. A loader call in between breaks the argument: a Loader may
// return different content each time, so that is not reported as a cycle.
func (r *renderer) repeatsFill(name, fill string, ctx *renderContext) bool {
	for _, f := range r.fills {
		if f.name == name && f.source == fill && f.depth == r.depth && f.loads == *r.loads &&
			maps.Equal(f.ctx.attrs, ctx.attrs) && maps.Equal(f.ctx.slots, ctx.slots) &&
			reflect.DeepEqual(f.ctx.data, ctx.data) {
			return true
		}
	}
	return false
}

// renderExtend renders the chain of templates ext starts. Its definitions
// override ctx's slots; an anonymous one parks the anonymous fill it
// replaces.
func (r *renderer) renderExtend(ext extension, ctx *renderContext) (string, error) {
	slots := maps.Clone(ctx.slots)
	if slots == nil {
		slots = map[string]string{}
	}
	for _, d := range ext.defines {
		if prev, ok := slots[""]; ok && d.name == "" {
			slots[parkedSlot] = prev
		}
		slots[d.name] = d.source
	}
	return r.resolveExtendChain(ext.template, slots, ext.pos, ctx)
}

// resolveExtendChain follows the extends from the template name, requested
// at pos in r.source, to the first template that does not extend, adding
// each parent's definitions for names not yet filled, and renders that
// template with a copy of ctx's data, its attrs and slots. Requesting a
// template already in the chain is a CircularReference at the requesting
// extend. An empty template renders as "".
func (r *renderer) resolveExtendChain(name string, slots map[string]string, pos int, ctx *renderContext) (string, error) {
	visited := map[string]bool{name: true}
	src, err := r.loadTemplate(name, pos)
	if err != nil {
		return "", err
	}
	cur := &renderer{source: src, loader: r.loader, loads: r.loads, templateName: name, depth: r.depth, fills: r.fills}
	for {
		tmpl, err := parse(cur.source)
		if err != nil {
			return "", err
		}
		if tmpl.deferred {
			return "", errDeferred
		}
		if len(tmpl.nodes) == 0 {
			return "", nil
		}
		if tmpl.nodes[0].kind != extendNode {
			leaf := &renderContext{data: cloneData(ctx.data), attrs: ctx.attrs, slots: slots}
			return cur.renderNodes(tmpl.nodes, leaf)
		}
		parent := tmpl.nodes[0].extension
		if visited[parent.template] {
			return "", errorAt(cur.source, parent.pos, CircularReference, parent.template)
		}
		visited[parent.template] = true
		for _, d := range parent.defines {
			if _, ok := slots[d.name]; !ok {
				slots[d.name] = d.source
			}
		}
		src, err := cur.loadTemplate(parent.template, parent.pos)
		if err != nil {
			return "", err
		}
		cur = &renderer{source: src, loader: r.loader, loads: r.loads, templateName: parent.template, depth: r.depth, fills: r.fills}
	}
}

// insertionIndent is detectIndent of out's content, read from its last LF.
func insertionIndent(out *bytes.Buffer) string {
	b := out.Bytes()
	lf := bytes.LastIndexByte(b, '\n')
	if lf < 0 {
		return ""
	}
	return detectIndent(string(b[lf:]))
}
