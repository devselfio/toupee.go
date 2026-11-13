package engine

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// Diagnostic is one problem Validate found in a template of the set.
type Diagnostic struct {
	Template string    // the name the template was supplied under
	Line     int       // 1-based, in that template's source
	Column   int       // 1-based, counted in bytes
	Kind     ErrorKind // MalformedElement, DuplicateSlotDefinition or TemplateNotFound
	Message  string
}

// Validate parses every template of the set, named by its keys, and checks
// without page data that each include and extend target is in the set or
// known to loader. A syntax error or a missing target is a Diagnostic; any
// other loader error stops validation and is returned wrapped. Diagnostics
// are ordered by template name, then line, then column. Templates reached
// only through loader are checked for existence, not parsed.
func Validate(templates map[string]string, loader Loader) ([]Diagnostic, error) {
	_, diags, err := check(templates, loader)
	return diags, err
}

// check is Validate's pass, also returning the parse of every template
// that parsed. The returned map is new; templates is read only during the
// call.
func check(templates map[string]string, loader Loader) (map[string]parsedTemplate, []Diagnostic, error) {
	parsed := make(map[string]parsedTemplate, len(templates))
	var diags []Diagnostic
	for _, name := range slices.Sorted(maps.Keys(templates)) {
		source := templates[name]
		tmpl, err := parse(source)
		if err != nil {
			var re *RenderError
			if !errors.As(err, &re) {
				return nil, nil, fmt.Errorf("toupee: validate %s: %w", name, err)
			}
			diags = append(diags, Diagnostic{Template: name, Line: re.Line, Column: re.Column, Kind: re.Kind, Message: re.Message})
			continue
		}
		v := &validator{name: name, source: source, templates: templates, loader: loader}
		if err := v.walk(tmpl.nodes); err != nil {
			return nil, nil, err
		}
		parsed[name] = tmpl
		slices.SortFunc(v.diags, func(a, b Diagnostic) int {
			return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
		})
		diags = append(diags, v.diags...)
	}
	return parsed, diags, nil
}

// validator walks one parsed template, collecting its missing targets.
type validator struct {
	name, source string
	templates    map[string]string
	loader       Loader
	diags        []Diagnostic
}

// walk visits the bodies Toupee's collectDiagnostics visits.
func (v *validator) walk(nodes []node) error {
	for _, n := range nodes {
		var bodies [][]node
		switch n.kind {
		case includeNode:
			if err := v.check(n.include.template, n.include.origin, "include"); err != nil {
				return err
			}
			bodies = append(bodies, n.include.anonymousBody)
			for _, d := range n.include.defines {
				bodies = append(bodies, d.body)
			}
		case extendNode:
			if err := v.check(n.extension.template, n.extension.pos, "extends"); err != nil {
				return err
			}
			for _, d := range n.extension.defines {
				bodies = append(bodies, d.body)
			}
		case conditionalNode:
			for _, b := range n.conditional.branches {
				bodies = append(bodies, b.body)
			}
			bodies = append(bodies, n.conditional.elseBody)
		case loopNode:
			bodies = append(bodies, n.loop.body, n.loop.elseBody)
		case slotNode:
			bodies = append(bodies, n.slot.body)
		case variableNode:
			bodies = append(bodies, n.variable.body)
		case letNode:
			bodies = append(bodies, n.let.body)
		}
		for _, b := range bodies {
			if err := v.walk(b); err != nil {
				return err
			}
		}
	}
	return nil
}

// check records a diagnostic when target is neither in the set nor known
// to the loader; pos is the referencing element's offset in v.source. A
// loader error other than TemplateNotFound is returned.
func (v *validator) check(target string, pos int, what string) error {
	if _, ok := v.templates[target]; ok {
		return nil
	}
	if v.loader != nil {
		_, err := v.loader(target)
		if err == nil {
			return nil
		}
		var re *RenderError
		if !errors.As(err, &re) || re.Kind != TemplateNotFound {
			return fmt.Errorf("toupee: validate %s: load %s: %w", v.name, target, err)
		}
	}
	e := errorAt(v.source, pos, TemplateNotFound, fmt.Sprintf("missing %s '%s'", what, target))
	v.diags = append(v.diags, Diagnostic{Template: v.name, Line: e.Line, Column: e.Column, Kind: e.Kind, Message: e.Message})
	return nil
}
