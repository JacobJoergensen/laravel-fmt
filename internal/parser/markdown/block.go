package markdown

import (
	"bytes"
	"hash/maphash"
)

type parser struct {
	src []byte
	pos Position

	// The line peekLine last scanned. Block-start checks peek the same line
	// many times, and a long line rescanned for each made deep nesting
	// quadratic in the line's length.
	peekOff, peekEnd, peekNext int
	peeked                     bool

	// probe skips container bodies and looseness: paragraphOpenAtEnd parses a
	// body for every lazy line and reads only its last block's Content.
	probe bool

	// lazy memoizes paragraphOpenAtEnd by a 128-bit hash of its content and is
	// shared by every parser of one document. Keying by the bytes themselves
	// would hold as much memory as a hostile document costs in time.
	lazy map[[2]uint64]bool
}

var lazySeeds = [2]maphash.Seed{maphash.MakeSeed(), maphash.MakeSeed()}

func (p *parser) child(src []byte, probe bool) *parser {
	return &parser{src: src, pos: Position{Line: 1, Column: 1}, probe: probe, lazy: p.lazy}
}

func (p *parser) eof() bool { return p.pos.Offset >= len(p.src) }

func (p *parser) peekLine() (line []byte, nextOff int) {
	if !p.peeked || p.peekOff != p.pos.Offset {
		line, nextOff = lineAt(p.src, p.pos.Offset)
		p.peekOff, p.peekEnd, p.peekNext, p.peeked = p.pos.Offset, p.pos.Offset+len(line), nextOff, true
	}

	return p.src[p.peekOff:p.peekEnd], p.peekNext
}

// lineAt returns the line starting at off without its terminator, and the offset
// of the next line. It accepts `\n`, `\r\n` and a lone `\r` as terminators.
func lineAt(src []byte, off int) (line []byte, nextOff int) {
	start := off
	n := len(src)

	for off < n && src[off] != '\n' && src[off] != '\r' {
		off++
	}

	termStart := off

	if off < n && src[off] == '\r' {
		off++

		if off < n && src[off] == '\n' {
			off++
		}
	} else if off < n && src[off] == '\n' {
		off++
	}

	return src[start:termStart], off
}

func (p *parser) advancePastLine() {
	_, nextOff := p.peekLine()

	for p.pos.Offset < nextOff {
		p.advanceByte()
	}
}

func (p *parser) advanceByte() {
	if p.eof() {
		return
	}

	if p.src[p.pos.Offset] == '\n' {
		p.pos.Line++
		p.pos.Column = 1
	} else {
		p.pos.Column++
	}

	p.pos.Offset++
}

func (p *parser) skipBlankLines() {
	for !p.eof() {
		line, _ := p.peekLine()

		if !isBlankLine(line) {
			return
		}

		p.advancePastLine()
	}
}

func (p *parser) parseFile(document bool) []Node {
	var blocks []Node

	if document {
		if fm := p.parseFrontmatter(); fm != nil {
			blocks = append(blocks, fm)
		}
	}

	for !p.eof() {
		p.skipBlankLines()

		if p.eof() {
			break
		}

		block := p.parseBlock()

		if block != nil {
			blocks = append(blocks, block)
		}
	}

	return blocks
}

func (p *parser) parseBlock() Node {
	line, _ := p.peekLine()

	if level, ok := classifyATXHeading(line); ok {
		return p.parseATXHeading(level)
	}

	if isThematicBreak(line) {
		return p.parseThematicBreak()
	}

	if char, length, info, ok := openFence(line); ok {
		return p.parseFencedCode(char, length, info)
	}

	if isBlockquote(line) {
		return p.parseBlockquote()
	}

	if _, marker, ordered, startNum, contentColumn, ok := listItemStart(line); ok {
		return p.parseList(marker, ordered, startNum, contentColumn)
	}

	if isIndentedCode(line) {
		return p.parseIndentedCode()
	}

	if isHTMLBlockStart(line) {
		return p.parseHTMLBlock()
	}

	return p.parseParagraph()
}

// OpensFrontmatter reports whether src begins with a frontmatter fence that a
// later line closes. The formatter checks its own output with it: a paragraph
// line dedented to column 0 can read as a closing fence the source never had.
func OpensFrontmatter(src []byte) bool {
	_, _, ok := frontmatterFence(src)

	return ok
}

func frontmatterFence(src []byte) (fenceChar byte, closeOff int, ok bool) {
	openLine, openNext := lineAt(src, 0)

	// Trailing spaces and tabs on a fence are insignificant, so `---` and
	// `--- ` classify the same.
	switch {
	case bytes.Equal(bytes.TrimRight(openLine, " \t"), []byte("---")):
		fenceChar = '-'
	case bytes.Equal(bytes.TrimRight(openLine, " \t"), []byte("+++")):
		fenceChar = '+'
	default:
		return 0, 0, false
	}

	fence := bytes.Repeat([]byte{fenceChar}, 3)

	for off := openNext; off < len(src); {
		line, nextOff := lineAt(src, off)

		if bytes.Equal(bytes.TrimRight(line, " \t"), fence) {
			return fenceChar, off, true
		}

		off = nextOff
	}

	return 0, 0, false
}

func (p *parser) parseFrontmatter() Node {
	if p.pos.Offset != 0 {
		return nil
	}

	fenceChar, closeOff, ok := frontmatterFence(p.src)
	if !ok {
		return nil
	}

	openStart := p.pos
	p.advancePastLine()
	openEnd := p.pos

	contentStart := p.pos.Offset

	for p.pos.Offset < closeOff {
		p.advanceByte()
	}

	content := p.src[contentStart:closeOff]

	if n := len(content); n > 0 && content[n-1] == '\n' {
		content = content[:n-1]

		if m := len(content); m > 0 && content[m-1] == '\r' {
			content = content[:m-1]
		}
	}

	closeStart := p.pos
	p.advancePastLine()
	closeEnd := p.pos

	return &Frontmatter{
		FenceChar:  fenceChar,
		OpenFence:  Span{Start: openStart, End: openEnd},
		Content:    content,
		CloseFence: Span{Start: closeStart, End: closeEnd},
	}
}

