package markdown

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/markdown"
)

func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".md")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		checkFormatKeepsMeaning(t, cfg, src)
	})
}

// goldmark is the oracle: the HTML it renders from src it must render from the
// output. It renders all 652 CommonMark 0.31.2 spec examples as the spec does,
// once whitespace between tags and `<br />` against `<br>` are not counted.
// Where it differed from the spec beyond them, commonmark.js 0.31.2 decided,
// and the forms it misreads are skipped:
//   - A lone CR, which it does not end a line at.
//   - A tab inside a container that does not start on a tab stop, or follows
//     nested markers: it loses the tab's column, so `*\n\t* \t*` reads the
//     last `*` as text and `>*\t  00` as code, where the same columns in
//     spaces read as written.
//   - A last line with no line break: `~~~0` loses its info string and
//     `* ~~~\n ` its blank code line.
//   - `<!` and a lowercase letter, which opens a declaration only since 0.31.
//   - A tab right after a tag name: `<p\t` opens no HTML block for it.
//   - An empty list item, after which it misplaces what follows a blank line:
//     `* * \n\n  0` puts the `0` outside the list.
//   - A vertical tab or form feed, which it keeps in an info string where
//     commonmark.js trims it.
//   - An unclosed fence in a list item, whose blank lines it counts as
//     separating the item from what follows: `* * ```\n\n  00` reads loose.
//   - An indented fence, whose indent it leaves on whitespace-only lines.
//   - A `*` or `_` run straight after a `>` marker, which it flanks by the
//     marker: `>0*0\n>*!` reads `<em>`.
//
// Tables are GFM, where readers disagree on rows without pipes, so a document
// the formatter reads a table in is left to the table tests.
func checkFormatKeepsMeaning(t *testing.T, cfg *config.File, src []byte) {
	t.Helper()

	format := func(b []byte) ([]byte, error) { return Format(b, cfg) }

	out, err := formattertest.FormatIdempotent(t, format, src)
	if err != nil || oracleMisreads(src) || leftToOtherTests(src) {
		return
	}

	want := renderedMeaning(src)
	if got := renderedMeaning(out); got != want {
		t.Fatalf("meaning differs\ninput  %q\noutput %q\nwant %s\ngot  %s", src, out, want, got)
	}
}

func oracleMisreads(src []byte) bool {
	return hasLoneCR(src) ||
		hasTabAfterNestedMarker(src) ||
		endsWithoutLineBreak(src) ||
		lowercaseDeclaration.Match(src) ||
		tabAfterTagName.Match(src) ||
		emptyListItemLine.Match(src) ||
		delimiterAfterQuoteMarker.Match(src) ||
		bytes.ContainsAny(src, "\v\f")
}

func hasLoneCR(src []byte) bool {
	for i, c := range src {
		if c == '\r' && (i+1 == len(src) || src[i+1] != '\n') {
			return true
		}
	}

	return false
}

func endsWithoutLineBreak(src []byte) bool {
	return len(src) > 0 && src[len(src)-1] != '\n'
}

// hasTabAfterNestedMarker reports a line whose leading run of whitespace and
// container markers holds a tab anywhere but straight after one marker that
// opens the line, unless it is on a tab stop with no marker before it.
func hasTabAfterNestedMarker(src []byte) bool {
	for line := range bytes.Lines(src) {
		if lineHasTabAfterNestedMarker(line) {
			return true
		}
	}

	return false
}

func lineHasTabAfterNestedMarker(line []byte) bool {
	tabBefore := false
	markers := 0
	firstMarkerCol := -1
	col := 0

	for i, c := range line {
		switch {
		case c == '\t':
			offStop := col%4 != 0
			plainGap := markers == 1 && firstMarkerCol == 0 && !tabBefore

			if markers >= 2 || (markers >= 1 && (tabBefore || firstMarkerCol > 0)) || (offStop && !plainGap) {
				return true
			}

			tabBefore = true
			col += 4 - col%4

			continue
		case c == ' ':
		case c == '>':
			if markers == 0 {
				firstMarkerCol = col
			}

			markers++
		case bytes.IndexByte(listMarkerBytes, c) >= 0:
			// A run like `10.` is one marker.
			if i == 0 || bytes.IndexByte(listMarkerBytes, line[i-1]) < 0 {
				if markers == 0 {
					firstMarkerCol = col
				}

				markers++
			}
		default:
			return false
		}

		col++
	}

	return false
}

