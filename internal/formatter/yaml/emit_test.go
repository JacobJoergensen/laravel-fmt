package yaml

import (
	"bytes"
	"cmp"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".yml", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".yml")
			want := formattertest.ReadFixture(t, name+".formatted.yml")

			got := mustFormat(t, input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".yml", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".yml")

			once := mustFormat(t, input, config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestFormatCRLFRoundTrips(t *testing.T) {
	cfg := config.Default()
	cfg.EndOfLine = config.EndOfLineCRLF

	input := []byte("a: 1\r\nb: 2\r\n")

	once := mustFormat(t, input, cfg)
	if !bytes.Equal(once, input) {
		t.Fatalf("CRLF not round-tripped.\n=== got ===\n%q\n=== want ===\n%q", once, input)
	}

	twice := mustFormat(t, once, cfg)
	if !bytes.Equal(twice, once) {
		t.Fatalf("CRLF format not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
	}
}

func TestFormatLoneCRIsIdempotent(t *testing.T) {
	cases := []string{
		"---\r",
		" ---\r ",
		"a: 1\rb: 2\r",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("lone-CR format not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

// YAML 1.2 ends a line on a lone CR as on an LF, and js-yaml reads every pair
// below as one identical document.
func TestFormatLoneCRMatchesLineFeedTwin(t *testing.T) {
	cases := []struct {
		name string
		cr   string
		lf   string
	}{
		{"tagged sequence value with continuation", "\r- !  \r   0", "\n- !  \n   0"},
		{"wider dash spacing", "\r-  !\r    0", "\n-  !\n    0"},
		{"leading comment before the sequence", "# \r- !\r   0", "# \n- !\n   0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCR := mustFormat(t, []byte(tc.cr), config.Default())
			gotLF := mustFormat(t, []byte(tc.lf), config.Default())

			if !bytes.Equal(gotCR, gotLF) {
				t.Fatalf("lone-CR output differs from its LF twin.\n=== CR ===\n%q\n=== LF ===\n%q", gotCR, gotLF)
			}

			if twice := mustFormat(t, gotCR, config.Default()); !bytes.Equal(twice, gotCR) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", gotCR, twice)
			}
		})
	}
}

func TestFormatNestedFlowIsIdempotent(t *testing.T) {
	cases := []string{
		"- : {\n}",
		"- {\n}",
		"a:\n  b: {\n}",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("nested flow not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatBlockScalarIndentIsAFixedPoint(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // empty means the source is already the fixed point
	}{
		{name: "folded under a sequence item", src: "- a: >\n   0\n", want: "- a: >\n    0\n"},
		{name: "literal under a sequence item", src: "- a: |\n   0\n", want: "- a: |\n    0\n"},
		{name: "empty key under a sequence item", src: "- : >\n   0\n", want: "- : >\n    0\n"},
		{name: "explicit indentation indicator", src: "- a: |2\n   0\n"},
		{name: "digit in the header comment", src: "a: | # v2\n    0\n", want: "a: | # v2\n  0\n"},
		{name: "keep chomping", src: "- a: |+\n   0\n", want: "- a: |+\n    0\n"},
		{name: "body one column past a nested mapping", src: ":\n : >\n  0 \n", want: ":\n  : >\n    0 \n"},
		{name: "body on the dash line", src: "- >\n   0\n", want: "- >\n  0\n"},
		{name: "plain key at the document root", src: "a: >\n   0\n", want: "a: >\n  0\n"},
		{name: "nested mapping", src: "k:\n  a: >\n     0\n", want: "k:\n  a: >\n    0\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := cmp.Or(tc.want, tc.src)
			once := mustFormat(t, []byte(tc.src), config.Default())

			if string(once) != want {
				t.Fatalf("block scalar body moved on the first pass.\n=== want ===\n%q\n=== out ===\n%q", want, once)
			}

			twice := mustFormat(t, once, config.Default())
			if !bytes.Equal(twice, once) {
				t.Fatalf("block scalar not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestBlockScalarBodySitsOneIndentUnderItsOwner(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"deeper line keeps its offset", "a: |\n    b\n      c\n    d\n", "a: |\n  b\n    c\n  d\n"},
		{"shallow body moves right", "k:\n  a: >\n   b\n", "k:\n  a: >\n    b\n"},
		{"kept blank lines stay", "a: |+\n    b\n\n\nc: d\n", "a: |+\n  b\n\n\nc: d\n"},
		{"indicator body keeps its value", "k:\n    a: |4\n            b\n        c\n", "k:\n  a: |4\n          b\n      c\n"},
		{"indicator body on a dash keeps its value", "k:\n    - |4\n            b\n        c\n", "k:\n  - |4\n          b\n      c\n"},
		{
			"dash body leaves the file formatted",
			"steps:\n  - |\n    echo hi\n  - name:    build\n",
			"steps:\n  - |\n    echo hi\n  - name: build\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestDashMappingKeysLineUpAtAnyIndentWidth(t *testing.T) {
	cfg := config.Default()
	cfg.TabWidth = 4
	cfg.IndentExplicit = true

	src := "steps:\n  - name: a\n    uses: b\n    with:\n      k: v\n    run: |\n      echo\n  - |\n    text\n"
	want := "steps:\n    - name: a\n      uses: b\n      with:\n          k: v\n      run: |\n          echo\n    - |\n        text\n"

	got := mustFormat(t, []byte(src), cfg)
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatAnchoredValueKeepsItsAbsorbedLines(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"nested one level", ":\n : !\n  :  0"},
		{"under a sequence item", "- : !\n   0"},
		// Directly on the dash, where the value's base is the Writer's indent plus
		// the `- ` the emitter writes, not the indent alone.
		{"value on the dash line", "- !\n     0"},
		{"two absorbed lines", ":\n : !\n   deep\n    deeper"},
		{"bailed line", ":\n : @\n  x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("absorbed lines moved out of the node.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}

			// The tail must still be indented past the node's own line, or it is no
			// longer part of the node at all.
			lines := bytes.Split(bytes.TrimRight(once, "\n"), []byte("\n"))
			if len(lines) < 2 {
				t.Fatalf("expected the absorbed lines to survive: %q", once)
			}

			head := len(lines[len(lines)-2]) - len(bytes.TrimLeft(lines[len(lines)-2], " "))
			tail := len(lines[len(lines)-1]) - len(bytes.TrimLeft(lines[len(lines)-1], " "))

			if tail <= head {
				t.Fatalf("tail indent %d does not clear its node's %d.\n=== out ===\n%q", tail, head, once)
			}
		})
	}
}

func TestFormatLoneCRInRawSpanIsALineBreak(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"anchored block", ":\n : !\r  :"},
		{"anchor value", "a: &x\r  b"},
		{"block scalar body", "- a: >\r   0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cr := mustFormat(t, []byte(tc.src), config.Default())
			lf := mustFormat(t, []byte(strings.ReplaceAll(tc.src, "\r", "\n")), config.Default())

			if !bytes.Equal(cr, lf) {
				t.Fatalf("lone CR formatted unlike its LF twin.\n=== CR ===\n%q\n=== LF ===\n%q", cr, lf)
			}

			twice := mustFormat(t, cr, config.Default())
			if !bytes.Equal(twice, cr) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", cr, twice)
			}
		})
	}
}

// js-yaml folds the break either way, so re-homing the continuation line is
// layout, not value.
func TestFormatMultilineQuotedScalarFollowsItsKey(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"double quoted",
			"root:\n      k: \"one\n         two\"\n",
			"root:\n  k: \"one\n     two\"\n",
		},
		{
			"single quoted",
			"root:\n      k: 'one\n         two'\n",
			"root:\n  k: 'one\n     two'\n",
		},
		{
			"three lines keep their relative alignment",
			"root:\n      k: \"one\n         two\n          three\"\n",
			"root:\n  k: \"one\n     two\n      three\"\n",
		},
		// Already at the canonical indent, so nothing moves.
		{
			"already canonical",
			"root:\n  k: \"one\n     two\"\n",
			"root:\n  k: \"one\n     two\"\n",
		},
		// One line is the common shape and is untouched.
		{"single line", "root:\n      k: \"one\"\n", "root:\n  k: \"one\"\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q -> %q", got, twice)
			}
		})
	}
}

