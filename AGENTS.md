# toupee.go — agent instructions

toupee.go is a Go port of Toupee, an HTML template language: one entry
point, `Render(input, ctx, loader)`. Module `github.com/devselfio/toupee.go`
(package `toupee`); consumers depend on tagged versions (`vN.N.N`), never a
branch. `README.md` documents the language and API; keep it true when
behaviour changes.

## The contract

- The specification is Toupee v0.4.0's corpus: 22 `.test` files, 519
  cases, vendored byte for byte under `testdata/spec/` and pinned in
  `testdata/spec/ORIGIN.md`. Never edit a `.test` file: a failing case is
  fixed in the implementation.
- `testdata/spec/known-failures.txt` lists cases that may fail (today: none).
  It only shrinks. A listed case that passes, an unlisted case that fails,
  or an entry naming no case fails `TestSpec`. Never add a line to hide a
  regression; delete a line in the same change that makes it pass.
- `Render` returns `""` and a `*RenderError` on failure, never partial
  output. Callers match errors with `errors.As` on `Kind`; the `Error()`
  text is not a contract.
- Moving the pin: see `ORIGIN.md` (`go run ./cmd/driftcheck -record`).

## Commands

Use the toolchain `go.mod` declares (`go 1.26`); tests are verified under
`GOTOOLCHAIN=go1.26.5`. Pin it if your default Go is newer.

- `make help` — list targets
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

The root package holds one non-test file, `toupee.go`: `Render` and the
public types, as aliases of `internal/engine`'s (gridapp's ADR 0018).
`TestRootLayout` enforces that the root has only that file and imports only
`internal/engine`.

- `internal/engine/` — the implementation and its white-box tests.
- Root `*_test.go` — `package toupee_test`: public API tests and the corpus
  runner, calling only the root.
- `cmd/driftcheck/` — the drift check behind `make spec-drift`.

## Code conventions

- Standard library only; `go.mod` has no requirements. A dependency needs a
  decision first, not a line in a diff.
- Tests sit beside the code they test. Fixtures live in `testdata/`.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`); never swallow.
- Comments only for non-obvious constraints or trade-offs.

## Working mode

Smallest correct diff. Write a failing test first, then make it pass. No
dead code. Stop at the scope of the change.
