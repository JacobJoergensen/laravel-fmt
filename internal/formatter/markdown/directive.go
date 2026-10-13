package markdown

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/markdown"
)

func isIgnoreDirectiveComment(block parser.Node) bool {
	h, ok := block.(*parser.HTMLBlock)

	if !ok {
		return false
	}

	return formatter.IsIgnore(bytes.TrimSpace(h.Content))
}

// A block span runs past its trailing newline; emitFile owns inter-block
// spacing, so trimming here keeps the gap from doubling.
func emitBlockVerbatim(w *formatter.Writer, src []byte, block parser.Node) {
	span := block.Span()
	end := span.End.Offset

	for end > span.Start.Offset && (src[end-1] == '\n' || src[end-1] == '\r') {
		end--
	}

	// No indent to strip: a block starts a line, and a lone CR ends one in
	// Markdown, which NaturalIndentBefore does not know.
	formatter.EmitVerbatimStrippingKeepTrailing(w, src[span.Start.Offset:end], 0)
}
