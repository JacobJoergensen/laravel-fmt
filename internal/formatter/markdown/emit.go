package markdown

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/markdown"
	"github.com/JacobJoergensen/laravel-fmt/internal/textwidth"
)

func emitFile(w *formatter.Writer, file *parser.File, cfg *config.File) {
	emitBlocks(w, file, cfg, true, true, false, false)

	if len(file.Blocks) > 0 {
		w.Newline()
	}
}

// closeQuote: what follows these blocks, outside them, would continue a quote
// that ends them as a lazy line.
func emitBlocks(w *formatter.Writer, file *parser.File, cfg *config.File, blankBetween, avoidLeadingFrontmatter, onMarkerLine, closeQuote bool) {
	var prev parser.Node

	for i, block := range file.Blocks {
		if i > 0 {
			w.Newline()

			if blankBetween && !linkRefRunStaysTight(file.Src, prev, block) {
				w.Newline()
			}
		}

		absorbed := closeQuote
		if i+1 < len(file.Blocks) {
			absorbed = !blankBetween && absorbsLazyLine(file.Blocks[i+1])
		}

		switch {
		case isIgnoreDirectiveComment(prev):
			emitBlockVerbatim(w, file.Src, block)
		case i == 0 && avoidLeadingFrontmatter && leadingBreakFormsFrontmatter(file.Blocks):
			// A `---` thematic break on line 1 pairs with a later `---` line into a
			// frontmatter fence when reparsed; emit `***` (an equivalent break) so
			// the document stays a thematic break rather than flipping to frontmatter.
			w.Write("***")
		case !blankBetween && breakReadsAsSetextUnderline(prev, block):
			// With no blank line above it a `---` break is a level-2 setext
			// underline, which turns the paragraph before it into a heading; `***`
			// is the same break and cannot underline anything.
			w.Write("***")
		default:
			clearAt := clearColumn(file.Blocks, cfg, i, listRunDepth, blankBetween, onMarkerLine)
			inMarkerRun := onMarkerLine && i > 0 && opensListRun(file.Blocks, i)
			emitBlock(w, block, cfg, clearAt, i == 0 && onMarkerLine, inMarkerRun, absorbed)
		}

		prev = block
	}
}

// clearColumn returns the column that must stay outside the list at block i,
// or 0 when nothing does. Emitted with its marker at column 0, a list's content
// column can drop to 2 or 3, and the block after it would then reparse as the
// last item's continuation instead of a sibling. The column is where a
// following code block or HTML block is emitted, or a following list's own
// indent, so the requirement travels back up a run of lists.
func clearColumn(blocks []parser.Node, cfg *config.File, i, depth int, blankBetween, besideMarker bool) int {
	list, ok := blocks[i].(*parser.List)
	if !ok || i+1 >= len(blocks) || depth == 0 {
		return 0
	}

	// An item that opens blank ends at the next blank line, so an empty last item
	// takes in nothing written below one. Clearing anyway puts the list's first
	// marker alone on its line, which cannot interrupt a paragraph above it.
	lastItem := list.Items[len(list.Items)-1]

	if blankBetween && len(bytes.Trim(lastItem.Content, " \t")) == 0 {
		return 0
	}

	switch next := blocks[i+1].(type) {
	case *parser.IndentedCodeBlock:
		return emittedCodeColumn(firstContentLine(next.Content))
	case *parser.List:
		return listIndent(blocks, cfg, i+1, depth-1, blankBetween, besideMarker)
	case *parser.HTMLBlock:
		// An HTML block keeps the indent it was written with, so the column to clear
		// is that indent: the list's content column has to stay above it or the block
		// is read as the last item's continuation instead of a sibling.
		return htmlBlockColumn(next)
	}

	return 0
}

// emittedCodeColumn returns the column the first character of a code line lands
// on once written after the mandatory four spaces. A tab in the line's own indent
// reaches the next tab stop, so it is worth more than the one byte it takes.
func emittedCodeColumn(line []byte) int {
	col := indentedCodeColumn

	for _, c := range line {
		switch c {
		case ' ':
			col++
		case '\t':
			col += tabStop - col%tabStop
		default:
			return col
		}
	}

	return col
}

func htmlBlockColumn(b *parser.HTMLBlock) int {
	first := firstContentLine(b.Content)

	return len(first) - len(bytes.TrimLeft(first, " "))
}

// listIndent returns the indent the list at block I is emitted with: enough
// that its content column clears whatever must stay outside it, capped at
// three because a fourth space makes the marker itself indented code.
func listIndent(blocks []parser.Node, cfg *config.File, i, depth int, blankBetween, besideMarker bool) int {
	clearAt := clearColumn(blocks, cfg, i, depth, blankBetween, besideMarker)
	if clearAt == 0 {
		return 0
	}

	list, ok := blocks[i].(*parser.List)
	if !ok {
		return 0
	}

	inMarkerRun := besideMarker && opensListRun(blocks, i)

	var lastBody []byte

	if inMarkerRun {
		lastItem := list.Items[len(list.Items)-1]
		formatted := formatBody(lastItem.Content, cfg, lastItem.Loose, true, false)
		lastBody = bytes.TrimRight(formatted, "\n")
	}

	pad, _ := listLayout(list, lastBody, clearAt, inMarkerRun)

	return len(pad)
}

func opensListRun(blocks []parser.Node, i int) bool {
	for _, block := range blocks[:i+1] {
		if _, ok := block.(*parser.List); !ok {
			return false
		}
	}

	return true
}

// leadingBreakFormsFrontmatter reports whether the first block is a dash thematic
// break that, emitted as `---` on line 1, would pair with a later `---` line
// (another dash break or a level-2 setext underline) into a frontmatter fence.
func leadingBreakFormsFrontmatter(blocks []parser.Node) bool {
	if len(blocks) < 2 {
		return false
	}

	first, ok := blocks[0].(*parser.ThematicBreak)
	if !ok || (first.Char != 0 && first.Char != '-') {
		return false
	}

	return slices.ContainsFunc(blocks[1:], emitsDashLine)
}

