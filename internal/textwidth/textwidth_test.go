package textwidth

import (
	"strings"
	"testing"
)

// printWidth is the formatter's central layout promise and it was counting
// runes. The error ran both ways: CJK and emoji measured about half their real
// width and were left overflowing the line, while combining marks measured
// nearly double and broke a line that would have fit.
func TestRuneColumns(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want int
	}{
		{"ascii", 'a', 1},
		{"latin-1", 'é', 1},
		{"cjk ideograph", '测', 2},
		{"hangul syllable", '한', 2},
		{"hiragana", 'あ', 2},
		{"fullwidth latin", 'Ａ', 2},
		{"emoji", '🙂', 2},
		{"combining acute", '́', 0},
		{"combining enclosing circle", '⃝', 0},
		{"zero width joiner", '‍', 0},
		{"zero width space", '​', 0},
		{"greek", 'α', 1},
		{"cyrillic", 'д', 1},
		{"box drawing", '─', 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Rune(tc.r); got != tc.want {
				t.Fatalf("Rune(%q) = %d, want %d", tc.r, got, tc.want)
			}
		})
	}
}

// The table was a hand-written list of blocks, so every Wide character a block
// boundary missed measured one column and the line it sat on overflowed
// printWidth. One representative from each range the sweep found missing, plus
// the soft hyphen the ASCII fast path used to claim as a painted column.
func TestRuneCoversTheWholeWidthProperty(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want int
	}{
		{"transport and map", 0x1F680, 2},      // 🚀
		{"colored shapes", 0x1F7E0, 2},         // 🟠
		{"pictographs extended-a", 0x1FA70, 2}, // 🩰
		{"enclosed cjk", 0x1F210, 2},           // 🈐
		{"enclosed alphanumeric", 0x1F191, 2},  // 🆑
		{"zodiac", 0x2648, 2},                  // ♈
		{"trigram", 0x2630, 2},                 // ☰
		{"yijing hexagram", 0x4DC0, 2},         // ䷀
		{"tangut", 0x17000, 2},
		{"kana supplement", 0x1B000, 2},
		{"tai xuan jing", 0x1D300, 2},
		{"vietnamese reading mark", 0x16FF0, 2},
		{"soft hyphen paints nothing", 0x00AD, 0},
		{"neutral stays one column", 0x1F5A5, 1},  // 🖥
		{"halfwidth katakana", 0xFF66, 1},         // ｦ
		{"ambiguous stays one column", 0x00A1, 1}, // ¡
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Rune(tc.r); got != tc.want {
				t.Fatalf("Rune(U+%04X) = %d, want %d", tc.r, got, tc.want)
			}
		})
	}
}

// Rune runs per character of every line the formatter measures, so its cost is
// paid on every layout decision. ASCII is the case that matters most and exits
// on the first comparison; the rest binary-search the generated table.
func BenchmarkRune(b *testing.B) {
	// A line of source with the shapes a real template mixes.
	line := "public function handle(): string // 測定 🚀 café\n"

	b.ReportAllocs()

	cols := 0

	for b.Loop() {
		for _, r := range line {
			cols += Rune(r)
		}
	}

	_ = cols
}

// The three measurements the sweep reproduced against the built binary, as
// column counts rather than rune counts.
func TestStringColumnsMatchWhatATerminalPaints(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want int
	}{
		{"70 cjk characters", strings.Repeat("测", 70), 140},
		{"55 emoji", strings.Repeat("🙂", 55), 110},
		{"60 combining pairs", strings.Repeat("é", 60), 60},
		{"ascii is unchanged", strings.Repeat("a", 40), 40},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cols := 0
			for _, r := range tc.s {
				cols += Rune(r)
			}

			if cols != tc.want {
				t.Fatalf("columns = %d, want %d", cols, tc.want)
			}
		})
	}
}
