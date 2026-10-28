package json

import (
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/json"
)

func hasIgnoreDirective(comments []parser.Comment) bool {
	for _, c := range comments {
		if formatter.IsIgnore(c.Text) {
			return true
		}
	}

	return false
}

func emitVerbatimRange(w *formatter.Writer, src []byte, start, end int) {
	formatter.EmitVerbatim(w, src, start, end)
}
