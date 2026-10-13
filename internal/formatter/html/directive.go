package html

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
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
		start += len(raw) - len(bytes.TrimLeft(raw, CollapsibleSpace))
		end = start + len(bytes.Trim(raw, CollapsibleSpace))
	}

	if formatter.ColumnUnmoved(w, src, start) {
		formatter.EmitVerbatimUnmoved(w, src, start, end)

		return
	}

	formatter.EmitVerbatim(w, src, start, end)
}
