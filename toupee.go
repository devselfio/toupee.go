// Package toupee is a Go port of the Toupee template engine, measured
// against Toupee v0.4.0's spec corpus.
//
// This is the bootstrap package: it fixes the public render boundary, and
// Render does not yet implement any template semantics.
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
// Rendering semantics are not implemented yet: this bootstrap Render
// returns "", nil and does not call loader.
func Render(input string, ctx Context, loader Loader) (string, error) {
	return "", nil
}
