package lsp

import (
	"cmp"
	"slices"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/dispatch"
)

// WARNING: the line table is built here from the LSP definition, not taken
// from the server. An applier sharing the server's table cannot see a
// line-model mismatch.
func applyEditsUTF8(src string, edits []TextEdit) string {
	b := []byte(src)

	starts := []int{0}

	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\n':
			starts = append(starts, i+1)
		case '\r':
			if i+1 < len(b) && b[i+1] == '\n' {
				i++
			}

			starts = append(starts, i+1)
		}
	}

	offset := func(p Position) int { return starts[p.Line] + p.Character }

	type span struct {
		start, end int
		text       string
	}

	spans := make([]span, 0, len(edits))
	for _, e := range edits {
		spans = append(spans, span{offset(e.Range.Start), offset(e.Range.End), e.NewText})
	}

	slices.SortFunc(spans, func(a, b span) int { return cmp.Compare(b.start, a.start) })

	for _, s := range spans {
		next := make([]byte, 0, len(b)-(s.end-s.start)+len(s.text))
		next = append(next, b[:s.start]...)
		next = append(next, s.text...)
		next = append(next, b[s.end:]...)
		b = next
	}

	return string(b)
}

func TestBuildEditsRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
	}{
		{"identical", "a\nb\n", "a\nb\n"},
		{"single line change", "a\nb\nc\n", "a\nB\nc\n"},
		{"multi hunk", "a\nb\nc\nd\n", "a\nB\nc\nD\n"},
		{"insert middle", "a\nc\n", "a\nb\nc\n"},
		{"delete middle", "a\nb\nc\n", "a\nc\n"},
		{"append at end", "a\nb", "a\nb\nc"},
		{"append after trailing newline", "a\nb\n", "a\nb\nc\n"},
		{"delete last line", "a\nb\nc", "a\nb"},
		{"add trailing newline", "a\nb", "a\nb\n"},
		{"remove trailing newline", "a\nb\n", "a\nb"},
		{"no newline change", "x", "y"},
		{"non ascii", "café\nz\n", "cafe\nz\n"},
		{"full rewrite", "one\ntwo\n", "uno\ndos\ntres\n"},
		{"empty to content", "", "a\nb\n"},
		{"content to empty", "a\nb\n", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edits := buildEdits([]byte(tc.old), []byte(tc.new), encodingUTF8)

			got := applyEditsUTF8(tc.old, edits)
			if got != tc.new {
				t.Fatalf("apply(edits) = %q, want %q (edits: %+v)", got, tc.new, edits)
			}
		})
	}
}

func TestBuildEditsIdenticalIsEmpty(t *testing.T) {
	edits := buildEdits([]byte("a\nb\n"), []byte("a\nb\n"), encodingUTF16)
	if len(edits) != 0 {
		t.Fatalf("expected no edits for identical input, got %+v", edits)
	}

	if edits == nil {
		t.Fatal("edits must be non-nil so the wire result is [] not null")
	}
}

func TestEncodedLenUTF16(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"abc", 3},
		{"café", 4},
		{"𝕏", 2},
		{"a𝕏b", 4},
	}

	for _, tc := range cases {
		if got := encodedLen([]byte(tc.in), encodingUTF16); got != tc.want {
			t.Errorf("encodedLen(%q, utf-16) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestByteToPositionUTF16Columns(t *testing.T) {
	src := []byte("a𝕏b\nc\n")
	starts := lspLineStarts(src)

	end := len("a𝕏")
	pos := byteToPosition(src, starts, end, encodingUTF16)

	if pos.Line != 0 {
		t.Fatalf("line = %d, want 0", pos.Line)
	}

	if pos.Character != 3 {
		t.Fatalf("character = %d, want 3 (a=1 + astral=2)", pos.Character)
	}
}

func TestPositionToByteRoundTripsByteToPosition(t *testing.T) {
	src := []byte("let a = 1\nlet 😀 = \"two\"\nlast")

	for _, enc := range []string{encodingUTF8, encodingUTF16} {
		t.Run(enc, func(t *testing.T) {
			lineStart := lspLineStarts(src)

			// Only rune boundaries are addressable positions: an offset inside a
			// rune names no character, and a hunk never starts at one.
			offsets := make([]int, 0, len(src)+1)
			offsets = append(offsets, len(src))
			for i := range string(src) {
				offsets = append(offsets, i)
			}

			for _, offset := range offsets {
				pos := byteToPosition(src, lineStart, offset, enc)

				if got := positionToByte(src, lineStart, pos, enc); got != offset {
					t.Fatalf("offset %d became %+v and came back as %d", offset, pos, got)
				}
			}
		})
	}
}

func TestPositionToByteClampsOutOfRangePositions(t *testing.T) {
	src := []byte("ab\ncd\n")
	lineStart := lspLineStarts(src)

	if got := positionToByte(src, lineStart, Position{Line: 99, Character: 0}, encodingUTF8); got != len(src) {
		t.Fatalf("a line past the document = %d, want %d", got, len(src))
	}

	if got := positionToByte(src, lineStart, Position{Line: 0, Character: 99}, encodingUTF8); got != 3 {
		t.Fatalf("a character past the line = %d, want the line's end 3", got)
	}
}

func TestBuildEditsHandlesLoneCarriageReturn(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
	}{
		{"lone CR mid-document", "<?php\n\n$a = 1;\r$b = 2;\n\necho   3  ;\n", "<?php\n\n$a = 1;\r$b = 2;\n\necho 3;\n"},
		{"lone CR before the change", "a\rb\nc   \nd\n", "a\rb\nc\nd\n"},
		{"several lone CRs", "a\rb\rc\nd   \n", "a\rb\rc\nd\n"},
		{"CRLF document", "a\r\nb   \r\nc\r\n", "a\r\nb\r\nc\r\n"},
		{"CR at end of document", "a\nb   \nc\r", "a\nb\nc\r"},
		{"mixed CR and CRLF", "a\rb\r\nc   \n", "a\rb\r\nc\n"},
		// Control: the LF path the round-trip cases already cover must still hold.
		{"plain LF", "a\nb   \nc\n", "a\nb\nc\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edits := buildEdits([]byte(tc.old), []byte(tc.new), encodingUTF8)

			if got := applyEditsUTF8(tc.old, edits); got != tc.new {
				t.Fatalf("applied edits gave %q, want %q", got, tc.new)
			}
		})
	}
}

