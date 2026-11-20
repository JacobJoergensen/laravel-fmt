package lsp

import (
	"bytes"
	"sort"
	"unicode/utf8"

	"github.com/JacobJoergensen/laravel-fmt/internal/diff"
)

type byteEdit struct {
	start, end int
	text       string
}

// Identical inputs return an empty, non-nil slice, so the reply is [] rather
// than null.
//
// A hunk at the end of a document reaches back for the newline before it, so
// it can start where the previous hunk ends. Such hunks are merged: an insert
// and a removal sharing a start depend on the client ordering them as LSP
// says, and not every client does.
func buildEdits(src, out []byte, enc string) []TextEdit {
	if bytes.Equal(src, out) {
		return []TextEdit{}
	}

	lfStart := diff.LineStarts(src)
	hunks := diff.LineEdits(string(src), string(out))

	spans := make([]byteEdit, 0, len(hunks))

	for _, h := range hunks {
		start, end, text := h.Replacement(src, lfStart)

		// A hunk at the end can start on the \n of a \r\n. No position lies
		// between the two, so the span takes the \r along.
		if start > 0 && start < len(src) && src[start-1] == '\r' && src[start] == '\n' {
			start--
			text = "\r" + text
		}

		if last := len(spans) - 1; last >= 0 && spans[last].end == start {
			spans[last].end = end
			spans[last].text += text

			continue
		}

		spans = append(spans, byteEdit{start: start, end: end, text: text})
	}

	lspStart := lspLineStarts(src)
	edits := make([]TextEdit, 0, len(spans))

	for _, s := range spans {
		start := byteToPosition(src, lspStart, s.start, enc)
		end := byteToPosition(src, lspStart, s.end, enc)

		edits = append(edits, TextEdit{Range: Range{Start: start, End: end}, NewText: s.text})
	}

	return edits
}

// positionToByte is the inverse of byteToPosition. A client may send a position
// outside its line or the document, so it is clamped rather than trusted: LSP
// puts a column past the line's text at the line's end, before its break.
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
		end = withoutLineBreak(src, start, lineStart[p.Line+1])
	}

	if enc == encodingUTF8 {
		offset := min(start+max(p.Character, 0), end)

		// A column inside a character moves past it, as the UTF-16 count does.
		for offset < end && !utf8.RuneStart(src[offset]) {
			offset++
		}

		return offset
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

func withoutLineBreak(src []byte, start, end int) int {
	if end > start && src[end-1] == '\n' {
		end--
	}

	if end > start && src[end-1] == '\r' {
		end--
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
