package formatter

import (
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

const (
	testHeadPrefix     = "head "
	blankLineRoundTrip = "a\n\nb"
)

func writerCfg() *config.File {
	cfg := config.Default()
	cfg.PrintWidth = 20
	cfg.TabWidth = 4
	cfg.IndentStyle = config.IndentStyleSpace
	cfg.EndOfLine = config.EndOfLineLF

	return cfg
}

func TestWriteAdvancesColumn(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Write("hello")

	if got := w.Column(); got != 5 {
		t.Fatalf("Column=%d, want 5", got)
	}

	if got := string(w.Bytes()); got != "hello" {
		t.Fatalf("Bytes=%q, want %q", got, "hello")
	}
}

func TestNewlineEmitsConfiguredEOL(t *testing.T) {
	cases := []struct {
		name string
		eol  string
		want string
	}{
		{"lf", config.EndOfLineLF, "a\n"},
		{"crlf", config.EndOfLineCRLF, "a\r\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := writerCfg()
			cfg.EndOfLine = tc.eol

			w := NewWriter(cfg)
			w.Write("a")
			w.Newline()

			if got := string(w.Bytes()); got != tc.want {
				t.Fatalf("Bytes=%q, want %q", got, tc.want)
			}

			if got := w.Column(); got != 0 {
				t.Fatalf("Column=%d, want 0 after Newline", got)
			}
		})
	}
}

func TestIndentUsesTabsOrSpaces(t *testing.T) {
	cases := []struct {
		name         string
		useTabs      bool
		tabWidth     int
		levels       int
		wantLead     string
		wantLeadCols int
	}{
		{"spaces tabWidth=4 one level", false, 4, 1, "    ", 4},
		{"spaces tabWidth=2 three levels", false, 2, 3, "      ", 6},
		{"tabs tabWidth=4 one level", true, 4, 1, "\t", 4},
		{"tabs tabWidth=8 two levels", true, 8, 2, "\t\t", 16},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := writerCfg()
			cfg.IndentStyle = config.IndentStyleFor(tc.useTabs)
			cfg.TabWidth = tc.tabWidth

			w := NewWriter(cfg)

			for range tc.levels {
				w.Indent()
			}

			w.Newline()
			w.Write("x")

			want := "\n" + tc.wantLead + "x"

			if got := string(w.Bytes()); got != want {
				t.Fatalf("Bytes=%q, want %q", got, want)
			}

			if got := w.Column(); got != tc.wantLeadCols+1 {
				t.Fatalf("Column=%d, want %d", got, tc.wantLeadCols+1)
			}
		})
	}
}

func TestDedentBelowZeroIsNoOp(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Dedent()
	w.Dedent()
	w.Indent()
	w.Newline()
	w.Write("x")

	if got := string(w.Bytes()); got != "\n    x" {
		t.Fatalf("Bytes=%q, want %q (indent underflowed)", got, "\n    x")
	}
}

func TestTryFlatCommitsWhenWithinPrintWidth(t *testing.T) {
	cfg := writerCfg()
	cfg.PrintWidth = 10

	w := NewWriter(cfg)
	w.Write(testHeadPrefix)

	fit := w.TryFlat(func() {
		w.Write("short")
	})

	if !fit {
		t.Fatalf("TryFlat=false, want true (content fits within printWidth)")
	}

	if got := string(w.Bytes()); got != "head short" {
		t.Fatalf("Bytes=%q, want %q", got, "head short")
	}
}

func TestTryOneLineKeepsOverflowButRollsBackALineBreak(t *testing.T) {
	cfg := writerCfg()
	cfg.PrintWidth = 10

	w := NewWriter(cfg)
	w.Write(testHeadPrefix)

	if !w.TryOneLine(func() { w.Write("this-is-too-long-to-fit") }) {
		t.Fatalf("TryOneLine=false for an overflowing single line, want true")
	}

	kept := string(w.Bytes())

	if w.TryOneLine(func() {
		w.Write("a")
		w.Newline()
		w.Write("b")
	}) {
		t.Fatalf("TryOneLine=true for output with a line break, want false")
	}

	if got := string(w.Bytes()); got != kept {
		t.Fatalf("Bytes=%q, want %q (the line break was not rolled back)", got, kept)
	}
}

