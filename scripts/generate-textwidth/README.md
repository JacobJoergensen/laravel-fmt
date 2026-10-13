# generate-textwidth

Writes `internal/textwidth/wideranges_generated.go`, the East Asian Wide and
Fullwidth ranges `textwidth.Rune` counts as two columns. It is not part of the
laravel-fmt build.

## Run

```bash
cd scripts/generate-textwidth
npm ci
node generate.mjs
```

`gofmt` must be on PATH; the script formats what it writes. Then verify from
the repo root:

```bash
go test ./internal/textwidth/
```

## New Unicode version

1. Bump `get-east-asian-width` in `package.json` and run `npm install`.
2. Run `node generate.mjs`, test, and commit the Go file.

Zero-width characters are not generated: they come from Go's own
`unicode.Mn`, `unicode.Me` and `unicode.Cf` tables, which update with the
toolchain.