func leftToOtherTests(src []byte) bool {
	file, _ := parser.Parse(src)

	return fileLeftToOtherTests(file, false)
}

func fileLeftToOtherTests(file *parser.File, inItem bool) bool {
	for _, block := range file.Blocks {
		switch b := block.(type) {
		case *parser.Table:
			return true
		case *parser.FencedCodeBlock:
			unclosedInItem := inItem && b.CloseFence.End.Offset == 0

			if unclosedInItem || file.Src[b.OpenFence.Start.Offset] == ' ' {
				return true
			}
		case *parser.Blockquote:
			if b.Body != nil && fileLeftToOtherTests(b.Body, inItem) {
				return true
			}
		case *parser.List:
			for _, item := range b.Items {
				if item.Body != nil && fileLeftToOtherTests(item.Body, true) {
					return true
				}
			}
		}
	}

	return false
}

var (
	listMarkerBytes           = []byte("*-+.)0123456789")
	oracle                    = goldmark.New(goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()))
	lowercaseDeclaration      = regexp.MustCompile(`<![a-z]`)
	tabAfterTagName           = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9-]*\t`)
	delimiterAfterQuoteMarker = regexp.MustCompile(`(?m)^[ >]*>[*_]`)
	emptyListItemLine         = regexp.MustCompile(`(?m)^[ \t>]*(?:(?:[-+*]|[0-9]{1,9}[.)])[ \t]+[ \t>]*)*(?:[-+*]|[0-9]{1,9}[.)])[ \t\r]*$`)
	codeBlockPattern          = regexp.MustCompile(`(?s)<pre><code(?: class="language-([^"]*)")?>.*?</code></pre>`)
	spacePattern              = regexp.MustCompile(`\s+`)
	tagGapPattern             = regexp.MustCompile(`\s*(<[^>]+>)\s*`)
)

// renderedMeaning renders src and normalizes what Markdown does not mean:
// whitespace outside code blocks, and the code in a language the formatter
// rewrites.
func renderedMeaning(src []byte) string {
	var rendered bytes.Buffer
	if err := oracle.Convert(src, &rendered); err != nil {
		return "render error: " + err.Error()
	}

	// goldmark keeps the CR of a CRLF line ending in code; it ends the line all
	// the same, and the formatter writes one line ending throughout.
	page := strings.ReplaceAll(rendered.String(), "\r\n", "\n")

	var out strings.Builder
	last := 0

	for _, m := range codeBlockPattern.FindAllStringSubmatchIndex(page, -1) {
		out.WriteString(collapseWhitespace(page[last:m[0]]))
		last = m[1]

		if m[2] < 0 || !formatsCode(page[m[2]:m[3]]) {
			out.WriteString(page[m[0]:m[1]])

			continue
		}

		out.WriteString("<pre>formatted ")
		out.WriteString(page[m[2]:m[3]])
		out.WriteString("</pre>")
	}

	out.WriteString(collapseWhitespace(page[last:]))

	return strings.TrimSpace(out.String())
}

func collapseWhitespace(s string) string {
	s = spacePattern.ReplaceAllString(s, " ")
	s = tagGapPattern.ReplaceAllString(s, "$1")

	return strings.ReplaceAll(s, " />", ">")
}

func formatsCode(class string) bool {
	path := codePath([]byte(html.UnescapeString(class)))
	lang := formatter.DetectLanguage(path)

	return lang != formatter.LangUnknown && lang != formatter.LangMarkdown
}
