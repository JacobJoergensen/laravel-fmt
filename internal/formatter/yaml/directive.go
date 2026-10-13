package yaml

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/yaml"
)

func leadingHasIgnore(leading []parser.Trivia) bool {
	for _, t := range leading {
		if t.Kind == parser.TriviaComment && formatter.IsIgnore(bytes.TrimSpace(t.Text)) {
			return true
		}
	}

	return false
}

// trimFrozenTrailing returns where a frozen span's content ends, with its
// trailing line breaks and blank lines dropped: the caller writes the closing
// break, and keeping them would double it on the next pass.
//
// Whitespace on a line that has content stays, since a block scalar carries
// its line-trailing spaces as value.
func trimFrozenTrailing(src []byte, start, end int) int {
	for end > start {
		lineEnd := end

		for lineEnd > start && (src[lineEnd-1] == ' ' || src[lineEnd-1] == '\t') {
			lineEnd--
		}

		if lineEnd == start || (src[lineEnd-1] != '\n' && src[lineEnd-1] != '\r') {
			return end
		}

		end = lineEnd - 1
	}

	return end
}

func emitFrozen(w *formatter.Writer, src []byte, span parser.Span) {
	start := span.Start.Offset
	end := trimFrozenTrailing(src, span.Start.Offset, span.End.Offset)

	// Per-line trailing whitespace in a frozen span can be value, hence the
	// KeepTrailing variant. Lone CRs were already rewritten by Format.
	formatter.EmitVerbatimStrippingKeepTrailing(w, src[start:end], formatter.NaturalIndentBefore(src, start))
	w.Newline()
}
