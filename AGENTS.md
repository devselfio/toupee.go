# toupee.go — agent instructions

toupee.go is a Go port of [Toupee](https://github.com/devselfio/toupee), an
HTML template language, at v0.4.0 (commit `36f77a31`). One entry point:
`Render(input, ctx, loader)`. Module `github.com/devselfio/toupee.go`
(package `toupee`); consumers depend on tagged versions (`vN.N.N`), never a
branch or a `replace`. `README.md` documents the language and API; keep it
true when behaviour changes.

## The contract

- The specification is Toupee v0.4.0's corpus: its 22 `test/*.test` files,
  519 cases, vendored byte for byte under `testdata/spec/` and pinned in
  `testdata/spec/ORIGIN.md`. Never edit a `.test` file: a failing case is
  fixed in the implementation.
- Every case passes. `testdata/spec/known-failures.txt` lists cases allowed
  to fail (today: none) and only shrinks. A listed case that passes, an
  unlisted case that fails, or an entry naming no case fails `TestSpec`.
  Never add a line to hide a regression.
- `Render` returns `""` and a `*RenderError` on failure, never partial
  output. Callers match errors with `errors.As` on `Kind`; the `Error()`
  text is not a contract.
- `<t-debug>` is always stripped, as Toupee renders with debug off; there
  is no debug mode.

## Commands

Use the toolchain `go.mod` declares (`go 1.26`); everything below is
verified under `GOTOOLCHAIN=go1.26.5` (the tests also pass under Go 1.27.1).

- `make help` — list targets (the default goal)
- `make build` — `go build ./...` (writes no binary)
- `make test` — `go test ./...`: unit tests and the corpus
- `make check` — `gofmt -l`, `go vet`, `go mod tidy -diff`; rewrites nothing
- `make spec-count` — the corpus line, today `corpus: 519 of 519 cases pass`
- `make spec-drift` — corpus against `SHA256SUMS`; also against Toupee at
  the pinned tag when the network answers
- `make tidy` — `go mod tidy`; refuses to run on a dirty tree, by design
- One case: `go test -run '^TestSpec/types-compare/L9$' -v .`
  (subtests are `TestSpec/<file>/L<line of the test marker>`)

CI (`.github/workflows/ci.yml`, Go from `go-version-file: go.mod`) runs
`make check`, `build`, `test`, `spec-count`, `spec-drift` on pushes to
`main` and pull requests. Run `make check && make test` before committing.

## Layout

The root package holds exactly one non-test file, `toupee.go`: `Render` and
the public types, as aliases of `internal/engine`'s, with the constants
re-exported. `TestRootLayout` enforces that the root has only that file and
imports only `internal/engine`.

- `internal/engine/` — the implementation and its white-box tests.
- Root `*_test.go` — `package toupee_test`: public API tests and the corpus
  runner, calling only the root.
- `cmd/driftcheck/` — the drift check behind `make spec-drift`.

## Testing

Write a failing test first, then make it pass.

- `TestSpec` reads the format Toupee's `src/test_runner.zig` reads (see
  README, "The method"), not the one Toupee's docs describe. One deliberate
  difference: a case with an empty `out` is rendered and expected to be
  empty, not skipped.
- Behaviour the corpus does not pin gets its own test in the root (public
  API) or in `internal/engine` (white-box). Fixtures live in `testdata/`.

## Syncing the corpus

`testdata/spec/ORIGIN.md` pins Toupee at a `vN.N.N` tag; `SHA256SUMS`
records the files at that tag. To move the pin: replace the whole fixture
set with the new tag's `test/*.test`, set `Ref:` in `ORIGIN.md`, and run
`go run ./cmd/driftcheck -record` (online only; writes `SHA256SUMS` only if
the files match upstream). Then reassess case counts and the
known-failures list. `go run ./cmd/driftcheck <ref>` measures another ref
without moving the pin.

## Code conventions

- Standard library only; `go.mod` has no requirements. A dependency needs a
  decision first, not a line in a diff.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`); never swallow.
- Comments only for non-obvious constraints or trade-offs.
- Smallest correct diff. No dead code. Stop at the scope of the change.