func (p *parser) parseATXHeading(level int) Node {
	startPos := p.pos
	line, _ := p.peekLine()

	i := skipOptionalIndent(line)

	for i < len(line) && line[i] == '#' {
		i++
	}

	i = skipSpaceTab(line, i)

	contentStart := i
	end := len(line)

	for end > contentStart && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}

	trailStart := end

	for trailStart > contentStart && line[trailStart-1] == '#' {
		trailStart--
	}

	if trailStart < end {
		if trailStart == contentStart || line[trailStart-1] == ' ' || line[trailStart-1] == '\t' {
			end = trailStart

			for end > contentStart && (line[end-1] == ' ' || line[end-1] == '\t') {
				end--
			}
		}
	}

	content := line[contentStart:end]

	p.advancePastLine()

	return &ATXHeading{
		Start:   startPos,
		End:     p.pos,
		Level:   level,
		Content: content,
	}
}

func (p *parser) parseThematicBreak() Node {
	startPos := p.pos
	line, _ := p.peekLine()

	var char byte

	for _, c := range line {
		if c == '-' || c == '_' || c == '*' {
			char = c

			break
		}
	}

	p.advancePastLine()

	return &ThematicBreak{
		Start: startPos,
		End:   p.pos,
		Char:  char,
	}
}

func (p *parser) parseFencedCode(fenceChar byte, fenceLen int, info []byte) Node {
	startPos := p.pos
	openLine, _ := p.peekLine()
	fenceIndent := skipOptionalIndent(openLine)
	p.advancePastLine()
	openEnd := p.pos

	contentStart := p.pos.Offset
	var closeFence Span

	for !p.eof() {
		linePos := p.pos
		line, _ := p.peekLine()

		if isClosingFence(line, fenceChar, fenceLen) {
			p.advancePastLine()
			closeFence = Span{Start: linePos, End: p.pos}

			break
		}

		p.advancePastLine()
	}

	var contentEnd int

	// Test whether a close fence was found by its own span, not by comparing to
	// contentStart: an empty block has the close on the line right after the open
	// (closeFence.Start == contentStart), which the comparison would misread as
	// no-close and pull the close fence in as content.
	if closeFence.End.Offset > 0 {
		contentEnd = closeFence.Start.Offset
	} else {
		contentEnd = p.pos.Offset
	}

	content := p.src[contentStart:contentEnd]

	if n := len(content); n > 0 && content[n-1] == '\n' {
		content = content[:n-1]

		if m := len(content); m > 0 && content[m-1] == '\r' {
			content = content[:m-1]
		}
	} else if n := len(content); n > 0 && content[n-1] == '\r' {
		content = content[:n-1]
	}

	if fenceIndent > 0 {
		content = stripFenceIndent(content, fenceIndent)
	}

	// A fence with no closing line is not an error: CommonMark closes it at the
	// end of the containing block, so `* ~~~` is a legal document.

	return &FencedCodeBlock{
		FenceChar:  fenceChar,
		OpenFence:  Span{Start: startPos, End: openEnd},
		Info:       info,
		Content:    content,
		CloseFence: closeFence,
		End:        p.pos,
	}
}

// stripFenceIndent removes up to n columns of leading whitespace from each
// line of content, which is what an opening fence indented n columns does to
// the block it opens (CommonMark). Each line keeps its own terminator, so the
// result differs from content in indentation alone.
func stripFenceIndent(content []byte, n int) []byte {
	out := make([]byte, 0, len(content))

	for off := 0; off < len(content); {
		line, next := lineAt(content, off)
		out = append(out, stripIndentColumns(line, n)...)
		out = append(out, content[off+len(line):next]...)
		off = next
	}

	return out
}

func (p *parser) parseIndentedCode() Node {
	startPos := p.pos
	var contentLines [][]byte
	endPos := p.pos

	for !p.eof() {
		line, _ := p.peekLine()

		if isIndentedCode(line) {
			contentLines = append(contentLines, stripIndentColumns(line, 4))
			p.advancePastLine()
			endPos = p.pos

			continue
		}

		if isBlankLine(line) {
			// Blank lines between indented chunks form one code block (CommonMark),
			// so look past the whole run, not just the next line: otherwise two
			// blanks split the block and it re-merges on the next pass.
			savedPos := p.pos

			blanks := 0

			for !p.eof() {
				next, _ := p.peekLine()

				if !isBlankLine(next) {
					break
				}

				p.advancePastLine()
				blanks++
			}

			if p.eof() {
				p.pos = savedPos

				break
			}

			nextLine, _ := p.peekLine()

			if !isIndentedCode(nextLine) {
				p.pos = savedPos

				break
			}

			for range blanks {
				contentLines = append(contentLines, nil)
			}

			continue
		}

		break
	}

	content := bytes.Join(contentLines, []byte{'\n'})

	return &IndentedCodeBlock{
		Start:   startPos,
		End:     endPos,
		Content: content,
	}
}

func (p *parser) parseHTMLBlock() Node {
	startPos := p.pos
	var lines [][]byte
	endPos := p.pos

	firstLine, _ := p.peekLine()
	closers := htmlBlockClosers(firstLine)

	for !p.eof() {
		line, _ := p.peekLine()

		// A block that has closers of its own (CommonMark types 1 to 5) holds
		// blank lines as content and runs to its closer, or to end of file.
		if closers == nil && isBlankLine(line) {
			break
		}

		lines = append(lines, line)
		p.advancePastLine()
		endPos = p.pos

		if closesHTMLBlock(line, closers) {
			break
		}
	}

	content := bytes.Join(lines, []byte{'\n'})

	return &HTMLBlock{
		Start:   startPos,
		End:     endPos,
		Content: content,
	}
}

// htmlRawTextClosers ends a CommonMark HTML block of type 1, whichever of the
// four tags opened it: the spec's end condition names all of them.
var htmlRawTextClosers = [][]byte{
	[]byte("</script>"), []byte("</pre>"), []byte("</style>"), []byte("</textarea>"),
}

// htmlBlockClosers returns the strings that end the HTML block line opens, or nil
// when it opens a type 6 or 7 block, which ends at a blank line instead.
func htmlBlockClosers(line []byte) [][]byte {
	i := skipOptionalIndent(line)

	if i >= len(line) || line[i] != '<' {
		return nil
	}

	rest := line[i+1:]

	switch {
	case bytes.HasPrefix(rest, []byte("!--")):
		return [][]byte{[]byte("-->")}
	case bytes.HasPrefix(rest, []byte("![CDATA[")):
		return [][]byte{[]byte("]]>")}
	case len(rest) > 0 && rest[0] == '?':
		return [][]byte{[]byte("?>")}
	case len(rest) > 1 && rest[0] == '!' && isASCIILetter(rest[1]):
		return [][]byte{[]byte(">")}
	}

	if ok, after := matchesTagName(rest, htmlRawTextTags); ok && tagNameEnds(after, false) {
		return htmlRawTextClosers
	}

	return nil
}

