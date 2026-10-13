package yaml

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/yaml"
)

func TestParseCoversAllContentBytes(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".yml", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".yml")
			file, errs := parser.Parse(src)

			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			c := formattertest.NewSpanCoverage(t, src)

			for _, doc := range file.Docs {
				coverDocument(t, c, src, doc)
			}

			c.AssertOnlyAllowedUncovered(" \t\r\n")
		})
	}
}

func coverDocument(t *testing.T, c *formattertest.SpanCoverage, src []byte, doc *parser.Document) {
	t.Helper()

	coverTrivia(c, doc.Leading)

	for _, d := range doc.Directives {
		c.Cover(d.Start.Offset, d.End.Offset)
	}

	c.Cover(doc.StartMarker.Start.Offset, doc.StartMarker.End.Offset)

	if doc.Body != nil {
		coverNode(t, c, src, doc.Body)
	}

	c.Cover(doc.EndMarker.Start.Offset, doc.EndMarker.End.Offset)
	coverTrivia(c, doc.Trailing)
}

func coverNode(t *testing.T, c *formattertest.SpanCoverage, src []byte, node parser.Node) {
	t.Helper()

	switch n := node.(type) {
	case *parser.Mapping:
		for _, e := range n.Entries {
			coverTrivia(c, e.Leading)

			if e.Key != nil {
				coverLeaf(t, c, src, e.Key.Range, e.Key.Raw)
			}

			c.Cover(e.Colon.Start.Offset, e.Colon.End.Offset)

			if e.Value != nil {
				coverNode(t, c, src, e.Value)
			}

			coverComment(c, e.Comment)
		}
	case *parser.Sequence:
		for _, item := range n.Items {
			coverTrivia(c, item.Leading)
			c.Cover(item.Dash.Start.Offset, item.Dash.End.Offset)

			if item.Value != nil {
				coverNode(t, c, src, item.Value)
			}

			coverComment(c, item.Comment)
		}
	case *parser.Scalar:
		coverLeaf(t, c, src, n.Range, n.Raw)
	case *parser.BlockScalar:
		coverLeaf(t, c, src, n.Range, n.Raw)
	case *parser.Flow:
		coverLeaf(t, c, src, n.Range, n.Raw)
	case *parser.Verbatim:
		coverLeaf(t, c, src, n.Range, n.Raw)
	default:
		t.Fatalf("unhandled YAML node %T", node)
	}
}

func coverLeaf(t *testing.T, c *formattertest.SpanCoverage, src []byte, span parser.Span, raw []byte) {
	t.Helper()

	c.Cover(span.Start.Offset, span.End.Offset)

	want := src[span.Start.Offset:span.End.Offset]

	if !bytes.Equal(raw, want) {
		t.Fatalf("Raw %q does not match source slice %q", raw, want)
	}
}

func coverComment(c *formattertest.SpanCoverage, tr *parser.Trivia) {
	if tr == nil {
		return
	}

	c.Cover(tr.Range.Start.Offset, tr.Range.End.Offset)
}

func coverTrivia(c *formattertest.SpanCoverage, ts []parser.Trivia) {
	for _, tr := range ts {
		c.Cover(tr.Range.Start.Offset, tr.Range.End.Offset)
	}
}
