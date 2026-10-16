package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/markdown"
	"github.com/JacobJoergensen/laravel-fmt/internal/textwidth"
)

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".md", true) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".md")
			want := formattertest.ReadFixture(t, name+".formatted.md")

			got := mustFormat(t, input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".md", true) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".md")

			once := mustFormat(t, input, config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestFormatIndentedCodeTabColumns(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{" \t0)", "    0)\n"},
		{"\t0)", "    0)\n"},
		{"     0)", "     0)\n"},
		{" \tcode\n\tmore", "    code\n    more\n"},
	}

	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			got := mustFormat(t, []byte(c.src), config.Default())
			if !bytes.Equal(got, []byte(c.want)) {
				t.Fatalf("got %q, want %q", got, c.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q vs %q", got, twice)
			}
		})
	}
}

func TestFormatContainerBodyHoldsNoFrontmatter(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"list item keeps two breaks loose", "* ---\n\n  -- -\n", "* ---\n\n  ---\n"},
		{"blockquote keeps two breaks apart", "> ---\n>\n> -- -\n", "> ---\n>\n> ---\n"},
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

// Expected values are league/commonmark's verdicts.
func TestFormatListTabAfterMarkerKeepsItemMembership(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"line short of the tab stop stays outside", "-\tfoo\n\n  bar\n", "- foo\n\nbar\n"},
		{"line at the tab stop stays inside", "-\tfoo\n\n    bar\n", "- foo\n\n  bar\n"},
		{"code block keeps its leading spaces", "-\t\tfoo\n", "-       foo\n"},
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

// CommonMark ignores up to three spaces before a setext heading's text.
func TestFormatSetextHeadingDedentsContent(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"   Setext\n   ===", "Setext\n===\n"},
		{"   Setext\n   ---", "Setext\n---\n"},
		{"   foo\n   bar\n   ===", "foo\nbar\n===\n"},
		{"Setext\n===", "Setext\n===\n"},
	}

	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			got := mustFormat(t, []byte(c.src), config.Default())
			if !bytes.Equal(got, []byte(c.want)) {
				t.Fatalf("got %q, want %q", got, c.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q vs %q", got, twice)
			}
		})
	}
}

func TestFormatEmptyBulletDoesNotInterrupt(t *testing.T) {
	idempotent := []string{"|\n    *  ", "|\n*  ", "x\n*  ", "a\n+  "}

	for _, src := range idempotent {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}

	if out := mustFormat(t, []byte("x\n* y"), config.Default()); !bytes.Equal(out, []byte("x\n\n* y\n")) {
		t.Fatalf("non-empty bullet should interrupt: %q", out)
	}
}

// league/commonmark lets only a literal `1` interrupt a paragraph, so `01)` and
// `2)` below one are its text.
func TestFormatOrderedListInterruptsOnlyAtOne(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"one_dot_interrupts", "x\n1. y\n", "x\n\n1. y\n"},
		{"one_paren_interrupts", "x\n1) y\n", "x\n\n1) y\n"},
		{"leading_zero_stays_text", "x\n01) y\n", "x\n01) y\n"},
		{"many_leading_zeros_stay_text", "x\n001. y\n", "x\n001. y\n"},
		{"two_stays_text", "x\n2. y\n", "x\n2. y\n"},
		{"zero_stays_text", "0\n0) 0\n", "0\n0) 0\n"},
		{"tab_indented_stays_text", "0\n\t0) 0", "0\n0) 0\n"},
		{"leading_zero_alone_is_a_list", "01) y\n", "1) y\n"},
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

func TestFormatIndentedCodeInteriorBlanks(t *testing.T) {
	cases := []string{"\t0\n\n\n\t0", "\t0\n\n\t0", "    a\n\n\n\n    b"}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}

	if out := mustFormat(t, []byte("\tx\n\ny"), config.Default()); !bytes.Equal(out, []byte("    x\n\ny\n")) {
		t.Fatalf("blank before paragraph should not be absorbed: %q", out)
	}
}