func closesHTMLBlock(line []byte, closers [][]byte) bool {
	for _, closer := range closers {
		if containsFold(line, closer) {
			return true
		}
	}

	return false
}

// containsFold reports whether s contains needle, matching ASCII case blind. The
// type 1 closers carry letters, and `</PRE>` closes a block just as `</pre>` does.
func containsFold(s, needle []byte) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if bytes.EqualFold(s[i:i+len(needle)], needle) {
			return true
		}
	}

	return false
}

func (p *parser) parseBlockquote() Node {
	startPos := p.pos
	var contentLines [][]byte
	endPos := p.pos
	prevHadContent := false
	lazyOpen := false

	for !p.eof() {
		line, _ := p.peekLine()

		if isBlockquote(line) {
			i := skipOptionalIndent(line)

			i++

			if i < len(line) && line[i] == ' ' {
				i++
			}

			stripped := expandStraddlingTab(line[i:], i)
			contentLines = append(contentLines, stripped)
			p.advancePastLine()
			endPos = p.pos
			prevHadContent = !isBlankLine(stripped)
			lazyOpen = lazyOpen && keepsParagraphOpen(stripped)

			continue
		}

		// Lazy continuation: a non-> line folds into the open paragraph when it's
		// non-blank and doesn't start a new block.
		if !prevHadContent || isBlankLine(line) {
			break
		}

		if interruptsParagraphLazily(line) {
			break
		}

		// Laziness only extends a paragraph: with a fence open inside the
		// quote, the next unmarked line is not part of the blockquote
		// (CommonMark).
		if !lazyOpen {
			body := bytes.Join(contentLines, []byte{'\n'})

			if !p.paragraphOpenAtEnd(append(body, '\n')) {
				break
			}
		}

		lazyOpen = true
		contentLines = append(contentLines, escapeLazyBlockStart(line))
		p.advancePastLine()
		endPos = p.pos
	}

	content := bytes.Join(contentLines, []byte{'\n'})

	quote := &Blockquote{Start: startPos, End: endPos, Content: content}
	if !p.probe {
		quote.Body = p.parseBody(content)
	}

	return quote
}

func (p *parser) parseList(firstMarker []byte, ordered bool, startNum, firstContentColumn int) Node {
	startPos := p.pos
	var items []*ListItem
	familyMarker := firstMarker[len(firstMarker)-1]
	loose := false

	firstItem, firstItemLoose := p.parseListItem(firstMarker, firstContentColumn)
	items = append(items, firstItem)

	if firstItemLoose {
		loose = true
	}

	endPos := p.pos

	for !p.eof() {
		sawBlank := false

		for !p.eof() {
			line, _ := p.peekLine()
			if !isBlankLine(line) {
				break
			}

			sawBlank = true
			p.advancePastLine()
		}

		if p.eof() {
			break
		}

		line, _ := p.peekLine()

		// A line that reads as both is a thematic break: CommonMark gives the break
		// precedence, so `* ** ` ends the list rather than adding an item to it.
		if isThematicBreak(line) {
			break
		}

		_, marker, itemOrdered, _, contentColumn, ok := listItemStart(line)

		if !ok {
			break
		}

		if itemOrdered != ordered {
			break
		}

		if marker[len(marker)-1] != familyMarker {
			break
		}

		if sawBlank {
			loose = true
		}

		item, itemLoose := p.parseListItem(marker, contentColumn)
		items = append(items, item)

		if itemLoose {
			loose = true
		}

		endPos = p.pos
	}

	return &List{
		Start:       startPos,
		End:         endPos,
		Ordered:     ordered,
		Marker:      familyMarker,
		StartNumber: startNum,
		Items:       items,
		Loose:       loose,
	}
}

// skipOptionalIndent returns the index after CommonMark's up-to-three-space block
// indent; a fourth space starts an indented code block instead.
func skipOptionalIndent(line []byte) int {
	i := 0

	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}

	return i
}

func skipSpaceTab(line []byte, i int) int {
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}

	return i
}

// expandStraddlingTab rewrites a leading tab that begins at column col into the
// spaces it stands for, so the slice can be measured on its own grid.
//
// CommonMark expands a tab to the next 4-column stop from where it sits, and
// stripping a block marker moves that grid: after `> ` a tab covers two columns,
// not four, so `> <TAB>quoted` is a paragraph rather than the indented code block a
// fresh measurement claimed. Only the one tab at the cut is expanded; everything
// after it keeps its own bytes, which is what an inner code fence needs.
func expandStraddlingTab(rest []byte, col int) []byte {
	if len(rest) == 0 || rest[0] != '\t' || col%4 == 0 {
		return rest
	}

	width := 4 - col%4
	out := make([]byte, 0, len(rest)+width)

	for range width {
		out = append(out, ' ')
	}

	return append(out, rest[1:]...)
}

// leadingIndentColumns counts the columns of leading whitespace, expanding a tab
// to the next 4-column tab stop (CommonMark). Measuring list-item continuation
// indent in columns, not bytes, keeps a tab-indented line claimed the same as the
// equivalent spaces, so the formatter's space output round-trips with tab input.
func leadingIndentColumns(line []byte) int {
	col := 0

	for i := range line {
		switch line[i] {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
		default:
			return col
		}
	}

	return col
}

// openFenceTracker strips a list item's content column from its lines, following
// the fenced blocks inside so each line is stripped the way its own role needs:
// see stripIndentColumnsAligned for why the two cannot be satisfied at once. Only
// the item's collection loop sees the lines in order, so the state lives with it.
type openFenceTracker struct {
	indent int
	// nested is how far in the fence sits when it opened behind the markers of
	// items starting on the same line (`* ~~~`).
	nested int
	char   byte
	length int
	inside bool
}

func (t *openFenceTracker) strip(line []byte) []byte {
	if t.inside {
		stripped := stripIndentColumns(line, t.indent)
		blank := len(bytes.Trim(stripped, " \t")) == 0

		switch {
		case !blank && leadingIndentColumns(stripped) < t.nested:
			// Left of the nested item's column the item has ended, and its fence.
			t.inside = false
		case isClosingFence(stripIndentColumns(stripped, t.nested), t.char, t.length):
			t.inside = false

			// The closer is structure, so it is the aligned form that decides where
			// the block ends.
			return stripIndentColumnsAligned(line, t.indent)
		default:
			return stripped
		}
	}

	aligned := stripIndentColumnsAligned(line, t.indent)
	t.observe(aligned)

	return aligned
}

