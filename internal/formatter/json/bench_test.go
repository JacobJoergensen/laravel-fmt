package json

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// genJSON mimics a package manifest, with arrays on both sides of the print
// width so both layout branches run.
func genJSON(entries int) []byte {
	var sb strings.Builder
	sb.WriteString(`{"name": "vendor/package", "version": "1.4.0", "private": true, "scripts": {`)

	for i := range entries {
		if i > 0 {
			sb.WriteString(", ")
		}

		fmt.Fprintf(&sb, `"task%d": "node ./scripts/run.mjs --stage %d --reporter dot --cache"`, i, i)
	}

	sb.WriteString(`}, "require": {`)

	for i := range entries {
		if i > 0 {
			sb.WriteString(", ")
		}

		fmt.Fprintf(&sb, `"vendor/dep%d": "^%d.2"`, i, i%9)
	}

	sb.WriteString(`}, "keywords": ["laravel", "formatter"], "files": [`)

	for i := range entries {
		if i > 0 {
			sb.WriteString(", ")
		}

		fmt.Fprintf(&sb, `"src/module%d/index.ts"`, i)
	}

	sb.WriteString(`], "extra": {"branch-alias": {"dev-main": "1.x-dev"}}}`)

	return []byte(sb.String())
}

// genNestedJSON builds a chain of objects too deep to fit, so all but the
// innermost levels break. A layout that emitted speculatively instead of
// measuring would grow exponentially here, not linearly.
func genNestedJSON(depth int) []byte {
	var sb strings.Builder

	for range depth {
		sb.WriteString(`{"a": `)
	}

	sb.WriteString("0")
	sb.WriteString(strings.Repeat("}", depth))

	return []byte(sb.String())
}

// canonicalJSON returns src already formatted, so the benchmark measures the
// steady-state re-format an editor runs on every save, not one-off reshaping.
func canonicalJSON(b *testing.B, src []byte) []byte {
	b.Helper()

	out, err := Format("package.json", src, config.Default())
	if err != nil {
		b.Fatalf("generated input did not format: %v", err)
	}

	return out
}

func BenchmarkFormat(b *testing.B) {
	cfg := config.Default()

	sizes := []struct {
		name string
		src  []byte
	}{
		{"typical", genJSON(12)},
		{"large", genJSON(900)},
		{"deeply nested", genNestedJSON(40)},
	}

	for _, tc := range sizes {
		src := canonicalJSON(b, tc.src)

		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Format("package.json", src, cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
