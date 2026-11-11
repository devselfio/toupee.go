package toupee_test

import (
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestRootLayout enforces ADR 0018: the root package holds one production
// file, toupee.go, and the implementation lives in internal/engine. Every
// non-test .go file counts, whatever its build constraints.
func TestRootLayout(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}
	if !slices.Equal(files, []string{"toupee.go"}) {
		t.Fatalf("root production files = %v, want [toupee.go]", files)
	}

	f, err := parser.ParseFile(token.NewFileSet(), "toupee.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	var imports []string
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		imports = append(imports, path)
	}
	want := []string{"github.com/devselfio/toupee.go/internal/engine"}
	if !slices.Equal(imports, want) {
		t.Fatalf("toupee.go imports %v, want %v", imports, want)
	}
}
