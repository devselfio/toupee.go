// Package engine is toupee.go's implementation: the parser and its private
// node types, control flow, composition, lookup, indentation, HTML escaping
// and the transforms, together with the public Value, Context, Loader and
// RenderError types. Callers import the root package
// github.com/devselfio/toupee.go, whose types are aliases of these; this
// package is internal so that the root is the only supported entry point.
package engine

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
	return renderEntry(tmpl, ctx, nil, loader)
}

// renderEntry renders tmpl in a new scope for ctx, resolving named templates
// in set, which may be nil, and then through loader.
func renderEntry(tmpl parsedTemplate, ctx Context, set *Set, loader Loader) (string, error) {
	r := &renderer{source: tmpl.source, set: set, loader: loader, loads: new(int)}
	out, err := r.renderNodes(tmpl.nodes, newRenderContext(ctx))
	if err != nil {
		return "", err
	}
	return out, nil
}