func TestTryFlatRollsBackOnOverflow(t *testing.T) {
	cfg := writerCfg()
	cfg.PrintWidth = 10

	w := NewWriter(cfg)
	w.Write(testHeadPrefix)

	fit := w.TryFlat(func() {
		w.Write("this-is-too-long-to-fit")
	})

	if fit {
		t.Fatalf("TryFlat=true, want false (overflow expected)")
	}

	if got := string(w.Bytes()); got != testHeadPrefix {
		t.Fatalf("Bytes=%q, want %q (flat emission was not rolled back)", got, testHeadPrefix)
	}

	if got := w.Column(); got != 5 {
		t.Fatalf("Column=%d, want 5 (column was not restored)", got)
	}
}

func TestTryFlatRollsBackOnInnerNewline(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Write(testHeadPrefix)

	fit := w.TryFlat(func() {
		w.Write("a")
		w.Newline()
		w.Write("b")
	})

	if fit {
		t.Fatalf("TryFlat=true, want false (newline inside flat must force rollback)")
	}

	if got := string(w.Bytes()); got != testHeadPrefix {
		t.Fatalf("Bytes=%q, want %q", got, testHeadPrefix)
	}
}

func TestWithTrailingTightensFlatFit(t *testing.T) {
	cfg := writerCfg()
	cfg.PrintWidth = 10

	w := NewWriter(cfg)
	w.Write(testHeadPrefix)

	if fit := w.TryFlat(func() { w.Write("12345") }); !fit {
		t.Fatalf("baseline TryFlat=false, want true (col 10 == printWidth)")
	}

	w2 := NewWriter(cfg)
	w2.Write(testHeadPrefix)

	var fit bool
	w2.WithTrailing(1, func() {
		fit = w2.TryFlat(func() { w2.Write("12345") })
	})

	if fit {
		t.Fatalf("TryFlat under WithTrailing(1)=true, want false (col 10 + 1 > printWidth)")
	}
}

func TestWithTrailingClearedByNewline(t *testing.T) {
	cfg := writerCfg()
	cfg.PrintWidth = 10

	w := NewWriter(cfg)

	var fit bool
	w.WithTrailing(1, func() {
		w.Newline()
		fit = w.TryFlat(func() { w.Write("1234567890") })
	})

	if !fit {
		t.Fatalf("TryFlat after Newline=false, want true (a real Newline must clear the trailing reserve)")
	}
}

func TestTryFlatRestoresLineTrailing(t *testing.T) {
	w := NewWriter(writerCfg())

	w.WithTrailing(3, func() {
		w.TryFlat(func() {
			w.Write("x")
			w.Newline()
		})

		if w.lineTrailing != 3 {
			t.Fatalf("lineTrailing=%d after a rolled-back probe, want 3 (not restored)", w.lineTrailing)
		}
	})

	if w.lineTrailing != 0 {
		t.Fatalf("lineTrailing=%d after WithTrailing, want 0 (not restored)", w.lineTrailing)
	}
}

func TestWithoutIndentDropsAndRestores(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Indent()
	w.Indent()

	w.WithoutIndent(func() {
		w.Newline()
		w.Write("raw")
	})

	w.Newline()
	w.Write("back")

	want := "\nraw\n        back"

	if got := string(w.Bytes()); got != want {
		t.Fatalf("Bytes=%q, want %q", got, want)
	}
}

func TestWithAlignAddsColumnsPastTheIndent(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Indent()
	w.Write("- a")

	w.WithAlign(2, func() {
		w.Newline()
		w.Write("b")

		if got := w.IndentColumn(); got != 6 {
			t.Fatalf("IndentColumn=%d, want 6", got)
		}
	})

	w.Newline()
	w.Write("c")

	want := "- a\n      b\n    c"

	if got := string(w.Bytes()); got != want {
		t.Fatalf("Bytes=%q, want %q", got, want)
	}
}

func TestRequestBlankLineInsertsOneBlankAfterNewline(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Write("a")
	w.Newline()
	w.RequestBlankLine()
	w.RequestBlankLine()
	w.Write("b")

	if got := string(w.Bytes()); got != blankLineRoundTrip {
		t.Fatalf("Bytes=%q, want %q", got, blankLineRoundTrip)
	}
}

