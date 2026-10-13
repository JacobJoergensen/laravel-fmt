package formatter

import (
	"bytes"
	"testing"
)

func TestNormalizeLineTerminatorsCollapsesCRRunsAndKeepsContentCRs(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		eol  string
		want string
	}{
		{"no CR passes through", "a\nb\n", "\n", "a\nb\n"},
		{"CRLF becomes the configured EOL", "a\r\nb", "\n", "a\nb"},
		{"a CR run closed by a line feed is one terminator", "a\r\r\nb", "\n", "a\nb"},
		{"a longer CR run is still one terminator", "a\r\r\r\nb", "\n", "a\nb"},
		{"a CR no line feed closes is content", "a\rb", "\n", "a\rb"},
		{"a trailing CR is content", "a\r", "\n", "a\r"},
		{"content CR survives beside a terminator", "a\rb\r\nc", "\n", "a\rb\nc"},
		{"LF becomes CRLF when configured", "a\nb", "\r\n", "a\r\nb"},
		{"CRLF stays CRLF when configured", "a\r\nb", "\r\n", "a\r\nb"},
		{"a CR run collapses to one CRLF", "a\r\r\nb", "\r\n", "a\r\nb"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeLineTerminators([]byte(tc.raw), tc.eol)

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			// The rewrite runs again on its own output every time the file is
			// formatted, so anything but a fixpoint peels bytes off per pass.
			if twice := NormalizeLineTerminators(got, tc.eol); !bytes.Equal(twice, got) {
				t.Fatalf("not a fixpoint: once %q, twice %q", got, twice)
			}
		})
	}
}

func TestUniformIndentKeepsRelativeAlignment(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		natural int
		want    int
	}{
		{"an outdented line caps the strip", "a\nb\n        c", 4, 0},
		{"the shallowest line wins", "a\n  b\n        c", 4, 2},
		{"every line deeper than the anchor gives up the anchor", "a\n      b\n        c", 4, 4},
		{"blank lines carry no alignment", "a\n\n      b", 4, 4},
		{"whitespace-only lines are skipped", "a\n   \n      b", 4, 4},
		{"a line ending in CR alone is blank", "a\n  \r\n      b", 4, 4},
		{"a single line has nothing to strip against", "a", 4, 4},
		{"a zero anchor stays zero", "a\n    b", 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uniformIndent([]byte(tc.raw), tc.natural); got != tc.want {
				t.Fatalf("uniformIndent(%q, %d) = %d, want %d", tc.raw, tc.natural, got, tc.want)
			}
		})
	}
}

func TestEmitVerbatimPreservesGaps(t *testing.T) {
	// The anchor line is indented so there is a budget to strip; at column 0
	// the test could not tell per-line stripping from a uniform strip.
	src := []byte("    x\nline-at-0\n      line-at-6\n        line-at-8")

	w := NewWriter(writerCfg())
	EmitVerbatim(w, src, 4, len(src))

	got := string(w.Bytes())
	want := "x\nline-at-0\n      line-at-6\n        line-at-8"

	if got != want {
		t.Fatalf("relative alignment changed.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}
}

func TestEmitVerbatimRebasedTrimIsPerVariant(t *testing.T) {
	src := []byte("<t>one   \n  two   \n  three</t>")
	start, end := 3, len(src)-4

	trimmed := NewWriter(writerCfg())
	EmitVerbatimRebased(trimmed, src, start, end, 0)

	kept := NewWriter(writerCfg())
	EmitVerbatimRebasedKeepTrailing(kept, src, start, end, 0)

	if got := string(trimmed.Bytes()); got != "one\n  two\n  three" {
		t.Fatalf("trimming variant: got %q", got)
	}

	if got := string(kept.Bytes()); got != "one   \n  two   \n  three" {
		t.Fatalf("keep-trailing variant deleted bytes: got %q", got)
	}
}

func TestEmitVerbatimRehomedStripsAroundNestedLines(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		nested []bool
		want   string
	}{
		{
			// `0</em>` is a nested copy at column 0: it keeps that column and
			// does not drag the strip below base 4.
			"nested line left of base",
			"@A!\n    <td>\n        <em>\n0</em>\n    </td>",
			[]bool{false, false, true, true, true},
			"\n    @A!\n    <td>\n        <em>\n0</em>\n    </td>",
		},
		{
			// The span's own line left of base lowers the strip, as authored.
			"own line left of base",
			"0\n   \t@if!",
			[]bool{false, false},
			"\n    0\n    @if!",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWriter(writerCfg())
			w.Indent()
			w.Newline()
			EmitVerbatimRehomed(w, []byte(tc.raw), 5, func(n int) bool { return tc.nested[n] })

			if got := string(w.Bytes()); got != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}
