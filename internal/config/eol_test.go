package config

import "testing"

func TestResolveEndOfLine(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
		in   string
		want string
	}{
		{"auto + LF-only source", []byte("line one\nline two\n"), EndOfLineAuto, EndOfLineLF},
		{"auto + CRLF source", []byte("line one\r\nline two\r\n"), EndOfLineAuto, EndOfLineCRLF},
		{"auto + mixed picks CRLF on first hit", []byte("one\ntwo\r\nthree\n"), EndOfLineAuto, EndOfLineCRLF},
		{"auto + no newlines falls back to LF", []byte("one line"), EndOfLineAuto, EndOfLineLF},
		{"explicit lf is not overridden by CRLF source", []byte("one\r\ntwo\r\n"), EndOfLineLF, EndOfLineLF},
		{"explicit crlf is not overridden by LF source", []byte("one\ntwo\n"), EndOfLineCRLF, EndOfLineCRLF},
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
