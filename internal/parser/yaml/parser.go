package yaml

import (
	"bytes"
	"fmt"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

type yamlParser struct {
	parserpkg.Cursor

	errors  []Error
	pending []Trivia
	depth   int

	// explicitKey is set once a `? ` key is read: the parser does not model
	// one, so a line it leaves over may belong to it.
	explicitKey bool
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

	// A later document without `---` is one the parser split off where a block
	// ended early, so what is left over after it has no block context to judge.
	splitOff := start.Offset > 0 && doc.StartMarker.End.Offset == doc.StartMarker.Start.Offset

	if doc.Body != nil && !splitOff && doc.EndMarker.End.Offset == doc.EndMarker.Start.Offset {
		p.checkStrandedKey()
	}

	doc.Trailing = p.drainPending()
	doc.Range = mkspan(start, p.Pos)

	return doc
}

// checkStrandedKey reports a key line left over once the body ends, indented
// to no open mapping, like `id: x` under `- type: input`'s dash. A key line is
// never a scalar's text, so this is never valid. Skipped where the parser
// cannot judge: column 0, a list item, a `: ` line, after an explicit `? ` key.
func (p *yamlParser) checkStrandedKey() {
	if p.Eof() {
		return
	}

	ind := p.measureIndent()
	if ind == 0 || p.explicitKey || p.isSeqDash(ind) || p.isDocStart(ind) || p.isDocEnd(ind) {
		return
	}

	sep := p.findSeparatorOnCurrentLine()
	if sep <= p.Pos.Offset+ind {
		return
	}

	saved := p.Pos
	p.AdvanceN(ind)
	p.errors = append(p.errors, Error{Pos: p.Pos, Msg: "key is indented to no open mapping"})
	p.Pos = saved
}

func (p *yamlParser) parseDocBody() Node {
	if p.Eof() {
		return nil
	}

	ind := p.measureIndent()

	if p.isSeqDash(ind) {
		return p.parseSequence(ind, false)
	}

	if p.findSeparatorOnCurrentLine() >= 0 {
		return p.parseMapping(ind, false)
	}

	// A flow collection is a whole document (`{}`, a JSON file) when nothing
	// but a line break follows it.
	if c := p.PeekAt(ind); c == '{' || c == '[' {
		saved := p.Pos
		p.AdvanceN(ind)
		flow := p.parseFlow(0)

		if p.finishLine() == nil {
			return flow
		}

		p.Pos = saved
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

// A scalar on the line below its owner is written back inline, so it is read
// only when nothing sits between the two that would move: a comment on the
// owner's line, or a blank or comment line before the scalar. leafIndent is
// the indent the scalar is re-homed by, as if it had been written inline.
func (p *yamlParser) parseBlockValue(parentIndent int, sameIndentSequence bool, leafIndent int, ownerHasComment bool) Node {
	p.depth++
	defer func() { p.depth-- }()

	pendingBefore := len(p.pending)
	p.collectTrivia()
	leafOK := !ownerHasComment && len(p.pending) == pendingBefore

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
		return p.parseNodeAt(ind, parentIndent, leafIndent, leafOK)
	}

	if sameIndentSequence && ind == parentIndent && p.isSeqDash(ind) {
		return p.parseSequence(parentIndent, false)
	}

	return nil
}

func (p *yamlParser) parseNodeAt(ind, parentIndent, leafIndent int, leafOK bool) Node {
	if p.isSeqDash(ind) {
		return p.parseSequence(ind, false)
	}

	if p.findSeparatorOnCurrentLine() >= 0 {
		return p.parseMapping(ind, false)
	}

	if !leafOK {
		return p.bailLine(ind)
	}

	saved := p.Pos
	p.AdvanceN(ind)

	node, comment := p.parseInlineValue(leafIndent, parentIndent)
	if comment != nil {
		p.Pos = saved

		return p.bailLine(ind)
	}

	return node
}

func (p *yamlParser) parseMapping(indent int, inlineFirst bool) *Mapping {
	m := &Mapping{}

	var seen map[string]bool

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
		seen = p.checkDuplicateKey(m.Entries, entry.Key, seen)
		m.Entries = append(m.Entries, entry)
	}

	if len(m.Entries) > 0 {
		m.Range = mkspan(m.Entries[0].Range.Start, m.Entries[len(m.Entries)-1].Range.End)
	}

	return m
}

// keySetThreshold is the entry count past which a mapping's keys go into a set:
// below it a scan is cheaper than the set's allocations, above it the scan
// turns a 50,000-key translation file quadratic.
const keySetThreshold = 16

// checkDuplicateKey reports key when an earlier entry has it, which every
// loader rejects. seen holds the mapping's keys once past keySetThreshold.
func (p *yamlParser) checkDuplicateKey(earlier []*MappingEntry, key *Scalar, seen map[string]bool) map[string]bool {
	name, quoted, ok := keyText(key.Raw)
	if !ok {
		return seen
	}

	if len(earlier) < keySetThreshold {
		for _, e := range earlier {
			other, otherQuoted, ok := keyText(e.Key.Raw)

			if ok && quoted == otherQuoted && bytes.Equal(name, other) {
				p.reportDuplicateKey(key, name)

				return seen
			}
		}

		return seen
	}

	if seen == nil {
		seen = make(map[string]bool, 2*len(earlier))

		for _, e := range earlier {
			if other, otherQuoted, ok := keyText(e.Key.Raw); ok {
				seen[keySetEntry(other, otherQuoted)] = true
			}
		}
	}

	entry := keySetEntry(name, quoted)

	if seen[entry] {
		p.reportDuplicateKey(key, name)
	}

	seen[entry] = true

	return seen
}

func (p *yamlParser) reportDuplicateKey(key *Scalar, name []byte) {
	p.errors = append(p.errors, Error{Pos: key.Range.Start, Msg: fmt.Sprintf("duplicate key %q", name)})
}

func keySetEntry(name []byte, quoted bool) string {
	if quoted {
		return "'" + string(name)
	}

	return string(name)
}

// keyText returns the text a key compares by. A plain key is only compared
// with plain ones, since `1` and `'1'` are different keys. It reports false for
// a key whose text takes more than removing quotes to read: one with escapes,
// an anchor, alias, tag, flow collection, or the `<<` merge key.
func keyText(raw []byte) (text []byte, quoted, ok bool) {
	raw = bytes.TrimRight(raw, " \t")

	if len(raw) == 0 || bytes.Equal(raw, []byte("<<")) {
		return nil, false, false
	}

	switch raw[0] {
	case '\'':
		if len(raw) < 2 || raw[len(raw)-1] != '\'' || bytes.Contains(raw[1:len(raw)-1], []byte("'")) {
			return nil, false, false
		}

		return raw[1 : len(raw)-1], true, true
	case '"':
		if len(raw) < 2 || raw[len(raw)-1] != '"' || bytes.ContainsAny(raw[1:len(raw)-1], `\"`) {
			return nil, false, false
		}

		return raw[1 : len(raw)-1], true, true
	case '&', '*', '!', '[', '{', '?':
		return nil, false, false
	}

	return raw, false, true
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

	if len(key.Raw) > 1 && key.Raw[0] == '?' && (key.Raw[1] == ' ' || key.Raw[1] == '\t') {
		p.explicitKey = true
	}

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

		return p.parseBlockValue(indent, sequenceMayShareIndent, indent, comment != nil), comment
	}

	return p.parseInlineValue(indent, indent)
}

