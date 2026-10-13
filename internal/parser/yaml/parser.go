package yaml

import (
	"bytes"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

type yamlParser struct {
	parserpkg.Cursor

	errors  []Error
	pending []Trivia
	depth   int
}

// maxNestingDepth caps block recursion so a pathological run of indentation
// returns a verbatim bail instead of overflowing the stack.
const maxNestingDepth = 500

func mkspan(start, end Position) Span {
	return Span{Start: start, End: end}
}

func (p *yamlParser) slice(start, end Position) []byte {
	return p.Src[start.Offset:end.Offset]
}

func (p *yamlParser) drainPending() []Trivia {
	t := p.pending
	p.pending = nil

	return t
}

// The progress guard keeps a malformed input that classifies to nothing
// from looping forever.
func (p *yamlParser) parseStream() []*Document {
	var docs []*Document

	for !p.Eof() {
		before := p.Pos.Offset
		docs = append(docs, p.parseDocument())

		if p.Pos.Offset == before {
			p.consumeLine()

			if p.Pos.Offset == before {
				break
			}
		}
	}

	return docs
}

func (p *yamlParser) parseDocument() *Document {
	start := p.Pos
	doc := &Document{}

	for !p.Eof() {
		if p.atBlankLine() || p.atCommentLine() {
			p.collectTrivia()

			continue
		}

		ind := p.measureIndent()

		if p.PeekAt(ind) == '%' {
			directiveStart := p.Pos
			end := p.consumeLineKeepEnd()
			doc.Directives = append(doc.Directives, mkspan(directiveStart, end))

			continue
		}

		if p.isDocStart(ind) {
			p.AdvanceN(ind)
			markerStart := p.Pos
			p.AdvanceN(3)
			doc.StartMarker = mkspan(markerStart, p.Pos)
			p.finishLine()

			break
		}

		break
	}

	doc.Leading = p.drainPending()

	// Blank and comment lines after the marker are trivia. Left in place, a
	// whitespace-only line would read as a bailed body and freeze the file.
	if !p.Eof() {
		p.collectTrivia()
	}

	if !p.Eof() {
		ind := p.measureIndent()

		if !p.isDocStart(ind) && !p.isDocEnd(ind) {
			doc.Body = p.parseDocBody()
		}
	}

	for !p.Eof() {
		if p.atBlankLine() || p.atCommentLine() {
			p.collectTrivia()

			continue
		}

		ind := p.measureIndent()

		if p.isDocEnd(ind) {
			p.AdvanceN(ind)
			markerStart := p.Pos
			p.AdvanceN(3)
			doc.EndMarker = mkspan(markerStart, p.Pos)
			p.finishLine()
		}

		break
	}

	doc.Trailing = p.drainPending()
	doc.Range = mkspan(start, p.Pos)

	return doc
}

func (p *yamlParser) parseDocBody() Node {
	if p.Eof() {
		return nil
	}

	ind := p.measureIndent()

	if p.isSeqDash(ind) {
		return p.parseSequence(ind)
	}

	if p.findSeparatorOnCurrentLine() >= 0 {
		return p.parseMapping(ind, false)
	}

	return p.bailLine(ind)
}

// Whether a block sequence may start at its parent's own column, the one
// YAML exception to "more indented than the parent". It holds under a mapping
// key and not under a sequence dash, where a dash at that column is the next
// sibling item and reading it as a nested sequence adopts every item after an
// empty one.
const (
	sequenceMayShareIndent = true
	sequenceMustIndent     = false
)

func (p *yamlParser) parseBlockValue(parentIndent int, sameIndentSequence bool) Node {
	p.depth++
	defer func() { p.depth-- }()

	p.collectTrivia()

	if p.Eof() {
		return nil
	}

	if p.depth > maxNestingDepth {
		ind := p.measureIndent()

		if ind <= parentIndent {
			return nil
		}

		return p.bailLine(ind)
	}

	ind := p.measureIndent()

	if ind > parentIndent {
		return p.parseNodeAt(ind)
	}

	if sameIndentSequence && ind == parentIndent && p.isSeqDash(ind) {
		return p.parseSequence(parentIndent)
	}

	return nil
}

func (p *yamlParser) parseNodeAt(ind int) Node {
	if p.isSeqDash(ind) {
		return p.parseSequence(ind)
	}

	if p.findSeparatorOnCurrentLine() >= 0 {
		return p.parseMapping(ind, false)
	}

	return p.bailLine(ind)
}

func (p *yamlParser) parseMapping(indent int, inlineFirst bool) *Mapping {
	m := &Mapping{}

	for {
		var leading []Trivia

		if inlineFirst {
			leading = p.drainPending()
			inlineFirst = false
		} else {
			p.collectTrivia()

			if p.Eof() || p.measureIndent() != indent || p.isSeqDash(indent) || p.findSeparatorOnCurrentLine() < 0 {
				break
			}

			leading = p.drainPending()
			p.AdvanceN(indent)
		}

		entry := p.parseMappingEntry(indent)
		entry.Leading = leading
		m.Entries = append(m.Entries, entry)
	}

	if len(m.Entries) > 0 {
		m.Range = mkspan(m.Entries[0].Range.Start, m.Entries[len(m.Entries)-1].Range.End)
	}

	return m
}

func (p *yamlParser) parseMappingEntry(indent int) *MappingEntry {
	start := p.Pos
	pendingBefore := len(p.pending)

	sep := p.findSeparatorOnCurrentLine()
	keyStart := p.Pos

	for p.Pos.Offset < sep {
		p.Advance()
	}

	key := &Scalar{Range: mkspan(keyStart, p.Pos), Raw: p.slice(keyStart, p.Pos)}

	colonStart := p.Pos
	p.Advance()
	colon := mkspan(colonStart, p.Pos)

	value, comment := p.parseValueAfterColon(indent)

	return &MappingEntry{
		Range:   mkspan(start, p.contentEnd(pendingBefore)),
		Key:     key,
		Colon:   colon,
		Value:   value,
		Comment: comment,
	}
}

func (p *yamlParser) parseValueAfterColon(indent int) (Node, *Trivia) {
	p.skipInlineSpace()

	if p.Eof() || p.Peek() == '\n' || p.Peek() == '\r' || p.Peek() == '#' {
		comment := p.finishLine()

		return p.parseBlockValue(indent, sequenceMayShareIndent), comment
	}

	return p.parseInlineValue(indent)
}

func (p *yamlParser) parseSequence(indent int) *Sequence {
	s := &Sequence{}

	for {
		p.collectTrivia()

		if p.Eof() || p.measureIndent() != indent || !p.isSeqDash(indent) {
			break
		}

		leading := p.drainPending()
		p.AdvanceN(indent)

		item := p.parseSequenceItem(indent)
		item.Leading = leading
		s.Items = append(s.Items, item)
	}

	if len(s.Items) > 0 {
		s.Range = mkspan(s.Items[0].Range.Start, s.Items[len(s.Items)-1].Range.End)
	}

	return s
}

// Trivia parked in p.pending belongs to an enclosing scope, so a node's range
// stops before it: a verbatim emit of the node would repeat it every pass.
func (p *yamlParser) contentEnd(pendingBefore int) Position {
	if len(p.pending) > pendingBefore {
		return p.pending[pendingBefore].Range.Start
	}

	return p.Pos
}

func (p *yamlParser) parseSequenceItem(indent int) *SequenceItem {
	start := p.Pos
	pendingBefore := len(p.pending)
	dashStart := p.Pos
	p.Advance()
	dash := mkspan(dashStart, p.Pos)

	save := p.Pos

	p.skipInlineSpace()

	if p.Eof() || p.Peek() == '\n' || p.Peek() == '\r' || p.Peek() == '#' {
		p.Pos = save
		comment := p.finishLine()
		value := p.parseBlockValue(indent, sequenceMustIndent)

		return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value, Comment: comment}
	}

	// A compact `- key: value` continues as a mapping whose indent is the
	// column the value starts at.
	valueColumn := p.lineColumn()

	if p.findSeparatorOnCurrentLine() >= 0 {
		value := p.parseMapping(valueColumn, true)

		return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value}
	}

	// A block scalar is read against its owner's column, here the dash's, as a
	// mapping value's is against its key's.
	if c := p.Peek(); c == '|' || c == '>' {
		value := p.parseBlockScalar(indent)

		return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value}
	}

	value, comment := p.parseInlineValue(valueColumn)

	return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value, Comment: comment}
}