// leadingFenceNeedsEscape reports whether out, the emitted document, opens a
// frontmatter block that file did not have. A paragraph or setext line dedented
// to column 0 can read as the closing fence, and the pair hides the leading text
// from the rendered document. Only text blocks qualify, because the remedy is a
// backslash escape: a leading `---` comes from a thematic break, which
// emitBlocks resolves with `***` instead.
//
// The check reads the emitted bytes rather than the CST because a paragraph's
// lines depend on the prose-wrap mode, and predicting them would be a second
// copy of three emitters.
func leadingFenceNeedsEscape(file *parser.File, out []byte) bool {
	if len(file.Blocks) == 0 {
		return false
	}

	switch file.Blocks[0].(type) {
	case *parser.Paragraph, *parser.SetextHeading:
		return parser.OpensFrontmatter(out)
	}

	return false
}

// breakReadsAsSetextUnderline reports whether block is a dash thematic break
// standing directly under a paragraph, the one position where `---` underlines
// instead of dividing.
func breakReadsAsSetextUnderline(prev, block parser.Node) bool {
	br, ok := block.(*parser.ThematicBreak)
	if !ok || (br.Char != 0 && br.Char != '-') {
		return false
	}

	_, ok = prev.(*parser.Paragraph)

	return ok
}

// linkRefRunStaysTight reports whether two adjacent definitions were written on
// consecutive lines. A run of them is one block of text, which the emitter's
// usual blank line between blocks would push apart. A blank line the author did
// write stays: definitions render as nothing, so grouping them is theirs.
func linkRefRunStaysTight(src []byte, prev, block parser.Node) bool {
	if _, ok := prev.(*parser.LinkRefDef); !ok {
		return false
	}

	if _, ok := block.(*parser.LinkRefDef); !ok {
		return false
	}

	return !blankLineBetween(src, prev.Span().End.Offset, block.Span().Start.Offset)
}

// blankLineBetween reports whether the source between two blocks holds a blank
// line. A block's span already covers its own terminator, so any newline left
// between them is one.
func blankLineBetween(src []byte, from, to int) bool {
	if from < 0 || to > len(src) || from >= to {
		return false
	}

	return bytes.Count(src[from:to], []byte{'\n'}) > 0
}

// absorbsLazyLine reports whether a quote emitted right above block, with no
// blank line between, would swallow block's first line. A lazy line is
// paragraph continuation text, so the blocks that qualify are those whose first
// line cannot interrupt a paragraph: prose, a setext heading's text, indented
// code, a table's header row, a definition (the head of a paragraph, see
// paragraphOpenAtEnd) and raw HTML of the one type that leaves a paragraph open.
func absorbsLazyLine(block parser.Node) bool {
	switch n := block.(type) {
	case *parser.Paragraph, *parser.SetextHeading, *parser.IndentedCodeBlock,
		*parser.Table, *parser.LinkRefDef:
		return true
	case *parser.HTMLBlock:
		return !parser.StartsHTMLBlockInParagraph(firstContentLine(n.Content))
	}

	return false
}

func firstContentLine(content []byte) []byte {
	line, _, _ := bytes.Cut(content, []byte{'\n'})

	return line
}

// emitsDashLine reports whether a block emits a lone `---` line. A frontmatter
// fence is lifted off the raw lines before any block parsing, so it does not
// care which block the line came from, and a code block or raw HTML emits its
// content verbatim. Indented code is absent because its four columns put the
// dashes out of a fence's reach, as does any container's indent.
func emitsDashLine(block parser.Node) bool {
	switch n := block.(type) {
	case *parser.ThematicBreak:
		return n.Char == 0 || n.Char == '-'
	case *parser.SetextHeading:
		return n.Level == 2
	case *parser.FencedCodeBlock:
		return holdsDashLine(n.Content)
	case *parser.HTMLBlock:
		return holdsDashLine(n.Content)
	}

	return false
}

// holdsDashLine reports whether content has a line that reads as a `---`
// frontmatter fence, trailing whitespace insignificant as in frontmatterFence.
// It splits on every line ending the emitter writes, lone CR included.
func holdsDashLine(content []byte) bool {
	for _, line := range splitContentLines(content) {
		trimmed := bytes.TrimRight(line, " \t")

		if bytes.Equal(trimmed, []byte("---")) {
			return true
		}
	}

	return false
}

// formatBody re-emits a list item or blockquote body. It does not go through
// Format because a body keeps its list's tightness and never holds frontmatter;
// it returns content unchanged on parse error.
func formatBody(content []byte, cfg *config.File, blankBetween, onMarkerLine, closeQuote bool) []byte {
	file, errs := parser.ParseBody(content)

	if formatter.JoinErrors(errs) != nil {
		return content
	}

	w := formatter.NewWriter(cfg)
	emitBlocks(w, file, cfg, blankBetween, false, onMarkerLine, closeQuote)

	return w.Bytes()
}

func nestedConfig(cfg *config.File, column int) *config.File {
	nested := *cfg
	nested.PrintWidth -= column

	return &nested
}

func emitBlock(w *formatter.Writer, node parser.Node, cfg *config.File, clearAt int, onMarkerLine, inMarkerRun, closeQuote bool) {
	switch n := node.(type) {
	case *parser.Frontmatter:
		emitFrontmatter(w, n)
	case *parser.ATXHeading:
		emitATXHeading(w, n)
	case *parser.SetextHeading:
		emitSetextHeading(w, n)
	case *parser.ThematicBreak:
		emitThematicBreak(w, n)
	case *parser.Paragraph:
		emitParagraph(w, n, cfg)
	case *parser.FencedCodeBlock:
		emitFencedCode(w, n, cfg)
	case *parser.IndentedCodeBlock:
		emitIndentedCode(w, n)
	case *parser.HTMLBlock:
		emitHTMLBlock(w, n)
	case *parser.LinkRefDef:
		emitLinkRefDef(w, n)
	case *parser.Blockquote:
		emitBlockquote(w, n, cfg, closeQuote)
	case *parser.List:
		emitList(w, n, cfg, clearAt, onMarkerLine, inMarkerRun, closeQuote)
	case *parser.Table:
		emitTable(w, n)
	}
}

