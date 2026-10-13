package css

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/css"
)

func TestParseCoversAllContentBytes(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".css", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".css")
			file, errs := parser.Parse(src)

			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			c := formattertest.NewSpanCoverage(t, src)

			for _, n := range file.Nodes {
				coverNode(t, c, n)
			}

			c.AssertOnlyAllowedUncovered(" \t\r\n")
		})
	}
}

func coverNode(t *testing.T, c *formattertest.SpanCoverage, node parser.Node) {
	t.Helper()

	switch n := node.(type) {
	case *parser.Rule:
		c.Cover(n.SelectorStart.Offset, n.SelectorEnd.Offset)
		c.Cover(n.LBrace.Start.Offset, n.LBrace.End.Offset)

		for _, child := range n.Children {
			coverNode(t, c, child)
		}

		c.Cover(n.RBrace.Start.Offset, n.RBrace.End.Offset)
	case *parser.AtRule:
		c.Cover(n.At.Start.Offset, n.At.End.Offset)
		// The name has no span of its own; it follows the @ directly.
		c.Cover(n.At.End.Offset, n.At.End.Offset+len(n.Name))
		c.Cover(n.PreludeStart.Offset, n.PreludeEnd.Offset)
		c.Cover(n.LBrace.Start.Offset, n.LBrace.End.Offset)

		for _, child := range n.Children {
			coverNode(t, c, child)
		}

		c.Cover(n.RBrace.Start.Offset, n.RBrace.End.Offset)
		c.Cover(n.Semicolon.Start.Offset, n.Semicolon.End.Offset)
	case *parser.Declaration:
		c.Cover(n.PropertyStart.Offset, n.PropertyEnd.Offset)
		c.Cover(n.Colon.Start.Offset, n.Colon.End.Offset)
		c.Cover(n.ValueStart.Offset, n.ValueEnd.Offset)
		c.Cover(n.ImportantSpan.Start.Offset, n.ImportantSpan.End.Offset)
		c.Cover(n.Semicolon.Start.Offset, n.Semicolon.End.Offset)
	case *parser.Comment:
		c.Cover(n.Open.Start.Offset, n.Close.End.Offset)
	default:
		t.Fatalf("unhandled CSS node %T", node)
	}
}
