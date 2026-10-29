package css

import (
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/css"
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

	// The block body owns the newline and indent around a node, so whitespace
	// left in the slice would add a blank line on every pass.
	for start < end && parser.IsSpaceByte(src[start]) {
		start++
	}

	for end > start && parser.IsSpaceByte(src[end-1]) {
		end--
	}

	formatter.EmitVerbatim(w, src, start, end)
}
