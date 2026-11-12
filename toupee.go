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
// This port has no debug mode. <t-debug /> and <t-debug> are always
// stripped, preserving surrounding text, as Toupee renders them with its
// debug option off, the default. A context dump would require a render
// option that this port does not offer. An unclosed debug opening tag is a
// MalformedElement with the message "unclosed <t-debug> tag".
//
// Validate checks a named set of templates before rendering, with no page
// data: it reports each template's syntax error and each include or extend
// target that is neither in the set nor known to the loader, located at the
// referencing element. Templates reached only through the loader are not
// parsed.
//
// # Layout
//
// This file, toupee.go, is the root package's only source file and holds
// its entry points, Render and Validate. The implementation (the parser
// and its node types, control flow, composition, lookup, indentation, HTML
// escaping and the transforms) lives in internal/engine. The root's types
// are aliases of that package's and its constants re-export that
// package's, so values and errors pass between the two unchanged; callers
// import only this package.
package toupee

import "github.com/devselfio/toupee.go/internal/engine"

// Context is the state a template renders against. The zero Context means
// empty data, attrs and slots. Map keys may be empty; the empty slot name is
// the unnamed slot. Its fields are Data map[string]Value, Attrs
// map[string]string and Slots map[string]string.
type Context = engine.Context

// Loader returns the source of the named template. For a name it does not
// know it returns ("", &RenderError{Kind: TemplateNotFound, Message: name});
// any other error is a loader failure, distinct from a missing name. An
// empty source is a found, empty template. A nil Loader knows no templates.
type Loader = engine.Loader

// Value is a template datum. The zero Value is nil. Only the payload field
// selected by Kind is meaningful; the others are ignored, whatever they hold.
// Lists and maps contain Values recursively. Integer and Float match
// Toupee's signed 64-bit integer and 64-bit float.
type Value = engine.Value

// ValueKind selects which payload of a Value is meaningful.
type ValueKind = engine.ValueKind

// The seven value forms ADR 0008 fixes for the template boundary.
const (
	KindNil     = engine.KindNil
	KindString  = engine.KindString
	KindBoolean = engine.KindBoolean
	KindInteger = engine.KindInteger
	KindFloat   = engine.KindFloat
	KindList    = engine.KindList
	KindMap     = engine.KindMap
)

// RenderError is a render failure. Line and Column are 1-based when known
// and 0 when not. Message is the unformatted detail, kept exactly. The text
// of Error is for people; programs read the fields.
type RenderError = engine.RenderError

// ErrorKind is the symbolic name of a render error, as the corpus's
// "error: KIND" directive spells it.
type ErrorKind = engine.ErrorKind

// The error kinds Toupee v0.4.0's corpus uses.
const (
	MalformedElement        = engine.MalformedElement
	UndefinedVariable       = engine.UndefinedVariable
	TemplateNotFound        = engine.TemplateNotFound
	DuplicateSlotDefinition = engine.DuplicateSlotDefinition
	CircularReference       = engine.CircularReference
)

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
	return engine.Render(input, ctx, loader)
}

// Diagnostic is one problem Validate found in a template of the set:
// the template's name, the 1-based line and byte column in its source, and
// Kind and Message. Kind is MalformedElement or DuplicateSlotDefinition for
// a syntax error, with the parser's message, or TemplateNotFound for a
// missing target, with the message "missing include 'NAME'" or
// "missing extends 'NAME'".
type Diagnostic = engine.Diagnostic

// Validate parses every template of the set, which maps names to sources,
// and checks without page data that every include and extend target is a
// name in the set or a template loader returns. Targets are checked in every
// conditional branch, loop body, default body, let, slot default, include
// fill and extend definition. A syntax error or a missing target is a
// Diagnostic; a template that failed to parse still counts as present.
// Diagnostics are ordered by template name, then line, then column. It reads
// its arguments without modifying them and keeps no state between calls.
//
// loader follows the Loader convention: it is called once for each
// reference outside the set; a nil error resolves the target, and a nil
// loader or a TemplateNotFound *RenderError makes it missing. Any other
// loader error stops validation and is returned, wrapped, with no
// diagnostics. A template found only through loader is checked for
// existence and is not parsed.
//
// Not checked: extend cycles, include depth, variables, transforms, slot
// names, and a fill whose text begins with <t-extend.
func Validate(templates map[string]string, loader Loader) ([]Diagnostic, error) {
	return engine.Validate(templates, loader)
}

// Set is a validated set of named templates, held as parses, made by
// Prepare. It is never written after Prepare: any number of goroutines may
// call its Render method at once, each with its own scope and output.
//
// Set.Render(name, ctx) renders the set's template name against ctx. A name
// the set lacks is ("", &RenderError{Kind: TemplateNotFound, Message: name})
// with Line and Column 0; the loader is not asked for it. An include or
// extend target in the set is rendered from its retained parse, without
// parsing it again or calling the loader. A target outside the set goes to
// the loader given to Prepare exactly as Render's loader does: called once
// per reference, parsed on each render, a missing name a TemplateNotFound
// at the element that named it, any other loader error returned unchanged.
// Output and errors are those Render gives for the same templates, with ""
// on failure, with one difference: a set lookup is not a loader call, so a
// slot fill that re-enters itself through set templates only is a
// CircularReference at the slot, where Render with a loader renders on.
// Slot fills are parsed when their slot is met, as Render parses them.
type Set = engine.Set

// Prepare validates the set of templates, which maps names to sources, with
// loader, returning the Diagnostics and error Validate returns for the same
// arguments. When both are empty it returns a *Set holding the parse of
// every template and loader; otherwise the *Set is nil. An empty or nil map
// prepares an empty set. Prepare reads templates without modifying it and
// keeps only its strings: changing the map afterwards does not change the
// set. loader is kept to resolve targets outside the set at render time and
// must be safe for concurrent use if the set is rendered concurrently.
func Prepare(templates map[string]string, loader Loader) (*Set, []Diagnostic, error) {
	return engine.Prepare(templates, loader)
}
