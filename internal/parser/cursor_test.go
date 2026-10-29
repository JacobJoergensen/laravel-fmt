package parser

import "testing"

func TestAdvanceEndsALineAtLFCRLFAndALoneCR(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want Position
	}{
		{"LF", "a\nb", Position{Offset: 3, Line: 2, Column: 2}},
		{"CRLF is one line end", "a\r\nb", Position{Offset: 4, Line: 2, Column: 2}},
		{"lone CR", "a\rb", Position{Offset: 3, Line: 2, Column: 2}},
		{"lone CR at the end", "a\r", Position{Offset: 2, Line: 2, Column: 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Cursor{Src: []byte(tc.src), Pos: Position{Line: 1, Column: 1}}
			c.AdvanceN(len(tc.src))

			if c.Pos != tc.want {
				t.Fatalf("Pos = %+v, want %+v", c.Pos, tc.want)
			}
		})
	}
}