func TestFormatBlockScalarKeepsTrailingSpace(t *testing.T) {
	// js-yaml reads `|⏎ 0   ` as "0   \n", so the last line's trailing spaces
	// are value; a run holding a line break is the emitter's own padding and
	// still goes.
	cases := []string{
		"- a: |\n    0  \n    1\n",
		"|\n 0  \n 1\n",
		"|\n 0  \n",
		"|\n 0  ",
		"|\n 0\t",
		"a: &x |\n  0  \n  1\nb: 2\n",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			want := src
			if !strings.HasSuffix(want, "\n") {
				want += "\n"
			}

			got := mustFormat(t, []byte(src), config.Default())
			if string(got) != want {
				t.Fatalf("trailing space in a literal block scalar was rewritten.\n=== in ===\n%q\n=== got ===\n%q\n=== want ===\n%q", src, got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatIndentedMarkerIsContent(t *testing.T) {
	cases := []string{
		" ...\r ",
		" ...\n",
		" ---\n",
		"...\n",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("indented marker not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatTrailingWhitespaceDocIsIdempotent(t *testing.T) {
	cases := []string{
		"\r...\r ",
		"...\r ",
		"a: 1\n... ",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("trailing-ws doc not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatAnchoredValueIsIdempotent(t *testing.T) {
	cases := []string{
		"- : !\n   0",
		"k: !\n  0",
		"- !\n  0",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("anchored value not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatTrailingCommentNotAbsorbed(t *testing.T) {
	idempotent := []string{
		":\n : !\n  #",
		"k: &a\n  # interior\n  x: 1",
		"k: &a\n  x: 1\n  y: 2",
	}

	for _, src := range idempotent {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatMarkerLineContentStaysContent(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"marker as content", "--- ---", "--- ---\n"},
		{"end marker as content", "--- ...", "--- ...\n"},
		{"between two markers", "... --- ...", "... --- ...\n"},
		// Controls: a body the parser can model still drops to its own line, and
		// markers that do start their line still are markers.
		{"mapping body", "--- a: 1", "---\na: 1\n"},
		{"sequence body", "--- - a", "---\n- a\n"},
		{"two empty documents", "---\n---", "---\n---\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestFormatFrozenFileDropsLeadingBlankLines(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"lone CR opens the file", "\r- !\r   0", "- !\n   0\n"},
		{"blank line opens the file", "\n- !\n   0", "- !\n   0\n"},
		{"several blank lines", "\n\n \n- !\n   0", "- !\n   0\n"},
		{"no leading blank", "- !\n   0", "- !\n   0\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

// A lone CR before a CRLF is two line breaks: inside a multi-line plain scalar
// the blank line folds to a newline where a single break folds to a space.
func TestFormatFrozenSpanKeepsLoneCRLines(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"lone CR then CRLF", "0\r\r\n0", "0\n\n0\n"},
		{"lone CR then CRLF, indented tail", "0\r\r\n 0", "0\n\n 0\n"},
		{"lone CR alone is a line break", "0\r0", "0\n0\n"},
		// Controls: neither may move.
		{"CRLF blank line", "0\r\n\r\n0", "0\n\n0\n"},
		{"no CR at all", "0\n\n0", "0\n\n0\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

// A dash at an item's own column is the next sibling; only a mapping key lets
// a sequence start at the parent's column.
func TestFormatKeepsEmptyItemSiblings(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"empty item between siblings", "- x\n-\n- y\n", "- x\n-\n- y\n"},
		{"two empty items", "-\n-\n", "-\n-\n"},
		{"staircase", "-\n-\n-\n", "-\n-\n-\n"},
		{"inside a mapping", "a:\n  - x\n  -\n  - y\n", "a:\n  - x\n  -\n  - y\n"},
		{"empty item then comment", "-\n-\n# c\n", "-\n-\n# c\n"},
		// Under a mapping key a sequence does start at the key's own column.
		{"sequence shares a key's column", "a:\n- x\n- y\n", "a:\n  - x\n  - y\n"},
		{"nested sequence still nests", "-\n  - inner\n", "-\n  - inner\n"},
		{"nested mapping still nests", "-\n  a: 1\n", "- a: 1\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestFormatKeepsItemFirstEntryTrivia(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"comment", "-\n  # note\n  a: 1\n", "-\n  # note\n  a: 1\n"},
		{"blank line", "-\n\n  a: 1\n", "-\n\n  a: 1\n"},
		{"no trivia stays compact", "-\n  a: 1\n", "- a: 1\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestFormatMarkerLookalikeKey(t *testing.T) {
	cases := []string{
		" --- :", " ... :", `"---": x`, "k: ---", "a:\n    ---: x",
		" ... 0: | ", " ... 0:    x", "...x: 1", "a:\n    ... 0: x",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}

	if out := mustFormat(t, []byte(" --- :"), config.Default()); !bytes.HasPrefix(out, []byte(" ---")) {
		t.Fatalf("marker key not frozen with indent: %q", out)
	}

	// Dedented to column 0 this key is a document-end marker and `0: x` starts a
	// new document, so the key itself is lost.
	if out := mustFormat(t, []byte(" ... 0:    x"), config.Default()); !bytes.HasPrefix(out, []byte(" ... 0:    x")) {
		t.Fatalf("dots-prefixed key not frozen with indent: %q", out)
	}

	// Without the space the dots are ordinary key bytes, so nothing freezes.
	if out := mustFormat(t, []byte("...x:    1"), config.Default()); !bytes.Equal(out, []byte("...x: 1\n")) {
		t.Fatalf("dots key without a marker not formatted normally: %q", out)
	}

	for _, tc := range []struct{ src, want string }{
		{"a:\n    ---: x", "a:\n  ---: x\n"},
		{"a:\n    ... 0: x", "a:\n  ... 0: x\n"},
	} {
		if out := mustFormat(t, []byte(tc.src), config.Default()); string(out) != tc.want {
			t.Fatalf("nested marker key not formatted normally: got %q, want %q", out, tc.want)
		}
	}
}

func TestFormatRejectsTabIndentedKey(t *testing.T) {
	cases := []struct{ name, src, wantPos string }{
		{"tab after a space indent", "-\n \t:", "2:2"},
		{"tab at column zero", ":\n\t- :", "2:1"},
		{"tab under a sequence item", "- :\n  \t- :", "2:3"},
		{"marker lookalike key", "\t---: 1\n", "1:1"},
		{"block scalar header", "\t: >\n  0\n", "1:1"},
		// A line that forms no mapping entry is just as invalid.
		{"tab before a dash", "\t- 1\n", "1:1"},
		{"tab before a scalar", "\tx\n", "1:1"},
		{"tab before a nested dash", "- a\n\t- b\n", "2:1"},
		{"tab under a key", "a:\n\tb: 1\n", "2:1"},
		{"tab before a comment", "\t#x: 1\n", "1:1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Format([]byte(tc.src), config.Default())
			if err == nil {
				t.Fatal("tab indentation accepted; want a parse error")
			}

			if want := tc.wantPos + ": tab in indentation"; !strings.Contains(err.Error(), want) {
				t.Fatalf("got %q, want it to contain %q", err, want)
			}
		})
	}
}

// Every case here is accepted by symfony/yaml.
func TestFormatAcceptsTabsOutsideIndentation(t *testing.T) {
	cases := []struct{ name, src string }{
		{"after a colon", "a:\tb\n"},
		{"after a dash", "-\tx\n"},
		{"after a dash and a space", "- \tx\n"},
		{"trailing", "a: b\t\n"},
		{"inside a comment", "#\tc\na: 1\n"},
		{"before a comment", "a: 1\t# c\n"},
		{"inside a quoted key", "\"a\tb\": 1\n"},
		{"inside a block scalar", "a: |\n  x\n  \ty\n"},
		// A flow collection's continuation lines are separation, not indentation,
		// so a tab leading one is legal (Symfony parses this).
		{"inside a flow collection", "a: [\n\t1,\n\t2 ]\n"},
		// Symfony rejects a tab-only line too. A line without content carries no
		// structure, and a stray tab on a blank line is an editor artifact, so
		// failing a whole file over one would be a false positive.
		{"blank line", "a: 1\n\t\nb: 2\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatRejectsForbiddenControlBytes(t *testing.T) {
	if _, err := Format([]byte("a: b\x00c"), config.Default()); err == nil {
		t.Fatal("NUL byte not rejected")
	}

	if _, err := Format([]byte("a:\tb\r\nc: d\n"), config.Default()); err != nil {
		t.Fatalf("valid tab/CR/LF rejected: %v", err)
	}
}

func TestFormatVerbatimTrailingWhitespace(t *testing.T) {
	idempotent := []string{":\n : ! \n  :", "k: &a\n  x: 1\n  y: 2"}

	for _, src := range idempotent {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}

	if out := mustFormat(t, []byte("k: |\n  a  \n  b"), config.Default()); !bytes.Contains(out, []byte("a  \n")) {
		t.Fatalf("block scalar trailing whitespace not preserved: %q", out)
	}
}

func TestFormatOnFormattedIsStable(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".yml", false) {
		t.Run(name, func(t *testing.T) {
			want := formattertest.ReadFixture(t, name+".formatted.yml")

			got := mustFormat(t, want, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("formatted input was not stable.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatEmptyInput(t *testing.T) {
	out, err := Format([]byte("   \n\t  "), config.Default())
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	if len(out) != 0 {
		t.Fatalf("Format returned %q, want empty", out)
	}
}

func mustFormat(t *testing.T, src []byte, cfg *config.File) []byte {
	t.Helper()

	out, err := Format(src, cfg)
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	return out
}

// Three dashes with no space after them are not a marker, so that key stays a
// scalar and is formatted normally.
func TestColumnZeroMarkerContentIsFrozen(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"dash_marker_inline", " --- 0:", " --- 0:\n"},
		{"dash_marker_inline_value", " --- 0: v", " --- 0: v\n"},
		{"dots_marker_inline", " ... 0:", " ... 0:\n"},
		{"dashes_no_space_stays_scalar", " ---foo:", "---foo:\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

// Keep chomping (`|+`, `>+`) makes the trailing line breaks part of the value,
// so the blank-line rules must not run inside the body. `|` and `|-` would hide
// a failure here, since their chomping discards those breaks.
func TestKeepChompedBlockScalarKeepsItsBlankLines(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"run mid document", "top: 1\na: |+\n  k\n\n\n\n\nb: 2\n"},
		{"trailing at eof", "top: 1\na: |+\n  k\n\n\n"},
		{"single blank mid document", "a: |+\n  k\n\nb: 2\n"},
		{"folded keep", "a: >+\n  fold\n\n"},
		{"nested in a sequence", "list:\n  - a: |+\n      k\n\n\n"},
		{"anchored mid document", "a: &x |+\n  k\n\n\nb: 2\n"},
		{"tagged item at eof", "- !!str |+\n  k\n\n\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.src {
				t.Fatalf("the block scalar body changed.\n=== in ===\n%q\n=== out ===\n%q", tc.src, got)
			}
		})
	}
}

// Clip and strip chomping discard the trailing breaks, so collapsing them there is
// invisible and stays the formatter's business.
func TestClipAndStripBlockScalarsStillCollapse(t *testing.T) {
	cases := map[string]string{
		"a: |\n  k\n\n\n\n":             "a: |\n  k\n",
		"a: |-\n  k\n\n\n":              "a: |-\n  k\n",
		"a: >\n  fold\n\n\n":            "a: >\n  fold\n",
		"a: | # c++\n  k\n\n\n\nb: 2\n": "a: | # c++\n  k\n\nb: 2\n",
	}

	for src, want := range cases {
		got := mustFormat(t, []byte(src), config.Default())

		if string(got) != want {
			t.Errorf("Format(%q) = %q, want %q", src, got, want)
		}
	}
}
