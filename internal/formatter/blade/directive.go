package blade

import (
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	htmlparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

func (e *emitter) isIgnoreDirectiveComment(node parser.Node) bool {
	switch c := node.(type) {
	case *parser.CommentNode:
		return formatter.IsIgnore(c.Text)
	case *htmlparser.Comment:
		return formatter.IsIgnore(c.Text)
	}

	return false
}

func (e *emitter) emitNodeVerbatim(node parser.Node) {
	span := node.Span()
	formatter.EmitVerbatim(e.w, e.src, span.Start.Offset, span.End.Offset)
}
