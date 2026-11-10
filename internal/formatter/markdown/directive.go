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

func endsInOpenFence(block parser.Node) bool {
	fence, ok := lastListLeaf(block).(*parser.FencedCodeBlock)

	return ok && fence.CloseFence.End.Offset == 0
}

func endsInOpenHTMLBlock(block parser.Node) bool {
	html, ok := lastListLeaf(block).(*parser.HTMLBlock)

	return ok && html.Unclosed
}

func openHTMLTrailingBlanks(body *parser.File) int {
	if body == nil || len(body.Blocks) == 0 {
		return 0
	}

	html, ok := lastListLeaf(body.Blocks[len(body.Blocks)-1]).(*parser.HTMLBlock)
	if !ok || !html.Unclosed {
		return 0
	}

	content := bytes.TrimRight(html.Content, "\r\n")

	return bytes.Count(html.Content[len(content):], []byte("\n"))
}

// A blank line after a quote ends the quote, so only lists can leave a block
// open to the lines after them.
func lastListLeaf(block parser.Node) parser.Node {
	for {
		list, ok := block.(*parser.List)
		if !ok {
			return block
		}

		body := list.Items[len(list.Items)-1].Body
		if body == nil || len(body.Blocks) == 0 {
			return nil
		}

		block = body.Blocks[len(body.Blocks)-1]
	}
}

// A block span runs past its last line's terminator; emitFile owns inter-block
// spacing, so trimming it keeps the gap from doubling. Only that one goes: blank
// lines before it, as an unclosed fence holds, are the block's own.
func emitBlockVerbatim(w *formatter.Writer, src []byte, block parser.Node) {
	span := block.Span()
	end := span.End.Offset

	if end > span.Start.Offset && src[end-1] == '\n' {
		end--
	}

	if end > span.Start.Offset && src[end-1] == '\r' {
		end--
	}

	// No indent to strip: a block starts a line, and a lone CR ends one in
	// Markdown, which NaturalIndentBefore does not know.
	formatter.EmitVerbatimStrippingKeepTrailing(w, src[span.Start.Offset:end], 0)
}