func emitFrontmatter(w *formatter.Writer, f *parser.Frontmatter) {
	fence := strings.Repeat(string(f.FenceChar), 3)
	w.Write(fence)
	w.Newline()

	if len(f.Content) > 0 {
		for _, line := range splitContentLines(f.Content) {
			w.WriteBytes(line)
			w.Newline()
		}
	}

	w.Write(fence)
}

func emitATXHeading(w *formatter.Writer, h *parser.ATXHeading) {
	for range h.Level {
		w.Write("#")
	}

	content := bytes.TrimSpace(h.Content)

	if len(content) > 0 {
		w.Space()
		w.WriteBytes(content)

		// Content ending in a `#` run that would reparse as a closing sequence
		// (the run is the whole content or is preceded by a space) needs an
		// explicit closing `#`, or the next pass would strip the content's own
		// trailing hash. `# foo` keeps no closer; `# #` becomes `# # #`.
		if atxContentNeedsClosingHash(content) {
			w.Write(" #")
		}
	}
}

func atxContentNeedsClosingHash(content []byte) bool {
	hashes := 0
	for hashes < len(content) && content[len(content)-1-hashes] == '#' {
		hashes++
	}

	if hashes == 0 {
		return false
	}

	if hashes == len(content) {
		return true
	}

	before := content[len(content)-1-hashes]

	return before == ' ' || before == '\t'
}

func emitSetextHeading(w *formatter.Writer, h *parser.SetextHeading) {
	lines := splitContentLines(h.Content)

	var prev []byte

	for i, line := range lines {
		if i > 0 {
			w.Newline()
		}

		// The content is a paragraph: leading whitespace is insignificant, trailing
		// spaces are a hard break. Only the last line's go, since the spec strips
		// the heading's final whitespace.
		line = bytes.TrimLeft(bytes.TrimRight(line, "\r"), " \t")

		if i == len(lines)-1 {
			line = bytes.TrimRight(line, " \t")
		}

		// A continuation line that would begin a block construct once dedented
		// splits the heading in two; same guard as emitParagraphPreserve.
		rest := line
		if i > 0 && paragraphLineNeedsEscape(prev, line) {
			rest = writeMarkerEscape(w, line)
		}

		// Raw HTML cannot be escaped, so it keeps an indent; same guard as
		// emitParagraphPreserve.
		if i > 0 && parser.StartsHTMLBlockInParagraph(line) {
			w.Write(strings.Repeat(" ", indentedCodeColumn))
		}

		w.WriteBytes(rest)
		prev = line
	}

	w.Newline()

	char := "="

	if h.Level == 2 {
		char = "-"
	}

	w.Write(strings.Repeat(char, 3))
}

func emitThematicBreak(w *formatter.Writer, t *parser.ThematicBreak) {
	char := byte('-')

	if t.Char != 0 {
		char = t.Char
	}

	w.Write(strings.Repeat(string(char), 3))
}

func emitParagraph(w *formatter.Writer, p *parser.Paragraph, cfg *config.File) {
	switch cfg.Markdown.ProseWrap {
	case config.ProseWrapNever:
		emitParagraphNever(w, p)
	case config.ProseWrapAlways:
		emitParagraphAlways(w, p, cfg.PrintWidth)
	default:
		emitParagraphPreserve(w, p)
	}
}

func emitParagraphPreserve(w *formatter.Writer, p *parser.Paragraph) {
	var prev []byte

	for i, line := range splitContentLines(p.Content) {
		if i > 0 {
			w.Newline()
		}

		// Leading whitespace on a paragraph line is insignificant in CommonMark.
		// Emitting it verbatim can indent a trailing paragraph enough to reparse
		// as a list-item continuation, flipping the prior list to lose next pass.
		line = bytes.TrimLeft(bytes.TrimRight(line, "\r"), " \t")

		// A continuation line whose stripped form would start a block construct
		// (heading, blockquote, list, thematic break, fence, setext underline)
		// must be escaped, or it would interrupt the paragraph on the next pass.
		// The first line is the paragraph's start, so it cannot be such a marker.
		rest := line
		if i > 0 && paragraphLineNeedsEscape(prev, line) {
			rest = writeMarkerEscape(w, line)
		}

		// Raw HTML is the one construct a backslash cannot rescue: `\<div>` renders
		// as literal text, a third meaning. Indentation is the remedy the grammar
		// offers instead, and four columns is the least that works, since a line
		// this indented can no longer open a block while a paragraph is open. An
		// indented code block cannot interrupt a paragraph either, so the line stays
		// the inline content it was.
		if i > 0 && parser.StartsHTMLBlockInParagraph(line) {
			w.Write(strings.Repeat(" ", indentedCodeColumn))
		}

		w.WriteBytes(rest)
		prev = line
	}
}

// paragraphLineNeedsEscape reports whether a paragraph continuation line (leading
// whitespace already removed) would begin a block construct that interrupts or
// reinterprets the paragraph. Escaping the leading marker with a backslash keeps
// it paragraph text; a line already starting with `\` does not match, so the
// escape round-trips.
//
// Only the delimiter-row case needs prev, the line emitted above.
func paragraphLineNeedsEscape(prev, line []byte) bool {
	if len(line) == 0 {
		return false
	}

	// A delimiter row reinterprets the paragraph by promoting the line above it
	// into a table header. The parser owns the rule so the two cannot disagree.
	if parser.TablePromotes(prev, line) {
		return true
	}

	switch line[0] {
	case '>':
		return true
	case '#':
		return mdATXStart(line)
	case '`', '~':
		return parser.OpensFence(line)
	case '=':
		return mdSetextUnderline(line, '=')
	case '-':
		return mdSetextUnderline(line, '-') || mdThematicBreak(line) || mdBulletStart(line)
	case '*':
		return mdThematicBreak(line) || mdBulletStart(line)
	case '_':
		// Not mdBulletStart: CommonMark's bullets are `-`, `+` and `*`, so an
		// underscore begins a thematic break and nothing else.
		return mdThematicBreak(line)
	case '+':
		return mdBulletStart(line)
	default:
		return mdOrderedStart(line)
	}
}

