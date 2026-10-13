package lsp

import (
	"bytes"
	"sort"

	"github.com/JacobJoergensen/laravel-fmt/internal/diff"
)

// Identical inputs return an empty, non-nil slice, so the reply is [] rather
// than null.
func buildEdits(src, out []byte, enc string) []TextEdit {
	if bytes.Equal(src, out) {
		return []TextEdit{}
	}

	lfStart := diff.LineStarts(src)
	lspStart := lspLineStarts(src)
	hunks := diff.LineEdits(string(src), string(out))

	edits := make([]TextEdit, 0, len(hunks))

	for _, h := range hunks {
		startByte, endByte, newText := h.Replacement(src, lfStart)

		edits = append(edits, TextEdit{
			Range:   Range{Start: byteToPosition(src, lspStart, startByte, enc), End: byteToPosition(src, lspStart, endByte, enc)},
			NewText: newText,
		})
	}

	return edits
}

// positionToByte is the inverse of byteToPosition. A client may send a position
// past the end of its line or of the document, so both are clamped rather than
// trusted.
func positionToByte(src []byte, lineStart []int, p Position, enc string) int {
	if p.Line < 0 {
		return 0
	}

	if p.Line >= len(lineStart) {
		return len(src)
	}

	start := lineStart[p.Line]
	end := len(src)

	if p.Line+1 < len(lineStart) {
		end = lineStart[p.Line+1]
	}

	if enc == encodingUTF8 {
		return min(start+p.Character, end)
	}

	units := 0

	for i, r := range string(src[start:end]) {
		if units >= p.Character {
			return start + i
		}

		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
	}

	return end
}

// LSP 3.17 counts \n, \r\n and a lone \r as line ends, so a lone CR adds a
// line the \n table does not see. The two tables must stay apart: hunk indices
// come from a \n split, while every Position sent to the client uses this one.
func lspLineStarts(src []byte) []int {
	starts := []int{0}

	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n':
			starts = append(starts, i+1)
		case '\r':
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}

			starts = append(starts, i+1)
		}
	}

	return starts
}

func byteToPosition(src []byte, lineStart []int, offset int, enc string) Position {
	line := max(sort.Search(len(lineStart), func(i int) bool { return lineStart[i] > offset })-1, 0)

	return Position{Line: line, Character: encodedLen(src[lineStart[line]:offset], enc)}
}

// UTF-16 counts an astral rune as a surrogate pair, two units.
func encodedLen(s []byte, enc string) int {
	if enc == encodingUTF8 {
		return len(s)
	}

	units := 0

	for _, r := range string(s) {
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
	}

	return units
}
