# toupee.go

A Go port of [Toupee](https://github.com/devselfio/toupee), the HTML
template engine, measured case by case against Toupee's own spec corpus.

    import "github.com/devselfio/toupee.go" // package toupee

**Status: variables, raw output, comments and composition.** `Render`
implements literal text and HTML, `t-var`, `t-raw`, `t-comment`,
`t-include`, `t-extend`, `t-define`, `t-slot`, `t-attr` and attribute
bindings:

- **Paths.** `name="a.b.0"` follows map keys and decimal list indices from
  `Data`; a segment `$(expr)` is replaced by the scalar `expr` resolves to
  from the root, used as one key, and may nest.
- **Values.** A string renders unchanged, a boolean as `true`/`false`, an
  integer in signed decimal and a float in the shortest fixed decimal form
  (no exponent). Nil, lists and maps have no text.
- **Defaults.** A scalar value is used even when it is `""`, `false` or 0.
  Otherwise a block element's body is rendered, even an empty one; a
  self-closing element is an `UndefinedVariable` error whose `Message` is
  the requested path.
- **Escaping.** `t-var` escapes its result, a rendered default included,
  mapping exactly `&`, `<`, `>`, `"` and `'` to `&amp;`, `&lt;`, `&gt;`,
  `&quot;` and `&#x27;`; `t-raw` writes it unchanged. Every other byte of
  the template is kept as it is: there is no HTML parsing or normalising.
- **Comments.** `<t-comment>…</t-comment>` and `<t-comment />` produce
  nothing; their bodies are not parsed.
- **Composition.** `<t-include template="NAME" … />` renders the named
  template, which sees a copy of the caller's data (none with `isolated`),
  overlaid with each `context="PATH[ as KEY], …"` binding that resolves,
  and only the include's own attributes (every attribute but `template`,
  `isolated` and `context`, as literal text) and fills: its `<t-define>`
  bodies and its other, trimmed body content as the unnamed fill. A
  template whose first non-whitespace element is `<t-extend
  template="PARENT">` is only definitions; the chain is followed to its
  first non-extending template, the most derived definition of each slot
  winning. `<t-slot name="N">default</t-slot>` renders its fill, parsed and
  rendered where the slot is, or else its default body; a present empty
  fill still wins. `<t-attr name="N" />` writes an include attribute, and
  `t-var:ATTR="PATH"` and `t-attr:ATTR="NAME"` on an ordinary tag write
  `ATTR="VALUE"`, omitted when there is no value. Included and slot output
  is indented to its insertion point.
- **Errors.** A malformed element is a `MalformedElement` error, an
  undefined variable an `UndefinedVariable` error, each at the line and
  column of the element's `<`, counted in bytes. Errors inside a loaded
  template are located in its source. A template the loader does not know
  is `TemplateNotFound` with the name as message, at the element that named
  it; other loader errors are returned unchanged. A repeated definition is
  `DuplicateSlotDefinition` at its second opener; an extend chain that
  revisits a template, or includes nested more than 50 deep, are
  `CircularReference`. A failing `Render` returns `""` and the error, never
  partial output.

The other elements are not implemented yet. Until each is, a render that
meets control flow (`t-if`, `t-elif`, `t-else`, `t-for`), `t-let`,
`t-debug` or a `transform=` attribute, in the input, a loaded template or a
slot fill, renders as `"", nil`, as the bootstrap did. This is a temporary
measure, not Toupee's semantics.

## API

```go
func Render(input string, ctx Context, loader Loader) (string, error)
```

- **`Value`** is the template datum, with seven forms selected by `Kind`:
  `KindNil` (the zero `Value`), `KindString`, `KindBoolean`, `KindInteger`
  (int64), `KindFloat` (float64), `KindList` (`[]Value`) and `KindMap`
  (`map[string]Value`). Only the payload field `Kind` selects is meaningful.
- **`Context`** groups `Data map[string]Value`, `Attrs map[string]string`
  and `Slots map[string]string`. The zero `Context` is empty; empty keys are
  allowed, and the empty slot name is the unnamed slot.
- **`Loader`** is `func(name string) (string, error)`. For an unknown name
  it returns `("", &RenderError{Kind: TemplateNotFound, Message: name})`;
  any other error is a loader failure. An empty source is a found template.
  A nil `Loader` knows no templates.
- **`*RenderError`** carries `Line` and `Column` (1-based, 0 when unknown),
  `Kind` and `Message` (the unformatted detail). The kinds are
  `MalformedElement`, `UndefinedVariable`, `TemplateNotFound`,
  `DuplicateSlotDefinition` and `CircularReference`. Use `errors.As`; the
  `Error()` text is for people, not a contract.

`Render` reads its arguments without modifying them and keeps no state
between calls.