// observe records a fence an already-stripped line opens. An item's first line
// carries no indent to strip, but a fence it opens still governs the lines below,
// and so does one behind a nested item's marker: that item strips its own column
// later, and the bytes inside its fence must reach it intact.
func (t *openFenceTracker) observe(line []byte) {
	nested := 0

	for {
		if char, length, _, ok := openFence(line); ok {
			t.char, t.length, t.nested, t.inside = char, length, nested, true

			return
		}

		indent, marker, _, _, contentColumn, ok := listItemStart(line)
		if !ok {
			return
		}

		nested += contentColumn
		line = listFirstLineContent(line, indent+len(marker), contentColumn)
	}
}

// stripIndentColumnsAligned drops the first n columns of indentation and rewrites
// what is left of it as spaces, so the content keeps the column it occupied in the
// document.
//
// A tab measures to the next 4-column stop from wherever it starts, so
// re-slicing a line whose indentation holds tabs moves the ones that survive
// onto a new grid: removing three columns from `\t\t\t0` would move the `0`
// from column 9 to column 8, and the item body would re-parse as a code block.
//
// Keeping both the tab bytes and the column is arithmetically impossible
// unless n is a multiple of the tab stop, so the caller has to choose per
// line: columns decide block structure, and inside a fenced block the bytes
// are the value. openFenceTracker makes that choice for list items.
func stripIndentColumnsAligned(line []byte, n int) []byte {
	width := leadingIndentColumns(line)
	rest := len(line) - len(bytes.TrimLeft(line, " \t"))

	if width <= n {
		return line[rest:]
	}

	return append(bytes.Repeat([]byte{' '}, width-n), line[rest:]...)
}

// stripIndentColumns drops the first n columns of leading whitespace from line. A
// tab straddling the cut expands so its columns past n survive as spaces.
func stripIndentColumns(line []byte, n int) []byte {
	col := 0

	for i := range line {
		switch line[i] {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
		default:
			return line[i:]
		}

		if col >= n {
			pad := col - n

			return append(bytes.Repeat([]byte{' '}, pad), line[i+1:]...)
		}
	}

	return nil
}

// paragraphOpenAtEnd reports whether a paragraph is the block still open at the
// end of content, which is the state CommonMark laziness requires. It asks the
// parser instead of tracking the answer alongside it: a body can end in a fence,
// a heading or a nested container, and laziness reaches a paragraph open inside
// one of those. content must end with a newline, so that a trailing blank line
// stays distinguishable from the last line's own terminator.
//
// The answer is memoized: a probe parses nested containers whose own lazy
// lines probe again, which without it is exponential in the line count.
func (p *parser) paragraphOpenAtEnd(content []byte) bool {
	key := [2]uint64{maphash.Bytes(lazySeeds[0], content), maphash.Bytes(lazySeeds[1], content)}
	if open, ok := p.lazy[key]; ok {
		return open
	}

	probe := p.child(content, true)
	open := probe.lastParagraphOpen(probe.parseFile(false), len(content))
	p.lazy[key] = open

	return open
}

func (p *parser) lastParagraphOpen(blocks []Node, end int) bool {
	if len(blocks) == 0 {
		return false
	}

	last := blocks[len(blocks)-1]

	// A block that stops short of the end was closed by what follows it: a
	// blockquote emptied by `>` on its own line ends there, whatever its content.
	if last.Span().End.Offset < end {
		return false
	}

	switch last := last.(type) {
	case *Paragraph:
		return true
	case *LinkRefDef:
		// CommonMark lifts a definition off the front of a paragraph, so a
		// paragraph is still open under one and the next line joins it lazily.
		return true
	case *Blockquote:
		return p.paragraphOpenAtEnd(append(last.Content, '\n'))
	case *List:
		if len(last.Items) == 0 {
			return false
		}

		tail := last.Items[len(last.Items)-1]

		return p.paragraphOpenAtEnd(append(tail.Content, '\n'))
	}

	return false
}

func (p *parser) parseListItem(marker []byte, contentColumn int) (*ListItem, bool) {
	startPos := p.pos
	firstLine, _ := p.peekLine()

	markerEnd := skipOptionalIndent(firstLine) + len(marker)
	contentLines := [][]byte{listFirstLineContent(firstLine, markerEnd, contentColumn)}

	p.advancePastLine()
	endPos := p.pos

	contIndent := contentColumn
	hadInternalBlank := false
	startedBlank := isBlankLine(contentLines[0])
	// A lazily absorbed line is paragraph text by construction, so the
	// paragraph it joined stays open and the next lazy line needs no query.
	lazyOpen := false
	fence := openFenceTracker{indent: contIndent}
	fence.observe(contentLines[0])

	for !p.eof() {
		line, _ := p.peekLine()

		if isBlankLine(line) {
			// An item that begins with a blank line holds at most that one blank
			// (CommonMark 5.2), so a second one closes it and what follows is
			// outside the item however deeply it is indented.
			if startedBlank && len(contentLines) == 1 {
				break
			}

			savedPos := p.pos

			for !p.eof() {
				p.advancePastLine()

				peekAhead, _ := p.peekLine()

				if isBlankLine(peekAhead) {
					continue
				}

				break
			}

			if p.eof() {
				p.pos = savedPos

				break
			}

			peekAhead, _ := p.peekLine()

			if leadingIndentColumns(peekAhead) < contIndent {
				p.pos = savedPos

				break
			}

			hadInternalBlank = true
			contentLines = append(contentLines, nil)
			endPos = p.pos

			lazyOpen = false

			continue
		}

		if nestedIndent, _, _, _, _, ok := listItemStart(line); ok && nestedIndent < contIndent {
			break
		}

		if leadingIndentColumns(line) < contIndent {
			// Lazy continuation (CommonMark): a line indented too little for the
			// item still belongs to it when it is paragraph continuation text.
			if interruptsParagraphLazily(line) {
				break
			}

			if !lazyOpen {
				// The whole body, not the lines since the last blank: a slice of it
				// starts a fresh grid, where the columns the nested markers above it
				// consume look like indented code instead. A blank line closes the
				// paragraph either way, so the answer only changes where the nesting
				// was what made it a paragraph.
				body := bytes.Join(contentLines, []byte{'\n'})

				if !p.paragraphOpenAtEnd(append(body, '\n')) {
					break
				}
			}

			lazyOpen = true
			absorbed := escapeLazyBlockStart(fence.strip(line))
			contentLines = append(contentLines, absorbed)
			p.advancePastLine()
			endPos = p.pos

			continue
		}

		stripped := fence.strip(line)
		lazyOpen = lazyOpen && keepsParagraphOpen(stripped)
		contentLines = append(contentLines, stripped)
		p.advancePastLine()
		endPos = p.pos
	}

	content := bytes.Join(contentLines, []byte{'\n'})
	item := &ListItem{Start: startPos, End: endPos, MarkerBytes: marker, Content: content}
	if !p.probe {
		item.Body = p.parseBody(content)
		item.Loose = hadInternalBlank && blankSeparatesBlocks(item.Body)
	}

	return item, item.Loose
}