// writeMarkerEscape writes line's escape and returns the rest of line. An
// ordered marker escapes its delimiter (`1\.`): a backslash before a digit
// is literal.
func writeMarkerEscape(w *formatter.Writer, line []byte) []byte {
	at := 0
	for at < len(line) && line[at] >= '0' && line[at] <= '9' {
		at++
	}

	w.WriteBytes(line[:at])
	w.Write("\\")

	return line[at:]
}

func mdATXStart(line []byte) bool {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}

	return n >= 1 && n <= 6 && (n == len(line) || line[n] == ' ' || line[n] == '\t')
}

func mdSetextUnderline(line []byte, c byte) bool {
	n := 0
	for n < len(line) && line[n] == c {
		n++
	}

	if n == 0 {
		return false
	}

	for _, b := range line[n:] {
		if b != ' ' && b != '\t' {
			return false
		}
	}

	return true
}

func mdThematicBreak(line []byte) bool {
	count := 0

	for _, b := range line {
		switch b {
		case line[0]:
			count++
		case ' ', '\t':
		default:
			return false
		}
	}

	return count >= 3
}

func mdBulletStart(line []byte) bool {
	if len(line) < 2 || (line[1] != ' ' && line[1] != '\t') {
		return false
	}

	return len(bytes.TrimLeft(line[2:], " \t")) > 0
}

// Only a start number of 1 interrupts a paragraph in CommonMark; other numbers
// need no escape.
func mdOrderedStart(line []byte) bool {
	n := 0
	for n < len(line) && line[n] >= '0' && line[n] <= '9' {
		n++
	}

	// league/commonmark lets only a literal `1` interrupt a paragraph, so `01.`
	// needs no escape, and one would render as a visible backslash.
	if string(line[:n]) != "1" {
		return false
	}

	if n >= len(line) || (line[n] != '.' && line[n] != ')') {
		return false
	}

	rest := line[n+1:]

	return len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') && len(bytes.TrimLeft(rest, " \t")) > 0
}

func emitParagraphNever(w *formatter.Writer, p *parser.Paragraph) {
	// A hard break is content, not layout: it renders as `<br>`, so it survives
	// even here, where every soft break becomes a space.
	segments, breaks := parser.SplitHardBreaks(p.Content)

	for i, segment := range segments {
		if i > 0 {
			w.WriteBytes(breaks[i-1])
			w.Newline()
		}

		joined := bytes.ReplaceAll(segment, []byte("\n"), []byte(" "))
		joined = collapseSpaces(joined)

		w.WriteBytes(bytes.TrimSpace(joined))
	}
}

// emitParagraphAlways word-wraps the paragraph to fit printWidth, splitting on
// parser.InlineWrapUnits so a wrap never falls inside a code span, link, image,
// or autolink. A unit longer than printWidth overflows on its own line;
// breaking it would corrupt its rendered meaning.
func emitParagraphAlways(w *formatter.Writer, p *parser.Paragraph, printWidth int) {
	// Each segment wraps on its own: a hard break renders as `<br>`, so it is a
	// line end the wrapper may not join away.
	segments, breaks := parser.SplitHardBreaks(p.Content)

	for i, segment := range segments {
		if i > 0 {
			w.WriteBytes(breaks[i-1])
			w.Newline()
		}

		wrapSegment(w, segment, printWidth)
	}
}

func wrapSegment(w *formatter.Writer, content []byte, printWidth int) {
	units := parser.InlineWrapUnits(content)
	col := w.Column()

	var prev []byte

	for start := 0; start < len(units); {
		end := fitUnits(units, start, col, printWidth)

		// A wrapped line that needs an escape is one column wider than its
		// units, so the width that chose them has to count that column or the
		// next pass wraps a word earlier.
		if start > 0 && wrapLineNeedsEscape(prev, units, start, end) {
			if narrowed := fitUnits(units, start, col+1, printWidth); narrowed > start &&
				wrapLineNeedsEscape(prev, units, start, narrowed) {
				end = narrowed
			}
		}

		// Breaking here would put raw HTML at the start of the next line, where it
		// opens a block instead of staying the paragraph's inline content. No escape
		// helps (`\<div>` renders as literal text), so the unit keeps its place on
		// this line and the width gives way: a wrong rendering costs more than a
		// long line. A paragraph never starts with one of these, so there is always
		// a line to keep it on.
		for end < len(units) && parser.StartsHTMLBlockInParagraph(units[end]) {
			end++
		}

		first := units[start]

		if start > 0 {
			w.Newline()

			// Same guard, and the same reason, as emitParagraphPreserve.
			if wrapLineNeedsEscape(prev, units, start, end) {
				first = writeMarkerEscape(w, first)
			}
		}

		// A hard break has already ended the line before this segment, so raw HTML
		// opening it cannot be kept company on the line above; four columns is what
		// stops it opening a block instead of staying inline content.
		if col == 0 && start == 0 && parser.StartsHTMLBlockInParagraph(units[0]) {
			w.Write(strings.Repeat(" ", indentedCodeColumn))
		}

		w.WriteBytes(first)

		for _, unit := range units[start+1 : end] {
			w.Space()
			w.WriteBytes(unit)
		}

		prev = bytes.Join(units[start:end], []byte(" "))
		start = end
		col = 0
	}
}

func collapseSpaces(b []byte) []byte {
	var out bytes.Buffer

	lastWasSpace := false

	for _, c := range b {
		if c == ' ' || c == '\t' {
			if !lastWasSpace {
				out.WriteByte(' ')
			}

			lastWasSpace = true

			continue
		}

		out.WriteByte(c)
		lastWasSpace = false
	}

	return out.Bytes()
}

// splitContentLines splits content on every CommonMark line ending: CRLF, a
// lone CR and LF. A lone CR swallowed into the line before it would merge two
// source lines, and the next pass would lay them out differently.
func splitContentLines(content []byte) [][]byte {
	lines := make([][]byte, 0, bytes.Count(content, []byte("\n"))+1)
	start := 0

	for i := 0; i < len(content); i++ {
		if c := content[i]; c != '\n' && c != '\r' {
			continue
		} else if c == '\r' && i+1 < len(content) && content[i+1] == '\n' {
			lines = append(lines, content[start:i])
			i++
		} else {
			lines = append(lines, content[start:i])
		}

		start = i + 1
	}

	return append(lines, content[start:])
}

