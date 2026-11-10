package toupee

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// specDir holds Toupee v0.4.0's corpus; see testdata/spec/ORIGIN.md.
const specDir = "testdata/spec"

// diagnosticKinds maps each symbolic error kind to the snake_case spelling
// of an error-kind directive. It is the runner's, not a public API.
var diagnosticKinds = map[ErrorKind]string{
	MalformedElement:        "malformed_element",
	UndefinedVariable:       "undefined_variable",
	TemplateNotFound:        "template_not_found",
	DuplicateSlotDefinition: "duplicate_slot",
	CircularReference:       "circular_reference",
}

// preparedCase is a case ready to render: its identity, its fixture and the
// Context and Loader decoded from it.
type preparedCase struct {
	id     caseID
	spec   specCase
	ctx    Context
	loader Loader
}

// renderFunc is Render's signature, so tests can inject a renderer.
type renderFunc func(string, Context, Loader) (string, error)

// specFile is one parsed and prepared corpus file.
type specFile struct {
	name  string // base name, e.g. types-compare.test
	cases []preparedCase
}

// prepareCase decodes a case's context. An error is a harness error, never a
// rendering verdict.
func prepareCase(c specCase) (preparedCase, error) {
	ctx, loader, err := decodeSpecContext(c.contextJSON, c.templates)
	if err != nil {
		return preparedCase{}, fmt.Errorf("L%d %q: %w", c.line, c.name, err)
	}
	return preparedCase{spec: c, ctx: ctx, loader: loader}, nil
}

// checkCase renders c with Render and returns nil when the result is what
// the case expects.
func checkCase(c preparedCase) error { return checkCaseWith(c, Render) }

// checkCaseWith renders c once with render. An output case needs a nil
// error and byte-equal output. An error case needs a *RenderError of the
// named kind, and each diagnostic the case gives must equal the error's.
func checkCaseWith(c preparedCase, render renderFunc) error {
	s := c.spec
	out, err := render(*s.input, c.ctx, c.loader)
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%s %q: %s", c.id, s.name, fmt.Sprintf(format, args...))
	}
	if s.errorName == nil {
		if err != nil {
			return fail("unexpected error: %v", err)
		}
		if out != *s.output {
			return fail("output differs\n--- want (%d bytes)\n%q\n--- got (%d bytes)\n%q", len(*s.output), *s.output, len(out), out)
		}
		return nil
	}
	want := ErrorKind(*s.errorName)
	if err == nil {
		return fail("expected error %s but got success with output %q", want, out)
	}
	var re *RenderError
	if !errors.As(err, &re) {
		return fail("expected a *RenderError of kind %s, got %T: %v", want, err, err)
	}
	if re.Kind != want {
		return fail("expected error %s, got %s (%v)", want, re.Kind, re)
	}
	if s.errorLine != nil && re.Line != *s.errorLine {
		return fail("expected error-line %d, got %d", *s.errorLine, re.Line)
	}
	if s.errorColumn != nil && re.Column != *s.errorColumn {
		return fail("expected error-column %d, got %d", *s.errorColumn, re.Column)
	}
	if s.errorKind != nil {
		got, ok := diagnosticKinds[re.Kind]
		if !ok {
			got = string(re.Kind)
		}
		if got != *s.errorKind {
			return fail("expected error-kind %s, got %s", *s.errorKind, got)
		}
	}
	if s.errorMessage != nil && re.Message != *s.errorMessage {
		return fail("expected error-message %q, got %q", *s.errorMessage, re.Message)
	}
	return nil
}

// loadCorpus parses and prepares every direct *.test file in specDir, in
// name order. Any I/O, format or context error is fatal: it is a harness or
// fixture problem, not a rendering failure the known-failures list may hold.
func loadCorpus(t *testing.T) []specFile {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(specDir, "*.test"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no *.test files in %s", specDir)
	}
	sort.Strings(paths)
	var files []specFile
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		cases, err := parseSpecFile(string(content))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		f := specFile{name: name}
		for _, c := range cases {
			p, err := prepareCase(c)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			p.id = caseID{name, c.line}
			f.cases = append(f.cases, p)
		}
		files = append(files, f)
	}
	return files
}

// corpusIDs is the set of every case in files.
func corpusIDs(files []specFile) map[caseID]bool {
	ids := map[caseID]bool{}
	for _, f := range files {
		for _, c := range f.cases {
			ids[c.id] = true
		}
	}
	return ids
}

// TestSpec renders every corpus case through Render and holds the result to
// the known-failures list: a listed case must fail and an unlisted case must
// pass. Subtests are TestSpec/<file without .test>/L<test-marker line>.
func TestSpec(t *testing.T) {
	files := loadCorpus(t)
	corpus := corpusIDs(files)
	known := loadKnownFailures(t)
	if stale := staleEntries(known, corpus); len(stale) > 0 {
		t.Fatalf("%s names cases that are not in the corpus: %v", knownFailuresPath, stale)
	}

	ran, pass := 0, 0
	for _, f := range files {
		t.Run(strings.TrimSuffix(f.name, ".test"), func(t *testing.T) {
			for _, c := range f.cases {
				t.Run(fmt.Sprintf("L%d", c.id.Line), func(t *testing.T) {
					ran++
					var err error
					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Fatalf("%s %q: Render panicked: %v", c.id, c.spec.name, r)
							}
						}()
						err = checkCase(c)
					}()
					if err == nil {
						pass++
					}
					switch classify(known[c.id], err) {
					case knownFailure:
						t.Skipf("known failure (%s): %v", knownFailuresPath, err)
					case unexpectedPass:
						t.Errorf("%s %q now passes: delete its entry from %s", c.id, c.spec.name, knownFailuresPath)
					case unexpectedFailure:
						t.Errorf("%v\n%s is not in %s: this is a regression to fix, not an entry to add", err, c.id, knownFailuresPath)
					}
				})
			}
		})
	}
	if ran == len(corpus) {
		t.Logf("corpus: %d of %d cases pass", pass, len(corpus))
	} else {
		t.Logf("corpus (filtered): %d of %d selected cases pass; %d cases in the corpus", pass, ran, len(corpus))
	}
}
