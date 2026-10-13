package xml

import (
	"bytes"

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
		start += len(raw) - len(bytes.TrimLeft(raw, " \t\r\n\f\v"))
		end = start + len(bytes.Trim(raw, " \t\r\n\f\v"))
	}

	formatter.EmitVerbatim(w, src, start, end)
}