func (p *yamlParser) parseInlineValue(indent int) (Node, *Trivia) {
	switch p.Peek() {
	case '|', '>':
		return p.parseBlockScalar(indent), nil
	case '{', '[':
		node := p.parseFlow()

		return node, p.finishLine()
	case '&', '*', '!':
		return p.parseAnchoredValue(indent), nil
	case '"', '\'':
		scalar := p.parseQuotedScalar(indent)

		return scalar, p.finishLine()
	default:
		scalar := p.parsePlainScalar(indent)

		return scalar, p.finishLine()
	}
}

func (p *yamlParser) parsePlainScalar(indent int) *Scalar {
	start := p.Pos
	contentEnd := p.Pos

	for !p.Eof() {
		c := p.Peek()

		if c == '\n' || c == '\r' {
			break
		}

		if c == '#' && p.Pos.Offset > start.Offset {
			prev := p.Src[p.Pos.Offset-1]

			if prev == ' ' || prev == '\t' {
				break
			}
		}

		p.Advance()

		if c != ' ' && c != '\t' {
			contentEnd = p.Pos
		}
	}

	return &Scalar{Range: mkspan(start, contentEnd), Raw: p.slice(start, contentEnd), Indent: indent}
}

func (p *yamlParser) parseQuotedScalar(indent int) *Scalar {
	start := p.Pos

	var closed bool

	if p.Peek() == '\'' {
		closed = p.skipSingleQuoted()
	} else {
		closed = p.skipDoubleQuoted()
	}

	if !closed {
		p.errors = append(p.errors, Error{Pos: start, Msg: "unterminated quoted scalar"})
	}

	return &Scalar{Range: mkspan(start, p.Pos), Raw: p.slice(start, p.Pos), Indent: indent}
}

