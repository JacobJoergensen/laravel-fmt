package markdown

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/markdown"
)

// Blocks are covered by their full span; fidelity inside a block (cells are
// trimmed, blockquote bodies stripped) is the golden fixtures' contract, not
// a span one.
func TestParseCoversAllContentBytes(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".md", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".md")
			file, errs := parser.Parse(src)

			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			c := formattertest.NewSpanCoverage(t, src)

			for _, b := range file.Blocks {
				span := b.Span()
				c.Cover(span.Start.Offset, span.End.Offset)
			}

			c.AssertOnlyAllowedUncovered(" \t\r\n")
		})
	}
}