func (p *yamlParser) parseSequence(indent int, inlineFirst bool) *Sequence {
	s := &Sequence{}

	for {
		var leading []Trivia

		if inlineFirst {
			inlineFirst = false
		} else {
			p.collectTrivia()

			if p.Eof() || p.measureIndent() != indent || !p.isSeqDash(indent) {
				break
			}

			leading = p.drainPending()
			p.AdvanceN(indent)
		}

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
		value := p.parseBlockValue(indent, sequenceMustIndent, indent+len("- "), comment != nil)

		return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value, Comment: comment}
	}

	// A compact `- key: value` continues as a mapping whose indent is the
	// column the value starts at.
	valueColumn := p.lineColumn()

	if p.isSeqDash(0) {
		value := p.parseSequence(valueColumn, true)

		return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value}
	}

	if p.findSeparatorOnCurrentLine() >= 0 {
		value := p.parseMapping(valueColumn, true)

		return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value}
	}

	// A block scalar is read against its owner's column, here the dash's, as a
	// mapping value's is against its key's. An anchor or tag before it does not
	// change its owner.
	if blockIndicator(p.Src[p.Pos.Offset:]) != nil {
		value := p.parseBlockScalar(indent)

		return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value}
	}

	// The value is re-homed from the dash, where the emitter writes `- ` again:
	// re-homed from its own column, a continuation line under `-  a` would end
	// up no deeper than the dash.
	value, comment := p.parseInlineValue(indent+len("- "), indent)

	return &SequenceItem{Range: mkspan(start, p.contentEnd(pendingBefore)), Dash: dash, Value: value, Comment: comment}
}