func (p *yamlParser) parseBlockScalar(indent int) *BlockScalar {
	start := p.Pos
	end := p.consumeIndentedBlock(indent)

	// `+` keep chomping makes the trailing line breaks part of the value, so
	// they stay in the span, out of reach of the blank-line rules that would
	// collapse or trim them.
	if keepChomped(p.slice(start, end)) {
		end = p.consumeTrailingBlanks(end)
	}

	raw := p.slice(start, end)

	return &BlockScalar{Range: mkspan(start, end), Raw: raw, Indent: indent, ContentIndent: contentIndent(raw)}
}

func contentIndent(span []byte) int {
	header, body, _ := bytes.Cut(span, []byte("\n"))
	if bytes.ContainsAny(header, "123456789") {
		return 0
	}

	column := 0

	for line := range bytes.SplitSeq(body, []byte("\n")) {
		spaces := len(line) - len(bytes.TrimLeft(line, " "))
		rest := bytes.TrimRight(line[spaces:], " \t\r")

		switch {
		case len(rest) == 0:
		case rest[0] == '\t':
			return 0
		case column == 0:
			column = spaces
		}
	}

	return column
}

func keepChomped(span []byte) bool {
	header := span

	if i := bytes.IndexByte(header, '\n'); i >= 0 {
		header = header[:i]
	}

	return bytes.IndexByte(header, '+') >= 0
}

// consumeTrailingBlanks returns the end before the run's last line break,
// since every span here excludes its own final break.
func (p *yamlParser) consumeTrailingBlanks(end Position) Position {
	for !p.Eof() && p.atBlankLine() {
		end = p.consumeLineKeepEnd()
	}

	return end
}

func (p *yamlParser) parseAnchoredValue(indent int) *Verbatim {
	start := p.Pos
	end := p.consumeIndentedBlock(indent)

	return &Verbatim{Range: mkspan(start, end), Raw: p.slice(start, end), Indent: indent}
}

func (p *yamlParser) bailLine(indent int) *Verbatim {
	start := p.Pos
	end := p.consumeIndentedBlock(indent)

	return &Verbatim{Range: mkspan(start, end), Raw: p.slice(start, end), Bail: true, Indent: indent}
}

func (p *yamlParser) parseFlow() Node {
	start := p.Pos
	depth := 0

	for !p.Eof() {
		switch p.Peek() {
		case '\'':
			p.skipSingleQuoted()
		case '"':
			p.skipDoubleQuoted()
		case '{', '[':
			depth++
			p.Advance()
		case '}', ']':
			depth--
			p.Advance()

			if depth <= 0 {
				return &Flow{Range: mkspan(start, p.Pos), Raw: p.slice(start, p.Pos)}
			}
		default:
			p.Advance()
		}
	}

	p.errors = append(p.errors, Error{Pos: start, Msg: "unterminated flow collection"})

	return &Flow{Range: mkspan(start, p.Pos), Raw: p.slice(start, p.Pos)}
}

