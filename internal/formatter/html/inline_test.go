package html

import (
	"testing"

	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

func TestInlineAtomsMarksABlankLineBeforeAnAtom(t *testing.T) {
	cases := []struct {
		name      string
		between   string
		wantBreak bool
		wantBlank bool
	}{
		{"space", " ", false, false},
		{"one LF", "\n    ", true, false},
		{"one CRLF", "\r\n    ", true, false},
		{"two LF", "\n\n    ", true, true},
		{"two LF around spaces", "\n \t\n", true, true},
		{"two CRLF", "\r\n\r\n", true, true},
		{"two lone CR", "\r\r", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			atoms, _, _ := InlineAtoms([]parser.Node{&parser.Text{Text: []byte("a" + tc.between + "b")}})
			if len(atoms) != 2 {
				t.Fatalf("got %d atoms, want 2", len(atoms))
			}

			if atoms[1].NewlineBefore != tc.wantBreak || atoms[1].BlankBefore != tc.wantBlank {
				t.Fatalf("NewlineBefore, BlankBefore = %v, %v; want %v, %v",
					atoms[1].NewlineBefore, atoms[1].BlankBefore, tc.wantBreak, tc.wantBlank)
			}
		})
	}
}
