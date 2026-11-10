package toupee

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestErrorKinds(t *testing.T) {
	want := map[ErrorKind]string{
		MalformedElement:        "MalformedElement",
		UndefinedVariable:       "UndefinedVariable",
		TemplateNotFound:        "TemplateNotFound",
		DuplicateSlotDefinition: "DuplicateSlotDefinition",
		CircularReference:       "CircularReference",
	}
	if len(want) != 5 {
		t.Fatalf("want five distinct kinds, got %d", len(want))
	}
	for k, s := range want {
		if string(k) != s {
			t.Errorf("kind %q, want the symbolic name %q", k, s)
		}
	}
}

func TestRenderErrorAs(t *testing.T) {
	var err error = &RenderError{Line: 3, Column: 7, Kind: UndefinedVariable, Message: "user.name"}
	wrapped := fmt.Errorf("rendering page: %w", err)
	var re *RenderError
	if !errors.As(wrapped, &re) {
		t.Fatal("errors.As did not find the *RenderError")
	}
	if re.Line != 3 || re.Column != 7 || re.Kind != UndefinedVariable || re.Message != "user.name" {
		t.Errorf("got %+v, want line 3, column 7, UndefinedVariable, %q", *re, "user.name")
	}
}

func TestRenderErrorPreservesMessage(t *testing.T) {
	msg := "  odd\tmessage: <t-var>  "
	re := &RenderError{Kind: MalformedElement, Message: msg}
	if re.Message != msg {
		t.Errorf("Message changed to %q", re.Message)
	}
	if !strings.Contains(re.Error(), msg) {
		t.Errorf("Error() %q does not contain the unformatted message %q", re.Error(), msg)
	}
}

func TestRenderErrorString(t *testing.T) {
	tests := []struct {
		name     string
		err      *RenderError
		contains []string
		absent   []string
	}{
		{
			name:     "with location",
			err:      &RenderError{Line: 2, Column: 5, Kind: CircularReference, Message: "a -> b -> a"},
			contains: []string{"CircularReference", "a -> b -> a", "2", "5"},
		},
		{
			name:     "line only",
			err:      &RenderError{Line: 9, Kind: TemplateNotFound, Message: "card"},
			contains: []string{"TemplateNotFound", "card", "9"},
		},
		{
			name:     "no location",
			err:      &RenderError{Kind: DuplicateSlotDefinition, Message: "body"},
			contains: []string{"DuplicateSlotDefinition", "body"},
			absent:   []string{"0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.err.Error()
			for _, c := range tt.contains {
				if !strings.Contains(s, c) {
					t.Errorf("Error() = %q, missing %q", s, c)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(s, a) {
					t.Errorf("Error() = %q, should not contain %q", s, a)
				}
			}
		})
	}
}
