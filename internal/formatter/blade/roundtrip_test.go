package blade

import (
	"reflect"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	htmlparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

func TestParseCoversAllContentBytes(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".blade.php", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".blade.php")
			file, errs := parser.Parse(src, nil)

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
	case *parser.EchoNode, *parser.RawEchoNode, *parser.CommentNode,
		*parser.VerbatimNode, *parser.EscapedAtNode, *parser.EscapedEchoNode,
		*parser.DirectiveNode, *parser.PhpBlockNode, *parser.RawPhpNode:
		span := n.Span()
		c.Cover(span.Start.Offset, span.End.Offset)
	case *parser.DirectiveBlockNode:
		coverNode(t, c, n.Opener)

		for _, child := range n.Body {
			coverNode(t, c, child)
		}

		for _, branch := range n.Branches {
			coverNode(t, c, branch.Directive)

			for _, child := range branch.Body {
				coverNode(t, c, child)
			}
		}

		if n.End != nil {
			coverNode(t, c, n.End)
		}
	case *htmlparser.Element:
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
	case *htmlparser.Text, *htmlparser.Comment, *htmlparser.Doctype:
		span := n.Span()
		c.Cover(span.Start.Offset, span.End.Offset)
	default:
		t.Fatalf("unhandled Blade node %T", node)
	}
}

// The emitter hands each node's tokens to the PHP builder with a slice of the
// source it cuts itself, and the builder reads comments and literals by offset
// into that slice, so the tokens must be the lex of exactly those bytes.
func TestNodeTokensAreTheFragmentTheEmitterBuilds(t *testing.T) {
	seen := map[string]int{}

	for _, name := range formattertest.GoldenFixtureNames(t, ".blade.php", false) {
		src := formattertest.ReadFixture(t, name+".blade.php")

		file, errs := parser.Parse(src, nil)
		if len(errs) > 0 {
			continue
		}

		check := func(kind string, fragment []byte, tokens []php.Token) {
			if tokens == nil {
				return
			}

			seen[kind]++

			want, _ := php.LexBare(fragment)
			if !reflect.DeepEqual(tokens, want) {
				t.Errorf("%s: %s tokens are not the lex of %q", name, kind, fragment)
			}
		}

		var walk func(n parser.Node)

		walk = func(n parser.Node) {
			switch v := n.(type) {
			case *parser.EchoNode:
				check("echo", src[v.Open.End.Offset:v.Close.Start.Offset], v.Tokens)
			case *parser.RawEchoNode:
				check("raw echo", src[v.Open.End.Offset:v.Close.Start.Offset], v.Tokens)
			case *parser.DirectiveNode:
				check("directive", src[v.LParen.End.Offset:v.RParen.Start.Offset], v.Tokens)
			case *parser.PhpBlockNode:
				check("@php block", src[v.Open.End.Offset:v.Close.Start.Offset], v.Tokens)
			case *parser.RawPhpNode:
				check("<?php island", src[v.Body.Start.Offset:v.Body.End.Offset], v.Tokens)
			case *parser.DirectiveBlockNode:
				walk(v.Opener)

				for _, c := range v.Body {
					walk(c)
				}

				for _, b := range v.Branches {
					walk(b.Directive)

					for _, c := range b.Body {
						walk(c)
					}
				}
			case *htmlparser.Element:
				for _, a := range v.Attributes {
					check("bound attr", a.Value, a.ExprTokens)

					if a.Directive != nil {
						walk(a.Directive)
					}

					for _, p := range a.Parts {
						walk(p)
					}
				}

				for _, c := range v.Children {
					walk(c)
				}
			}
		}

		for _, n := range file.Nodes {
			walk(n)
		}
	}

	for _, kind := range []string{"echo", "raw echo", "directive", "@php block", "<?php island", "bound attr"} {
		if seen[kind] == 0 {
			t.Errorf("no fixture exercises a %s, so its pairing is unchecked", kind)
		}
	}
}
