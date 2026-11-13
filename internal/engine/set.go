package engine

// Set is a validated set of named templates, held as parses. It is never
// written after Prepare, so any number of goroutines may render it at once.
type Set struct {
	templates map[string]parsedTemplate
	loader    Loader
}

// Prepare validates templates as Validate does and, when there is no error
// and no diagnostic, returns a Set holding each template's parse and loader.
// Otherwise the Set is nil and Validate's diagnostics or error are returned.
// The Set keeps nothing of the templates map itself.
func Prepare(templates map[string]string, loader Loader) (*Set, []Diagnostic, error) {
	parsed, diags, err := check(templates, loader)
	if err != nil || len(diags) > 0 {
		return nil, diags, err
	}
	return &Set{templates: parsed, loader: loader}, nil, nil
}

// Render renders the set's template name against ctx, as Render renders it,
// taking include and extend targets in the set from their retained parses
// and the others from the set's loader. A name not in the set is a
// TemplateNotFound with no position; the loader is not asked for it.
// A set lookup is not a loader call, so a slot fill that re-enters itself
// through set templates only is a CircularReference at the slot, where
// Render with a loader does not detect it.
func (s *Set) Render(name string, ctx Context) (string, error) {
	tmpl, ok := s.templates[name]
	if !ok {
		return "", &RenderError{Kind: TemplateNotFound, Message: name}
	}
	return renderEntry(tmpl, ctx, s, s.loader)
}
