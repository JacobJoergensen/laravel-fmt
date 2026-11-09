package xml

import (
	"bytes"
	"slices"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/xml"
)

func isIgnoreDirectiveComment(node parser.Node) bool {
	c, ok := node.(*parser.Comment)
	if !ok {
		return false
	}

	return formatter.IsIgnore(c.Text)
}

func emitNodeVerbatim(w *formatter.Writer, src []byte, node parser.Node) {
	span := node.Span()
	start, end := span.Start.Offset, span.End.Offset

	// A text span, unlike an element span, includes the whitespace around it.
	// The formatter already lays that whitespace out, so emitting it as well
	// would add a blank line on every pass.
	if _, ok := node.(*parser.Text); ok {
		raw := src[start:end]
		start += len(raw) - len(bytes.TrimLeft(raw, parser.Whitespace))
		end = start + len(bytes.Trim(raw, parser.Whitespace))
	}

	if !holdsLineBreakData(src, node) {
		formatter.EmitVerbatim(w, src, start, end)

		return
	}

	first, rest, multiline := bytes.Cut(src[start:end], []byte("\n"))
	w.WriteBytes(bytes.TrimSuffix(first, []byte("\r")))

	if multiline {
		w.Newline()
		formatter.EmitVerbatimExact(w, rest)
	}
}

// Re-homing a node's lines would rewrite a line break that is data, not layout.
func holdsLineBreakData(src []byte, node parser.Node) bool {
	switch n := node.(type) {
	case *parser.Text:
		return bytes.ContainsRune(bytes.Trim(n.Text, parser.Whitespace), '\n')
	case *parser.CDATA:
		return bytes.ContainsRune(n.Text, '\n')
	case *parser.Element:
		return elementHoldsLineBreakData(src, n)
	}

	return false
}

func elementHoldsLineBreakData(src []byte, el *parser.Element) bool {
	for _, attr := range el.Attributes {
		if bytes.ContainsRune(attr.Value, '\n') {
			return true
		}
	}

	if el.SelfClose || len(el.Children) == 0 {
		return false
	}

	if preservesSpace(el) || !isStructuralContent(el.Children) {
		content := src[el.OpenClose.End.Offset:el.CloseOpen.Start.Offset]

		return bytes.ContainsRune(content, '\n')
	}

	return slices.ContainsFunc(el.Children, func(child parser.Node) bool {
		return holdsLineBreakData(src, child)
	})
}
