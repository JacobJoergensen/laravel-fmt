package textwidth

import "testing"

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