// Trailing blank lines are left for the caller to collect as trivia, so they
// survive as blank-line layout instead of being absorbed into the verbatim
// span.
func (p *yamlParser) consumeIndentedBlock(indent int) Position {
	contentEnd := p.consumeLineKeepEnd()

	for !p.Eof() {
		if p.atBlankLine() {
			if !p.triviaPrecedesDeeperLine(indent) {
				break
			}

			p.consumeLine()

			continue
		}

		if p.measureIndent() > indent {
			// A trailing more-indented comment (nothing deeper after it) is not
			// part of the block: absorbing it would tie the block extent to the
			// source indent, which shifts under normalization. Interior
			// comments (deeper content follows) are still absorbed.
			if p.atCommentLine() && !p.triviaPrecedesDeeperLine(indent) {
				break
			}

			contentEnd = p.consumeLineKeepEnd()

			continue
		}

		break
	}

	return contentEnd
}

func (p *yamlParser) consumeLineKeepEnd() Position {
	for !p.Eof() && !p.atLineEnd() {
		p.Advance()
	}

	end := p.Pos
	p.consumeNewline()

	return end
}

func (p *yamlParser) triviaPrecedesDeeperLine(indent int) bool {
	saved := p.Pos

	for !p.Eof() {
		if p.atBlankLine() || p.atCommentLine() {
			p.consumeLine()

			continue
		}

		deeper := p.measureIndent() > indent
		p.Pos = saved

		return deeper
	}

	p.Pos = saved

	return false
}

func (p *yamlParser) collectTrivia() {
	for !p.Eof() {
		if p.atBlankLine() {
			start := p.Pos
			p.consumeLine()
			p.pending = append(p.pending, Trivia{Kind: TriviaBlank, Range: mkspan(start, p.Pos), Text: p.slice(start, p.Pos)})

			continue
		}

		if p.atCommentLine() {
			start := p.Pos
			p.consumeLine()
			p.pending = append(p.pending, Trivia{Kind: TriviaComment, Range: mkspan(start, p.Pos), Text: p.slice(start, p.Pos)})

			continue
		}

		break
	}

	p.checkTabIndentation()
}

// checkTabIndentation records the one position YAML forbids a tab: where a
// line's indentation ends and its content begins. Every structural line is
// classified after collectTrivia, so one site covers every dispatch. Block
// scalar bodies and flow continuation lines never pass through here, and a
// tab leading one of those is legal separation, not indentation. A tab-only
// line is exempt, since it indents nothing.
func (p *yamlParser) checkTabIndentation() {
	ind := p.measureIndent()

	if p.PeekAt(ind) != '\t' {
		return
	}

	pos := p.Pos
	pos.Offset += ind
	pos.Column += ind

	// Nested dispatches classify the same line more than once, and Format joins
	// every diagnostic, so without this the one tab is reported repeatedly.
	if n := len(p.errors); n > 0 && p.errors[n-1].Pos.Offset == pos.Offset {
		return
	}

	p.errors = append(p.errors, Error{Pos: pos, Msg: "tab in indentation"})
}

func (p *yamlParser) skipInlineSpace() {
	for !p.Eof() && (p.Peek() == ' ' || p.Peek() == '\t') {
		p.Advance()
	}
}

func (p *yamlParser) finishLine() *Trivia {
	p.skipInlineSpace()

	var comment *Trivia

	if !p.Eof() && p.Peek() == '#' {
		start := p.Pos

		for !p.Eof() && !p.atLineEnd() {
			p.Advance()
		}

		comment = &Trivia{Kind: TriviaComment, Range: mkspan(start, p.Pos), Text: p.slice(start, p.Pos)}
	}

	p.consumeNewline()

	return comment
}

func (p *yamlParser) atLineEnd() bool {
	c := p.Peek()

	return c == '\n' || c == '\r'
}

func (p *yamlParser) consumeNewline() {
	if p.Eof() {
		return
	}

	if p.Peek() == '\r' {
		p.Advance()
	}

	if !p.Eof() && p.Peek() == '\n' {
		p.Advance()
	}
}