// applyEditsUTF16 measures columns in UTF-16, the encoding the server
// negotiates by default; the UTF-8 applier agrees with it only on ASCII. Its
// line table is its own for the same reason as applyEditsUTF8's.
func applyEditsUTF16(src string, edits []TextEdit) string {
	b := []byte(src)
	starts := clientLineStarts(b)

	offset := func(p Position) int {
		lineStart := starts[p.Line]
		lineEnd := len(b)

		if p.Line+1 < len(starts) {
			lineEnd = starts[p.Line+1]
		}

		units := 0

		for i, r := range string(b[lineStart:lineEnd]) {
			if units >= p.Character {
				return lineStart + i
			}

			if r > 0xFFFF {
				units += 2
			} else {
				units++
			}
		}

		return lineEnd
	}

	type span struct {
		start, end int
		text       string
	}

	spans := make([]span, 0, len(edits))
	for _, e := range edits {
		spans = append(spans, span{offset(e.Range.Start), offset(e.Range.End), e.NewText})
	}

	slices.SortStableFunc(spans, func(x, y span) int { return cmp.Compare(y.start, x.start) })

	for _, s := range spans {
		b = append(b[:s.start], append([]byte(s.text), b[s.end:]...)...)
	}

	return string(b)
}

// clientLineStarts follows LSP 3.17: \n, \r\n and a lone \r end a line.
func clientLineStarts(b []byte) []int {
	starts := []int{0}

	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\n':
			starts = append(starts, i+1)
		case '\r':
			if i+1 < len(b) && b[i+1] == '\n' {
				i++
			}

			starts = append(starts, i+1)
		}
	}

	return starts
}

func TestFormatDocumentEditsApplyUnderUTF16Columns(t *testing.T) {
	const astral = "\U0001D54F" // 𝕏
	const bmp = "é"

	cases := []struct{ name, src string }{
		{"astral on the edited line", "<?php\n$x   =   '" + astral + "';"},
		{"bmp on the edited line", "<?php\n$y   =   '" + bmp + "';"},
		{"astral before an inserted operator space", "<?php\n$x = '" + astral + "'.$y;"},
		{"astral before a later edit", "<?php\n$x = '" + astral + "';\n$y   =   2;"},
		{"astral after an earlier line", "<?php\n$a = 1;\n$b = '" + astral + "'.$c;"},
		{"mixed widths on one line", "<?php\n$m   =   '" + bmp + astral + bmp + "';"},
		{"astral with a lone CR line", "<?php\r$x = '" + astral + "';\r$y   =   2;"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			config.ResolveEndOfLine([]byte(tc.src), cfg)

			formatted, err := dispatch.File("/t.php", []byte(tc.src), cfg)
			if err != nil {
				t.Fatalf("dispatch baseline: %v", err)
			}

			want := string(formatted)

			s, uri := newDirectServer(t, "t.php", tc.src)
			s.encoding = encodingUTF16

			edits := mustDocumentEdits(t, s, uri)

			if got := applyEditsUTF16(tc.src, edits); got != want {
				t.Fatalf("utf-16 client applying the edits did not reproduce the formatted text.\n=== got ===\n%q\n=== want ===\n%q\n=== edits ===\n%+v", got, want, edits)
			}
		})
	}
}