// indent is what the value is re-homed by; parentIndent is its owner's, which
// a plain scalar's continuation lines must sit past.
func (p *yamlParser) parseInlineValue(indent, parentIndent int) (Node, *Trivia) {
	switch p.Peek() {
	case '|', '>':
		return p.parseBlockScalar(indent), nil
	case '{', '[':
		node := p.parseFlow(indent)

		return node, p.finishLine()
	case '&', '*', '!':
		return p.parseAnchoredValue(indent, parentIndent), nil
	case '"', '\'':
		scalar := p.parseQuotedScalar(indent)

		return scalar, p.finishLine()
	default:
		start := p.Pos
		scalar := p.parsePlainScalar(indent)
		comment := p.finishLine()

		if comment == nil {
			if end, ok := p.plainContinuationEnd(parentIndent); ok {
				return &Verbatim{Range: mkspan(start, end), Raw: p.slice(start, end), Indent: indent}, nil
			}
		}

		return scalar, comment
	}
}

// plainContinuationEnd consumes the lines YAML folds into a plain scalar,
// those indented past parentIndent. It consumes nothing and reports false when
// there are none or one reads as more than text, leaving the input as parsed.
func (p *yamlParser) plainContinuationEnd(parentIndent int) (Position, bool) {
	saved := p.Pos

	var end, after Position

	found := false

	for !p.Eof() {
		if p.atBlankLine() {
			p.consumeLine()

			continue
		}

		ind := p.measureIndent()
		if ind <= parentIndent {
			break
		}

		if !isPlainContinuation(p.restOfLine(ind)) {
			p.Pos = saved

			return Position{}, false
		}

		end = p.consumeLineKeepEnd()
		after = p.Pos
		found = true
	}

	if !found {
		p.Pos = saved

		return Position{}, false
	}

	p.Pos = after

	return end, true
}

// isPlainContinuation rejects a comment, a dash, or a `: ` that would make
// the line a key, leaving those to the structure around the scalar.
func isPlainContinuation(line []byte) bool {
	line = bytes.TrimRight(line, " \t\r")

	switch {
	// A tab after the indent is separation in place but would lead the line
	// once the spaces before it are re-homed away.
	case line[0] == '#', line[0] == '\t', line[0] == '-' && (len(line) == 1 || line[1] == ' ' || line[1] == '\t'):
		return false
	case bytes.HasPrefix(line, []byte("? ")), bytes.HasPrefix(line, []byte(": ")):
		return false
	case bytes.HasSuffix(line, []byte(":")), bytes.Contains(line, []byte(": ")), bytes.Contains(line, []byte(":\t")):
		return false
	case bytes.Contains(line, []byte(" #")), bytes.Contains(line, []byte("\t#")):
		return false
	}

	return true
}

func (p *yamlParser) restOfLine(from int) []byte {
	start := p.Pos.Offset + from
	end := start

	for end < len(p.Src) && p.Src[end] != '\n' && p.Src[end] != '\r' {
		end++
	}

	return p.Src[start:end]
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
	end := p.consumeValueBlock(indent, blockIsScalarBody)
	raw := p.slice(start, end)

	return &BlockScalar{Range: mkspan(start, end), Raw: raw, Indent: indent, ContentIndent: contentIndent(raw)}
}

