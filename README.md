# toupee.go

A Go port of [Toupee](https://github.com/devselfio/toupee), the HTML
template engine, measured case by case against Toupee's own spec corpus.

    import "github.com/devselfio/toupee.go" // package toupee

**Status: variables, raw output, comments, conditionals, loops, let,
composition, transforms and a stripped t-debug: every element of Toupee
v0.4.0.** `Render` implements literal text and HTML,
`t-var`, `t-raw`, `t-comment`, `t-if`/`t-elif`/`t-else`, `t-for`,
`t-let`, `t-include`, `t-extend`, `t-define`, `t-slot`, `t-attr`, `t-debug`, attribute
bindings, and the `transform=` attribute on `t-var`, `t-raw` and `t-let`:

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
- **Transforms.** `transform="a|b:arg"` runs a chain of built-ins left to
  right on the chosen value or body, before `t-var` escapes it. Steps split
  on `|` (empty steps skipped) and arguments on `:` (empty ones kept), with
  no trimming or quoting. A `default` step anywhere in the chain stands in
  for a missing value, nil, list or map with `""` instead of raising
  `UndefinedVariable`; a body still comes first. All 25 of Toupee's
  built-ins are implemented, and only those: `upper`, `lower`,
  `capitalize`, `trim`, `slugify`, `truncate`, `replace`, `default`,
  `length`, `escape`, `url_encode`, `url_decode`, `js_escape`, `join`,
  `split`, `first` and `last` with Toupee's byte-wise semantics; `abs`,
  `floor`, `ceil`, `int`, `float` and `decimal`, which read the text as a
  number (`true` is 1, anything unparseable 0; `decimal:N` cuts to N
  places, default 2, without rounding); `bool`, false only for empty,
  `false` or `0` after trimming; and `date:FORMAT`, which formats an ISO
  date with an optional time and offset, normalised to UTC, using
  `%Y %m %d %e %H %M %S %B %b %A %a %p %P %I %F %T %%`, and returns any
  other text unchanged. A colon always separates arguments, so a format
  with a colon is written with `%T`. An unknown name or a bad argument is
  a `MalformedElement` at the element, raised only when the element
  renders.
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
  revisits a template, includes nested more than 50 deep, and a slot fill
  that reaches its own slot again in the same state with no loader call
  in between are `CircularReference`. A failing `Render` returns `""` and
  the error, never partial output. Slot fills have no nesting limit of
  their own: a chain of fills that ends renders however deep it goes. A
  fill that never ends but changes its scope or calls the loader each time
  is not detected, so `Render` gives no termination guarantee for it.
- **Conditionals.** `<t-if var="a.b" equals="x">…<t-elif … />…<t-else />…</t-if>`
  renders the first branch whose condition holds, else the else body.
  The subject is the first of `var`, `attr` and `slot`; with none it is
  the existence of the root. The test is the first of `equals`,
  `not-equals`, `contains`, `starts-with`, `ends-with`, `matches` (a
  whole-string, byte-wise `*`/`?` glob), then the flags `not-exists`,
  `truthy`, `not-truthy`, else existence. A present variable exists even
  when nil, false, zero or empty; it compares by its text, nil, lists and
  maps as `""`. A missing variable is never an error in a condition: it
  fails every positive test and passes `not-exists`, `not-truthy` and
  `not-equals`. Truthy is a nonempty string, list or map, `true` or a
  nonzero number. An attr is truthy when nonempty; a slot condition tests
  presence only. Every branch is parsed, whichever renders.
- **Loops.** `<t-for item in path as loop sort="f" order="desc"
  offset="1" limit="2">…<t-else />…</t-for>` iterates a list, or the list
  under a map's `items` key. It copies the items, sorts them by the
  string at `f` (bytewise; a missing or non-string key sorts as `""`;
  equal keys keep their order; `order="desc"` reverses), skips `offset`,
  then keeps `limit`. The alias holds `index`, `number` and `length` of
  that window as decimal strings, and `first`/`last` as `"true"` only on
  those iterations. A missing or non-iterable collection, or an empty
  window, renders the else body. `limit` and `offset` are read as Zig's
  `parseInt` reads an unsigned decimal (a leading `+`, `_` between
  digits and `-0` are accepted); anything else, or a value over
  2^64-1, is a `MalformedElement`.
- **Let.** `<t-let name="n">…</t-let>` renders its body and stores the
  string under the literal top-level key `n`, emitting nothing and
  replacing any earlier value; a later `t-var` escapes it, `t-raw` does
  not. With `transform=`, the chain runs on the rendered body before it
  is stored, unescaped; a failing step stores nothing.
- **Scope.** A conditional renders in its parent's scope, so a `t-let`
  inside it is visible afterwards. Each loop iteration starts from a fresh
  copy of the parent scope, so its item, alias and lets end with the
  iteration. An include or a slot fill renders in its own copy too, so
  its lets end with it. Bindings never reach the caller's `Context`.

This port has no debug mode. `<t-debug />` and `<t-debug>` are always
stripped, preserving surrounding text. A context dump would require a render
option that this port does not offer. An unclosed debug opening tag is a
`MalformedElement` with the message `unclosed <t-debug> tag`.

## API

```go
func Render(input string, ctx Context, loader Loader) (string, error)
func Validate(templates map[string]string, loader Loader) ([]Diagnostic, error)
func Prepare(templates map[string]string, loader Loader) (*Set, []Diagnostic, error)
func (s *Set) Render(name string, ctx Context) (string, error)
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

- **`Diagnostic`** is one problem `Validate` found: `Template` (the name
  the template was supplied under), `Line` and `Column` (1-based, in that
  template's source, the column counted in bytes), `Kind` and `Message`.
  `Kind` is `MalformedElement` or `DuplicateSlotDefinition` for a syntax
  error, whose `Message` is the parser's own, as `Render` would report it;
  or `TemplateNotFound` for a missing target, whose `Message` is
  `missing include 'NAME'` or `missing extends 'NAME'`.

`Render`, `Validate` and `Prepare` read their arguments without modifying
them; `Render` and `Validate` keep no state between calls. A `*Set` is
immutable and safe for concurrent `Render` calls.

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

### Validation

`Validate` checks a named set of templates before anything renders, with
no page data. `templates` maps each name to its source; `template="…"`
attributes are matched against the names verbatim.

- Every supplied template is parsed. A syntax error is one diagnostic for
  that template (the parser stops at its first), and the template still
  counts as present for references to it.
- Every `<t-include>` and `<t-extend>` target is checked, in every branch
  of `t-if`, both bodies of `t-for`, `t-var` and `t-raw` default bodies,
  `t-let` bodies, `t-slot` defaults, include fills (named and anonymous) and
  extend definitions, whether or not they would render.
- A target resolves if it is a name in the set, or if `loader` returns it
  without error. A `TemplateNotFound` `*RenderError` from the loader, or a
  nil loader, makes it a missing target. The loader is called once for each
  reference outside the set.
- A template reached only through the loader is checked for existence and
  is not parsed: supply every source that must be checked.
- Any other loader error stops validation: `Validate` returns no
  diagnostics and an error wrapping the loader's, with the text
  `toupee: validate TEMPLATE: load TARGET: …`; `errors.Is` and `errors.As`
  reach the loader's error.
- Diagnostics are ordered by template name (bytewise), then line, then
  column. A clean set gives none and a nil error.
- Not checked: extend cycles, include depth, variables, transforms, slot
  names, and a fill whose text begins with `<t-extend`, which is text when
  parsed as a definition body.

```go
diags, err := toupee.Validate(map[string]string{
	"page.html": "<main>\n<t-include template=\"nav.html\" />\n</main>",
}, nil)
// err == nil
for _, d := range diags {
	fmt.Printf("%s:%d:%d: %s\n", d.Template, d.Line, d.Column, d.Message)
}
// page.html:2:1: missing include 'nav.html'
```

The compiled version of this example is `ExampleValidate` in
`validate_test.go`.

### Prepared sets

`Prepare` validates a named set once and keeps its parses, so its templates
render for any number of contexts without being read or parsed again.

- `Prepare` returns the diagnostics and error `Validate` returns for the
  same arguments, in the same order, with the same loader calls. The
  `*Set` is non-nil exactly when the error is nil and there are no
  diagnostics. An empty or nil map prepares to an empty set.
- `Prepare` keeps nothing of the map but its strings: changing or deleting
  its entries afterwards does not change the set.
- `Set.Render(name, ctx)` renders the set's template `name`. A name the set
  lacks is `("", &RenderError{Kind: TemplateNotFound, Message: name})` with
  `Line` and `Column` 0; the loader is not asked for an entry.
- An include or extend target in the set is rendered from its retained
  parse: it is not parsed again and the loader is not called. A target
  outside the set goes to the loader given to `Prepare`, exactly as
  `Render` treats its loader: called once per reference, parsed on each
  render, `TemplateNotFound` located at the naming element, any other
  loader error returned unchanged.
- Output and errors are those `Render` gives for the same templates: kinds,
  messages and positions (located in the source of the template the
  failing element is in), and `""` on failure. One difference: a set
  lookup is not a loader call, so a slot fill that re-enters itself through
  set templates only is a `CircularReference` at the slot, where `Render`
  with a loader does not detect it.
- Not retained: slot fills (a `<t-define>` body or `Context.Slots` text is
  parsed when its slot is met), and templates reached only through the
  loader.
- A `*Set` is never written after `Prepare`. `Set.Render` may be called
  from any number of goroutines at once; each call has its own scope and
  output. A loader given to `Prepare` must itself be safe for concurrent
  use.

```go
set, diags, err := toupee.Prepare(map[string]string{
	"layout.html":   `<main><t-include template="greeting.html" /></main>`,
	"greeting.html": `Hello <t-var name="name" />!`,
}, nil)
// set != nil, diags == nil, err == nil
for _, name := range []string{"Alice", "Bob"} {
	ctx := toupee.Context{Data: map[string]toupee.Value{
		"name": {Kind: toupee.KindString, String: name},
	}}
	out, _ := set.Render("layout.html", ctx)
	fmt.Println(out)
}
// <main>Hello Alice!</main>
// <main>Hello Bob!</main>
```

The compiled version of this example is `ExampleSet` in `set_test.go`.

## Layout

Callers import only the root package. Following gridapp's ADR 0018, it
holds one source file, `toupee.go`, with `Render`, `Validate`, `Prepare`
and the public types; `Context`, `Loader`, `Value`, `ValueKind`,
`RenderError`, `ErrorKind`, `Diagnostic` and `Set` are type aliases, and
their constants re-exports, of the implementation's own, so values and errors pass through unchanged and `errors.As` works as
before. (Reflection reports the defining package as `internal/engine`.)

- `toupee.go`: the entry points and the package documentation.
- `internal/engine/`: the implementation (the parser and its node types,
  control flow, composition, lookup, indentation, HTML escaping and the
  transforms) and its white-box tests.
- Root `*_test.go`: the public API, layout and rendering tests and the
  corpus runner, all in `package toupee_test`, calling only the root.
- `testdata/spec/`: the vendored corpus, its record and the
  known-failures list.
- `cmd/driftcheck/`: the drift check behind `make spec-drift`.

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

    corpus: 519 of 519 cases pass

No cases are on the known-failures list.
A filtered run reports its selected count instead.

Validation is not part of Toupee's corpus. `Validate` is pinned by
`validate_test.go` against the reference's `engine validate` unit tests in
Toupee v0.4.0's `src/root.zig`.

Prepared sets are not part of Toupee's corpus either. `set_test.go` pins
them against the reference's `engine addTemplate and renderTemplate` unit
test, and `TestSpecThroughSet` renders every corpus case from a set of its
named templates and input: 489 render and meet their expectation, and 30
are refused by validation with the error kind the case expects.

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
