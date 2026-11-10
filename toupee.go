// Package toupee is a Go port of the Toupee template engine, measured
// against Toupee v0.4.0's spec corpus.
//
// Render implements literal text and HTML, t-var, t-raw and t-comment:
// static and dynamic ($(expr)) data paths, the rendering of every Value
// kind, default bodies, HTML escaping, and MalformedElement and
// UndefinedVariable errors with their source line and column. It also
// implements t-if, t-elif and t-else with every condition Toupee defines,
// t-for with sorting, offset, limit, alias metadata and an else body, and
// t-let captures, scoped as Toupee scopes them. A transform="a|b:arg"
// attribute on t-var, t-raw or t-let runs a chain of Toupee's 25 built-in
// transforms (string, escaping, list, numeric, boolean and date) left to
// right: on t-var before the value is escaped, on t-raw before it is
// written verbatim, and on t-let before the capture is stored. And it
// implements composition: t-include with attributes, fills, isolation and
// context= bindings; t-extend chains with t-define; t-slot with default
// bodies; t-attr; and t-var:/t-attr: bindings on ordinary tags, with
// TemplateNotFound, DuplicateSlotDefinition and CircularReference errors.
//
// The other elements are not implemented yet. Until each is, a render that
// meets t-debug, in the input, a loaded template or a slot fill, returns
// "", nil, as the bootstrap did. This is temporary, not Toupee semantics.
package toupee

import "errors"

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
// Render calls loader once each time an include or extend names a
// template. A missing template is a *RenderError of kind TemplateNotFound
// located at the element that named it; any other loader error is returned
// as the loader returned it. Errors inside a loaded template are located in
// that template's source.
//
// On failure Render returns "" and the error, never partial output.
func Render(input string, ctx Context, loader Loader) (string, error) {
	tmpl, err := parse(input)
	if err != nil {
		return "", err
	}
	if tmpl.deferred {
		return "", nil
	}
	r := &renderer{source: input, loader: loader, loads: new(int)}
	out, err := r.renderNodes(tmpl.nodes, newRenderContext(ctx))
	if errors.Is(err, errDeferred) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return out, nil
}