func TestFormatListKeepsFollowingIndentedCodeOutside(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"empty item", "   *\n    0"},
		{"item with content", "   * a\n\n    0"},
		{"dash marker", "   -\n    0"},
		{"plus marker", "   +\n    0"},
		{"ordered marker", "   1.\n    0"},
		{"wide ordered markers", "   9. a\n   10. b\n\n    0"},
		// The code block's own leading spaces are emitted after its mandatory
		// four, so the pad has to clear 4+k, not 4.
		{"code content indented further", "  00)\n     0"},
		// Renumbering `000)` to `0)` would shrink the content column past what any
		// legal pad can clear, so the source markers have to stand.
		{"renumbering would shrink the marker", "   000)\n      0"},
		{"renumbering, wider still", "   0000)\n       0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())

			file, errs := parser.Parse(once)
			if formatter.JoinErrors(errs) != nil {
				t.Fatalf("output does not re-parse: %v", errs)
			}

			if len(file.Blocks) != 2 {
				t.Fatalf("code block was absorbed into the list: %d blocks, want 2.\n=== out ===\n%q", len(file.Blocks), once)
			}

			if _, ok := file.Blocks[1].(*parser.IndentedCodeBlock); !ok {
				t.Fatalf("second block is %T, want *parser.IndentedCodeBlock.\n=== out ===\n%q", file.Blocks[1], once)
			}

			twice := mustFormat(t, once, config.Default())
			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

// The middle list needs an indent below the first list's content column and a
// content column that clears the code block, so the first list takes an
// indent too.
func TestFormatListRunKeepsFollowingIndentedCodeOutside(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"ordered then ordered", "0.\n000) 0)\n    0"},
		{"bullet then bullet", "  +\n   * #\n    0"},
		{"bullet then nested bullet", "  +\n\n   * -\n\n    0"},
		{"bullet then wide ordered", "+\n\n  000) b\n\n    0"},
		{"three lists deep", "+\n\n  * a\n\n   - b\n\n    0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())

			file, errs := parser.Parse(once)
			if formatter.JoinErrors(errs) != nil {
				t.Fatalf("output does not re-parse: %v", errs)
			}

			last := file.Blocks[len(file.Blocks)-1]
			if _, ok := last.(*parser.IndentedCodeBlock); !ok {
				t.Fatalf("last block is %T, want *parser.IndentedCodeBlock.\n=== out ===\n%q", last, once)
			}

			for i, block := range file.Blocks[:len(file.Blocks)-1] {
				if _, ok := block.(*parser.List); !ok {
					t.Fatalf("block %d is %T, want *parser.List (a list was nested away).\n=== out ===\n%q", i, block, once)
				}
			}

			srcFile, errs := parser.Parse([]byte(tc.src))
			if formatter.JoinErrors(errs) == nil && len(srcFile.Blocks) != len(file.Blocks) {
				t.Fatalf("block count changed: %d -> %d.\n=== out ===\n%q", len(srcFile.Blocks), len(file.Blocks), once)
			}

			twice := mustFormat(t, once, config.Default())
			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatLinkRefKeepsAngleDestination(t *testing.T) {
	cases := []struct {
		name string
		src  string
		dest string
	}{
		{"leading space", "[0]:< 0>", " 0"},
		{"interior spaces", "[a]:<b c d>", "b c d"},
		{"empty destination", "[0]:<>", ""},
		{"unbalanced paren", "[a]:<b(c>", "b(c"},
		{"starts with a bracket", "[a]:<<b>", "<b"},
		{"no brackets needed", "[0]: x", "x"},
		{"balanced parens stay bare", "[a]: b(c)d", "b(c)d"},
		// CommonMark escapes only punctuation, so a backslashed space still
		// ends a bare destination and forces the angle form.
		{"escaped space", "[0]:<\\ >", "\\ "},
		{"escaped space among text", "[a]:<b\\ c>", "b\\ c"},
		{"escaped paren stays bare", "[a]:<b\\(c>", "b\\(c"},
		{"escaped tab", "[0]:<\\\t>", "\\\t"},
		// A control character forces the angle form, where a bare `>` would close it
		// early. Escaped, it comes back in the destination and renders the same.
		{"closing bracket is escaped", "[0]:\x1d>0", "\x1d\\>0"},
		{"already escaped stays single", "[0]:\x1d\\>0", "\x1d\\>0"},
		{"trailing backslash is escaped", "[0]:\x13\\", "\x13\\\\"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())

			file, errs := parser.Parse(once)
			if formatter.JoinErrors(errs) != nil {
				t.Fatalf("output does not re-parse: %v", errs)
			}

			def, ok := file.Blocks[0].(*parser.LinkRefDef)
			if !ok {
				t.Fatalf("block is %T, want *parser.LinkRefDef.\n=== out ===\n%q", file.Blocks[0], once)
			}

			if string(def.Dest) != tc.dest {
				t.Fatalf("destination became %q, want %q.\n=== out ===\n%q", def.Dest, tc.dest, once)
			}

			twice := mustFormat(t, once, config.Default())
			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatLinkRefKeepsTitleDelimited(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		title string
	}{
		{"canonical title is left alone", `[a]: /b "plain"`, "plain"},
		{"double quote inside", `[a]: /b (x "q" y)`, `x "q" y`},
		{"both quotes inside", `[a]: /b (it's "q")`, `it's "q"`},
		{"already escaped quote survives", `[a]: /b "x \" y"`, `x " y`},
		{"single quote alone keeps the canonical form", `[a]: /b (it's)`, "it's"},
		// A parenthesised source can hold an unescaped instance of all three
		// delimiters, and then only escaping keeps the definition parseable.
		{"all three inside", `[a]: /b (it's "q" ( )`, `it's "q" ( `},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())

			file, errs := parser.Parse(once)
			if formatter.JoinErrors(errs) != nil {
				t.Fatalf("output does not re-parse: %v", errs)
			}

			def, ok := file.Blocks[0].(*parser.LinkRefDef)
			if !ok {
				t.Fatalf("block is %T, want *parser.LinkRefDef.\n=== out ===\n%q", file.Blocks[0], once)
			}

			if got := unescapeTitle(def.Title); got != tc.title {
				t.Fatalf("title became %q, want %q.\n=== out ===\n%q", got, tc.title, once)
			}

			twice := mustFormat(t, once, config.Default())
			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

// unescapeTitle drops the backslashes the emitter may have added, so a case can
// state the title the document means rather than the bytes that carry it.
func unescapeTitle(title []byte) string {
	out := make([]byte, 0, len(title))

	for i := range title {
		if title[i] == '\\' && i+1 < len(title) && title[i+1] == '"' {
			continue
		}

		out = append(out, title[i])
	}

	return string(out)
}

func TestFormatSetextContinuationStaysText(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"tab-indented hash", "0\n\t#\n="},
		{"tab-indented blockquote", "0\n\t> q\n="},
		{"tab-indented bullet", "0\n\t- x\n="},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())

			file, errs := parser.Parse(once)
			if formatter.JoinErrors(errs) != nil {
				t.Fatalf("output does not re-parse: %v", errs)
			}

			if len(file.Blocks) != 1 {
				t.Fatalf("heading split into %d blocks.\n=== out ===\n%q", len(file.Blocks), once)
			}

			if _, ok := file.Blocks[0].(*parser.SetextHeading); !ok {
				t.Fatalf("block is %T, want *parser.SetextHeading.\n=== out ===\n%q", file.Blocks[0], once)
			}

			twice := mustFormat(t, once, config.Default())
			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestFormatFencedCodeBareCR(t *testing.T) {
	cases := []string{"```\r\r```", "```\rabc\rdef\r```", "```\r\n\r\n```"}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestProseWrapNever(t *testing.T) {
	cfg := config.Default()
	cfg.Markdown.ProseWrap = config.ProseWrapNever

	input := formattertest.ReadFixture(t, "prose_wrap_never_input.md")
	want := formattertest.ReadFixture(t, "prose_wrap_never.formatted.md")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("ProseWrapNever output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}
}

// Under the default preserve mode a misread HTML block start is invisible;
// always reflows it into the paragraph.
func TestProseWrapAlwaysKeepsHTMLBlockOut(t *testing.T) {
	cfg := config.Default()
	cfg.Markdown.ProseWrap = config.ProseWrapAlways

	cases := []struct{ name, src, want string }{
		{"block tag", "foo\n<div>\nbar\n", "foo\n\n<div>\nbar\n"},
		{"comment", "foo\n<!-- c -->\n", "foo\n\n<!-- c -->\n"},
		// Type 7 is not a block start here, so it stays part of the prose.
		{"inline tag", "foo\n<span>x</span>\n", "foo <span>x</span>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, cfg); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestProseWrapAlways(t *testing.T) {
	cfg := config.Default()
	cfg.Markdown.ProseWrap = config.ProseWrapAlways
	cfg.PrintWidth = 40

	input := formattertest.ReadFixture(t, "prose_wrap_always_input.md")
	want := formattertest.ReadFixture(t, "prose_wrap_always.formatted.md")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("ProseWrapAlways output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("ProseWrapAlways is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestProseWrapAlwaysProtectsInlineAtoms(t *testing.T) {
	cfg := config.Default()
	cfg.Markdown.ProseWrap = config.ProseWrapAlways
	cfg.PrintWidth = 30

	input := formattertest.ReadFixture(t, "prose_wrap_inline_input.md")
	want := formattertest.ReadFixture(t, "prose_wrap_inline.formatted.md")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("ProseWrapAlwaysProtectsInlineAtoms output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("ProseWrapAlwaysProtectsInlineAtoms is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestIgnoreDirectivePreservesProse(t *testing.T) {
	cfg := config.Default()
	cfg.Markdown.ProseWrap = config.ProseWrapAlways
	cfg.PrintWidth = 40

	input := formattertest.ReadFixture(t, "directive_ignore_prose_input.md")
	want := formattertest.ReadFixture(t, "directive_ignore_prose.formatted.md")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("IgnoreDirectivePreservesProse output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("IgnoreDirectivePreservesProse is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// A fence with no closing line is valid CommonMark; the output writes the
// closer, which reparses as the same document.
func TestFormatClosesAnUnterminatedFence(t *testing.T) {
	out, err := Format([]byte("```\nstill open"), config.Default())
	if err != nil {
		t.Fatalf("Format rejected a legal unterminated fence: %v", err)
	}

	if want := "```\nstill open\n```\n"; string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}

	twice, err := Format(out, config.Default())
	if err != nil {
		t.Fatalf("Format rejected its own output: %v", err)
	}

	if !bytes.Equal(twice, out) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", out, twice)
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

func TestLeadingThematicBreakAvoidsFrontmatter(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"two_dash_breaks", " ---\n---", "***\n\n---\n"},
		{"break_then_heading_kept", "---\n# Heading", "---\n\n# Heading\n"},
		{"already_asterisk_kept", "***\n---", "***\n\n---\n"},
		// Frontmatter is lifted off the raw lines before any block parsing, so the
		// closing fence can just as well come from inside a code block or a chunk of
		// raw HTML. Both emit their content verbatim at column 0.
		{"break_then_fenced_dashes", " ---\n```\n---", "***\n\n```\n---\n```\n"},
		{"break_then_html_dashes", " ---\n<div>\n---\n", "***\n\n<div>\n---\n"},
		{"break_then_fenced_dashes_after_lone_cr", " ---\r~~~\r\r---", "***\n\n~~~\n\n---\n~~~\n"},
		{"break_then_indented_dashes", " ---\n\n    ---\n", "---\n\n    ---\n"},
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

// A list item that opens with a blank line holds only that blank. The last
// three cases have content before the blank and continue as usual.
func TestEmptyListItemEndsAtBlankLine(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"paragraph_after_blank", "-\n\n  foo\n", "- \n\nfoo\n"},
		// The marker pad that keeps a following code block out of a list is not
		// needed once the item is empty, since the blank line already closes it.
		{"code_after_blank", "* \n\n    0\n", "* \n\n    0\n"},
		{"sibling_item_after_blank", "* \n\n  - x\n", "* \n\n- x\n"},
		{"list_code_list", "* \n\n    0\n- y\n", "* \n\n    0\n\n- y\n"},
		// Padding it inside an item puts the nested marker alone on its line, and
		// an empty item cannot interrupt the paragraph above it.
		{"nested_code_after_blank", "* 0\n  * *\n\n\t\t0", "* 0\n  * * \n\n        0\n"},
		{"ordered_paragraph_after_blank", "1.\n\n   foo\n", "1. \n\nfoo\n"},
		{"no_blank_absorbs", "* \n    0\n", "* 0\n"},
		{"item_with_content_absorbs", "* x\n\n    0\n", "* x\n\n  0\n"},
		{"content_before_blank_absorbs", "* \n  a\n\n  b\n", "* a\n\n  b\n"},
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

func TestFormatKeepsLazyContinuationInItem(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"bare_line", "- foo\nbar\n", "- foo\n  bar\n"},
		{"wide_marker", "100) foo\n  bar\n", "100) foo\n     bar\n"},
		{"last_item_of_several", "- a\n- b\nlazy\n", "- a\n- b\n  lazy\n"},
		{"mixed_indents", "- foo\n  bar\n baz\nqux\n", "- foo\n  bar\n  baz\n  qux\n"},
		// Four spaces read as an indented code block only outside the item; as a
		// continuation line they are just paragraph text.
		{"four_spaces", "0.\n000) 0\n    0", "0. \n\n0) 0\n   0\n"},
		// A setext underline is never lazy, so it was text; the escape keeps it
		// text now that it sits directly under the line it continues.
		{"setext_underline_escaped", "- foo\n===\n", "- foo\n  \\===\n"},
		{"setext_underline_escaped_in_quote", "> foo\n===\n", "> foo\n> \\===\n"},
		// The escape goes on the underline character; before the indent it
		// would escape a space, which renders as a literal backslash.
		{"setext_underline_escaped_indented_in_quote", "> foo\n ===\n", "> foo\n> \\===\n"},
		{"setext_underline_escaped_indented_in_item", "- foo\n ===\n", "- foo\n  \\===\n"},
		// A delimiter row is no more allowed to arrive lazily than an underline is,
		// so it was text too; the escape keeps it text now that it sits under the
		// line it would otherwise promote into a table header.
		{"delimiter_row_escaped_in_quote", "> a|b\n-|-\n", "> a|b\n> \\-|-\n"},
		{"delimiter_row_escaped_in_item", "- a|b\n-|-\n", "- a|b\n  \\-|-\n"},
		{"aligned_delimiter_row_escaped", "> a|b\n:-|-:\n", "> a|b\n> \\:-|-:\n"},
		{
			"marked_delimiter_row_promotes",
			"> a|b\n> -|-\n",
			"> | a   | b   |\n> | --- | --- |\n",
		},
		{
			"indented_delimiter_row_promotes",
			"- a|b\n  -|-\n",
			"- | a   | b   |\n  | --- | --- |\n",
		},
		// CommonMark parses a link reference definition as the head of a
		// paragraph, so a paragraph is still open under one and the line
		// below it is a lazy continuation.
		{"lazy_line_after_linkref_in_quote", "> [a]: #\nlazy\n", "> [a]: #\n>\n> lazy\n"},
		{"lazy_line_after_linkref_in_item", "- [a]: #\nlazy\n", "- [a]: #\n  lazy\n"},
		{"blank_after_linkref_ends_quote", "> [a]: #\n\nlazy\n", "> [a]: #\n\nlazy\n"},
		{"heading_after_linkref_ends_quote", "> [a]: #\n# h\n", "> [a]: #\n\n# h\n"},
		// HTML block type 7 does not interrupt a paragraph, type 6 does.
		{"inline_tag_continues", "- foo\n<span>x</span>\n", "- foo\n  <span>x</span>\n"},
		{"block_tag_interrupts", "- foo\n<div>x</div>\n", "- foo\n\n<div>x</div>\n"},
		{"block_tag_ends_quote", "> foo\n<div>\nbar\n", "> foo\n\n<div>\nbar\n"},
		// `>` alone empties the inner quote, which closes the paragraph above it.
		// Nothing is open to continue, so the line after belongs outside entirely.
		{"emptied_inner_quote_ends_laziness", ">>2\n>\n4 b\n", "> > 2\n\n4 b\n"},
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

func TestFormatKeepsHTMLBlockWithInteriorBlanks(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"comment",
			"<!--\n    prose\n\n        deeper\n-->\n\nafter\n",
			"<!--\n    prose\n\n        deeper\n-->\n\nafter\n",
		},
		{
			"comment holding a list",
			"<!--\n- a\n\n- b\n-->\n",
			"<!--\n- a\n\n- b\n-->\n",
		},
		{
			"raw text",
			"<pre>\nx\n\ny\n</pre>\n\nafter\n",
			"<pre>\nx\n\ny\n</pre>\n\nafter\n",
		},
		{"block tag still ends at a blank", "<div>\na\n\nb\n", "<div>\na\n\nb\n"},
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

func TestFormatKeepsOuterListTightAroundLooseChild(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"nested_item_blank",
			"- a\n  - b\n\n    c\n",
			"- a\n  - b\n\n    c\n",
		},
		{
			"nested_items_separated",
			" - Deco\n   - LU x\n\n     such that y\n   - QR\n\n     such z\n",
			"- Deco\n  - LU x\n\n    such that y\n\n  - QR\n\n    such z\n",
		},
		// The item's own two blocks still spread it, and that spreads the list.
		{"own_blocks", "- a\n\n  b\n- c\n", "- a\n\n  b\n\n- c\n"},
		{"blank_inside_fence", "- ```\n  x\n\n  y\n  ```\n- z\n", "- ```\n  x\n\n  y\n  ```\n- z\n"},
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

// An indented opening fence removes up to its own indentation from every
// content line.
func TestFormatDedentsIndentedFenceContent(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"one_space", " ```php\n    code\n```\n", "```php\n   code\n```\n"},
		{"tab_content", "  ```\n\tcode\n```\n", "```\n  code\n```\n"},
		{"whitespace_only_line", " ```\n   \n```\n", "```\n  \n```\n"},
		// Inside an item the body is already dedented to the content column, so
		// what is left is the fence's own indent and the two compose.
		{"inside_item", "- foo\n\n   ```\n     x\n   ```\n", "- foo\n\n  ```\n    x\n  ```\n"},
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

func TestNestedMarkersSplitBeforeReadingAsAThematicBreak(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"under_a_paragraph", "* a\n  * *\n      *\n", "* a\n  * *\n      * \n"},
		{"at_the_top", "* *\n    *\n", "* *\n    * \n"},
		{"nested_item_opening_with_code", "* a\n  * *\n          *\n", "* a\n  * *\n          *\n"},
		{"padded_nested_list", "*\n\t* \t*\n\t\t#\n\t<a>\n", "*\n   *     *\n     #\n    <a>\n"},
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

// A dash break directly under a paragraph only happens in a tight list item.
func TestTightItemBreakAvoidsSetextUnderline(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"tight_item", "* 0\n  - - -\n", "* 0\n  ***\n"},
		{"loose_item_keeps_dashes", "* 0\n\n  - - -\n", "* 0\n\n  ---\n"},
		{"break_alone_keeps_dashes", "* - - -\n", "* ---\n"},
		{"after_heading_keeps_dashes", "* # h\n  - - -\n", "* # h\n  ---\n"},
		{"underscore_break_kept", "* 0\n  ___\n", "* 0\n  ___\n"},
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

// A blockquote directly above another block only happens in a tight list item.
func TestTightItemQuoteKeepsMarkerOnlyLine(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"tight_item", "* >0\n  >\n  0\n", "* > 0\n  >\n  0\n"},
		// The quote ends a nested item, so its marker-only line stays inside that
		// item; the text below continues the outer item.
		{"nested_item", "* * >0\n    >\n  0\n", "* * > 0\n    >\n  0\n"},
		{"nested_in_ordered_item", "1. * > quote\n     >\n   tail\n", "1. * > quote\n     >\n   tail\n"},
		{"tight_item_setext", "* >0\n  >\n  0\n  -\n", "* > 0\n  >\n  0\n  ---\n"},
		// Indented code cannot interrupt a paragraph, so its first line is lazy too.
		{"indented_code_absorbed", "* >0\n  >\n      code\n", "* > 0\n  >\n      code\n"},
		// A table's header row is paragraph text until the delimiter row promotes the
		// pair, and a definition is the head of a paragraph, so both are absorbed.
		{
			"table_absorbed",
			"* >0\n  >\n  a|b\n  -|-\n",
			"* > 0\n  >\n  | a   | b   |\n  | --- | --- |\n",
		},
		{"linkref_absorbed", "* >0\n  >\n  [a]: /a\n", "* > 0\n  >\n  [a]: /a\n"},
		{"inline_html_absorbed", "* >0\n  >\n  <em>x</em>\n", "* > 0\n  >\n  <em>x</em>\n"},
		{"block_html_interrupts", "* >0\n  <div>x</div>\n", "* > 0\n  <div>x</div>\n"},
		{"loose_item_needs_no_marker", "* >0\n  >\n\n  0\n", "* > 0\n\n  0\n"},
		{"quote_alone", "* >0\n", "* > 0\n"},
		{"list_interrupts", "* >0\n  - y\n", "* > 0\n  - y\n"},
		{"heading_interrupts", "* >0\n  # h\n", "* > 0\n  # h\n"},
		{"fence_interrupts", "* >0\n  >\n  ```\n  x\n  ```\n", "* > 0\n  ```\n  x\n  ```\n"},
		{"break_interrupts", "* >0\n  >\n  ***\n", "* > 0\n  ***\n"},
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

// `_ _ _` is a thematic break only as a whole line, and `--- x` is a construct
// only as its first unit.
func TestProseWrapAlwaysEscapesWrappedBlockMarkers(t *testing.T) {
	cfg := config.Default()
	cfg.PrintWidth = 20
	cfg.Markdown.ProseWrap = config.ProseWrapAlways

	cases := []struct{ name, src, want string }{
		{"heading_marker", "aaaa bbbb cccc dddd # eee fff\n", "aaaa bbbb cccc dddd\n\\# eee fff\n"},
		{"break_needs_whole_line", "aaaa bbbb cccc dddd _ _ _\n", "aaaa bbbb cccc dddd\n\\_ _ _\n"},
		{"dash_run_with_text_kept", "aaaa bbbb cccc dddd --- x\n", "aaaa bbbb cccc dddd\n--- x\n"},
		// CommonMark's bullets are `-`, `+` and `*`, so a lone underscore
		// starts nothing.
		{"underscore_is_never_a_bullet", "aaaa bbbb cccc dddd _ eee\n", "aaaa bbbb cccc dddd\n_ eee\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, cfg); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestLeadingFenceTextAvoidsFrontmatter(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"indented_closer", "+++\n\n +++\n", "\\+++\n\n+++\n"},
		{"lazy_continuation_closer", "+++\nfoo\n +++\n", "\\+++\nfoo\n+++\n"},
		{"closer_inside_paragraph", "+++\n\ntext\n +++\nmore\n", "\\+++\n\ntext\n+++\nmore\n"},
		{"setext_opener", "+++\n===\n\n +++\n", "\\+++\n===\n\n+++\n"},
		{"no_closer_kept", "+++\n\nfoo\n", "+++\n\nfoo\n"},
		{"frontmatter_kept", "+++\ntitle = \"x\"\n+++\n\nfoo\n", "+++\ntitle = \"x\"\n+++\n\nfoo\n"},
		{"escape_round_trips", "\\+++\n\n+++\n", "\\+++\n\n+++\n"},
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

// A blank line inside a fenced code block is content: it renders.
func TestFormatFencedCodeKeepsBlankContentLines(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"indented fence, trailing blank", " ```ts\n x\n\n ```\n", "```ts\nx\n\n```\n"},
		{"indented fence, two trailing blanks", " ```ts\n x\n\n\n ```\n", "```ts\nx\n\n\n```\n"},
		{"indented fence, interior blank", "  ```\n  a\n\n  b\n  ```\n", "```\na\n\nb\n```\n"},
		{"blank-only content", "```\n\n```\n", "```\n\n```\n"},
		{"blank-only content, indented", " ```\n\n ```\n", "```\n\n```\n"},
		{"empty block stays empty", "```\n```\n", "```\n```\n"},
		{"empty block with info string", "```ts\n```\n", "```ts\n```\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// Expected values are league/commonmark's: a piped line becomes a table header
// only over a real delimiter row.
func TestFormatPromotesOnlyRealTables(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"vertical tab is not padding", "0|0\n|-|\v-\n", "0|0\n|-|\v-\n"},
		{"bullet wins over delimiter", "a|b\n - | - \n", "a|b\n\n- | - \n"},
		{"star bullet wins", "a|b\n* | *\n", "a|b\n\n* | *\n"},
		{"empty bullet interrupts nothing", "|\n*  \n", "|\n*  \n"},
		{"tight delimiter is a table", "a|b\n-|-\n", "| a   | b   |\n| --- | --- |\n"},
		{"table with a row", "a|b\n---|---\n1|2\n", "| a   | b   |\n| --- | --- |\n| 1   | 2   |\n"},
		{"alignment markers", "a|b\n:-:|:-:\n", "|   a   |   b   |\n| :---: | :---: |\n"},
		{"indented delimiter is still a table", "a|b\n  ---|---\n", "| a   | b   |\n| --- | --- |\n"},
		{"tabs pad a delimiter cell", "a|b\n\t---\t|\t---\t\n", "| a   | b   |\n| --- | --- |\n"},
		// GFM lets a table interrupt a paragraph, so the header needs no
		// blank line above it.
		{
			"header follows a paragraph line",
			"Accepts the following:\na|b\n---|---\n1|2\n",
			"Accepts the following:\n\n| a   | b   |\n| --- | --- |\n| 1   | 2   |\n",
		},
		{
			"header follows several paragraph lines",
			"One.\nTwo.\na|b\n---|---\n",
			"One.\nTwo.\n\n| a   | b   |\n| --- | --- |\n",
		},
		{
			"bold lead-in",
			"**Key mappings:**\na|b\n---|---\n",
			"**Key mappings:**\n\n| a   | b   |\n| --- | --- |\n",
		},
		// The delimiter row has to hold a pipe and match the header cell for
		// cell, so both of these stay paragraphs.
		{"last line without a pipe", "a|b\nplain\n:-:|:-:\n", "a|b\nplain\n:-:|:-:\n"},
		{"cell count must match", "text\na|b\n:-:|:-:|:-:\n", "text\na|b\n:-:|:-:|:-:\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatKeepsLinkRefDefsTogether(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"two definitions", "[a]: /a\n[b]: /b\n", "[a]: /a\n[b]: /b\n"},
		{"three definitions", "[a]: /a\n[b]: /b\n[c]: /c\n", "[a]: /a\n[b]: /b\n[c]: /c\n"},
		// A gap the author wrote is theirs: definitions render as nothing either
		// way, so grouping them is not a question the formatter answers.
		{"blank between them is kept", "[a]: /a\n\n[b]: /b\n", "[a]: /a\n\n[b]: /b\n"},
		{"after a paragraph", "text\n\n[a]: /a\n[b]: /b\n", "text\n\n[a]: /a\n[b]: /b\n"},
		{"in a quote", "> [a]: /a\n> [b]: /b\n", "> [a]: /a\n> [b]: /b\n"},
		{"paragraph after keeps its blank", "[a]: /a\n\ntext\n", "[a]: /a\n\ntext\n"},
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

// GFM reads a bare line as a one-cell body row; a blank line or another block
// start ends the table. Both oracles agree on every case.
func TestFormatKeepsTableRowsWithoutPipes(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"bare row", "a|b\n-|-\nc\n", "| a   | b   |\n| --- | --- |\n| c   |     |\n"},
		{
			"two bare rows",
			"a|b\n-|-\nc\nd\n",
			"| a   | b   |\n| --- | --- |\n| c   |     |\n| d   |     |\n",
		},
		{"underline is a row", "a|b\n-|-\n=\n", "| a   | b   |\n| --- | --- |\n| =   |     |\n"},
		{"one column absorbs the row", "=`a>|\n|-\n4]", "| =`a> |\n| ---- |\n| 4]   |\n"},
		{"indented code ends it", "a|b\n-|-\n    c\n", "| a   | b   |\n| --- | --- |\n\n    c\n"},
		{"blank line ends it", "a|b\n-|-\n\nc\n", "| a   | b   |\n| --- | --- |\n\nc\n"},
		{"heading ends it", "a|b\n-|-\n# h\n", "| a   | b   |\n| --- | --- |\n\n# h\n"},
		{"thematic break ends it", "a|b\n-|-\n---\n", "| a   | b   |\n| --- | --- |\n\n---\n"},
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

// A tab measures to the next 4-column stop, so stripping an item's content
// column cannot keep both a tab's bytes and its columns: block structure keeps
// the columns and fenced content keeps the bytes. Every case renders as its
// source does under league/commonmark.
func TestFormatKeepsListItemTabColumnsAndFencedBytes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			// Two tabs put the code at column 8, three columns of which belong
			// to the item.
			"indented code keeps its column",
			"- a\n\n\t\tcode\n",
			"- a\n\n        code\n",
		},
		{
			// The tab inside the fence is content, so it survives; the fence lines
			// around it are structure and re-home.
			"fenced content keeps its tabs",
			"3. x\n\n\t```json\n\t\"a\": [\n\t\t\"b\"\n\t]\n\t```\n",
			"3. x\n\n   ```json\n   \"a\": [\n   \t\"b\"\n   ]\n   ```\n",
		},
		{"fence inside an item", "- a\n\t```\n\t\tx\n\t```\n", "- a\n  ```\n  \tx\n  ```\n"},
		// Expanded, the tab after `* ` reaches column 4, a gap under five
		// columns, so the content is text, not code.
		{"space then tab is content", "* \tfoo\n", "* foo\n"},
		{"space tab space is content", "- \t 33\n", "- 33\n"},
		{"four columns are content", "*    0\n", "* 0\n"},
		// Five columns really do pin the content, code block and all.
		{"five columns pin code", "*     0\n", "*     0\n"},
		// A fence opened on the marker line governs the lines under it just as one
		// opened further down does: the item's first line carries no indent to strip,
		// but the block it opens still decides that the tab below is content.
		{"fence opens on the marker line", "* ~~~\n  \t0\n  ~~~\n", "* ~~~\n  \t0\n  ~~~\n"},
		// The closer the source left out is written, so the output reparses; the
		// tab inside the fence is still content and survives byte for byte.
		{"unterminated fence", "- a\n\t```\n\t\tx\n", "- a\n  ```\n  \tx\n  ```\n"},
		{
			// The tab-indented line is the inner item's content.
			"nested list continuation",
			"0) 0000000)\n\t\t\t0",
			"0) 0) 0\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatKeepsHardBreaksInEveryProseWrapMode(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"two spaces", "a  \nb\n", "a  \nb\n"},
		{"three spaces", "a   \nb\n", "a   \nb\n"},
		{"backslash", "a\\\nb\n", "a\\\nb\n"},
		{"two breaks", "a  \nb  \nc\n", "a  \nb  \nc\n"},
		// One space is a soft break, so these modes may join it.
		{"one space joins", "a \nb\n", "a b\n"},
		// A break at the end of a paragraph has nothing to break from, so the
		// spaces are insignificant and go away.
		{"trailing break is dropped", "a  \n", "a\n"},
		// The second break belongs after `c`, where the author put it: the oracle
		// renders both source and output as `a<br> b c<br> d`.
		{"soft break between hard ones", "a  \nb\nc  \nd\n", "a  \nb c  \nd\n"},
		// A setext heading's content is a paragraph, so a break inside it renders
		// the same `<br>` and survives the same way.
		{"setext heading", "a  \nb\n---\n", "a  \nb\n---\n"},
		// The spec strips the heading's final whitespace, so the last line's break
		// has nothing to break from, exactly as at the end of a paragraph.
		{"setext trailing break is dropped", "a\nb  \n---\n", "a\nb\n---\n"},
	}

	for _, mode := range []string{config.ProseWrapAlways, config.ProseWrapNever} {
		for _, tc := range cases {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				cfg := config.Default()
				cfg.Markdown.ProseWrap = mode

				got := mustFormat(t, []byte(tc.src), cfg)

				if string(got) != tc.want {
					t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
				}

				if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
					t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
				}
			})
		}
	}
}

// league/commonmark decides these: raw HTML on a continuation line stays
// inline only at four or more columns; one to three open a block, and a
// backslash would render the tag as text.
func TestFormatParagraphKeepsInlineHTMLIndented(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"four spaces", "0\n    <div>\n", "0\n    <div>\n"},
		{"tab", "0\n\t<P\n", "0\n    <P\n"},
		{"deeper than four", "0\n      <div>\n", "0\n    <div>\n"},
		{"two tags in a row", "0\n    <div>\n    <p>\n", "0\n    <div>\n    <p>\n"},
		// Fewer than four columns already opened a block in the source, so the
		// blank line is right.
		{"three spaces stays a block", "0\n   <div>\n", "0\n\n   <div>\n"},
		{"flush stays a block", "0\n<div>\n", "0\n\n<div>\n"},
		// Type 7 cannot interrupt a paragraph, so it is prose and still dedents.
		{"inline tag dedents", "0\n    <em>x</em>\n", "0\n<em>x</em>\n"},
		{"plain text dedents", "0\n    plain\n", "0\nplain\n"},
		// A setext heading's content is a paragraph and needs the same indent.
		{"setext heading keeps it", "0\n\t<P\n===\n", "0\n    <P\n===\n"},
		{"setext level 2 keeps it", "0\n\t<P\n---\n", "0\n    <P\n---\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// Under always, a wrap never lands the tag at a line start: the line
// overflows instead, and after a backslash hard break the tag gets four
// columns.
func TestProseWrapAlwaysKeepsInlineHTMLOffLineStart(t *testing.T) {
	cfg := config.Default()
	cfg.Markdown.ProseWrap = config.ProseWrapAlways
	cfg.PrintWidth = 20

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"wrap would start a line with the tag", "aaa bbb ccc ddd eee\n    <div>\n", "aaa bbb ccc ddd eee <div>\n"},
		{"short paragraph keeps it inline", "0\n    <div>\n", "0 <div>\n"},
		{"hard break forces the indent", "w\\\n    <div>\n", "w\\\n    <div>\n"},
		{"inline tag wraps normally", "aaa bbb ccc ddd eee\n    <em>x</em>\n", "aaa bbb ccc ddd eee\n<em>x</em>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)

			if string(got) != tc.want {
				t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// CommonMark puts a list item's content column at the marker width plus the 1
// to 4 spaces after it.
func TestFormatUsesCommonMarkListContentColumn(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			// The code block sits 4 columns past the content column and must stay
			// there; at 5 its content would start with a space.
			"indented code keeps its content",
			"1.  A paragraph\n    with two lines.\n\n        indented code\n\n    > A block quote.\n",
			"1. A paragraph\n   with two lines.\n\n       indented code\n\n   > A block quote.\n",
		},
		{
			"two spaces after a bullet",
			"-  item\n\n       code\n",
			"- item\n\n      code\n",
		},
		{
			// Over four spaces means the item opens with an indented code block, so
			// the content column falls back to the marker plus one.
			"more than four spaces is an indented code block",
			"-      over four\n",
			"-      over four\n",
		},
		{
			"a blank rest of line is an empty item",
			"-\n  empty item\n",
			"- empty item\n",
		},
		{
			"a single space is unchanged",
			"- one space\n  cont\n",
			"- one space\n  cont\n",
		},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestFormatProseWrapMeasuresColumns(t *testing.T) {
	cfg := config.Default()
	cfg.PrintWidth = 40
	cfg.Markdown.ProseWrap = config.ProseWrapAlways

	cjk := []byte("测试 宽度 计算 全角 字符 换行 行为 验证 结果 正确 处理 情况\n")

	got := mustFormat(t, cjk, cfg)

	if bytes.Count(got, []byte("\n")) < 2 {
		t.Fatalf("a 59-column line was left unwrapped at printWidth 40:\n%s", got)
	}

	for line := range bytes.SplitSeq(bytes.TrimRight(got, "\n"), []byte("\n")) {
		cols := 0
		for _, r := range string(line) {
			cols += textwidth.Rune(r)
		}

		if cols > cfg.PrintWidth {
			t.Fatalf("line of %d columns exceeds printWidth %d:\n%s", cols, cfg.PrintWidth, line)
		}
	}

	ascii := []byte("aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk\n")
	if twice := mustFormat(t, mustFormat(t, ascii, cfg), cfg); !bytes.Equal(twice, mustFormat(t, ascii, cfg)) {
		t.Fatal("ascii prose wrap is not idempotent")
	}
}

func TestFormatProseWrapAlwaysCountsTheEscape(t *testing.T) {
	cfg := config.Default()
	cfg.PrintWidth = 20
	cfg.Markdown.ProseWrap = config.ProseWrapAlways

	cases := []string{
		"a ab abc abcd abcde ## abcd abcde abcdef\n",
		"a ab abc abcd abcde ## abcd abcde abcdef abcdefgh\n",
		"a ab abc abcd abcde abcdef abcdefgh a ab ## abcd abcde abcdef\n",
		"a ab abc abcd abcde # abcd abcde abcdef\n",
		"a ab abc abcd abcde > abcd abcde abcdef\n",
		"a ab abc abcd abcde - abcd abcde abcdef\n",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), cfg)
			twice := mustFormat(t, once, cfg)

			if !bytes.Equal(once, twice) {
				t.Fatalf("not idempotent.\n=== once ===\n%s=== twice ===\n%s", once, twice)
			}

			for line := range bytes.SplitSeq(bytes.TrimRight(once, "\n"), []byte("\n")) {
				if columns(line) > cfg.PrintWidth {
					t.Fatalf("line of %d columns exceeds printWidth %d:\n%s", columns(line), cfg.PrintWidth, once)
				}
			}
		})
	}
}

// Tilde fences carry the risk: their info string may start with a tilde.
func TestFenceKeepsItsInfoStringSeparate(t *testing.T) {
	cases := []string{
		"~~~   ~\n~~~\n",
		"~~~ ~lang\n~~~\n",
		"~~~   a~b\n~~~\n",
		"```   x\n```\n",
		"~~~   php\n~~~\n",
	}

	for _, src := range cases {
		t.Run(strings.ReplaceAll(src, "\n", "~n"), func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not a fixpoint.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}

			// A fence run longer than the author's means the info string joined it.
			opener := once[:bytes.IndexByte(once, '\n')]
			srcOpener := []byte(src[:strings.IndexByte(src, '\n')])

			runOf := func(line []byte, c byte) int {
				n := 0
				for n < len(line) && line[n] == c {
					n++
				}

				return n
			}

			for _, c := range []byte{'~', '`'} {
				if runOf(opener, c) > runOf(srcOpener, c) {
					t.Fatalf("the info string merged into the fence.\n=== in ===\n%q\n=== out ===\n%q", src, once)
				}
			}
		})
	}
}

func TestEmptyListItemWithContinuationIsAFixpoint(t *testing.T) {
	cases := []string{
		"*\n   <A\n",
		"*\n  text\n",
		"-\n    deep\n",
		"1.\n   numbered\n",
		"* already\n",
	}

	for _, src := range cases {
		t.Run(strings.ReplaceAll(src, "\n", "~n"), func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not a fixpoint.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

// After `> ` a tab covers two columns, not four, so a tab-indented blockquote
// line is a paragraph, not indented code. Expected values come from the
// reference implementation.
func TestBlockStructureSurvivesTabsAndNesting(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"tab after a blockquote marker is not code", "> \tquoted\n", "> quoted\n"},
		{"nested list keeps a sibling outside it", "* \n\t* \n\t<A\n", "*\n   * \n    <A\n"},
		{"nested list with no sibling still hoists", "- a\n\t- b\n", "- a\n  - b\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// A block that was a list's sibling has to stay one, so the list buys back
// the content columns its dedented marker loses: first with marker indent,
// then, past the three-column cap, with the space after the marker. An item
// opening with indented code keeps a one-space gap, since CommonMark pins its
// content there.
func TestListClearsAFollowingBlock(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"code block one column past the cap", "  *   #\n     0\n", "   *  #\n\n     0\n"},
		{"code block two columns past", "  *    x\n\n      c\n", "   *   x\n\n      c\n"},
		{"item opening with indented code keeps one space", "0)     0\n00000) x\n\n      0\n", "   0)     0\n   00000) x\n\n      0\n"},
		{"earlier item opening with code leaves the last its gap", " *     0\n  *   #\n     0", "   *     0\n   *  #\n\n     0\n"},
		{"code block indented by a tab", "0000000) 0)\n\t\t0", "0000000) 0) \n\n    \t0\n"},
		{"html block sibling", "* \n  * \n  <A\n", "* * \n  <A\n"},
		{"list beside its parent's marker clears with its gap", "- a\n  - -    b\n\n        c\n", "- a\n  - -    b\n\n        c\n"},
		{"list two levels beside a marker", "* a\n    * *    1)\n          b\n", "* a\n  * *    1) \n        b\n"},
		{"empty item beside a marker keeps its source width", "* a\n    * 000)\n          b\n", "* a\n  * 000) \n        b\n"},
		{"last item opening below its marker takes no gap", "*\n\t*\n\t*\n\t\t<a>\n\t<b>\n", "* * \n   *\n       <a>\n    <b>\n"},
		{"a pad beside a marker stays left of the first item's content", "* a\n  * *\n     *    b\n\n         c\n", "* a\n  * * \n     *    b\n\n         c\n"},
		{"run of lists beside a marker", "* a\n    * *\n      000)\n          b\n", "* a\n  * * \n    000) \n        b\n"},
		{"longer run of lists beside a marker", "* a\n    * *\n      + c\n      000)\n          b\n", "* a\n  * * \n    + c\n    000) \n        b\n"},
		{"list in a run clears with its gap", "* a\n\t* 0)\n\t   *    0)\n\t      b\n", "* a\n  * 0) \n    *    0) \n        b\n"},
		{"items after the first keep a pad beside a marker", "* a\n   * 00)\n       00)\n         b\n", "* a\n  * 0) \n      0) \n        b\n"},
		{"nothing to clear stays canonical", "- a\n\n- b\n", "- a\n\n- b\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}
