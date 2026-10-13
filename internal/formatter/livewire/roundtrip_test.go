package livewire

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/livewire"
)

// Template node spans must be absolute to the original file; relative spans
// would cover the PHP region and leave template content unclaimed.
func TestParseSplitsSectionsLosslessly(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".blade.php", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".blade.php")
			file, errs := parser.Parse(src, nil)

			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			tokens := file.PHP.Tokens

			if len(tokens) == 0 {
				t.Fatal("PHP frontmatter produced no tokens")
			}

			var rebuilt bytes.Buffer

			for _, tok := range tokens {
				for _, tr := range tok.Leading {
					rebuilt.Write(tr.Text)
				}

				rebuilt.Write(tok.Text)
			}

			phpEnd := tokens[len(tokens)-1].Span.End.Offset
			diff := formattertest.FirstDiff(rebuilt.Bytes(), src[:phpEnd])

			if diff >= 0 {
				t.Fatalf("PHP token stream diverges from source at offset %d", diff)
			}

			if file.Template == nil {
				if phpEnd != len(src) {
					t.Fatalf("no template, but PHP section ends at %d of %d bytes", phpEnd, len(src))
				}

				return
			}

			c := formattertest.NewSpanCoverage(t, src)
			c.Cover(0, phpEnd)

			for _, n := range file.Template.Nodes {
				span := n.Span()
				c.Cover(span.Start.Offset, span.End.Offset)
			}

			c.AssertOnlyAllowedUncovered(" \t\r\n")
		})
	}
}