// blankSeparatesBlocks reports whether body holds a blank line directly
// between two of its own blocks, which is what makes a list item loose
// (CommonMark). A blank line that belongs to a child, one between the items of a
// nested list or one held inside a fence, leaves the item tight. Consecutive
// blocks are adjacent, so a gap between two spans is exactly a blank run.
func blankSeparatesBlocks(body *File) bool {
	for i := 1; i < len(body.Blocks); i++ {
		if body.Blocks[i].Span().Start.Offset > body.Blocks[i-1].Span().End.Offset {
			return true
		}
	}

	return false
}

func (p *parser) parseParagraph() Node {
	startPos := p.pos
	var contentLines [][]byte
	endPos := p.pos

	// Where the most recent content line began, and where the paragraph ended
	// before it: a table interrupting the paragraph takes that line as its header,
	// so the paragraph has to end one line earlier.
	lastLineStart := p.pos
	endBeforeLast := p.pos

	firstLine, _ := p.peekLine()

	if label, dest, title, ok := tryLinkRefDef(firstLine); ok {
		p.advancePastLine()

		return &LinkRefDef{
			Start: startPos,
			End:   p.pos,
			Label: label,
			Dest:  dest,
			Title: title,
		}
	}

	for !p.eof() {
		line, _ := p.peekLine()

		if isBlankLine(line) {
			break
		}

		if len(contentLines) > 0 {
			// Setext must come before the thematic-break and list checks: ---
			// matches all three but promotes to a heading when a paragraph is open.
			if level := setextLevel(line); level > 0 {
				underlineStart := p.pos
				p.advancePastLine()
				underlineEnd := p.pos
				content := bytes.Join(contentLines, []byte{'\n'})

				return &SetextHeading{
					Start:         startPos,
					End:           p.pos,
					Level:         level,
					Content:       content,
					UnderlineSpan: Span{Start: underlineStart, End: underlineEnd},
				}
			}

			if interruptsParagraphNotList(line) {
				break
			}

			// A list item interrupts a paragraph only with non-whitespace content
			// after the marker (a trailing space is not content), and an ordered
			// item only when its number is written as exactly `1` (CommonMark); else
			// `*  `/`0) x`/`2. x`/`01) x` would flip to a list on the next pass.
			if indent, marker, ordered, _, _, ok := listItemStart(line); ok &&
				!isBlankLine(line[indent+len(marker):]) &&
				(!ordered || orderedMarkerStartsAtOne(marker)) {
				break
			}

			// GFM table promotion: the paragraph's last line, holding a
			// pipe and followed by a separator whose cell count matches it,
			// is the header row, and a table may interrupt a paragraph. It
			// comes after the checks above because a line they claim is that
			// construct, not a separator: ` - | - ` is a bullet holding
			// `| -`, and GFM reads it that way too.
			last := contentLines[len(contentLines)-1]
			if aligns, headerCells, ok := tableRowsPromote(last, line); ok {
				if len(contentLines) == 1 {
					return p.parseTable(startPos, headerCells, aligns)
				}

				// Hand the header line back so the table is parsed on its own.
				p.pos = lastLineStart

				return &Paragraph{
					Start:   startPos,
					End:     endBeforeLast,
					Content: bytes.Join(contentLines[:len(contentLines)-1], []byte{'\n'}),
				}
			}
		}

		lastLineStart, endBeforeLast = p.pos, endPos
		contentLines = append(contentLines, line)
		p.advancePastLine()
		endPos = p.pos
	}

	content := bytes.Join(contentLines, []byte{'\n'})

	return &Paragraph{
		Start:   startPos,
		End:     endPos,
		Content: content,
	}
}

func (p *parser) parseTable(startPos Position, headerCells [][]byte, aligns []TableAlign) Node {
	header := make([]TableCell, len(headerCells))

	for i, raw := range headerCells {
		header[i] = TableCell{Content: raw}
	}

	p.advancePastLine()
	endPos := p.pos

	var rows [][]TableCell

	for !p.eof() {
		line, _ := p.peekLine()

		if isBlankLine(line) {
			break
		}

		if _, ok := classifyATXHeading(line); ok {
			break
		}

		if isThematicBreak(line) {
			break
		}

		if _, _, _, ok := openFence(line); ok {
			break
		}

		if isBlockquote(line) {
			break
		}

		if _, _, _, _, _, ok := listItemStart(line); ok {
			break
		}

		// No pipe check: GFM reads a bare line as one cell and pads the rest, so a
		// row without one is still a row. Four columns of indent do end the table.
		if isIndentedCode(line) {
			break
		}

		cells := splitTableRow(line)
		row := make([]TableCell, len(header))

		for i := range header {
			if i < len(cells) {
				row[i] = TableCell{Content: cells[i]}
			}
		}

		rows = append(rows, row)
		p.advancePastLine()
		endPos = p.pos
	}

	return &Table{
		Start:  startPos,
		End:    endPos,
		Aligns: aligns,
		Header: header,
		Rows:   rows,
	}
}

func splitTableRow(line []byte) [][]byte {
	i := skipOptionalIndent(line)

	end := len(line)

	for end > i && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}

	startedWithPipe := i < end && line[i] == '|'
	endedWithPipe := false

	if end > i && line[end-1] == '|' {
		bs := 0

		for k := end - 2; k >= i && line[k] == '\\'; k-- {
			bs++
		}

		endedWithPipe = bs%2 == 0
	}

	if startedWithPipe {
		i++
	}

	if endedWithPipe {
		end--
	}

	var cells [][]byte
	var cur []byte

	for i < end {
		c := line[i]

		if c == '\\' && i+1 < end && line[i+1] == '|' {
			cur = append(cur, '|')
			i += 2

			continue
		}

		if c == '|' {
			cells = append(cells, trimTableCell(cur))
			cur = nil
			i++

			continue
		}

		cur = append(cur, c)
		i++
	}

	cells = append(cells, trimTableCell(cur))

	return cells
}

// trimTableCell trims a cell's padding, which GFM writes with spaces and tabs.
// Not unicode.IsSpace, which counts 0x0B and 0x0C: trimming those would make
// `|-|\v-` a delimiter row that GFM leaves as paragraph text. In a content cell
// they are text and stay put for the same reason.
func trimTableCell(cell []byte) []byte {
	return bytes.Trim(cell, " \t")
}