```go
data := map[string]toupee.Value{
	"title": {Kind: toupee.KindString, String: "Grid"},
	"count": {Kind: toupee.KindInteger, Integer: 3},
	"tags": {Kind: toupee.KindList, List: []toupee.Value{
		{Kind: toupee.KindString, String: "a"},
	}},
	"user": {Kind: toupee.KindMap, Map: map[string]toupee.Value{
		"name": {Kind: toupee.KindString, String: "Ada"},
	}},
}
templates := map[string]string{"card": `<div><t-slot /></div>`}
loader := func(name string) (string, error) {
	src, ok := templates[name]
	if !ok {
		return "", &toupee.RenderError{Kind: toupee.TemplateNotFound, Message: name}
	}
	return src, nil
}
ctx := toupee.Context{
	Data:  data,
	Attrs: map[string]string{"class": "wide"},
	Slots: map[string]string{"": "body"},
}
out, err := toupee.Render(`<p><t-var name="title" />: <t-var name="count" /></p>`, ctx, loader)
// out == "<p>Grid: 3</p>", err == nil
```

The complete, compiled version of this example is `Example` in
`toupee_test.go`.

## The method

The port is measured the way gjot is, copied rather than redesigned.

**A pinned corpus.** `testdata/spec/` holds Toupee's 22 `test/*.test`
files at tag v0.4.0, byte for byte: 519 cases. `ORIGIN.md` pins the
repository and the tag (`Ref: v0.4.0`, always a `vN.N.N` tag) and
`SHA256SUMS` records the files' digests at that tag. Fixtures are never
edited; a failing case is fixed in the package.

**A runner that reads what Toupee's runner reads.** `TestSpec` parses the
format `src/test_runner.zig` reads, not the one Toupee's docs describe:
`<!-- test: NAME -->` opens a case; `<!-- in -->`, `<!-- in:NAME -->`
(a named template the case's loader serves), `<!-- context -->` (JSON with
`data`, `attrs`, `slots`, `templates`) and `<!-- out -->` start sections;
`<!-- error: KIND -->` on one line expects an error, and
`<!-- error-line: N -->`, `<!-- error-column: N -->`,
`<!-- error-kind: snake_case -->` and `<!-- error-message: TEXT -->` must
then equal the error's fields exactly. Marker lines are matched after
trimming spaces, tabs and CR; `in`, `in:NAME` and `out` bodies lose one
leading LF and all trailing whitespace, and output is compared byte for
byte. JSON integers become int64 without passing through float64.

One deliberate difference: Toupee's runner skips a case whose `out` marker
is followed by nothing (`slot.test L97`); this runner renders it and
expects the empty string, so every one of the 519 cases goes through
`Render`.

**A known-failures list that can only shrink.** A case is named
`<file>.test L<line>`, the line of its `<!-- test: -->` marker, for example
`types-compare.test L16`. `testdata/spec/known-failures.txt` lists every
case that fails, sorted by file then line. Each case has one of four
outcomes:

| listed | renders correctly | outcome |
|--------|-------------------|---------|
| no     | yes               | pass |
| yes    | no                | known failure (skipped, not a pass) |
| yes    | yes               | **fails**: delete the entry |
| no     | no                | **fails**: a regression to fix |

An entry naming no corpus case also fails. When a change makes cases pass,
delete their lines in the same change. Never add a line to cover a
regression.

Subtests are `TestSpec/<file without .test>/L<line>`, so
`go test -run '^TestSpec/types-compare/L9$' -v .` runs one case. The list is
still validated against the whole corpus, so a filtered run does not call
real entries stale. An unfiltered run logs the measurement:

    corpus: 183 of 519 cases pass

336 cases are on the list.
A filtered run reports its selected count instead.

**A drift check.** `make spec-drift` runs `cmd/driftcheck`. It checks the
local files against `SHA256SUMS`, which must record the pinned tag, and,
when the network answers, against Toupee's `test/` at that tag. Without a
network (no HTTP response at all) it says so and the record alone decides;
an HTTP error, a failed redirect or a corrupt archive fails.
`go run ./cmd/driftcheck -record` rewrites `SHA256SUMS` only after the
local files match the tag upstream;
`go run ./cmd/driftcheck main` measures another ref without moving the pin.
Exit status: 0 match, 1 difference, 2 unable to check. See
`testdata/spec/ORIGIN.md` for moving the pin.

## Building

    make help        # list the targets
    make check       # gofmt, go vet, go mod tidy -diff
    make build       # go build ./...
    make test        # go test ./...
    make spec-count  # print the corpus line
    make spec-drift  # check the corpus against its record and upstream
    make tidy        # go mod tidy, on a clean worktree only

Go 1.26, standard library only.

### Continuous integration

CI runs on pushes to `main` and on pull requests. It reads the Go version
from `go.mod` and runs these gates in order: `make check`, `make build`,
`make test`, `make spec-count`, then `make spec-drift`. A failed gate
fails the job and stops subsequent gates.

The last gate checks the corpus against its recorded hashes and Toupee
at the tag pinned in `testdata/spec/ORIGIN.md`. If no HTTP response is
received, it reports an offline check and uses the record alone; an
unrecorded corpus edit still fails. An HTTP error, failed redirect or
invalid archive also fails. CI never rewrites the record. Green CI
means the current corpus measurement and known-failures policy pass;
it does not mean every template feature is implemented.
