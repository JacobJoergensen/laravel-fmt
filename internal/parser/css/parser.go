package css

import (
	"bytes"
	"strings"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

type cssParser struct {
	parserpkg.Cursor

	errors        []Error
	depth         int
	depthExceeded bool
}

// maxNestingDepth caps block recursion: deeper, a pathological file exhausts
// the Go stack, a fatal error no recover can catch.
const maxNestingDepth = 500

func (p *cssParser) addError(pos Position, msg string) {
	if p.depthExceeded {
		return
	}

	p.errors = append(p.errors, Error{Pos: pos, Msg: msg})
}

func (p *cssParser) exceedDepth() {
	p.addError(p.Pos, "maximum nesting depth exceeded")
	p.depthExceeded = true
	p.AdvanceN(len(p.Src) - p.Pos.Offset)
}

func (p *cssParser) parseNodes(inBlock bool) []Node {
	var nodes []Node

	if inBlock {
		p.depth++
		defer func() { p.depth-- }()

		if p.depth > maxNestingDepth {
			p.exceedDepth()

			return nil
		}
	}

	for !p.Eof() {
		p.skipWhitespace()

		if p.Eof() {
			break
		}

		c := p.Peek()

		if inBlock && c == '}' {
			break
		}

		if c == '}' {
			p.addError(p.Pos, "unexpected }")
			p.Advance()

			continue
		}

		if c == ';' {
			p.Advance()

			continue
		}

		if c == '/' && p.PeekAt(1) == '*' {
			nodes = append(nodes, p.parseComment())

			continue
		}

		if c == '@' {
			nodes = append(nodes, p.parseAtRule())

			continue
		}

		if p.peekTerminator() == '{' {
			nodes = append(nodes, p.parseRule())
		} else {
			nodes = append(nodes, p.parseDeclaration())
		}
	}

	return nodes
}

func (p *cssParser) parseComment() Node {
	openStart := p.Pos
	p.Advance()
	p.Advance()
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && (p.Peek() != '*' || p.PeekAt(1) != '/') {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]

	closeStart := p.Pos

	if !p.Eof() {
		p.Advance()
		p.Advance()
	} else {
		p.addError(p.Pos, "expected */ to close comment")
	}

	return &Comment{
		Open:  open,
		Text:  text,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *cssParser) parseAtRule() Node {
	atStart := p.Pos
	p.Advance()
	name := p.readIdent()

	at := Span{Start: atStart, End: p.Pos}

	if len(name) == 0 {
		p.addError(atStart, "at-rule missing name")
	}

	preludeStart, preludeEnd, prelude := p.readRangeUntil(";{}")

	rule := &AtRule{
		At:           at,
		Name:         name,
		PreludeStart: preludeStart,
		PreludeEnd:   preludeEnd,
		Prelude:      prelude,
	}

	if p.Eof() {
		p.addError(p.Pos, "at-rule missing terminator")

		return rule
	}

	switch p.Peek() {
	case '{':
		braceStart := p.Pos
		p.Advance()
		rule.LBrace = Span{Start: braceStart, End: p.Pos}
		rule.Block = true
		rule.Children = p.parseNodes(true)

		if p.Peek() == '}' {
			rbStart := p.Pos
			p.Advance()
			rule.RBrace = Span{Start: rbStart, End: p.Pos}
		} else {
			p.addError(p.Pos, "expected } to close at-rule block")
		}
	case ';':
		semiStart := p.Pos
		p.Advance()
		rule.Semicolon = Span{Start: semiStart, End: p.Pos}
	case '}':
		// A statement at-rule that is the last in a block may omit its
		// trailing ;, the enclosing } terminates it. Leave the } for the
		// parent block to consume.
	}

	return rule
}

func (p *cssParser) parseRule() Node {
	selStart, selEnd, sel := p.readRangeUntil("{;}")

	rule := &Rule{
		SelectorStart: selStart,
		SelectorEnd:   selEnd,
		Selector:      sel,
	}

	if p.Eof() || p.Peek() != '{' {
		p.addError(p.Pos, "expected { after selector")

		return rule
	}

	braceStart := p.Pos
	p.Advance()
	rule.LBrace = Span{Start: braceStart, End: p.Pos}
	rule.Children = p.parseNodes(true)

	if p.Peek() == '}' {
		rbStart := p.Pos
		p.Advance()
		rule.RBrace = Span{Start: rbStart, End: p.Pos}
	} else {
		p.addError(p.Pos, "expected } to close rule block")
	}

	return rule
}

func (p *cssParser) parseDeclaration() Node {
	propStart, propEnd, prop := p.readRangeUntil(":;{}")

	decl := &Declaration{
		PropertyStart: propStart,
		PropertyEnd:   propEnd,
		Property:      prop,
	}

	if p.Eof() || p.Peek() != ':' {
		p.addError(propStart, "expected : after property name")

		if !p.Eof() && p.Peek() == ';' {
			semiStart := p.Pos
			p.Advance()
			decl.Semicolon = Span{Start: semiStart, End: p.Pos}
		}

		return decl
	}

	colonStart := p.Pos
	p.Advance()
	decl.Colon = Span{Start: colonStart, End: p.Pos}

	valStart, valEnd, val := p.readRangeUntil(";}")
	decl.ValueStart = valStart
	decl.ValueEnd = valEnd
	decl.Value = val

	if impOffset := findImportantOffset(val); impOffset >= 0 {
		trimmedLen := impOffset

		for trimmedLen > 0 && isSpaceByte(val[trimmedLen-1]) {
			trimmedLen--
		}

		impPos := p.positionAt(valStart, valStart.Offset+impOffset)
		decl.Important = true
		decl.ImportantSpan = Span{Start: impPos, End: valEnd}
		decl.ValueEnd = p.positionAt(valStart, valStart.Offset+trimmedLen)
		decl.Value = val[:trimmedLen]
	}

	if !p.Eof() && p.Peek() == ';' {
		semiStart := p.Pos
		p.Advance()
		decl.Semicolon = Span{Start: semiStart, End: p.Pos}
	}

	return decl
}

// readRangeUntil advances to the first byte in stops at depth zero (parens and
// brackets), or EOF, skipping strings and block comments so their contents
// don't trigger a stop. Leading and trailing whitespace are excluded from the
// returned range.
func (p *cssParser) readRangeUntil(stops string) (Position, Position, []byte) {
	p.skipWhitespace()

	start := p.Pos
	lastNonWS := p.Pos
	paren := 0
	bracket := 0

	for !p.Eof() {
		c := p.Peek()

		// A } (outside a string or comment) is always a block terminator, so it
		// ends the range even inside an unbalanced (/[; a stray bracket can't
		// swallow the rest of the file. The imbalance is diagnosed after the loop.
		if c == '}' {
			break
		}

		if paren == 0 && bracket == 0 && strings.IndexByte(stops, c) >= 0 {
			break
		}

		switch {
		case c == '\\':
			// A CSS escape is two code points, so the escaped one is content
			// and not syntax: `.a\}` names a class `a}` rather than closing a
			// block, and `--semi: a\;b` holds a semicolon.
			p.Advance()

			if !p.Eof() {
				p.Advance()
			}

			lastNonWS = p.Pos
		case c == '"' || c == '\'':
			p.skipString(c)
			lastNonWS = p.Pos
		case c == '/' && p.PeekAt(1) == '*':
			p.skipBlockComment()
			lastNonWS = p.Pos
		case c == '(':
			paren++
			p.Advance()
			lastNonWS = p.Pos
		case c == ')':
			if paren > 0 {
				paren--
			}

			p.Advance()
			lastNonWS = p.Pos
		case c == '[':
			bracket++
			p.Advance()
			lastNonWS = p.Pos
		case c == ']':
			if bracket > 0 {
				bracket--
			}

			p.Advance()
			lastNonWS = p.Pos
		case isSpaceByte(c):
			p.Advance()
		default:
			p.Advance()
			lastNonWS = p.Pos
		}
	}

	if paren > 0 || bracket > 0 {
		p.addError(start, "unbalanced brackets")
	}

	return start, lastNonWS, p.Src[start.Offset:lastNonWS.Offset]
}

func (p *cssParser) peekTerminator() byte {
	off := p.Pos.Offset
	paren := 0
	bracket := 0

	for off < len(p.Src) {
		c := p.Src[off]

		switch {
		case c == '\\':
			// The same escape the range scanner honors, so the `}` in `.a\}`
			// does not end the lookahead.
			off += 2
		case c == '"' || c == '\'':
			off = scanStringEnd(p.Src, off, c)
		case c == '/' && off+1 < len(p.Src) && p.Src[off+1] == '*':
			off = scanBlockCommentEnd(p.Src, off)
		case c == '(':
			paren++
			off++
		case c == ')':
			if paren > 0 {
				paren--
			}

			off++
		case c == '[':
			bracket++
			off++
		case c == ']':
			if bracket > 0 {
				bracket--
			}

			off++
		case c == '}':
			return c
		case paren == 0 && bracket == 0 && (c == '{' || c == ';'):
			return c
		default:
			off++
		}
	}

	return 0
}

func (p *cssParser) skipWhitespace() {
	for !p.Eof() && isSpaceByte(p.Peek()) {
		p.Advance()
	}
}

func (p *cssParser) skipString(quote byte) {
	p.Advance()

	for !p.Eof() {
		c := p.Peek()

		if c == '\\' {
			p.Advance()

			if !p.Eof() {
				p.Advance()
			}

			continue
		}

		if c == quote {
			p.Advance()

			return
		}

		p.Advance()
	}

	p.addError(p.Pos, "unterminated string")
}

func (p *cssParser) skipBlockComment() {
	p.Advance()
	p.Advance()

	for !p.Eof() {
		if p.Peek() == '*' && p.PeekAt(1) == '/' {
			p.Advance()
			p.Advance()

			return
		}

		p.Advance()
	}

	p.addError(p.Pos, "unterminated comment")
}

func (p *cssParser) readIdent() []byte {
	startOff := p.Pos.Offset

	for !p.Eof() {
		c := p.Peek()

		if isIdentByte(c) {
			p.Advance()

			continue
		}

		break
	}

	return p.Src[startOff:p.Pos.Offset]
}

func (p *cssParser) positionAt(from Position, targetOffset int) Position {
	pos := from

	for pos.Offset < targetOffset && pos.Offset < len(p.Src) {
		if p.Src[pos.Offset] == '\n' {
			pos.Line++
			pos.Column = 1
		} else {
			pos.Column++
		}

		pos.Offset++
	}

	return pos
}

func scanStringEnd(src []byte, off int, quote byte) int {
	off++

	for off < len(src) {
		c := src[off]

		if c == '\\' && off+1 < len(src) {
			off += 2

			continue
		}

		if c == quote {
			return off + 1
		}

		off++
	}

	return off
}

func scanBlockCommentEnd(src []byte, off int) int {
	off += 2

	for off+1 < len(src) {
		if src[off] == '*' && src[off+1] == '/' {
			return off + 2
		}

		off++
	}

	return len(src)
}

func findImportantOffset(val []byte) int {
	const tag = "important"

	end := len(val)

	for end > 0 && isSpaceByte(val[end-1]) {
		end--
	}

	if end < len(tag) {
		return -1
	}

	if !bytes.EqualFold(val[end-len(tag):end], []byte(tag)) {
		return -1
	}

	before := end - len(tag)

	for before > 0 && isSpaceByte(val[before-1]) {
		before--
	}

	if before == 0 || val[before-1] != '!' {
		return -1
	}

	return before - 1
}

func isIdentByte(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}