func emitFencedCode(w *formatter.Writer, b *parser.FencedCodeBlock, cfg *config.File) {
	content := b.Content
	if formatted, ok := formatCode(b, cfg); ok {
		content = formatted
	}

	length := fenceLength(content, b.FenceChar)
	fence := strings.Repeat(string(b.FenceChar), length)

	w.Write(fence)

	if len(b.Info) > 0 {
		// CommonMark measures an opening fence by its run length, so a tilde
		// info string written tight would lengthen a `~~~` fence past its
		// closer. A backtick fence's info string cannot hold a backtick.
		if b.FenceChar == '~' && b.Info[0] == '~' {
			w.Space()
		}

		w.WriteBytes(b.Info)
	}

	w.Newline()

	switch {
	case len(content) > 0:
		for _, line := range splitContentLines(content) {
			w.WriteBytes(line)
			w.Newline()
		}
	case fencedContentIsOneBlankLine(b):
		w.Newline()
	}

	w.Write(fence)
}

// fencedContentIsOneBlankLine reports a block whose content is a single blank
// line, which Content cannot tell from no content since it drops its last
// terminator. A blank line in a code block renders, so the spans decide.
func fencedContentIsOneBlankLine(b *parser.FencedCodeBlock) bool {
	contentEnd := b.End.Offset
	if b.CloseFence.End.Offset > 0 {
		contentEnd = b.CloseFence.Start.Offset
	}

	return contentEnd > b.OpenFence.End.Offset
}

// fenceLength returns the shortest fence that safely wraps content: at least 3,
// and at least one longer than the longest run of char in content so an
// internal run cannot close the block early.
func fenceLength(content []byte, char byte) int {
	longest := 0
	current := 0

	for _, c := range content {
		if c == char {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}

	return max(longest+1, 3)
}

// emitIndentedCode re-indents each line with exactly four spaces, the
// CommonMark trigger width, regardless of tabWidth/useTabs, because indented
// code is a syntactic construct, not a stylistic choice.
func emitIndentedCode(w *formatter.Writer, b *parser.IndentedCodeBlock) {
	for i, line := range splitContentLines(b.Content) {
		if i > 0 {
			w.Newline()
		}

		w.Write("    ")
		w.WriteBytes(line)
	}
}

// emitHTMLBlock passes the block through line for line: CommonMark's HTML-block
// triggers are looser than a real HTML parser would accept.
func emitHTMLBlock(w *formatter.Writer, b *parser.HTMLBlock) {
	for i, line := range splitContentLines(b.Content) {
		if i > 0 {
			w.Newline()
		}

		w.WriteBytes(line)
	}
}

func emitLinkRefDef(w *formatter.Writer, d *parser.LinkRefDef) {
	w.Write("[")
	w.WriteBytes(d.Label)
	w.Write("]: ")

	// The parser strips the destination's `<>`, so they have to be put back
	// whenever the bare form would not read as the same destination.
	if linkDestNeedsAngles(d.Dest) {
		writeAngleLinkDest(w, d.Dest)
	} else {
		w.WriteBytes(d.Dest)
	}

	if d.Title != nil {
		w.Space()
		emitLinkTitle(w, d.Title)
	}
}

// emitLinkTitle writes a title in the narrowest delimiters that read it back
// whole. The parser strips them, so re-emitting a title inside a quote it
// carries ends the title early and the definition stops parsing altogether. A
// title holding an unescaped instance of all three delimiters, which a
// parenthesized source can produce, leaves escaping as the only way out.
func emitLinkTitle(w *formatter.Writer, title []byte) {
	switch {
	case !containsUnescaped(title, '"'):
		writeDelimited(w, '"', '"', title)
	case !containsUnescaped(title, '\''):
		writeDelimited(w, '\'', '\'', title)
	case !containsUnescaped(title, '(') && !containsUnescaped(title, ')'):
		writeDelimited(w, '(', ')', title)
	default:
		writeDelimited(w, '"', '"', escapeUnescaped(title, '"'))
	}
}

func writeDelimited(w *formatter.Writer, open, closer byte, body []byte) {
	w.WriteBytes([]byte{open})
	w.WriteBytes(body)
	w.WriteBytes([]byte{closer})
}

func containsUnescaped(s []byte, ch byte) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case ch:
			return true
		}
	}

	return false
}

func escapeUnescaped(s []byte, ch byte) []byte {
	out := make([]byte, 0, len(s)+2)

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			out = append(out, s[i], s[i+1])
			i++

			continue
		}

		if s[i] == ch {
			out = append(out, '\\')
		}

		out = append(out, s[i])
	}

	return out
}

// writeAngleLinkDest writes a link destination inside `<>`, escaping a `>` that
// would otherwise close the form early and leave the rest as junk. One that is
// already escaped is left as it is, so the emission round-trips. A trailing
// backslash is literal in a bare destination but would escape the closing `>`,
// so it is doubled.
func writeAngleLinkDest(w *formatter.Writer, dest []byte) {
	w.Write("<")

	for i := 0; i < len(dest); i++ {
		if dest[i] == '\\' && i+1 < len(dest) {
			w.WriteBytes(dest[i : i+2])
			i++

			continue
		}

		if dest[i] == '>' || dest[i] == '\\' {
			w.Write("\\")
		}

		w.WriteBytes(dest[i : i+1])
	}

	w.Write(">")
}

