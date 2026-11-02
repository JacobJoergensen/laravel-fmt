package config

import "testing"

func TestEndOfLineResolvesOnlyAutoFromTheFirstLineEnding(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
		in   string
		want string
	}{
		{"auto + mixed follows a first LF", []byte("one\ntwo\r\nthree\n"), EndOfLineAuto, EndOfLineLF},
		{"auto + mixed follows a first CRLF", []byte("one\r\ntwo\nthree\n"), EndOfLineAuto, EndOfLineCRLF},
		{"auto + no newlines falls back to LF", []byte("one line"), EndOfLineAuto, EndOfLineLF},
		{"auto + source opening with a line break", []byte("\nx\r\n"), EndOfLineAuto, EndOfLineLF},
		{"explicit lf is not overridden by CRLF source", []byte("one\r\ntwo\r\n"), EndOfLineLF, EndOfLineLF},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.EndOfLine = tc.in

			ResolveEndOfLine(tc.src, cfg)

			if cfg.EndOfLine != tc.want {
				t.Fatalf("EndOfLine = %q, want %q", cfg.EndOfLine, tc.want)
			}
		})
	}
}

func TestEOLWritesCRLFOnlyWhenSetAndLFOtherwise(t *testing.T) {
	cases := []struct {
		endOfLine, want string
	}{
		{EndOfLineLF, "\n"},
		{EndOfLineCRLF, "\r\n"},
		{EndOfLineAuto, "\n"},
	}

	for _, tc := range cases {
		t.Run(tc.endOfLine, func(t *testing.T) {
			f := &File{EndOfLine: tc.endOfLine}
			if got := f.EOL(); got != tc.want {
				t.Fatalf("EOL() = %q, want %q", got, tc.want)
			}
		})
	}
}
