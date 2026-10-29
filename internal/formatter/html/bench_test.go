package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// genPage builds a page of `sections` sections covering inline runs, attribute
// lists and style and script bodies; the sizes bracket a landing page and a
// long documentation page.
func genPage(sections int) []byte {
	var sb strings.Builder

	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	sb.WriteString("<title>Example</title>\n<link rel=\"stylesheet\" href=\"/css/app.css\">\n")
	sb.WriteString("<style>\nbody { margin: 0; font-family: system-ui, sans-serif; }\n</style>\n</head>\n<body>\n")
	sb.WriteString("<nav class=\"navbar\"><ul><li><a href=\"/\">Home</a></li><li><a href=\"/docs\">Docs</a></li></ul></nav>\n<main>\n")

	for i := range sections {
		fmt.Fprintf(&sb, "<section id=\"section-%d\" class=\"section section--wide\" data-index=\"%d\" aria-labelledby=\"heading-%d\">\n", i, i, i)
		fmt.Fprintf(&sb, "<h2 id=\"heading-%d\">Section %d</h2>\n", i, i)
		sb.WriteString("<p>Some <strong>bold</strong> text, a <a href=\"/more\" title=\"Read more\">link</a> and <em>emphasis</em> that runs long enough to wrap across the print width.</p>\n")
		sb.WriteString("<ul>\n<li>First item</li>\n<li>Second item with <code>code</code></li>\n</ul>\n")
		sb.WriteString("<img src=\"/img/photo.jpg\" alt=\"A photo\" width=\"640\" height=\"480\" loading=\"lazy\">\n")
		sb.WriteString("<button type=\"button\" class=\"btn btn-primary\" disabled>Save</button>\n</section>\n")
	}

	sb.WriteString("</main>\n<script>\ndocument.querySelector('.btn').addEventListener('click', () => {});\n</script>\n</body>\n</html>\n")

	return []byte(sb.String())
}

func canonicalPage(b *testing.B, sections int) []byte {
	b.Helper()

	out, err := Format(genPage(sections), config.Default())
	if err != nil {
		b.Fatalf("generated input did not format: %v", err)
	}

	return out
}

func BenchmarkFormat(b *testing.B) {
	cfg := config.Default()

	sizes := []struct {
		name     string
		sections int
	}{
		{"typical", 20},
		{"large", 400},
	}

	for _, tc := range sizes {
		src := canonicalPage(b, tc.sections)

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
