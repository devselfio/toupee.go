package toupee_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	toupee "github.com/devselfio/toupee.go"
)

// Compile-time checks that a root-only caller keeps the whole contract.
var (
	_ func(string, toupee.Context, toupee.Loader) (string, error) = toupee.Render
	_ toupee.Loader                                               = func(name string) (string, error) { return "", nil }
	_ error                                                       = (*toupee.RenderError)(nil)
	_                                                             = toupee.Context{
		Data:  map[string]toupee.Value{},
		Attrs: map[string]string{},
		Slots: map[string]string{},
	}
	_ = toupee.Value{
		Kind:    toupee.KindMap,
		String:  "",
		Boolean: false,
		Integer: int64(0),
		Float:   float64(0),
		List:    []toupee.Value{},
		Map:     map[string]toupee.Value{},
	}
	_                                                                     = toupee.RenderError{Line: 0, Column: 0, Kind: toupee.MalformedElement, Message: ""}
	_ func(map[string]string, toupee.Loader) ([]toupee.Diagnostic, error) = toupee.Validate
	_                                                                     = toupee.Diagnostic{Template: "", Line: 0, Column: 0, Kind: toupee.TemplateNotFound, Message: ""}
)

// TestPublicAPI checks the root's exported top-level names, the kinds of
// ValueKind and ErrorKind, and the constants' values.
func TestPublicAPI(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					names = append(names, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							names = append(names, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								names = append(names, n.Name)
							}
						}
					}
				}
			}
		}
	}
	slices.Sort(names)
	want := []string{
		"CircularReference", "Context", "Diagnostic", "DuplicateSlotDefinition",
		"ErrorKind", "KindBoolean", "KindFloat", "KindInteger", "KindList",
		"KindMap", "KindNil", "KindString", "Loader", "MalformedElement",
		"Render", "RenderError", "TemplateNotFound", "UndefinedVariable",
		"Validate", "Value", "ValueKind",
	}
	if !slices.Equal(names, want) {
		t.Errorf("root exports = %v\nwant %v", names, want)
	}

	if k := reflect.TypeFor[toupee.ValueKind]().Kind(); k != reflect.Uint8 {
		t.Errorf("ValueKind underlying kind = %v, want uint8", k)
	}
	kinds := []toupee.ValueKind{
		toupee.KindNil, toupee.KindString, toupee.KindBoolean, toupee.KindInteger,
		toupee.KindFloat, toupee.KindList, toupee.KindMap,
	}
	for i, k := range kinds {
		if int(k) != i {
			t.Errorf("value kind %d = %d", i, k)
		}
	}
	if (toupee.Value{}).Kind != toupee.KindNil {
		t.Error("zero Value is not KindNil")
	}

	if k := reflect.TypeFor[toupee.ErrorKind]().Kind(); k != reflect.String {
		t.Errorf("ErrorKind underlying kind = %v, want string", k)
	}
	errKinds := map[toupee.ErrorKind]string{
		toupee.MalformedElement:        "MalformedElement",
		toupee.UndefinedVariable:       "UndefinedVariable",
		toupee.TemplateNotFound:        "TemplateNotFound",
		toupee.DuplicateSlotDefinition: "DuplicateSlotDefinition",
		toupee.CircularReference:       "CircularReference",
	}
	if len(errKinds) != 5 {
		t.Errorf("error kinds are not distinct: %v", errKinds)
	}
	for k, s := range errKinds {
		if string(k) != s {
			t.Errorf("error kind %q, want %q", k, s)
		}
	}

	// An engine-produced error is a root *RenderError.
	_, err = toupee.Render(`<t-var name="missing" />`, toupee.Context{}, nil)
	var re *toupee.RenderError
	if !errors.As(err, &re) || re.Kind != toupee.UndefinedVariable || re.Message != "missing" {
		t.Errorf("Render error = %#v, want UndefinedVariable missing", err)
	}
}
