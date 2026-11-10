package toupee

import "fmt"

// ErrorKind is the symbolic name of a render error, as the corpus's
// "error: KIND" directive spells it.
type ErrorKind string

// The error kinds Toupee v0.4.0's corpus uses.
const (
	MalformedElement        ErrorKind = "MalformedElement"
	UndefinedVariable       ErrorKind = "UndefinedVariable"
	TemplateNotFound        ErrorKind = "TemplateNotFound"
	DuplicateSlotDefinition ErrorKind = "DuplicateSlotDefinition"
	CircularReference       ErrorKind = "CircularReference"
)

// RenderError is a render failure. Line and Column are 1-based when known
// and 0 when not. Message is the unformatted detail, kept exactly. The text
// of Error is for people; programs read the fields.
type RenderError struct {
	Line    int
	Column  int
	Kind    ErrorKind
	Message string
}

func (e *RenderError) Error() string {
	switch {
	case e.Line > 0 && e.Column > 0:
		return fmt.Sprintf("toupee: %d:%d: %s: %s", e.Line, e.Column, e.Kind, e.Message)
	case e.Line > 0:
		return fmt.Sprintf("toupee: line %d: %s: %s", e.Line, e.Kind, e.Message)
	default:
		return fmt.Sprintf("toupee: %s: %s", e.Kind, e.Message)
	}
}
