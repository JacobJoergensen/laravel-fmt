package css

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// genStylesheet builds a stylesheet of `components` rule groups covering what
// the value, selector and prelude passes rewrite; the sizes bracket a component
// stylesheet and a large app.css.
func genStylesheet(components int) []byte {
	var sb strings.Builder

	sb.WriteString(":root {\n  --accent: #3b82f6;\n  --gutter: 1rem;\n}\n\n")

	for i := range components {
		fmt.Fprintf(&sb, "/* Component %d */\n", i)
		fmt.Fprintf(&sb, ".card-%d > .title, .card-%d [data-state=\"open\"] + .body {\n", i, i)
		sb.WriteString("  font-family: ui-sans-serif, system-ui, sans-serif;\n")
		sb.WriteString("  margin: 0 auto calc(var(--gutter) * 2);\n")
		sb.WriteString("  background: url(images/card.png) no-repeat, rgba(0, 0, 0, .5);\n")
		sb.WriteString("  color: var(--accent) !important;\n")
		sb.WriteString("}\n\n")
		fmt.Fprintf(&sb, "@media (min-width: 768px) and (prefers-reduced-motion: no-preference) {\n  .card-%d:hover {\n", i)
		sb.WriteString("    transform: translateY(-2px);\n    transition: transform .2s ease-in-out, box-shadow .2s ease-in-out;\n  }\n}\n\n")
		fmt.Fprintf(&sb, ".grid-%d {\n  display: grid;\n  grid-template-areas: \"head head\" \"side main\";\n\n  & .cell {\n    padding: var(--gutter);\n  }\n}\n\n", i)
	}

	return []byte(sb.String())
}

func canonicalStylesheet(b *testing.B, components int) []byte {
	b.Helper()

	out, err := Format(genStylesheet(components), config.Default())
	if err != nil {
		b.Fatalf("generated input did not format: %v", err)
	}

	return out
}

func BenchmarkFormat(b *testing.B) {
	cfg := config.Default()

	sizes := []struct {
		name       string
		components int
	}{
		{"typical", 10},
		{"large", 250},
	}

	for _, tc := range sizes {
		src := canonicalStylesheet(b, tc.components)

		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Format(src, cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
