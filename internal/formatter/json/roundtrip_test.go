package json

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/json"
)

func TestParseCoversAllContentBytes(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".json", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".json")
			file, errs := parser.Parse(src)

			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			c := formattertest.NewSpanCoverage(t, src)

			if file.Root != nil {
				coverValue(t, c, src, file.Root)
			}

			// Covering a container's Range would hide gaps between its
			// children, so its own punctuation is allowed uncovered instead.
			c.AssertOnlyAllowedUncovered(" \t\r\n{}[],:")
		})
	}
}

func coverValue(t *testing.T, c *formattertest.SpanCoverage, src []byte, v parser.Value) {
	t.Helper()

	switch n := v.(type) {
	case *parser.Object:
		for _, m := range n.Members {
			coverComments(c, m.Leading)
			coverValue(t, c, src, m.Key)
			coverValue(t, c, src, m.Value)
			coverComment(c, m.Trailing)
		}

		coverComments(c, n.Dangling)
	case *parser.Array:
		for _, e := range n.Elements {
			coverComments(c, e.Leading)
			coverValue(t, c, src, e.Value)
			coverComment(c, e.Trailing)
		}

		coverComments(c, n.Dangling)
	case *parser.String:
		c.Cover(n.Range.Start.Offset, n.Range.End.Offset)

		want := src[n.Range.Start.Offset+1 : n.Range.End.Offset-1]

		if !bytes.Equal(n.Raw, want) {
			t.Fatalf("String.Raw %q does not match source slice %q", n.Raw, want)
		}
	case *parser.Number:
		c.Cover(n.Range.Start.Offset, n.Range.End.Offset)

		want := src[n.Range.Start.Offset:n.Range.End.Offset]

		if !bytes.Equal(n.Raw, want) {
			t.Fatalf("Number.Raw %q does not match source slice %q", n.Raw, want)
		}
	case *parser.Bool:
		c.Cover(n.Range.Start.Offset, n.Range.End.Offset)
	case *parser.Null:
		c.Cover(n.Range.Start.Offset, n.Range.End.Offset)
	default:
		t.Fatalf("unhandled JSON node %T", v)
	}
}

func coverComments(c *formattertest.SpanCoverage, comments []parser.Comment) {
	for i := range comments {
		c.Cover(comments[i].Range.Start.Offset, comments[i].Range.End.Offset)
	}
}

func coverComment(c *formattertest.SpanCoverage, comment *parser.Comment) {
	if comment != nil {
		c.Cover(comment.Range.Start.Offset, comment.Range.End.Offset)
	}
}

func TestParseJSONCCoversAllContentBytes(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".jsonc", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".jsonc")
			file, errs := parser.ParseJSONC(src)

			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			c := formattertest.NewSpanCoverage(t, src)
			coverComments(c, file.Leading)

			if file.Root != nil {
				coverValue(t, c, src, file.Root)
			}

			coverComments(c, file.Trailing)
			c.AssertOnlyAllowedUncovered(" \t\r\n{}[],:")
		})
	}
}
