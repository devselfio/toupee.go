package toupee

import (
	"bytes"
	"errors"
	"maps"
)

// parkedSlot holds the anonymous fill an anonymous definition replaced, so
// the replacement's own anonymous slot can render it.
const parkedSlot = "__t_inner_body__"

// renderTemplate parses source, the template name, and renders it against
// ctx at depth. A source that uses a deferred construct is errDeferred.
func (r *renderer) renderTemplate(source, name string, ctx Context, depth int) (string, error) {
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
	child := &renderer{source: source, ctx: ctx, loader: r.loader, templateName: name, depth: depth, fills: r.fills}
	return child.renderNodes(tmpl.nodes)
}

// loadTemplate returns the source of the template name, requested by the
// element at pos in r.source. A nil loader, or a loader error that is a
// TemplateNotFound RenderError, is a TemplateNotFound located at pos whose
// message is name; any other loader error is returned unchanged.
func (r *renderer) loadTemplate(name string, pos int) (string, error) {
	if r.loader == nil {
		return "", errorAt(r.source, pos, TemplateNotFound, name)
	}
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
func (r *renderer) renderAttrOutput(a attrOutput) string {
	if v, ok := r.ctx.Attrs[a.name]; ok {
		return escapeHTML(v)
	}
	return ""
}

// renderBoundTag renders tag's literal segments unchanged and each binding
// as ` ATTR="VALUE"` with VALUE escaped. A variable binding takes a scalar
// from the data; an attr binding takes an include attribute. A binding with
// no value is omitted with its leading space; it is never an error.
func (r *renderer) renderBoundTag(tag boundTag) (string, error) {
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
			if val, ok = resolve(r.ctx, seg.binding.refName); ok {
				value, ok = scalarString(val)
			}
		} else {
			value, ok = r.ctx.Attrs[seg.binding.refName]
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
// insertion point. The template sees a copy of the caller's data, or no
// data when isolated, overlaid with the context bindings that resolve, and
// only the include's own attributes and fills.
func (r *renderer) renderInclude(inc include, out *bytes.Buffer) error {
	src, err := r.loadTemplate(inc.template, inc.pos)
	if err != nil {
		return err
	}
	var child Context
	if inc.isolated {
		child.Data = map[string]Value{}
	} else {
		child = cloneContext(r.ctx)
		if child.Data == nil {
			child.Data = map[string]Value{}
		}
	}
	for _, b := range inc.bindings {
		if v, ok := resolve(r.ctx, b.path); ok {
			child.Data[b.key] = v
		}
	}
	child.Attrs = maps.Clone(inc.attrs)
	child.Slots = map[string]string{}
	for _, d := range inc.defines {
		child.Slots[d.name] = d.source
	}
	if inc.anonymousSource != "" {
		child.Slots[""] = inc.anonymousSource
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
// point: the fill's source parsed and rendered in a copy of the current
// context, or else s's default body, or else nothing. A present fill wins
// even when empty. An anonymous slot whose fill replaced a parked one sees
// the parked fill as its own anonymous slot.
func (r *renderer) renderSlot(s slot, out *bytes.Buffer) error {
	indent := insertionIndent(out)
	fill, ok := r.ctx.Slots[s.name]
	if !ok {
		if len(s.body) == 0 {
			return nil
		}
		rendered, err := r.renderNodes(s.body)
		if err != nil {
			return err
		}
		appendIndented(out, rendered, indent)
		return nil
	}
	if r.fills >= maxDepth {
		return errorAt(r.source, s.pos, CircularReference, s.name)
	}
	tmpl, err := parse(fill)
	if err != nil {
		return err
	}
	if tmpl.deferred {
		return errDeferred
	}
	ctx := cloneContext(r.ctx)
	if parked, ok := ctx.Slots[parkedSlot]; ok && s.name == "" {
		ctx.Slots[""] = parked
		delete(ctx.Slots, parkedSlot)
	}
	child := &renderer{source: fill, ctx: ctx, loader: r.loader, templateName: r.templateName, depth: r.depth, fills: r.fills + 1}
	rendered, err := child.renderNodes(tmpl.nodes)
	if err != nil {
		return err
	}
	appendIndented(out, rendered, indent)
	return nil
}

// renderExtend renders the chain of templates ext starts. Its definitions
// override the context's slots; an anonymous one parks the anonymous fill
// it replaces.
func (r *renderer) renderExtend(ext extension) (string, error) {
	slots := maps.Clone(r.ctx.Slots)
	if slots == nil {
		slots = map[string]string{}
	}
	for _, d := range ext.defines {
		if prev, ok := slots[""]; ok && d.name == "" {
			slots[parkedSlot] = prev
		}
		slots[d.name] = d.source
	}
	return r.resolveExtendChain(ext.template, slots, ext.pos)
}

// resolveExtendChain follows the extends from the template name, requested
// at pos in r.source, to the first template that does not extend, adding
// each parent's definitions for names not yet filled, and renders that
// template with slots. Requesting a template already in the chain is a
// CircularReference at the requesting extend. An empty template renders as
// "".
func (r *renderer) resolveExtendChain(name string, slots map[string]string, pos int) (string, error) {
	visited := map[string]bool{name: true}
	src, err := r.loadTemplate(name, pos)
	if err != nil {
		return "", err
	}
	cur := &renderer{source: src, ctx: r.ctx, loader: r.loader, templateName: name, depth: r.depth, fills: r.fills}
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
			cur.ctx = cloneContext(r.ctx)
			cur.ctx.Slots = slots
			return cur.renderNodes(tmpl.nodes)
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
		cur = &renderer{source: src, ctx: r.ctx, loader: r.loader, templateName: parent.template, depth: r.depth, fills: r.fills}
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
