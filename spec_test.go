package toupee_test

import (
	toupee "github.com/devselfio/toupee.go"

	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// specDir holds Toupee v0.4.0's corpus; see testdata/spec/ORIGIN.md.
const specDir = "testdata/spec"

// diagnosticKinds maps each symbolic error kind to the snake_case spelling
// of an error-kind directive. It is the runner's, not a public API.
var diagnosticKinds = map[toupee.ErrorKind]string{
	toupee.MalformedElement:        "malformed_element",
	toupee.UndefinedVariable:       "undefined_variable",
	toupee.TemplateNotFound:        "template_not_found",
	toupee.DuplicateSlotDefinition: "duplicate_slot",
	toupee.CircularReference:       "circular_reference",
}

// preparedCase is a case ready to render: its identity, its fixture and the
// Context and Loader decoded from it.
type preparedCase struct {
	id     caseID
	spec   specCase
	ctx    toupee.Context
	loader toupee.Loader
}

// renderFunc is Render's signature, so tests can inject a renderer.
type renderFunc func(string, toupee.Context, toupee.Loader) (string, error)

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
func checkCase(c preparedCase) error { return checkCaseWith(c, toupee.Render) }

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
	want := toupee.ErrorKind(*s.errorName)
	if err == nil {
		return fail("expected error %s but got success with output %q", want, out)
	}
	var re *toupee.RenderError
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

// setEntry is the name under which TestSpecThroughSet puts a case's input.
const setEntry = "\x00entry"

// TestSpecThroughSet renders every corpus case from a prepared set of the
// case's named templates and its input. A case the set's validation refuses
// must expect an error of the first diagnostic's kind; every other case must
// meet its expectation through Set.Render. The counts pin how many cases
// take each path at the corpus's pinned tag.
func TestSpecThroughSet(t *testing.T) {
	clean := 0
	refused := map[toupee.ErrorKind]int{}
	for _, f := range loadCorpus(t) {
		for _, c := range f.cases {
			templates := map[string]string{}
			for _, tmpl := range c.spec.templates {
				templates[tmpl.name] = tmpl.source
			}
			if _, ok := templates[setEntry]; ok {
				t.Fatalf("%s %q names a template %q", c.id, c.spec.name, setEntry)
			}
			templates[setEntry] = *c.spec.input
			set, diags, err := toupee.Prepare(templates, nil)
			if err != nil {
				t.Fatalf("%s %q: Prepare: %v", c.id, c.spec.name, err)
			}
			if len(diags) > 0 {
				refused[diags[0].Kind]++
				if c.spec.errorName == nil || toupee.ErrorKind(*c.spec.errorName) != diags[0].Kind {
					t.Errorf("%s %q: refused with %v, but the case expects %v", c.id, c.spec.name, diags, c.spec.errorName)
				}
				continue
			}
			clean++
			if err := checkCaseWith(c, func(string, toupee.Context, toupee.Loader) (string, error) {
				return set.Render(setEntry, c.ctx)
			}); err != nil {
				t.Error(err)
			}
		}
	}
	want := map[toupee.ErrorKind]int{toupee.MalformedElement: 22, toupee.TemplateNotFound: 6, toupee.DuplicateSlotDefinition: 2}
	if clean != 489 || !maps.Equal(refused, want) {
		t.Errorf("prepared clean %d, refused %v; want 489, %v", clean, refused, want)
	}
}

// controlCoverage is the inventory for "toupee.go renders conditionals,
// loops and let": every case of its four files except the deferred ones
// (none now), and the cases elsewhere that use control flow with only
// implemented elements. The cases of the four files that need composition
// (#13) or a transform on t-let (#14) were deferred until those landed;
// both have, so every case of the four files is checked here. See
// .notes/toupee-v0.4.0-control-flow.md in the planning workspace.
var controlCoverage = struct {
	files        []string
	deferred     []caseID
	supplemental []caseID
}{
	files:    []string{"conditional.test", "let.test", "loop.test", "types-compare.test"},
	deferred: []caseID{},
	supplemental: []caseID{
		{"comment.test", 13}, {"comment.test", 31},
		{"dynamic-path.test", 57}, {"dynamic-path.test", 65}, {"dynamic-path.test", 73},
		{"edge.test", 71}, {"edge.test", 119}, {"edge.test", 176}, {"edge.test", 201}, {"edge.test", 209},
		{"error.test", 6}, {"error.test", 11}, {"error.test", 16}, {"error.test", 21},
		{"error.test", 26}, {"error.test", 102}, {"error.test", 107},
		{"integration.test", 56},
		{"nesting.test", 1}, {"nesting.test", 9}, {"nesting.test", 17}, {"nesting.test", 56},
		{"nesting.test", 64}, {"nesting.test", 72}, {"nesting.test", 88}, {"nesting.test", 127},
		{"types-extremes.test", 113}, {"types-extremes.test", 121},
		{"types-extremes.test", 129}, {"types-extremes.test", 217},
	},
}

// TestControlCorpusCoverage checks that every control-flow case the
// inventory names passes and is not in the known-failures list, and that the
// deferred cases stay listed. It is a coverage check; TestSpec stays the
// authority on the whole corpus.
func TestControlCorpusCoverage(t *testing.T) {
	files := loadCorpus(t)
	known := loadKnownFailures(t)
	byID := map[caseID]preparedCase{}
	for _, f := range files {
		for _, c := range f.cases {
			byID[c.id] = c
		}
	}
	deferred := map[caseID]bool{}
	for _, id := range controlCoverage.deferred {
		if _, ok := byID[id]; !ok {
			t.Fatalf("deferred %s is not a corpus case", id)
		}
		deferred[id] = true
		if !known[id] {
			t.Errorf("deferred %s is not in %s; it needs an element that has not landed", id, knownFailuresPath)
		}
	}
	candidates := map[caseID]bool{}
	for _, f := range files {
		if slices.Contains(controlCoverage.files, f.name) {
			for _, c := range f.cases {
				if !deferred[c.id] {
					candidates[c.id] = true
				}
			}
		}
	}
	for _, id := range controlCoverage.supplemental {
		if _, ok := byID[id]; !ok {
			t.Fatalf("supplemental %s is not a corpus case", id)
		}
		candidates[id] = true
	}
	if len(candidates) != 175 {
		t.Fatalf("inventory has %d candidates, want 175 (145 from the four files, 30 elsewhere)", len(candidates))
	}
	for id := range candidates {
		if known[id] {
			t.Errorf("%s is still in %s", id, knownFailuresPath)
		}
		if err := checkCase(byID[id]); err != nil {
			t.Error(err)
		}
	}
}
