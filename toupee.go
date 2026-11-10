// Package toupee is a Go port of the Toupee template engine, measured
// against Toupee v0.4.0's spec corpus.
//
// Render implements literal text and HTML, t-var, t-raw and t-comment:
// static and dynamic ($(expr)) data paths, the rendering of every Value
// kind, default bodies, HTML escaping, and MalformedElement and
// UndefinedVariable errors with their source line and column. It also
// implements t-if, t-elif and t-else with every condition Toupee defines,
// t-for with sorting, offset, limit, alias metadata and an else body, and
// t-let captures, scoped as Toupee scopes them.
//
// The other elements are not implemented yet. Until each is, an input that
// uses t-debug, t-attr, t-slot, composition (t-include, t-extend,
// t-define), a transform= attribute or a t-var:/t-attr: binding on an
// ordinary tag renders as "", nil, as the bootstrap did. This is temporary,
// not Toupee semantics.
package toupee

// Context is the state a template renders against. The zero Context means
// empty data, attrs and slots. Map keys may be empty; the empty slot name is
// the unnamed slot.
type Context struct {
	Data  map[string]Value
	Attrs map[string]string
	Slots map[string]string
}

// Loader returns the source of the named template. For a name it does not
// know it returns ("", &RenderError{Kind: TemplateNotFound, Message: name});
// any other error is a loader failure, distinct from a missing name. An
// empty source is a found, empty template. A nil Loader knows no templates.
type Loader func(name string) (string, error)

// Render renders input against ctx, resolving named templates through
// loader. It reads its arguments without modifying them and keeps no state
// between calls. Errors from rendering are *RenderError; use errors.As.
//
// On failure Render returns "" and the error, never partial output. It does
// not call loader yet: no implemented element names a template.
func Render(input string, ctx Context, loader Loader) (string, error) {
	tmpl, err := parse(input)
	if err != nil {
		return "", err
	}
	if tmpl.deferred {
		return "", nil
	}
	r := &renderer{source: input}
	out, err := r.renderNodes(tmpl.nodes, newRenderContext(ctx))
	if err != nil {
		return "", err
	}
	return out, nil
}
