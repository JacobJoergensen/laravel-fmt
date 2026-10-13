package xml

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/xml"
)

func TestParseCoversAllContentBytes(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".xml", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".xml")
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
	case *parser.Element:
		c.Cover(n.Open.Start.Offset, n.Open.End.Offset)
		// The tag name has no span of its own; it follows the < directly.
		c.Cover(n.Open.End.Offset, n.Open.End.Offset+len(n.Name))

		for _, a := range n.Attributes {
			c.Cover(a.Start.Offset, a.End.Offset)
		}

		c.Cover(n.OpenClose.Start.Offset, n.OpenClose.End.Offset)

		for _, child := range n.Children {
			coverNode(t, c, child)
		}

		if n.CloseOpen.End.Offset > 0 {
			c.Cover(n.CloseOpen.Start.Offset, n.CloseFinal.End.Offset)
		}
	case *parser.Text:
		c.Cover(n.Start.Offset, n.End.Offset)
	case *parser.Comment:
		c.Cover(n.Open.Start.Offset, n.Close.End.Offset)
	case *parser.CDATA:
		c.Cover(n.Open.Start.Offset, n.Close.End.Offset)
	case *parser.PI:
		c.Cover(n.Open.Start.Offset, n.Close.End.Offset)
	case *parser.Doctype:
		c.Cover(n.Open.Start.Offset, n.Close.End.Offset)
	default:
		t.Fatalf("unhandled XML node %T", node)
	}
}
