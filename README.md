# toupee.go

A Go port of [Toupee](https://github.com/devselfio/toupee), the HTML
template engine, measured case by case against Toupee's own spec corpus.

    import "github.com/devselfio/toupee.go" // package toupee

**Status: bootstrap.** The public render boundary is fixed and the
measurement is in place, but `Render` does not implement any template
semantics yet: it returns `"", nil` and never calls the loader. Rendering
arrives in later changes, each measured by the corpus below.

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
out, err := toupee.Render(`<t-include template="card" />`, ctx, loader)
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

    corpus: 14 of 519 cases pass

That is the honest line for the bootstrap stub; 505 cases are on the list.
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
