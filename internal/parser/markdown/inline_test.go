package markdown

import (
	"slices"
	"testing"
)

func TestInlineWrapUnits(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty content", "", nil},
		{"whitespace only", "   \t\n  ", nil},
		{"single word", "foo", []string{"foo"}},
		{"two words", "foo bar", []string{"foo", "bar"}},
		{"three words separated by tabs and newlines", "a\tb\nc", []string{"a", "b", "c"}},
		{"code span with internal whitespace stays one unit", "see `code with spaces`", []string{"see", "`code with spaces`"}},
		{"adjacent text and code span merge", "foo`bar baz`", []string{"foo`bar baz`"}},
		{"unterminated code span is plain text", "see `unclosed and more", []string{"see", "`unclosed", "and", "more"}},
		{"inline link with internal space", "click [link text](http://x) here", []string{"click", "[link text](http://x)", "here"}},
		{"image with title", "see ![alt text](src.png \"my title\")", []string{"see", "![alt text](src.png \"my title\")"}},
		{"reference link", "see [label][ref] here", []string{"see", "[label][ref]", "here"}},
		{"collapsed reference link", "see [label][] here", []string{"see", "[label][]", "here"}},
		{"shortcut reference link", "see [label] here", []string{"see", "[label]", "here"}},
		{"uri autolink", "go to <https://example.com> now", []string{"go", "to", "<https://example.com>", "now"}},
		{"email autolink", "mail <user@example.com> please", []string{"mail", "<user@example.com>", "please"}},
		{"angle brackets without scheme are not autolinks", "see <plain> tag", []string{"see", "<plain>", "tag"}},
		{"escaped bracket is plain text", "foo \\[ bar", []string{"foo", "\\[", "bar"}},
		{"emphasis is not atomic", "*foo bar baz*", []string{"*foo", "bar", "baz*"}},
		{"strong emphasis is not atomic", "**foo bar**", []string{"**foo", "bar**"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InlineWrapUnits([]byte(tc.in))

			if len(got) != len(tc.want) {
				t.Fatalf("got %d units %s, want %d %v", len(got), bytesSliceToStrings(got), len(tc.want), tc.want)
			}

			for i, u := range got {
				if string(u) != tc.want[i] {
					t.Fatalf("unit[%d]=%q, want %q", i, string(u), tc.want[i])
				}
			}
		})
	}
}

func TestSplitHardBreaks(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		want       []string
		wantBreaks []string
	}{
		{"no break", "a\nb", []string{"a\nb"}, nil},
		{"two spaces", "a  \nb", []string{"a", "b"}, []string{"  "}},
		{"three spaces", "a   \nb", []string{"a", "b"}, []string{"   "}},
		{"one space is soft", "a \nb", []string{"a \nb"}, nil},
		{"backslash", "a\\\nb", []string{"a", "b"}, []string{"\\"}},
		{"escaped backslash is soft", "a\\\\\nb", []string{"a\\\\\nb"}, nil},
		{"three backslashes break", "a\\\\\\\nb", []string{"a\\\\", "b"}, []string{"\\"}},
		{"two breaks", "a  \nb  \nc", []string{"a", "b", "c"}, []string{"  ", "  "}},
		{"break at the end is prose", "a  ", []string{"a  "}, nil},
		{"backslash at the end is prose", "a\\", []string{"a\\"}, nil},
		{"mixed forms", "a  \nb\\\nc", []string{"a", "b", "c"}, []string{"  ", "\\"}},
		{"soft break between hard ones", "a  \nb\nc  \nd", []string{"a", "b\nc", "d"}, []string{"  ", "  "}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, breaks := SplitHardBreaks([]byte(tc.in))

			if gotStrings := bytesSliceToStrings(got); !slices.Equal(gotStrings, tc.want) {
				t.Fatalf("segments = %q, want %q", gotStrings, tc.want)
			}

			if gotBreaks := bytesSliceToStrings(breaks); !slices.Equal(gotBreaks, tc.wantBreaks) {
				t.Fatalf("breaks = %q, want %q", gotBreaks, tc.wantBreaks)
			}
		})
	}
}

func bytesSliceToStrings(b [][]byte) []string {
	out := make([]string, len(b))

	for i, x := range b {
		out[i] = string(x)
	}

	return out
}