func (p *yamlParser) consumeLine() {
	for !p.Eof() && !p.atLineEnd() {
		p.Advance()
	}

	p.consumeNewline()
}

// skipSingleQuoted and skipDoubleQuoted report whether the closing quote was
// found. They leave the error to the caller, because findSeparatorOnCurrentLine
// also skips a quote that sits inside a plain scalar, such as `it's`.
func (p *yamlParser) skipSingleQuoted() bool {
	p.Advance()

	for !p.Eof() {
		if p.Peek() == '\'' {
			if p.PeekAt(1) == '\'' {
				p.AdvanceN(2)

				continue
			}

			p.Advance()

			return true
		}

		p.Advance()
	}

	return false
}

func (p *yamlParser) skipDoubleQuoted() bool {
	p.Advance()

	for !p.Eof() {
		switch p.Peek() {
		case '\\':
			p.AdvanceN(2)
		case '"':
			p.Advance()

			return true
		default:
			p.Advance()
		}
	}

	return false
}

func (p *yamlParser) findSeparatorOnCurrentLine() int {
	saved := p.Pos
	startOffset := p.Pos.Offset
	result := -1

	for !p.Eof() && !p.atLineEnd() {
		c := p.Peek()

		switch {
		case c == '\'':
			p.skipSingleQuoted()

			continue
		case c == '"':
			p.skipDoubleQuoted()

			continue
		case c == '#' && p.Pos.Offset > startOffset:
			prev := p.Src[p.Pos.Offset-1]

			if prev == ' ' || prev == '\t' {
				p.Pos = saved

				return result
			}
		case c == ':':
			next := p.PeekAt(1)

			if next == ' ' || next == '\t' || next == '\n' || next == '\r' || next == 0 {
				result = p.Pos.Offset
				p.Pos = saved

				return result
			}
		}

		p.Advance()
	}

	p.Pos = saved

	return result
}

func (p *yamlParser) measureIndent() int {
	n := 0

	for p.PeekAt(n) == ' ' {
		n++
	}

	return n
}

func (p *yamlParser) atBlankLine() bool {
	n := 0

	for c := p.PeekAt(n); c == ' ' || c == '\t'; c = p.PeekAt(n) {
		n++
	}

	c := p.PeekAt(n)

	return c == '\n' || c == '\r' || p.Pos.Offset+n >= len(p.Src)
}

func (p *yamlParser) atCommentLine() bool {
	return p.PeekAt(p.measureIndent()) == '#'
}

func (p *yamlParser) isSeqDash(ind int) bool {
	if p.PeekAt(ind) != '-' {
		return false
	}

	next := p.PeekAt(ind + 1)

	return next == ' ' || next == '\t' || next == '\n' || next == '\r' || next == 0
}

func (p *yamlParser) isDocStart(ind int) bool {
	return p.isThreeMarker(ind, '-')
}

func (p *yamlParser) isDocEnd(ind int) bool {
	return p.isThreeMarker(ind, '.')
}

// atLineStart reports whether the cursor sits on a line's first byte.
// Pos.Column cannot answer it: the shared Cursor opens a new line on `\n`
// alone, while YAML ends one on a lone `\r` too, so the byte before decides.
func (p *yamlParser) atLineStart() bool {
	if p.Pos.Offset == 0 {
		return true
	}

	prev := p.Src[p.Pos.Offset-1]

	return prev == '\n' || prev == '\r'
}

// lineColumn counts back to the line start because Pos.Column does not reset
// after a lone `\r`, and an indent read from it would be too large.
func (p *yamlParser) lineColumn() int {
	n := 0

	for p.Pos.Offset-n > 0 {
		prev := p.Src[p.Pos.Offset-n-1]
		if prev == '\n' || prev == '\r' {
			break
		}

		n++
	}

	return n
}

func (p *yamlParser) isThreeMarker(ind int, b byte) bool {
	// `---`/`...` are document markers only at the start of a line and at
	// column 0; indented, or trailing a marker that already opened the line,
	// they are plain content that must not be moved to column 0.
	if ind != 0 || !p.atLineStart() {
		return false
	}

	if p.PeekAt(ind) != b || p.PeekAt(ind+1) != b || p.PeekAt(ind+2) != b {
		return false
	}

	tail := p.PeekAt(ind + 3)

	return tail == '\n' || tail == '\r' || tail == ' ' || tail == '\t' || tail == 0
}
