# Contributing to cli4go

Thanks for your interest in `cli4go`. The project is small and still
pre-`v1.0.0`, so the process is lightweight -- but a few conventions are
enforced consistently and PRs that don't follow them will be asked to
change before merge.

## Requirements

- Go 1.25 or newer.

## Building and testing

```sh
git clone https://github.com/ecanuto/cli4go.git
cd cli4go
make build     # go build ./... -- compiles the library and examples
make fmt       # go fmt ./...
make vet       # go vet ./...
make test      # go test -v ./...
make coverage  # test coverage summary, scoped to the cli package
make tidy      # go mod tidy
```

Run `make help` to list every available target.

Before opening a PR, all of `make fmt`, `make vet`, `make test`, and
`make coverage` must pass clean.

## Coverage bar

The `cli` package is held to **100% test coverage**, checked with
`make coverage`. A PR that drops coverage below 100% will not be merged
without either added tests or a clear justification in the PR
description for why a given line can't reasonably be covered.

`examples/*/main.go` is intentionally excluded from the coverage target
-- a `func main()` demo isn't a realistic unit-test candidate.

## Code conventions

- Everything in the codebase -- code, comments, identifiers, and docs --
  must be written in English.
- Tests are table-driven, with one `t.Run` per case. Multi-line expected
  output (e.g. from `UsageString()`/`HelpString()`) is asserted as
  `want []string{...}`, joined with `strings.Join(tt.want, "\n") + "\n"`
  before comparing.
- No `os.Exit()` calls anywhere in the library -- every function returns
  values, and only a caller's own `main()` may exit.
- If a design decision matters to someone reading the code, it belongs
  in a godoc comment next to the code it affects, not in a separate
  document. Comments and doc strings in this repository should be
  self-sufficient: don't write a comment that only makes sense to
  someone reading alongside outside notes.

## Proposing a change

1. Open an issue first for anything beyond a small fix or typo --
   `cli4go`'s public API is deliberately minimal, and some shapes
   (constructors/functional options, a combined `Argument`/`Option`
   type, built-in i18n, `GetX()`-style getters, an `os.Exit()` anywhere
   in the library) have already been considered and rejected. Checking
   in first avoids wasted work on a PR that won't be accepted.
2. Keep the change focused. Prefer several small PRs over one large one.
3. Add or update tests for any behavior change -- see "Coverage bar"
   above.
4. Run `make fmt vet test coverage` locally before pushing.
5. Describe *why* the change is needed in the PR description, not just
   what it does.

## Reporting bugs

Open an issue with:

- The `cli4go` usage that triggers the problem (a minimal `Program`/
  `Command` definition and the arguments passed to it is ideal).
- What you expected to happen and what happened instead.
- Your Go version (`go version`).

## License

By contributing, you agree that your contributions will be licensed
under the project's [MIT license](LICENSE).
