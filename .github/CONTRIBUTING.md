# Contributing

## Reporting bugs

Found a bug? [Open an issue](https://github.com/JacobJoergensen/laravel-fmt/issues/new/choose) and fill in the bug report.

## Setup

Fork the repo. You need Go 1.27 and [golangci-lint](https://golangci-lint.run).

```sh
go build
```

## Pull requests

Before opening one:

1. [File an issue](https://github.com/JacobJoergensen/laravel-fmt/issues/new?template=feature.yml) first for anything beyond a small fix.
2. Branch from `main`.
3. Add a test. When fixing a bug, add a test that would have caught it. A formatter change also needs an idempotence test: formatting already-formatted output must produce byte-identical output.
4. Run `go fix ./...` before pushing. It applies the Go modernizers and is behavior-preserving.

Keep the diff focused. Don't bundle a fix with a refactor.

## Coding standards

laravel-fmt is a formatter, not a linter. Two rules are non-negotiable: formatting must never change runtime behavior, and it must be idempotent. Invalid input fails with a location; it is never partially formatted.

Explicit return types on exported functions, named constants over magic values, no unnecessary abstractions. gofmt and golangci-lint must pass; don't suppress a lint without a comment saying why.

Changes to a parser or formatter hot path should include before/after `go test -bench` numbers.

## Running tests

```sh
go test ./...
golangci-lint run
go vet ./...
```
