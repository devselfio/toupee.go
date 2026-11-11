package toupee_test

import (
	toupee "github.com/devselfio/toupee.go"

	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// specTemplate is a named template from an <!-- in:NAME --> section.
type specTemplate struct{ name, source string }

// specCase is one corpus case as Toupee's src/test_runner.zig reads it. A
// nil pointer is an absent marker; a pointer to "" is a present empty
// section.
type specCase struct {
	line         int // 1-based line of the <!-- test: NAME --> marker
	name         string
	input        *string
	templates    []specTemplate
	contextJSON  string
	output       *string
	errorName    *string
	errorLine    *int
	errorColumn  *int
	errorKind    *string
	errorMessage *string
}

type specSection int

const (
	sectionNone specSection = iota
	sectionNamed
	sectionIn
	sectionContext
	sectionOut
)

// markerValue returns the text after prefix up to " -->", or to the end of
// the trimmed line when there is no " -->", as test_runner.zig does.
func markerValue(trimmed, prefix string) string {
	rest := trimmed[len(prefix):]
	if i := strings.Index(rest, " -->"); i >= 0 {
		return rest[:i]
	}
	return rest
}

// trimBody drops exactly one leading LF and all trailing LF, CR, space and
// tab bytes, as test_runner.zig's trimEnds does for in, in:NAME and out.
func trimBody(s string) string {
	s = strings.TrimPrefix(s, "\n")
	return strings.TrimRight(s, "\n\r \t")
}

// parseSpecFile reads a .test file in the format test_runner.zig reads.
// Marker lines are recognised after trimming spaces, tabs and CR; bodies
// keep their bytes, including CRLF. Unlike the Zig runner, a present but
// empty section is kept as "", so an out marker at EOF expects "" and the
// case is still rendered. A case that cannot be checked is an error naming
// its line, never a pass.
func parseSpecFile(content string) ([]specCase, error) {
	var cases []specCase
	var cur *specCase
	section := sectionNone
	start := 0
	templateName := ""

	flush := func(end int) {
		if cur == nil || section == sectionNone {
			return
		}
		body := ""
		if start < end {
			body = content[start:end]
		}
		switch section {
		case sectionNamed:
			cur.templates = append(cur.templates, specTemplate{templateName, trimBody(body)})
		case sectionIn:
			s := trimBody(body)
			cur.input = &s
		case sectionContext:
			cur.contextJSON = strings.Trim(body, " \t\r\n")
		case sectionOut:
			s := trimBody(body)
			cur.output = &s
		}
	}
	var badDirective error
	number := func(trimmed, prefix string) *int {
		v := markerValue(trimmed, prefix)
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || strings.HasPrefix(v, "+") {
			if badDirective == nil {
				badDirective = fmt.Errorf("L%d %q: invalid %s %q", cur.line, cur.name, strings.TrimSuffix(prefix[len("<!-- "):], ": "), v)
			}
			return nil
		}
		return &n
	}

	lineNo := 0
	for i := 0; i < len(content); {
		lineNo++
		lineStart := i
		lineEnd := len(content)
		if j := strings.IndexByte(content[i:], '\n'); j >= 0 {
			lineEnd = i + j
		}
		trimmed := strings.Trim(content[lineStart:lineEnd], " \t\r")
		next := lineEnd + 1 // the next line's start; past the end at EOF

		switch {
		case strings.HasPrefix(trimmed, "<!-- test: "):
			flush(lineStart)
			cases = append(cases, specCase{line: lineNo, name: markerValue(trimmed, "<!-- test: ")})
			cur = &cases[len(cases)-1]
			section = sectionNone
		case strings.HasPrefix(trimmed, "<!-- in:"):
			flush(lineStart)
			templateName = strings.Trim(markerValue(trimmed, "<!-- in:"), " \t")
			section, start = sectionNamed, next
		case trimmed == "<!-- in -->":
			flush(lineStart)
			section, start = sectionIn, next
		case trimmed == "<!-- context -->":
			flush(lineStart)
			section, start = sectionContext, next
		case trimmed == "<!-- out -->":
			flush(lineStart)
			section, start = sectionOut, next
		case strings.HasPrefix(trimmed, "<!-- error: "):
			if cur != nil {
				flush(lineStart)
				v := markerValue(trimmed, "<!-- error: ")
				cur.errorName = &v
			}
			section = sectionNone
		case cur != nil && strings.HasPrefix(trimmed, "<!-- error-line: "):
			cur.errorLine = number(trimmed, "<!-- error-line: ")
		case cur != nil && strings.HasPrefix(trimmed, "<!-- error-column: "):
			cur.errorColumn = number(trimmed, "<!-- error-column: ")
		case cur != nil && strings.HasPrefix(trimmed, "<!-- error-kind: "):
			v := markerValue(trimmed, "<!-- error-kind: ")
			cur.errorKind = &v
		case cur != nil && strings.HasPrefix(trimmed, "<!-- error-message: "):
			v := markerValue(trimmed, "<!-- error-message: ")
			cur.errorMessage = &v
		}
		i = next
	}
	flush(len(content))
	if badDirective != nil {
		return nil, badDirective
	}

	for _, c := range cases {
		if err := validateSpecCase(c); err != nil {
			return nil, fmt.Errorf("L%d %q: %v", c.line, c.name, err)
		}
	}
	return cases, nil
}

// validateSpecCase rejects a case the runner cannot check.
func validateSpecCase(c specCase) error {
	hasDetail := c.errorLine != nil || c.errorColumn != nil || c.errorKind != nil || c.errorMessage != nil
	switch {
	case c.input == nil:
		return errors.New("no <!-- in --> section")
	case c.output == nil && c.errorName == nil:
		return errors.New("neither <!-- out --> nor <!-- error: KIND -->")
	case c.errorName == nil && hasDetail:
		return errors.New("error-line/-column/-kind/-message without <!-- error: KIND -->")
	}
	if c.errorName != nil {
		if _, ok := diagnosticKinds[toupee.ErrorKind(*c.errorName)]; !ok {
			return fmt.Errorf("unknown error kind %q", *c.errorName)
		}
	}
	if c.errorKind != nil {
		known := false
		for _, d := range diagnosticKinds {
			known = known || d == *c.errorKind
		}
		if !known {
			return fmt.Errorf("unknown error-kind %q", *c.errorKind)
		}
	}
	return nil
}

func strp(s string) *string { return &s }
func intp(n int) *int       { return &n }

func TestParseSpecFile(t *testing.T) {
	content := "# Prose before any case is ignored\n" +
		"<!-- a comment that is not a marker -->\n" +
		"<!-- test: plain -->\n" + // line 3
		"<!-- in -->\n" +
		"  <p>indented</p>\n" +
		"\n" +
		"<!-- not a marker: kept in input -->\n" +
		"  \t\n" +
		"<!-- context -->\n" +
		"\n" +
		"  {\"data\": {\"x\": 1}}  \n" +
		"\n" +
		"<!-- out -->\n" +
		"\n" +
		"\n" +
		"  <p>indented</p>\r\n" +
		"line two \t\n" +
		"\n" +
		"  <!-- test: indented marker with CRLF -->  \r\n" + // line 19
		"\t<!-- in -->\r\n" +
		"a\r\nb\r\n" +
		"<!-- in:card.html -->\r\n" +
		"<div>card</div>\r\n" +
		"<!-- in: \tspaced.html  -->\n" +
		"spaced\n" +
		"<!-- context -->\n" +
		"<!-- out -->\n" +
		"a\r\nb\r\n" +
		"<!-- test: errors -->\n" + // line 31
		"<!-- in -->\n" +
		"{{ x }}\n" +
		"<!-- error: UndefinedVariable -->\n" +
		"this prose is ignored, not a section\n" +
		"<!-- error-line: 2 -->\n" +
		"<!-- error-column: 1 -->\n" +
		"<!-- error-kind: undefined_variable -->\n" +
		"<!-- error-message: x is  undefined -->\n" +
		"<!-- test: empty sections -->\n" + // line 40
		"<!-- in -->\n" +
		"<!-- in:blank.html -->\n" +
		"<!-- context -->\n" +
		"<!-- out -->\n" +
		"<!-- test: out at EOF -->\n" + // line 45
		"<!-- in -->\n" +
		"x\n" +
		"<!-- out -->" // no final newline

	got, err := parseSpecFile(content)
	if err != nil {
		t.Fatal(err)
	}
	want := []specCase{
		{
			line:        3,
			name:        "plain",
			input:       strp("  <p>indented</p>\n\n<!-- not a marker: kept in input -->"),
			contextJSON: `{"data": {"x": 1}}`,
			output:      strp("\n  <p>indented</p>\r\nline two"),
		},
		{
			line:  19,
			name:  "indented marker with CRLF",
			input: strp("a\r\nb"),
			templates: []specTemplate{
				{name: "card.html", source: "<div>card</div>"},
				{name: "spaced.html", source: "spaced"},
			},
			output: strp("a\r\nb"),
		},
		{
			line:         31,
			name:         "errors",
			input:        strp("{{ x }}"),
			errorName:    strp("UndefinedVariable"),
			errorLine:    intp(2),
			errorColumn:  intp(1),
			errorKind:    strp("undefined_variable"),
			errorMessage: strp("x is  undefined"),
		},
		{
			line:      40,
			name:      "empty sections",
			input:     strp(""),
			templates: []specTemplate{{name: "blank.html", source: ""}},
			output:    strp(""),
		},
		{
			line:   45,
			name:   "out at EOF",
			input:  strp("x"),
			output: strp(""),
		},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d cases, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("case %d:\n got %s\nwant %s", i, dumpCase(got[i]), dumpCase(want[i]))
		}
	}
}

