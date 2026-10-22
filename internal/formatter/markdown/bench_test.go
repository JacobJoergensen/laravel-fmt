package markdown

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func genMarkdown(sections int) []byte {
	var sb strings.Builder

	for i := range sections {
		fmt.Fprintf(&sb, "## Section %d\n\n", i)
		fmt.Fprintf(&sb, "Section %d explains the option and links to [the guide](https://example.com/%d). ", i, i)
		sb.WriteString("It wraps across a line or two of prose with *emphasis* and `inline code`.\n\n")
		sb.WriteString("- first item\n- second item with more words\n- third item\n\n")
		sb.WriteString("```php\n$value = config('app.name');\n```\n\n")
		sb.WriteString("| Option | Default | Description |\n| --- | :---: | --- |\n")
		fmt.Fprintf(&sb, "| `width%d` | 120 | How wide a line may get |\n| `indent` | 4 | Spaces per level |\n\n", i)
	}

	return []byte(sb.String())
}

// genTables is table-heavy, so the cell measuring dominates.
func genTables(tables, rows int) []byte {
	var sb strings.Builder

	for t := range tables {
		fmt.Fprintf(&sb, "| Name %d | Type | Description |\n| --- | --- | ---: |\n", t)

		for r := range rows {
			fmt.Fprintf(&sb, "| item%d | string | The value of setting number %d |\n", r, r)
		}

		sb.WriteString("\n")
	}

	return []byte(sb.String())
}

// canonicalMarkdown returns src already formatted, so the benchmark measures
// the steady-state re-format an editor runs on every save.
func canonicalMarkdown(b *testing.B, src []byte) []byte {
	b.Helper()

	out, err := Format(src, config.Default())
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
		{"typical", genMarkdown(8)},
		{"large", genMarkdown(400)},
		{"tables", genTables(40, 30)},
	}

	for _, tc := range sizes {
		src := canonicalMarkdown(b, tc.src)

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