// linkDestNeedsAngles reports whether a link destination has to be emitted
// inside `<>`. CommonMark accepts the bare form only for a nonempty run that
// holds no space or control character, does not open with `<`, and keeps its
// parentheses balanced. Emitting a destination that breaks any of those bare
// changes where the link points, or ends the definition early: `<b c d>` written
// as `b c d` leaves `b` as the destination and the rest as junk.
func linkDestNeedsAngles(dest []byte) bool {
	if len(dest) == 0 || dest[0] == '<' {
		return true
	}

	depth := 0

	for i := 0; i < len(dest); i++ {
		switch c := dest[i]; {
		case c == '\\':
			// Skipping the escaped byte keeps an escaped paren from nesting,
			// but CommonMark escapes only punctuation, so a backslashed space
			// or control character still ends a bare destination.
			if i+1 < len(dest) && (dest[i+1] <= ' ' || dest[i+1] == 0x7f) {
				return true
			}

			i++
		case c <= ' ' || c == 0x7f:
			return true
		case c == '(':
			depth++
		case c == ')':
			depth--

			if depth < 0 {
				return true
			}
		}
	}

	return depth != 0
}

// emitBlockquote re-formats the body and prefixes each line with `> `, so inner
// constructs get the same canonical layout (`>    # H` becomes `> # H`). A blank
// line emits as a lone `>` to avoid trailing whitespace.
func emitBlockquote(w *formatter.Writer, b *parser.Blockquote, cfg *config.File, closeQuote bool) {
	formatted := bytes.TrimRight(formatBody(b.Content, nestedConfig(cfg, len("> ")), true, false, false), "\n")

	for i, line := range bytes.Split(formatted, []byte("\n")) {
		if i > 0 {
			w.Newline()
		}

		line = bytes.TrimRight(line, "\r")

		if len(line) == 0 {
			w.Write(">")

			continue
		}

		w.Write("> ")

		// After `> ` a leading tab covers two columns, not four, and the parser
		// reads it that way, so it is written as those spaces and the next pass
		// measures the same.
		w.WriteBytes(expandTabOnGrid(line, len("> ")))
	}

	// The marker-only line the source closed the quote with: without it the
	// text below is a lazy continuation and moves inside the quote.
	if closeQuote {
		w.Newline()
		w.Write(">")
	}
}

// expandTabOnGrid rewrites a leading tab that lands at column col into the spaces
// it covers there, so the bytes measure the same wherever they are re-read.
func expandTabOnGrid(line []byte, col int) []byte {
	if len(line) == 0 || line[0] != '\t' || col%tabStop == 0 {
		return line
	}

	return append(bytes.Repeat([]byte(" "), tabStop-col%tabStop), line[1:]...)
}

// tabStop is CommonMark's tab width for the purposes of block structure.
const tabStop = 4

func emitList(w *formatter.Writer, l *parser.List, cfg *config.File, clearAt int, onMarkerLine, inMarkerRun, closeQuote bool) {
	repeats := l.Ordered && listRepeatsStart(l)

	bodies := make([][]byte, len(l.Items))

	// The widest marker is the last one, or the repeated start; with the one
	// space after it, that is where the bodies start.
	bodyCfg := nestedConfig(cfg, len(listMarker(l, len(l.Items)-1, repeats))+1)

	for i, item := range l.Items {
		last := i == len(l.Items)-1
		bodies[i] = bytes.TrimRight(formatBody(item.Content, bodyCfg, item.Loose, true, closeQuote && last), "\n")
	}

	pad := ""
	firstPad := ""
	sourceMarkers := false

	if clearAt > 0 {
		pad, sourceMarkers = listLayout(l, bodies[len(bodies)-1], clearAt, inMarkerRun)
		firstPad = pad

		// A pad beside the parent's marker pushes that marker onto a line of its
		// own, and an item opening blank cannot interrupt a paragraph above it. The
		// source markers clear where the source shared the line.
		if onMarkerLine {
			for _, source := range []bool{false, true} {
				if restPad, ok := padBesideMarker(l, bodies, repeats, source, clearAt); ok {
					pad, firstPad, sourceMarkers = restPad, "", source

					break
				}
			}
		}
	}

	for i := range l.Items {
		if i > 0 {
			w.Newline()

			if l.Loose {
				w.Newline()
			}
		}

		marker := itemMarker(l, i, repeats, sourceMarkers)

		itemPad := pad
		if i == 0 {
			itemPad = firstPad
		}

		// The pad tops out at three columns, since a fourth would make the marker
		// itself indented code, so whatever clearance it still owes goes into the
		// space after the marker, where the item takes a gap at all.
		gap := 1
		if clearAt > 0 && takesGap(marker, bodies[i]) {
			gap = listGapClearing(marker, itemPad, clearAt)
		}

		emitListItem(w, bodies[i], marker, itemPad, gap)
	}
}

// indentedCodeColumn is CommonMark's indented-code trigger width: a chunk
// indented this far opens a code block, and a list item whose content column
// reaches it swallows one that follows the list.
const indentedCodeColumn = 4

// listRunDepth bounds how far back up a run of lists an indented code block's
// clearance requirement travels. Past the first list every requirement is at most
// indentedCodeColumn-1 and each marker takes a column off it, so a list further
// away than that needs no indent of its own.
const listRunDepth = indentedCodeColumn

// listLayout returns the pad of a list that keeps clearAt outside it and
// whether it keeps its source markers. A list following one beside a parent's
// marker in a run takes the least pad its last item clears with: every column
// of it is one that list has to clear without a pad of its own.
func listLayout(l *parser.List, lastBody []byte, clearAt int, inMarkerRun bool) (string, bool) {
	repeats := l.Ordered && listRepeatsStart(l)

	if inMarkerRun {
		last := len(l.Items) - 1

		for pad := range indentedCodeColumn {
			for _, source := range []bool{false, true} {
				if clearsWithoutPad(itemMarker(l, last, repeats, source), lastBody, clearAt-pad) {
					return strings.Repeat(" ", pad), source
				}
			}
		}
	}

	// Renumbering can shorten a marker (`000)` to `0)`), taking the content
	// column with it and putting the code block beyond what a legal pad can
	// clear. Renumbering is cosmetic and block structure is not, so the source
	// markers win: with their original widths the source's own indent is always
	// a large enough pad, because the block was a sibling there.
	sourceMarkers := clearAt-shortestMarker(l, repeats) > indentedCodeColumn-1

	return listPadClearing(l, repeats, sourceMarkers, clearAt), sourceMarkers
}