// A deferred blank sits above the next emission, so the one-line checks must
// not read its line break as the emission's own.
func TestOneLineChecksIgnoreADeferredBlank(t *testing.T) {
	cases := []struct {
		name  string
		check func(w *Writer) bool
		want  string
	}{
		{"MeasureOneLine rolls back", func(w *Writer) bool { return w.MeasureOneLine(func() { w.Write("b") }) }, "a\n"},
		{"TryOneLine keeps output", func(w *Writer) bool { return w.TryOneLine(func() { w.Write("b") }) }, blankLineRoundTrip},
		{"TryOneLine with nothing written", func(w *Writer) bool { return w.TryOneLine(func() {}) }, "a\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWriter(writerCfg())
			w.Write("a")
			w.Newline()
			w.RequestBlankLine()

			if !tc.check(w) {
				t.Fatalf("one-line check=false for a single-line emission after a blank, want true")
			}

			if got := string(w.Bytes()); got != tc.want {
				t.Fatalf("Bytes=%q, want %q", got, tc.want)
			}

			w.Newline()
			w.Write("c")

			if got := string(w.Bytes()); strings.Contains(got, "\n\n\n") {
				t.Fatalf("Bytes=%q: the deferred blank was written twice", got)
			}
		})
	}
}

func TestRequestBlankLineSkippedAtStartOfBuffer(t *testing.T) {
	w := NewWriter(writerCfg())
	w.RequestBlankLine()
	w.Write("a")

	if got := string(w.Bytes()); got != "a" {
		t.Fatalf("Bytes=%q, want %q", got, "a")
	}
}

func TestRequestBlankLineMidLineTerminatesFirst(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Write("a")
	w.RequestBlankLine()
	w.Write("b")

	if got := string(w.Bytes()); got != blankLineRoundTrip {
		t.Fatalf("Bytes=%q, want %q", got, blankLineRoundTrip)
	}
}

func TestRequestBlankLineSatisfiedByNewline(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Write("a")
	w.Newline()
	w.RequestBlankLine()
	w.Newline()
	w.Write("b")

	if got := string(w.Bytes()); got != blankLineRoundTrip {
		t.Fatalf("Bytes=%q, want %q", got, blankLineRoundTrip)
	}
}

func TestRequestBlankLineForcesFlatOverflow(t *testing.T) {
	w := NewWriter(writerCfg())
	w.Write(testHeadPrefix)

	fit := w.TryFlat(func() {
		w.Write("a")
		w.RequestBlankLine()
		w.Write("b")
	})

	if fit {
		t.Fatalf("TryFlat=true, want false (blank-line request inside flat must force rollback)")
	}

	if got := string(w.Bytes()); got != testHeadPrefix {
		t.Fatalf("Bytes=%q, want %q (flat emission was not rolled back)", got, testHeadPrefix)
	}
}

func TestTryFlatNestedComposes(t *testing.T) {
	cfg := writerCfg()
	cfg.PrintWidth = 12

	w := NewWriter(cfg)
	w.Write("a ")

	outerFit := w.TryFlat(func() {
		w.Write("b ")

		innerFit := w.TryFlat(func() {
			w.Write("cccccccccc")
		})

		if innerFit {
			t.Fatalf("inner TryFlat=true, want false (overflow expected)")
		}

		w.Write("d")
	})

	if !outerFit {
		t.Fatalf("outer TryFlat=false, want true (inner rollback must not taint outer fit)")
	}

	if got := string(w.Bytes()); got != "a b d" {
		t.Fatalf("Bytes=%q, want %q", got, "a b d")
	}
}

func TestMeasureAlwaysRollsBack(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantFit bool
	}{
		{"fitting output", "short", true},
		{"overflowing output", "this-is-too-long-to-fit", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWriter(writerCfg())
			w.Write(testHeadPrefix)

			fit := w.Measure(func() {
				w.Write(tc.content)
			})

			if fit != tc.wantFit {
				t.Fatalf("Measure=%v, want %v", fit, tc.wantFit)
			}

			if got := string(w.Bytes()); got != testHeadPrefix {
				t.Fatalf("Bytes=%q, want %q (measured output was kept)", got, testHeadPrefix)
			}

			if got := w.Column(); got != len(testHeadPrefix) {
				t.Fatalf("Column=%d, want %d", got, len(testHeadPrefix))
			}

			w.Write("x")

			if got := string(w.Bytes()); got != testHeadPrefix+"x" {
				t.Fatalf("Bytes=%q after a later Write, want %q (flat state leaked)", got, testHeadPrefix+"x")
			}
		})
	}
}
