# generate-tailwind-registry

Reads Tailwind v4's default design system and writes
`internal/formatter/tailwind/registry_generated.go`, the class order the
Tailwind sorter uses. It is not part of the laravel-fmt build.

## Run

```bash
cd scripts/generate-tailwind-registry
npm ci
node generate.mjs
```

`gofmt` must be on PATH; the script formats what it writes. Then verify
from the repo root:

```bash
go test ./internal/formatter/tailwind/...
```

## New Tailwind release

1. Pin the new version in `package.json` and run `npm install`.
2. Replace `propertyOrder` in `generate.mjs` with that release's
   `packages/tailwindcss/src/property-order.ts`.
3. Run `node generate.mjs`, test, and commit the Go file.
