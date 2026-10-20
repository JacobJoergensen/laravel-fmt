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

func lastLeaf(node parser.Node) parser.Node {
	for {
		switch n := node.(type) {
		case *parser.Mapping:
			if len(n.Entries) == 0 {
				return nil
			}

			node = n.Entries[len(n.Entries)-1].Value
		case *parser.Sequence:
			if len(n.Items) == 0 {
				return nil
			}

			node = n.Items[len(n.Items)-1].Value
		default:
			return node
		}
	}
}

func emitFrozen(w *formatter.Writer, src []byte, span parser.Span, last parser.Node) {
	start := span.Start.Offset

	// A leaf's span stops before its final line break unless that break is
	// value: a `+` block scalar's span runs over the breaks it keeps.
	floor := start
	if leaf := lastLeaf(last); leaf != nil {
		if end := leaf.Span().End.Offset; end > start && src[end-1] == '\n' {
			floor = end
		}
	}

	end := trimFrozenTrailing(src, floor, span.End.Offset)

	// Per-line trailing whitespace in a frozen span can be value, hence the
	// KeepTrailing variant. Lone CRs were already rewritten by Format.
	formatter.EmitVerbatimStrippingKeepTrailing(w, src[start:end], formatter.NaturalIndentBefore(src, start))
	w.Newline()
}
