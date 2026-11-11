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
// # Layout
//
// This file, toupee.go, is the root package's only source file and holds
// its entry points. The implementation (the parser and its node types,
// control flow, composition, lookup, indentation, HTML escaping and the
// transforms) lives in internal/engine. The root's types are aliases of
// that package's and its constants re-export that package's, so values and
// errors pass between the two unchanged; callers import only this package.
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
