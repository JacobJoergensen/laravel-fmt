package blade

import (
	"bytes"

	htmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/html"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	htmlparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

// phpSpans returns the spans in nodes that Blade compiles into PHP. The HTML
// line model reads a lone CR as a line break, which is right for the markup and
// wrong inside these: there the CR is a byte of PHP data, and rewriting it
// changes what the page renders. A comment, an escaped echo and `@verbatim`
// compile to no PHP and are not included.
func phpSpans(nodes []htmlparser.Node) []htmlparser.Span {
	var spans []htmlparser.Span

	for _, n := range nodes {
		spans = appendPHPSpans(spans, n)
	}

	return spans
}

// phpSpansFor is phpSpans for the nodes making up raw, collected only when raw
// holds a lone CR: that is the one byte the spans decide, and without one the
// walk would cost every body an allocation for nothing. A CRLF file has CRs
// throughout and no lone one.
func phpSpansFor(raw []byte, nodes []htmlparser.Node) []htmlparser.Span {
	if !hasLoneCR(raw) {
		return nil
	}

	return phpSpans(nodes)
}

func hasLoneCR(b []byte) bool {
	for {
		at := bytes.IndexByte(b, '\r')
		if at < 0 {
			return false
		}

		if at+1 == len(b) || b[at+1] != '\n' {
			return true
		}

		b = b[at+2:]
	}
}

func appendPHPSpans(spans []htmlparser.Span, n any) []htmlparser.Span {
	switch node := n.(type) {
	case *parser.EchoNode:
		return append(spans, node.Span())
	case *parser.RawEchoNode:
		return append(spans, node.Span())
	case *parser.PhpBlockNode:
		return append(spans, node.Span())
	case *parser.RawPhpNode:
		return append(spans, node.Span())
	case *parser.DirectiveNode:
		return appendDirectiveSpan(spans, node)
	case *parser.DirectiveBlockNode:
		spans = appendDirectiveSpan(spans, node.Opener)

		for _, c := range node.Body {
			spans = appendPHPSpans(spans, c)
		}

		for _, branch := range node.Branches {
			spans = appendDirectiveSpan(spans, branch.Directive)

			for _, c := range branch.Body {
				spans = appendPHPSpans(spans, c)
			}
		}

		return appendDirectiveSpan(spans, node.End)
	case *htmlparser.Text:
		return append(spans, node.PHP...)
	case *htmlparser.Element:
		component := parser.IsComponentTag(node.Name)
		spans = appendPHPSpans(spans, node.NameEcho)

		for _, a := range node.Attributes {
			spans = appendAttrPHPSpans(spans, a, component)
		}

		for _, c := range node.Children {
			spans = appendPHPSpans(spans, c)
		}

		return appendPHPSpans(spans, node.CloseNameEcho)
	}

	return spans
}

// directiveSpans covers the only PHP whose line breaks the formatter writes
// itself: a directive's wrapped arguments.
func directiveSpans(nodes []htmlparser.Node) []htmlparser.Span {
	var spans []htmlparser.Span

	for _, n := range nodes {
		spans = appendDirectiveSpans(spans, n)
	}

	return spans
}

func appendDirectiveSpans(spans []htmlparser.Span, n any) []htmlparser.Span {
	switch node := n.(type) {
	case *parser.DirectiveNode:
		return appendDirectiveSpan(spans, node)
	case *parser.DirectiveBlockNode:
		spans = appendDirectiveSpan(spans, node.Opener)

		for _, c := range node.Body {
			spans = appendDirectiveSpans(spans, c)
		}

		for _, branch := range node.Branches {
			spans = appendDirectiveSpan(spans, branch.Directive)

			for _, c := range branch.Body {
				spans = appendDirectiveSpans(spans, c)
			}
		}

		return appendDirectiveSpan(spans, node.End)
	case *htmlparser.Element:
		for _, c := range node.Children {
			spans = appendDirectiveSpans(spans, c)
		}
	}

	return spans
}

func appendDirectiveSpan(spans []htmlparser.Span, d *parser.DirectiveNode) []htmlparser.Span {
	if d == nil || d.RParen.End.Offset <= d.LParen.Start.Offset {
		return spans
	}

	return append(spans, htmlparser.Span{Start: d.At, End: d.RParen.End})
}

func appendAttrPHPSpans(spans []htmlparser.Span, a *htmlparser.Attr, component bool) []htmlparser.Span {
	for _, part := range a.Parts {
		spans = appendPHPSpans(spans, part)
	}

	if a.Directive != nil || a.RawPhp != nil {
		return append(spans, a.Span())
	}

	if component && len(a.Value) > 0 {
		spans = append(spans, htmlparser.Span{
			Start: htmlparser.Position{Offset: a.ValueStart},
			End:   htmlparser.Position{Offset: a.ValueStart + len(a.Value)},
		})
	}

	return spans
}

// attrSpansLines reports whether an attribute's source spans lines, reading a
// lone CR as a line break except inside a value that compiles into PHP.
func attrSpansLines(src []byte, a *htmlparser.Attr, component bool) bool {
	raw := src[a.Start.Offset:a.End.Offset]

	var php []htmlparser.Span
	if hasLoneCR(raw) {
		php = appendAttrPHPSpans(nil, a, component)
	}

	return htmlfmt.HasLineBreakIn(raw, a.Start.Offset, php)
}

func hasValueLineBreak(b []byte, php bool) bool {
	at, _ := valueLineBreak(b, php)

	return at >= 0
}

// valueLineBreak returns the offset and width of the first line break in part
// of an attribute value, or -1 and 0. A value that compiles into PHP breaks on
// an LF or a CRLF only: a lone CR there is a byte of the string.
func valueLineBreak(b []byte, php bool) (int, int) {
	if !php {
		return htmlfmt.LineBreak(b)
	}

	at := bytes.IndexByte(b, '\n')

	switch {
	case at < 0:
		return -1, 0
	case at > 0 && b[at-1] == '\r':
		return at - 1, 2
	default:
		return at, 1
	}
}

func (e *emitter) bodyPHPSpans(el *htmlparser.Element) []htmlparser.Span {
	lo, hi := el.OpenClose.End.Offset, el.CloseOpen.Start.Offset

	if lo < 0 || hi > len(e.src) || lo >= hi {
		return nil
	}

	return phpSpansFor(e.src[lo:hi], el.Children)
}