func dumpCase(c specCase) string {
	s := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%q", *p)
	}
	n := func(p *int) string {
		if p == nil {
			return "<nil>"
		}
		return fmt.Sprint(*p)
	}
	return fmt.Sprintf("{L%d %q in=%s ctx=%q out=%s err=%s line=%s col=%s kind=%s msg=%s templates=%+v}",
		c.line, c.name, s(c.input), c.contextJSON, s(c.output), s(c.errorName),
		n(c.errorLine), n(c.errorColumn), s(c.errorKind), s(c.errorMessage), c.templates)
}

func fmtWrap(err error) error { return fmt.Errorf("wrapped: %w", err) }

// The two-line form the docs describe is prose to the runner, not an error
// directive; a case using it has neither output nor error and is rejected.
func TestParseSpecFileDocsErrorForm(t *testing.T) {
	_, err := parseSpecFile("<!-- test: docs form -->\n<!-- in -->\nx\n<!-- error -->\nMalformedElement\n")
	if err == nil {
		t.Fatal("the two-line <!-- error --> form was read as an error directive")
	}
	if !strings.Contains(err.Error(), "L1") {
		t.Errorf("error %q does not name the case's line", err)
	}
}

func TestParseSpecFileRejectsMalformed(t *testing.T) {
	tests := []struct {
		name, content, wantErr string
	}{
		{"no input", "<!-- test: a -->\n<!-- out -->\nx\n", "L1"},
		{"no output or error", "<!-- test: a -->\n<!-- in -->\nx\n", "L1"},
		{"bad error-line", "<!-- test: a -->\n<!-- in -->\nx\n<!-- error: MalformedElement -->\n<!-- error-line: two -->\n", "error-line"},
		{"negative error-column", "<!-- test: a -->\n<!-- in -->\nx\n<!-- error: MalformedElement -->\n<!-- error-column: -1 -->\n", "error-column"},
		{"unknown symbolic kind", "<!-- test: a -->\n<!-- in -->\nx\n<!-- error: Oops -->\n", "Oops"},
		{"unknown diagnostic kind", "<!-- test: a -->\n<!-- in -->\nx\n<!-- error: MalformedElement -->\n<!-- error-kind: oops -->\n", "oops"},
		{"detail without error", "<!-- test: a -->\n<!-- in -->\nx\n<!-- out -->\nx\n<!-- error-line: 1 -->\n", "L1"},
		{"second case reported", "<!-- test: a -->\n<!-- in -->\nx\n<!-- out -->\nx\n<!-- test: b -->\n<!-- in -->\n", "L6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSpecFile(tt.content)
			if err == nil {
				t.Fatal("malformed fixture accepted")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestSpecCorpusShape pins the dimensions of the v0.4.0 corpus. A pin update
// must update these deliberately.
func TestSpecCorpusShape(t *testing.T) {
	want := map[string]int{
		"bind.test": 11, "comment.test": 8, "conditional.test": 67, "dynamic-path.test": 18,
		"edge.test": 28, "error.test": 26, "extend.test": 11, "include.test": 18,
		"integration.test": 15, "isolation.test": 6, "let.test": 11, "loop.test": 28,
		"nesting.test": 16, "raw.test": 13, "regression.test": 4, "slot.test": 10,
		"transform.test": 43, "types-compare.test": 39, "types-extremes.test": 28,
		"types-render.test": 18, "types-transform.test": 78, "var.test": 23,
	}
	files := loadCorpus(t)
	got := map[string]int{}
	total, outputs, errs, empty := 0, 0, 0, 0
	for _, f := range files {
		got[f.name] = len(f.cases)
		for _, c := range f.cases {
			total++
			switch {
			case c.spec.errorName != nil:
				errs++
			case c.spec.output != nil:
				outputs++
				if *c.spec.output == "" {
					empty++
				}
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cases per file:\n got %v\nwant %v", got, want)
	}
	if total != 519 || outputs != 479 || errs != 40 || empty != 14 {
		t.Errorf("got %d cases (%d output, %d error, %d empty output), want 519 (479, 40, 14)", total, outputs, errs, empty)
	}
}

// slot.test L97 ends with an out marker at EOF: its expected output is
// present and empty, so it is rendered, not skipped as Toupee's runner does.
func TestSpecEmptyOutputAtEOF(t *testing.T) {
	for _, f := range loadCorpus(t) {
		for _, c := range f.cases {
			if c.id == (caseID{"slot.test", 97}) {
				if c.spec.output == nil || *c.spec.output != "" {
					t.Fatalf("slot.test L97 output = %v, want a present empty string", c.spec.output)
				}
				return
			}
		}
	}
	t.Fatal("slot.test L97 not found")
}

// TestSpecInvocationCoverage proves every corpus case reaches the renderer
// once, independently of TestSpec's own counter.
func TestSpecInvocationCoverage(t *testing.T) {
	calls := map[caseID]int{}
	total := 0
	for _, f := range loadCorpus(t) {
		for _, c := range f.cases {
			total++
			render := func(input string, ctx toupee.Context, loader toupee.Loader) (string, error) {
				calls[c.id]++
				return "", nil
			}
			_ = checkCaseWith(c, render)
		}
	}
	if total != 519 || len(calls) != 519 {
		t.Fatalf("rendered %d distinct cases of %d, want 519", len(calls), total)
	}
	for id, n := range calls {
		if n != 1 {
			t.Errorf("%s rendered %d times, want 1", id, n)
		}
	}
	if calls[caseID{"slot.test", 97}] != 1 {
		t.Error("slot.test L97 (out marker at EOF) was not rendered")
	}
}

func TestCheckCase(t *testing.T) {
	outCase := preparedCase{
		id:   caseID{"x.test", 1},
		spec: specCase{line: 1, name: "out", input: strp("in"), output: strp("want")},
	}
	errCase := func(mod func(*specCase)) preparedCase {
		s := specCase{line: 5, name: "err", input: strp("in"), errorName: strp("UndefinedVariable")}
		if mod != nil {
			mod(&s)
		}
		return preparedCase{id: caseID{"x.test", 5}, spec: s}
	}
	full := func(s *specCase) {
		s.errorLine, s.errorColumn = intp(2), intp(3)
		s.errorKind, s.errorMessage = strp("undefined_variable"), strp("titl")
	}
	ret := func(out string, err error) renderFunc {
		return func(string, toupee.Context, toupee.Loader) (string, error) { return out, err }
	}
	good := &toupee.RenderError{Line: 2, Column: 3, Kind: toupee.UndefinedVariable, Message: "titl"}

	tests := []struct {
		name    string
		c       preparedCase
		render  renderFunc
		wantErr string // "" means pass
	}{
		{"output matches", outCase, ret("want", nil), ""},
		{"output differs", outCase, ret("wan", nil), "want"},
		{"output trailing newline", outCase, ret("want\n", nil), "want"},
		{"output with error", outCase, ret("want", good), "unexpected error"},
		{"error kind only", errCase(nil), ret("", &toupee.RenderError{Kind: toupee.UndefinedVariable}), ""},
		{"error wrapped", errCase(nil), ret("", fmtWrap(good)), ""},
		{"unexpected success", errCase(nil), ret("x", nil), "success"},
		{"not a RenderError", errCase(nil), ret("", errors.New("UndefinedVariable")), "RenderError"},
		{"wrong kind", errCase(nil), ret("", &toupee.RenderError{Kind: toupee.TemplateNotFound}), "TemplateNotFound"},
		{"all details match", errCase(full), ret("", good), ""},
		{"wrong line", errCase(full), ret("", &toupee.RenderError{Line: 1, Column: 3, Kind: toupee.UndefinedVariable, Message: "titl"}), "line"},
		{"wrong column", errCase(full), ret("", &toupee.RenderError{Line: 2, Column: 4, Kind: toupee.UndefinedVariable, Message: "titl"}), "column"},
		{"message contains expected", errCase(full), ret("", &toupee.RenderError{Line: 2, Column: 3, Kind: toupee.UndefinedVariable, Message: "titl is undefined"}), "message"},
		{"message prefix of expected", errCase(full), ret("", &toupee.RenderError{Line: 2, Column: 3, Kind: toupee.UndefinedVariable, Message: "tit"}), "message"},
		{"only line given", errCase(func(s *specCase) { s.errorLine = intp(2) }), ret("", &toupee.RenderError{Line: 2, Kind: toupee.UndefinedVariable, Message: "anything"}), ""},
		{"only line wrong", errCase(func(s *specCase) { s.errorLine = intp(2) }), ret("", &toupee.RenderError{Line: 7, Kind: toupee.UndefinedVariable}), "line"},
		{"only column given", errCase(func(s *specCase) { s.errorColumn = intp(3) }), ret("", &toupee.RenderError{Column: 3, Kind: toupee.UndefinedVariable}), ""},
		{"only column wrong", errCase(func(s *specCase) { s.errorColumn = intp(3) }), ret("", &toupee.RenderError{Column: 0, Kind: toupee.UndefinedVariable}), "column"},
		{"only diagnostic kind given", errCase(func(s *specCase) { s.errorKind = strp("undefined_variable") }), ret("", &toupee.RenderError{Kind: toupee.UndefinedVariable}), ""},
		{
			"diagnostic kind wrong",
			errCase(func(s *specCase) {
				s.errorName, s.errorKind = strp("DuplicateSlotDefinition"), strp("circular_reference")
			}),
			ret("", &toupee.RenderError{Kind: toupee.DuplicateSlotDefinition}), "duplicate_slot",
		},
		{"empty expected message matches", errCase(func(s *specCase) { s.errorMessage = strp("") }), ret("", &toupee.RenderError{Kind: toupee.UndefinedVariable}), ""},
		{"empty expected message differs", errCase(func(s *specCase) { s.errorMessage = strp("") }), ret("", &toupee.RenderError{Kind: toupee.UndefinedVariable, Message: "x"}), "message"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkCaseWith(tt.c, tt.render)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want a pass, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want a failure mentioning %q, got a pass", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("verdict %q does not mention %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.c.id.String()) {
				t.Errorf("verdict %q does not name the case %s", err, tt.c.id)
			}
		})
	}
}

func TestDiagnosticKinds(t *testing.T) {
	want := map[toupee.ErrorKind]string{
		toupee.MalformedElement:        "malformed_element",
		toupee.UndefinedVariable:       "undefined_variable",
		toupee.TemplateNotFound:        "template_not_found",
		toupee.DuplicateSlotDefinition: "duplicate_slot",
		toupee.CircularReference:       "circular_reference",
	}
	if !reflect.DeepEqual(diagnosticKinds, want) {
		t.Errorf("got %v, want %v", diagnosticKinds, want)
	}
}