// listPadClearing returns the marker indent that keeps whatever sits at
// clearAt outside the list: an item's content column is pad+len(marker)+1, so
// the shortest marker needs the most pad and sets the width for the whole
// list. The pad is capped at three, since a fourth space makes the marker
// indented code.
func listPadClearing(l *parser.List, repeats, sourceMarkers bool, clearAt int) string {
	shortest := shortestMarker(l, repeats)

	if sourceMarkers {
		shortest = shortestSourceMarker(l)
	}

	return strings.Repeat(" ", min(max(clearAt-shortest, 0), indentedCodeColumn-1))
}

// listGapClearing returns how many spaces separate marker from its content. One
// is canonical; when the pad has hit its cap and the content column is still not
// above what has to stay outside the list, the rest is made up here. CommonMark
// reads up to four spaces after a marker as separation. Each item gets the gap
// its own marker needs, so every content column lands on the same one.
func listGapClearing(marker, pad string, clearAt int) int {
	return min(max(clearAt+1-len(pad)-len(marker), 1), indentedCodeColumn)
}

// padBesideMarker returns the pad for the items after the first when the first
// shares its parent's marker line and so takes none, and whether the last item
// then clears clearAt. The pad stays left of the first item's content column,
// or the second item would nest inside the first.
func padBesideMarker(l *parser.List, bodies [][]byte, repeats, sourceMarkers bool, clearAt int) (string, bool) {
	last := len(l.Items) - 1
	lastMarker := itemMarker(l, last, repeats, sourceMarkers)

	if clearsWithoutPad(lastMarker, bodies[last], clearAt) {
		return "", true
	}

	if last == 0 {
		return "", false
	}

	first := itemMarker(l, 0, repeats, sourceMarkers)
	firstColumn := len(first) + 1

	if takesGap(first, bodies[0]) {
		firstColumn = len(first) + listGapClearing(first, "", clearAt)
	}

	fullPad := listPadClearing(l, repeats, sourceMarkers, clearAt)
	pad := min(len(fullPad), firstColumn-1)

	return strings.Repeat(" ", pad), clearsWithoutPad(lastMarker, bodies[last], clearAt-pad)
}

func clearsWithoutPad(marker string, body []byte, clearAt int) bool {
	widest := 1
	if takesGap(marker, body) {
		widest = indentedCodeColumn
	}

	return len(marker)+widest > clearAt
}

// takesGap reports whether the spaces after marker set the item's content
// column. CommonMark pins it one space past the marker when the first line is
// empty or opens with indented code, and emitListItem moves an indented first
// line, or one completing a thematic break it cannot split, below the marker,
// which opens the item blank.
func takesGap(marker string, body []byte) bool {
	first := firstContentLine(body)

	if len(first) == 0 || first[0] == ' ' {
		return false
	}

	return !markerLineIsThematicBreak(marker, first) || splitsAfterNestedMarker(marker, first)
}

func shortestMarker(l *parser.List, repeats bool) int {
	shortest := 0

	for i := range l.Items {
		if width := len(listMarker(l, i, repeats)); i == 0 || width < shortest {
			shortest = width
		}
	}

	return shortest
}

func shortestSourceMarker(l *parser.List) int {
	shortest := 0

	for i := range l.Items {
		if width := len(l.Items[i].MarkerBytes); i == 0 || width < shortest {
			shortest = width
		}
	}

	return shortest
}

func itemMarker(l *parser.List, i int, repeats, sourceMarkers bool) string {
	if sourceMarkers {
		return string(l.Items[i].MarkerBytes)
	}

	return listMarker(l, i, repeats)
}

// listMarker returns the marker for item i. Unordered items keep the bullet
// verbatim; rewriting `*` to `-` could merge an adjacent list of the other
// family into this one. Ordered items renumber from Start (or repeat Start when
// repeats is set); the terminator (`.` or `)`) is kept, since `1.` and `1)` are
// distinct CommonMark list types.
func listMarker(l *parser.List, i int, repeats bool) string {
	if !l.Ordered {
		return string(l.Marker)
	}

	number := l.StartNumber

	if !repeats {
		number = l.StartNumber + i
	}

	return strconv.Itoa(number) + string(l.Marker)
}

func listRepeatsStart(l *parser.List) bool {
	if len(l.Items) < 2 {
		return false
	}

	return markerNumber(l.Items[1].MarkerBytes) == l.StartNumber
}

func markerNumber(marker []byte) int {
	n := 0

	for _, c := range marker {
		if c < '0' || c > '9' {
			break
		}

		n = n*10 + int(c-'0')
	}

	return n
}

// emitTable pads each column to its widest cell, measured in its `\|`-escaped
// form so a cell holding a literal `|` stays aligned.
func emitTable(w *formatter.Writer, t *parser.Table) {
	cols := len(t.Header)

	header := escapeTableRow(t.Header, cols)

	rows := make([][]string, len(t.Rows))

	for i, r := range t.Rows {
		rows[i] = escapeTableRow(r, cols)
	}

	widths := make([]int, cols)

	for i, cell := range header {
		widths[i] = max(widths[i], utf8.RuneCountInString(cell))
	}

	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}

	for i, a := range t.Aligns {
		minWidth := 3

		switch a {
		case parser.TableAlignLeft, parser.TableAlignRight:
			minWidth = 4
		case parser.TableAlignCenter:
			minWidth = 5
		case parser.TableAlignNone:
		}

		widths[i] = max(widths[i], minWidth)
	}

	emitTableRow(w, header, widths, t.Aligns)
	w.Newline()
	emitTableSeparator(w, t.Aligns, widths)

	for _, row := range rows {
		w.Newline()
		emitTableRow(w, row, widths, t.Aligns)
	}
}

func escapeTableRow(cells []parser.TableCell, cols int) []string {
	out := make([]string, cols)

	for i := range cols {
		if i < len(cells) {
			out[i] = escapeTableCell(cells[i].Content)
		}
	}

	return out
}

func escapeTableCell(content []byte) string {
	if !bytes.ContainsRune(content, '|') {
		return string(content)
	}

	var b strings.Builder

	b.Grow(len(content) + 1)

	for _, c := range content {
		if c == '|' {
			b.WriteByte('\\')
		}

		b.WriteByte(c)
	}

	return b.String()
}