// tableRowsPromote is asked by the parser before promoting a table and by the
// formatter before escaping a delimiter row. The two must not drift: escaping a
// line the parser would not promote writes a visible backslash into the text,
// and leaving one it would promote turns a paragraph into a table next pass.
func tableRowsPromote(header, delimiter []byte) ([]TableAlign, [][]byte, bool) {
	if bytes.IndexByte(header, '|') < 0 {
		return nil, nil, false
	}

	aligns, ok := tableSeparatorAlignments(delimiter)
	if !ok {
		return nil, nil, false
	}

	cells := splitTableRow(header)

	if len(aligns) != len(cells) {
		return nil, nil, false
	}

	return aligns, cells, true
}

// TablePromotes reports whether delimiter, written directly under header, forms
// a GFM table.
func TablePromotes(header, delimiter []byte) bool {
	_, _, ok := tableRowsPromote(header, delimiter)

	return ok
}

// A delimiter row is a row, so it carries a pipe of its own: GFM reads `-:` under
// `|0` as paragraph text, not a one-column table.
func tableSeparatorAlignments(line []byte) ([]TableAlign, bool) {
	if bytes.IndexByte(line, '|') < 0 {
		return nil, false
	}

	cells := splitTableRow(line)

	aligns := make([]TableAlign, len(cells))

	for i, cell := range cells {
		if len(cell) == 0 {
			return nil, false
		}

		left := cell[0] == ':'
		right := cell[len(cell)-1] == ':'

		from := 0

		if left {
			from = 1
		}

		to := len(cell)

		if right {
			to--
		}

		if from >= to {
			return nil, false
		}

		for k := from; k < to; k++ {
			if cell[k] != '-' {
				return nil, false
			}
		}

		switch {
		case left && right:
			aligns[i] = TableAlignCenter
		case left:
			aligns[i] = TableAlignLeft
		case right:
			aligns[i] = TableAlignRight
		default:
			aligns[i] = TableAlignNone
		}
	}

	return aligns, true
}

func isBlankLine(line []byte) bool {
	for _, c := range line {
		if c != ' ' && c != '\t' {
			return false
		}
	}

	return true
}

func classifyATXHeading(line []byte) (int, bool) {
	i := skipOptionalIndent(line)

	count := 0

	for i < len(line) && line[i] == '#' {
		count++
		i++
	}

	if count < 1 || count > 6 {
		return 0, false
	}

	if i < len(line) && line[i] != ' ' && line[i] != '\t' {
		return 0, false
	}

	return count, true
}

func isThematicBreak(line []byte) bool {
	i := skipOptionalIndent(line)

	if i >= len(line) {
		return false
	}

	char := line[i]

	if char != '-' && char != '_' && char != '*' {
		return false
	}

	count := 0

	for ; i < len(line); i++ {
		c := line[i]

		if c == char {
			count++
		} else if c != ' ' && c != '\t' {
			return false
		}
	}

	return count >= 3
}

func setextLevel(line []byte) int {
	i := skipOptionalIndent(line)

	if i >= len(line) {
		return 0
	}

	char := line[i]

	if char != '=' && char != '-' {
		return 0
	}

	start := i

	for i < len(line) && line[i] == char {
		i++
	}

	if i == start {
		return 0
	}

	i = skipSpaceTab(line, i)

	if i != len(line) {
		return 0
	}

	if char == '=' {
		return 1
	}

	return 2
}

func openFence(line []byte) (byte, int, []byte, bool) {
	indent := skipOptionalIndent(line)

	if indent >= len(line) {
		return 0, 0, nil, false
	}

	char := line[indent]

	if char != '`' && char != '~' {
		return 0, 0, nil, false
	}

	length := 0

	for indent+length < len(line) && line[indent+length] == char {
		length++
	}

	if length < 3 {
		return 0, 0, nil, false
	}

	rest := line[indent+length:]
	info := bytes.TrimSpace(rest)

	if char == '`' && bytes.IndexByte(info, '`') >= 0 {
		return 0, 0, nil, false
	}

	return char, length, info, true
}

// OpensFence reports whether line opens a fenced code block. A line opening
// with a code span (```x```) does not: a backtick fence's info has no backtick.
func OpensFence(line []byte) bool {
	_, _, _, ok := openFence(line)

	return ok
}

func isClosingFence(line []byte, fenceChar byte, minLen int) bool {
	i := skipOptionalIndent(line)

	count := 0

	for i < len(line) && line[i] == fenceChar {
		count++
		i++
	}

	if count < minLen {
		return false
	}

	for ; i < len(line); i++ {
		if line[i] != ' ' && line[i] != '\t' {
			return false
		}
	}

	return true
}

// isIndentedCode reports an indented code block: four or more leading columns,
// counting a tab as a 4-column stop so `space+tab` matches four spaces.
func isIndentedCode(line []byte) bool {
	return leadingIndentColumns(line) >= 4
}

func isHTMLBlockStart(line []byte) bool {
	i := skipOptionalIndent(line)

	if i >= len(line) || line[i] != '<' {
		return false
	}

	i++

	if i >= len(line) {
		return false
	}

	c := line[i]

	return c == '!' || c == '?' || c == '/' || isASCIILetter(c)
}

func isBlockquote(line []byte) bool {
	i := skipOptionalIndent(line)

	return i < len(line) && line[i] == '>'
}

// htmlBlockTags is CommonMark's HTML block type 6 tag list.
var htmlBlockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "base": true,
	"basefont": true, "blockquote": true, "body": true, "caption": true,
	"center": true, "col": true, "colgroup": true, "dd": true,
	"details": true, "dialog": true, "dir": true, "div": true,
	"dl": true, "dt": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "frame": true,
	"frameset": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "head": true,
	"header": true, "hr": true, "html": true, "iframe": true,
	"legend": true, "li": true, "link": true, "main": true,
	"menu": true, "menuitem": true, "nav": true, "noframes": true,
	"ol": true, "optgroup": true, "option": true, "p": true,
	"param": true, "search": true, "section": true, "summary": true,
	"table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "title": true, "tr": true,
	"track": true, "ul": true,
}

// htmlRawTextTags is CommonMark's HTML block type 1 tag list.
var htmlRawTextTags = map[string]bool{
	"pre": true, "script": true, "style": true, "textarea": true,
}

// longestHTMLBlockTag bounds the name buffer in matchesTagName; anything longer
// is in neither table.
const longestHTMLBlockTag = len("figcaption")