func contentIndent(span []byte) int {
	_, body, _ := bytes.Cut(span, []byte("\n"))
	if bytes.ContainsAny(blockIndicator(span), "123456789") {
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

func blockIndicator(span []byte) []byte {
	header, _, _ := bytes.Cut(span, []byte("\n"))

	for field := range bytes.FieldsSeq(header) {
		switch field[0] {
		case '&', '!':
			continue
		case '|', '>':
			return field
		}

		return nil
	}

	return nil
}

// A `+` in the header's comment chomps nothing.
func keepChomped(span []byte) bool {
	return bytes.IndexByte(blockIndicator(span), '+') >= 0
}

// consumeTrailingBlanks returns the end before the run's last line break,
// since every span here excludes its own final break.
func (p *yamlParser) consumeTrailingBlanks(end Position) Position {
	for !p.Eof() && p.atBlankLine() {
		end = p.consumeLineKeepEnd()
	}

	return end
}

// A value whose header goes on to `|` or `>` is a block scalar: its trailing
// whitespace and kept breaks are value, which only a BlockScalar preserves.
// The value owns every line indented past its owner, parentIndent, so `- &a`
// takes the mapping that starts below it at the column after the dash.
func (p *yamlParser) parseAnchoredValue(indent, parentIndent int) Node {
	start := p.Pos
	isBlockScalar := blockIndicator(p.Src[p.Pos.Offset:]) != nil
	end := p.consumeValueBlock(parentIndent, isBlockScalar)
	raw := p.slice(start, end)

	if isBlockScalar {
		return &BlockScalar{Range: mkspan(start, end), Raw: raw, Indent: parentIndent, ContentIndent: contentIndent(raw)}
	}

	return &Verbatim{Range: mkspan(start, end), Raw: raw, Indent: indent}
}

func (p *yamlParser) bailLine(indent int) *Verbatim {
	start := p.Pos
	end := p.consumeValueBlock(indent, blockIsValue)

	return &Verbatim{Range: mkspan(start, end), Raw: p.slice(start, end), Bail: true, Indent: indent}
}

func (p *yamlParser) parseFlow(indent int) *Flow {
	start := p.Pos

	if !p.skipFlowCollection() {
		p.errors = append(p.errors, Error{Pos: start, Msg: "unterminated flow collection"})
	}

	return &Flow{Range: mkspan(start, p.Pos), Raw: p.slice(start, p.Pos), Indent: indent}
}

// skipFlowCollection reports whether the collection closed. A quote opens a
// scalar only where a node starts; inside a plain scalar (`[it's]`) it is text.
func (p *yamlParser) skipFlowCollection() bool {
	depth := 0
	prev := byte(0)

	for !p.Eof() {
		c := p.Peek()

		switch {
		case c == '\'' && startsFlowNode(prev):
			p.skipSingleQuoted()
			prev = c

			continue
		case c == '"' && startsFlowNode(prev):
			p.skipDoubleQuoted()
			prev = c

			continue
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--

			if depth <= 0 {
				p.Advance()

				return true
			}
		case c == '#':
			// A comment line inside a multi-line collection can hold a bracket
			// (`# see ]`) that is not structure.
			if b := p.Src[p.Pos.Offset-1]; b == ' ' || b == '\t' || b == '\n' || b == '\r' {
				for !p.Eof() && !p.atLineEnd() {
					p.Advance()
				}

				continue
			}
		}

		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			prev = c
		}

		p.Advance()
	}

	return false
}

func startsFlowNode(prev byte) bool {
	return prev == '[' || prev == '{' || prev == ',' || prev == ':'
}

// Whether a consumed block is a block scalar's body, where a `#` line is text
// and not a comment, or any other value.
const (
	blockIsScalarBody = true
	blockIsValue      = false
)

// Trailing blank lines are left for the caller to collect as trivia, so they
// survive as blank-line layout instead of being absorbed into the verbatim
// span.
// `+` keep chomping makes the trailing line breaks part of the value, so they
// stay in the span, out of reach of the blank-line rules that would collapse or
// trim them.
func (p *yamlParser) consumeValueBlock(indent int, scalarBody bool) Position {
	start := p.Pos
	end := p.consumeIndentedBlock(indent, scalarBody)

	if keepChomped(p.slice(start, end)) {
		end = p.consumeTrailingBlanks(end)
	}

	return end
}

func (p *yamlParser) consumeIndentedBlock(indent int, scalarBody bool) Position {
	contentEnd := p.consumeLineKeepEnd()
	bodyIndent := -1

	for !p.Eof() {
		if p.atBlankLine() {
			// A line of spaces deeper than a scalar body's own indent is text.
			if scalarBody && bodyIndent >= 0 && p.measureIndent() > bodyIndent {
				contentEnd = p.consumeLineKeepEnd()

				continue
			}

			if !p.triviaPrecedesDeeperLine(indent, scalarBody) {
				break
			}

			p.consumeLine()

			continue
		}

		if ind := p.measureIndent(); ind > indent {
			// A trailing more-indented comment (nothing deeper after it) is not
			// part of the block: absorbing it would tie the block extent to the
			// source indent, which shifts under normalization. Interior
			// comments (deeper content follows) are still absorbed.
			if !scalarBody && p.atCommentLine() && !p.triviaPrecedesDeeperLine(indent, scalarBody) {
				break
			}

			if bodyIndent < 0 {
				bodyIndent = ind
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

func (p *yamlParser) triviaPrecedesDeeperLine(indent int, scalarBody bool) bool {
	saved := p.Pos

	for !p.Eof() {
		if p.atBlankLine() || !scalarBody && p.atCommentLine() {
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

	// Only a node's first character opens a quoted or flow key. Past it a quote
	// is text (`it's: x`), and a key never runs onto the next line.
	atNodeStart := true

	for !p.Eof() && !p.atLineEnd() {
		c := p.Peek()

		switch {
		case atNodeStart && (c == ' ' || c == '\t'):
			p.Advance()

			continue
		case atNodeStart && (c == '&' || c == '!'):
			for !p.Eof() && !p.atLineEnd() && p.Peek() != ' ' && p.Peek() != '\t' {
				p.Advance()
			}

			continue
		case atNodeStart && (c == '\'' || c == '"' || c == '[' || c == '{'):
			line := p.Pos.Line

			var closed bool

			switch c {
			case '\'':
				closed = p.skipSingleQuoted()
			case '"':
				closed = p.skipDoubleQuoted()
			default:
				closed = p.skipFlowCollection()
			}

			if !closed || p.Pos.Line != line {
				p.Pos = saved

				return -1
			}

			atNodeStart = false

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

		atNodeStart = false
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