func emitTableRow(w *formatter.Writer, cells []string, widths []int, aligns []parser.TableAlign) {
	w.Write("|")

	for i, cell := range cells {
		w.Space()

		pad := widths[i] - utf8.RuneCountInString(cell)

		switch aligns[i] {
		case parser.TableAlignRight:
			if pad > 0 {
				w.Write(strings.Repeat(" ", pad))
			}

			w.Write(cell)
		case parser.TableAlignCenter:
			left := pad / 2
			right := pad - left

			if left > 0 {
				w.Write(strings.Repeat(" ", left))
			}

			w.Write(cell)

			if right > 0 {
				w.Write(strings.Repeat(" ", right))
			}
		default:
			w.Write(cell)

			if pad > 0 {
				w.Write(strings.Repeat(" ", pad))
			}
		}

		w.Space()
		w.Write("|")
	}
}

func emitTableSeparator(w *formatter.Writer, aligns []parser.TableAlign, widths []int) {
	w.Write("|")

	for i, a := range aligns {
		w.Space()

		switch a {
		case parser.TableAlignLeft:
			w.Write(":")
			w.Write(strings.Repeat("-", widths[i]-1))
		case parser.TableAlignRight:
			w.Write(strings.Repeat("-", widths[i]-1))
			w.Write(":")
		case parser.TableAlignCenter:
			w.Write(":")
			w.Write(strings.Repeat("-", widths[i]-2))
			w.Write(":")
		default:
			w.Write(strings.Repeat("-", widths[i]))
		}

		w.Space()
		w.Write("|")
	}
}

// emitListItem writes a pre-formatted item body. A blank line emits without the
// continuation indent so it carries no trailing whitespace.
func emitListItem(w *formatter.Writer, formatted []byte, marker, pad string, gap int) {
	indent := pad + strings.Repeat(" ", len(marker)+gap)
	lines := bytes.Split(formatted, []byte("\n"))

	raw := []byte(nil)
	first := []byte(nil)

	if len(lines) > 0 {
		raw = bytes.TrimRight(lines[0], "\r")
		first = trimInsignificantIndent(raw)
	}

	w.Write(pad)
	w.Write(marker)

	// A marker whose first content line is a nested list (`- - a`) shares the
	// marker line, but if that combined line reads as a thematic break (`- - -`)
	// the rendered item flips to a horizontal rule.
	switch {
	case splitsAfterNestedMarker(marker, raw):
		w.Write(strings.Repeat(" ", gap))
		w.WriteBytes(raw[:1])
		w.Newline()
		w.Write(indent + "  ")
		w.WriteBytes(raw[2:])
	case len(first) > 0 && markerLineIsThematicBreak(marker, first):
		w.Newline()
		w.Write(indent)
		w.WriteBytes(raw)
	case len(first) < len(raw):
		// An indented first line is load-bearing: clearColumn sized that indent so the
		// block after it stays outside, and sharing the marker line drops the indent
		// (or, kept, shifts every column after it). The marker takes its own line and
		// the body keeps the geometry it was formatted with.
		w.Newline()
		w.Write(indent)
		w.WriteBytes(raw)
	default:
		w.Write(strings.Repeat(" ", gap))
		w.WriteBytes(first)
	}

	for i := 1; i < len(lines); i++ {
		line := bytes.TrimRight(lines[i], "\r")

		w.Newline()

		if len(line) > 0 {
			w.Write(indent)
			w.WriteBytes(line)
		}
	}
}

// splitsAfterNestedMarker reports whether a marker line that would read as a
// thematic break splits after the nested marker, what follows keeping its
// column. Splitting before it opens this item blank, which cannot interrupt a
// paragraph above. A break emits without spaces and text is escaped, so `* *`
// here is a marker and its gap. A wider gap sets the nested content column,
// which opening blank would move; indented code is pinned there already.
func splitsAfterNestedMarker(marker string, first []byte) bool {
	if len(first) < 3 || first[1] != ' ' || !markerLineIsThematicBreak(marker, first) {
		return false
	}

	rest := first[2:]
	spaces := len(rest) - len(bytes.TrimLeft(rest, " "))

	return spaces == 0 || spaces >= indentedCodeColumn
}

func markerLineIsThematicBreak(marker string, line []byte) bool {
	char := marker[0]
	if char != '-' && char != '*' && char != '_' {
		return false
	}

	count := 1

	for _, c := range line {
		switch c {
		case char:
			count++
		case ' ', '\t':
		default:
			return false
		}
	}

	return count >= 3
}

// columns returns the width b paints in terminal columns, not runes, since a
// CJK character takes two.
func columns(b []byte) int {
	n := 0

	for _, r := range string(b) {
		n += textwidth.Rune(r)
	}

	return n
}

// fitUnits returns the index one past the last unit that fits on a line
// starting at column col, always taking at least one unit so a unit wider than
// the whole line still makes progress.
func fitUnits(units [][]byte, start, col, printWidth int) int {
	end := start + 1
	width := col + columns(units[start])

	for end < len(units) {
		next := 1 + columns(units[end])

		if width+next > printWidth {
			break
		}

		width += next
		end++
	}

	return end
}

// wrapLineNeedsEscape reports whether the units chosen for one wrapped line
// would begin a block construct, which would interrupt the paragraph on the
// next pass. The whole line is joined for the check because a thematic break or
// a setext underline is only one as a complete line.
func wrapLineNeedsEscape(prev []byte, units [][]byte, start, end int) bool {
	return paragraphLineNeedsEscape(prev, bytes.Join(units[start:end], []byte(" ")))
}

// trimInsignificantIndent drops fewer than four leading spaces from a list
// item's first content line, where no renderer shows them and where they would
// move again on the next pass. Four or more are an indented code block's
// content and stay.
func trimInsignificantIndent(line []byte) []byte {
	spaces := 0

	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}

	if spaces == 0 || spaces >= indentedCodeColumn {
		return line
	}

	return line[spaces:]
}