// StartsHTMLBlockInParagraph reports whether line, emitted inside a paragraph,
// would open an HTML block and so stop being part of it. Exported for the
// emitter, which has to keep such a line indented rather than flush: raw HTML on
// a paragraph's continuation line is inline content, and at column 0 the same
// bytes are a block. The rule stays here so the tag tables have one copy.
func StartsHTMLBlockInParagraph(line []byte) bool {
	return htmlBlockInterruptsParagraph(line)
}

// htmlBlockInterruptsParagraph reports whether line opens one of the HTML blocks
// that may interrupt a paragraph, CommonMark types 1 to 6. Type 7, any other
// complete tag alone on its line, may not, which is why isHTMLBlockStart cannot
// answer this: it admits type 7 because parseBlock needs it to.
func htmlBlockInterruptsParagraph(line []byte) bool {
	i := skipOptionalIndent(line)

	if i >= len(line) || line[i] != '<' {
		return false
	}

	rest := line[i+1:]

	if len(rest) == 0 {
		return false
	}

	switch {
	case rest[0] == '?':
		return true
	case bytes.HasPrefix(rest, []byte("!--")), bytes.HasPrefix(rest, []byte("![CDATA[")):
		return true
	case rest[0] == '!':
		return len(rest) > 1 && isASCIILetter(rest[1])
	}

	if ok, after := matchesTagName(rest, htmlRawTextTags); ok {
		return tagNameEnds(after, false)
	}

	rest = bytes.TrimPrefix(rest, []byte("/"))
	ok, after := matchesTagName(rest, htmlBlockTags)

	return ok && tagNameEnds(after, true)
}

// matchesTagName reports whether the ASCII tag name at the start of s is in set,
// and returns what follows it. The name is lowercased into a stack buffer, so
// matching a mixed-case tag allocates nothing.
func matchesTagName(s []byte, set map[string]bool) (bool, []byte) {
	n := 0

	for n < len(s) && (isASCIILetter(s[n]) || (s[n] >= '0' && s[n] <= '9')) {
		n++
	}

	if n == 0 || n > longestHTMLBlockTag {
		return false, nil
	}

	var lower [longestHTMLBlockTag]byte

	for k, c := range s[:n] {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}

		lower[k] = c
	}

	return set[string(lower[:n])], s[n:]
}

func tagNameEnds(after []byte, selfClosing bool) bool {
	if len(after) == 0 {
		return true
	}

	switch after[0] {
	case ' ', '\t', '>':
		return true
	case '/':
		return selfClosing && len(after) > 1 && after[1] == '>'
	}

	return false
}

// escapeLazyBlockStart prefixes a lazily absorbed line with a backslash when it
// would start a block in its new home that CommonMark never lets a lazy line
// start: a setext underline, or a table delimiter row. Laziness is the proof that
// the source meant paragraph text, but the line now sits at its container's own
// column, directly under the line it continues, where it reads as that marker.
//
// A delimiter row is judged on its shape alone, not on whether this parser would
// promote the pair: once the line is no longer lazy it is exposed to whatever
// promotion rule the renderer applies, and implementations differ on which header
// a one-column row may carry. A row that is already paragraph text at its own
// level is escaped far more narrowly, by the reading this parser itself gives it
// (see tableRowsPromote).
//
// The backslash goes on the marker rather than at the start of the line: a marker
// may carry up to three columns of indent, and a backslash before a space is not
// an escape in CommonMark, so it would render as literal text.
func escapeLazyBlockStart(line []byte) []byte {
	if setextLevel(line) == 0 && !delimiterRowShaped(line) {
		return line
	}

	indent := len(line) - len(bytes.TrimLeft(line, " \t"))
	escaped := make([]byte, 0, len(line)+1)
	escaped = append(escaped, line[:indent]...)
	escaped = append(escaped, '\\')

	return append(escaped, line[indent:]...)
}

// delimiterRowShaped reports whether line is made only of the characters a table
// delimiter row is made of, and holds at least one dash and one pipe. A bare dash
// run is a thematic break or a setext underline, which are judged on their own.
func delimiterRowShaped(line []byte) bool {
	hasPipe := false
	hasDash := false

	for _, c := range line {
		switch c {
		case '|':
			hasPipe = true
		case '-':
			hasDash = true
		case ':', ' ', '\t':
		default:
			return false
		}
	}

	return hasPipe && hasDash
}

// keepsParagraphOpen reports whether appending line to a body that ends in an
// open paragraph leaves it open. It only has to be sufficient: answering false
// where the paragraph does survive costs parseListItem one re-parse, never a
// wrong reading.
func keepsParagraphOpen(line []byte) bool {
	if isBlankLine(line) || interruptsParagraphLazily(line) {
		return false
	}

	if setextLevel(line) > 0 {
		return false
	}

	return bytes.IndexByte(line, '|') < 0
}

// interruptsParagraphNotList reports whether line starts a block that may
// interrupt a paragraph, leaving out the list marker: parseParagraph accepts
// one only with content and, when ordered, only at 1, while a marker outside
// a container closes it whatever its number. Everything else is common to
// both.
func interruptsParagraphNotList(line []byte) bool {
	if _, ok := classifyATXHeading(line); ok {
		return true
	}

	if isThematicBreak(line) {
		return true
	}

	if _, _, _, ok := openFence(line); ok {
		return true
	}

	if isBlockquote(line) {
		return true
	}

	return htmlBlockInterruptsParagraph(line)
}

// interruptsParagraphLazily reports whether line starts a new block instead of
// continuing an open paragraph as a lazy continuation line.
func interruptsParagraphLazily(line []byte) bool {
	if interruptsParagraphNotList(line) {
		return true
	}

	_, _, _, _, _, ok := listItemStart(line)

	return ok
}

// orderedMarkerStartsAtOne reports whether a marker's number is written as the
// single digit `1`. Only that may interrupt a paragraph, so `01.` may not.
func orderedMarkerStartsAtOne(marker []byte) bool {
	if len(marker) < 2 {
		return false
	}

	return bytes.Equal(marker[:len(marker)-1], []byte("1"))
}

// listItemStart returns, for a line that starts a list item: the marker indent,
// the raw marker bytes (-, 1., …), whether it's ordered, the start number, the
// content column, and ok.
func listItemStart(line []byte) (int, []byte, bool, int, int, bool) {
	indent := skipOptionalIndent(line)

	if indent >= len(line) {
		return 0, nil, false, 0, 0, false
	}

	c := line[indent]

	if c == '-' || c == '+' || c == '*' {
		markerEnd := indent + 1

		if markerEnd < len(line) && line[markerEnd] != ' ' && line[markerEnd] != '\t' {
			return 0, nil, false, 0, 0, false
		}

		contentColumn, ok := listContentColumn(line, markerEnd)
		if !ok {
			return 0, nil, false, 0, 0, false
		}

		return indent, line[indent:markerEnd], false, 0, contentColumn, true
	}

	j := indent

	for j < len(line) && line[j] >= '0' && line[j] <= '9' {
		j++
	}

	if j == indent || j-indent > 9 {
		return 0, nil, false, 0, 0, false
	}

	if j >= len(line) {
		return 0, nil, false, 0, 0, false
	}

	term := line[j]

	if term != '.' && term != ')' {
		return 0, nil, false, 0, 0, false
	}

	markerEnd := j + 1

	if markerEnd < len(line) && line[markerEnd] != ' ' && line[markerEnd] != '\t' {
		return 0, nil, false, 0, 0, false
	}

	num := 0

	for k := indent; k < j; k++ {
		num = num*10 + int(line[k]-'0')
	}

	contentColumn, ok := listContentColumn(line, markerEnd)
	if !ok {
		return 0, nil, false, 0, 0, false
	}

	return indent, line[indent:markerEnd], true, num, contentColumn, true
}

// listContentColumn returns the column where a list item's content begins.
// CommonMark puts it at the marker width plus N for 1 <= N <= 4 columns of
// following whitespace; more than four means the item opens with an indented
// code block, and nothing but whitespace to the end of the line means an empty
// item, and both of those sit at marker width plus 1. The indent before a
// marker is spaces only, so markerEnd is also its column.
func listContentColumn(line []byte, markerEnd int) (int, bool) {
	// A marker at end of line is an empty item; its continuation indents one
	// column past the marker, so a less-indented following line is not swallowed.
	if markerEnd >= len(line) {
		return markerEnd + 1, true
	}

	if line[markerEnd] != ' ' && line[markerEnd] != '\t' {
		return 0, false
	}

	columns, length := markerGap(line, markerEnd)

	// Five or more columns of whitespace after the marker, or nothing
	// but whitespace to the end of the line, pin the content one column
	// past it: CommonMark then reads the rest as an indented code block.
	// The count is in columns because in `*   \t0` a tab carries the
	// run past four.
	if columns > 4 || markerEnd+length >= len(line) {
		return markerEnd + 1, true
	}

	return markerEnd + columns, true
}

// listFirstLineContent returns what follows an item's content column on its
// first line. Whitespace left past that column becomes spaces, because a tab in
// it would measure to a different stop once the body is parsed on its own.
func listFirstLineContent(line []byte, markerEnd, contentColumn int) []byte {
	if markerEnd >= len(line) {
		return nil
	}

	columns, length := markerGap(line, markerEnd)
	text := line[markerEnd+length:]

	if len(text) == 0 {
		return line[markerEnd+1:]
	}

	pad := markerEnd + columns - contentColumn

	if pad == 0 {
		return text
	}

	return append(bytes.Repeat([]byte{' '}, pad), text...)
}

// markerGap returns the width in columns and the length in bytes of the
// whitespace run that follows a marker ending at markerEnd. A tab measures to the
// next 4-column stop from where it sits, so `   \t` is seven columns after a
// marker in column three, not four bytes' worth.
func markerGap(line []byte, markerEnd int) (columns, length int) {
	col := 0

	for i := range markerEnd {
		if line[i] == '\t' {
			col += 4 - col%4

			continue
		}

		col++
	}

	start := col

	for markerEnd+length < len(line) {
		switch line[markerEnd+length] {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
		default:
			return col - start, length
		}

		length++
	}

	return col - start, length
}

// tryLinkRefDef recognizes a single-line [label]: dest ["title"]. Multi-line
// definitions (title on its own line) aren't handled; they fall through to a
// paragraph.
func tryLinkRefDef(line []byte) ([]byte, []byte, []byte, bool) {
	i := skipOptionalIndent(line)

	if i >= len(line) || line[i] != '[' {
		return nil, nil, nil, false
	}

	i++
	labelStart := i

	for i < len(line) && line[i] != ']' {
		if line[i] == '\\' && i+1 < len(line) {
			i += 2

			continue
		}

		// A label holds no unescaped bracket (CommonMark), so this is no definition.
		if line[i] == '[' {
			return nil, nil, nil, false
		}

		i++
	}

	if i >= len(line) {
		return nil, nil, nil, false
	}

	label := line[labelStart:i]

	if len(bytes.TrimSpace(label)) == 0 {
		return nil, nil, nil, false
	}

	i++

	if i >= len(line) || line[i] != ':' {
		return nil, nil, nil, false
	}

	i++

	i = skipSpaceTab(line, i)

	if i >= len(line) {
		return nil, nil, nil, false
	}

	var dest []byte

	if line[i] == '<' {
		i++
		destStart := i

		for i < len(line) && line[i] != '>' {
			if line[i] == '\\' && i+1 < len(line) {
				i += 2

				continue
			}

			i++
		}

		if i >= len(line) {
			return nil, nil, nil, false
		}

		dest = line[destStart:i]
		i++
	} else {
		destStart := i

		for i < len(line) && line[i] != ' ' && line[i] != '\t' {
			i++
		}

		if i == destStart {
			return nil, nil, nil, false
		}

		dest = line[destStart:i]

		if !destParensBalanced(dest) {
			return nil, nil, nil, false
		}
	}

	i = skipSpaceTab(line, i)

	var title []byte

	if i < len(line) {
		var endCh byte

		switch line[i] {
		case '"':
			endCh = '"'
		case '\'':
			endCh = '\''
		case '(':
			endCh = ')'
		default:
			return nil, nil, nil, false
		}

		i++
		titleStart := i

		for i < len(line) && line[i] != endCh {
			if line[i] == '\\' && i+1 < len(line) {
				i += 2

				continue
			}

			i++
		}

		if i >= len(line) {
			return nil, nil, nil, false
		}

		title = line[titleStart:i]
		i++

		i = skipSpaceTab(line, i)
	}

	if i != len(line) {
		return nil, nil, nil, false
	}

	return label, dest, title, true
}

// destParensBalanced checks a bare link destination's parentheses: CommonMark
// allows them escaped or in balanced pairs, and leaves brackets free.
func destParensBalanced(dest []byte) bool {
	depth := 0

	for i := 0; i < len(dest); i++ {
		switch dest[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--

			if depth < 0 {
				return false
			}
		}
	}

	return depth == 0
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
