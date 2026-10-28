package xml

import (
	"bytes"
	"slices"
	"unicode/utf8"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

type xmlParser struct {
	parserpkg.Cursor

	errors        []Error
	depth         int
	depthExceeded bool
}

// maxNestingDepth caps element recursion: deeper, a pathological file
// exhausts the Go stack, a fatal error no recover can catch.
const maxNestingDepth = 500

func (p *xmlParser) addError(pos Position, msg string) {
	if p.depthExceeded {
		return
	}

	p.errors = append(p.errors, Error{Pos: pos, Msg: msg})
}

func (p *xmlParser) exceedDepth() {
	p.addError(p.Pos, "maximum nesting depth exceeded")
	p.depthExceeded = true
	p.AdvanceN(len(p.Src) - p.Pos.Offset)
}

func (p *xmlParser) parseNodes(stack [][]byte) []Node {
	var nodes []Node

	for !p.Eof() {
		if p.atContainerBoundary(stack) {
			break
		}

		if n := p.parseNext(stack); n != nil {
			nodes = append(nodes, n)
		}
	}

	return nodes
}

// Stopping at an ancestor's close tag reports a missing end tag on the element
// that lacks it, instead of swallowing the ancestor's close as a stray one.
func (p *xmlParser) atContainerBoundary(stack [][]byte) bool {
	if len(stack) == 0 {
		return false
	}

	current := stack[len(stack)-1]

	if p.matchesCloseTag(current) {
		return true
	}

	if p.isTagClose() && slices.ContainsFunc(stack[:len(stack)-1], p.matchesCloseTag) {
		return true
	}

	return false
}

func (p *xmlParser) parseNext(stack [][]byte) Node {
	switch {
	case p.StartsWith("<!--"):
		return p.parseComment()
	case p.StartsWith("<![CDATA["):
		return p.parseCDATA()
	case p.StartsWithFold("<!doctype"):
		return p.parseDoctype()
	case p.StartsWith("<?"):
		return p.parseProcessingInstruction()
	case p.isTagOpen():
		return p.parseElement(stack)
	case p.isTagClose():
		return p.parseStrayClose()
	default:
		return p.parseText()
	}
}

func (p *xmlParser) isTagOpen() bool {
	if p.Peek() != '<' {
		return false
	}

	r := p.runeAt(1)

	return isNameStart(r)
}

func (p *xmlParser) isTagClose() bool {
	if p.Peek() != '<' || p.PeekAt(1) != '/' {
		return false
	}

	r := p.runeAt(2)

	return isNameStart(r)
}

// A `<` that starts none of these is not well-formed XML.
func (p *xmlParser) atMarkupBoundary() bool {
	return p.isTagOpen() || p.isTagClose() ||
		p.StartsWith("<!--") || p.StartsWith("<![CDATA[") ||
		p.StartsWithFold("<!doctype") || p.StartsWith("<?")
}

func (p *xmlParser) parseText() Node {
	start := p.Pos

	for !p.Eof() {
		if p.Peek() == '<' {
			if p.atMarkupBoundary() {
				break
			}

			p.addError(p.Pos, "unescaped < in text")
		}

		p.Advance()
	}

	return &Text{Start: start, End: p.Pos, Text: p.Src[start.Offset:p.Pos.Offset]}
}

func (p *xmlParser) parseComment() Node {
	openStart := p.Pos
	p.AdvanceN(len("<!--"))
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith("-->") {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.StartsWith("-->") {
		p.AdvanceN(3)
	} else {
		p.addError(p.Pos, "expected --> to close comment")
	}

	return &Comment{Open: open, Text: text, Close: Span{Start: closeStart, End: p.Pos}}
}

func (p *xmlParser) parseCDATA() Node {
	openStart := p.Pos
	p.AdvanceN(len("<![CDATA["))
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith("]]>") {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.StartsWith("]]>") {
		p.AdvanceN(3)
	} else {
		p.addError(p.Pos, "expected ]]> to close CDATA section")
	}

	return &CDATA{Open: open, Text: text, Close: Span{Start: closeStart, End: p.Pos}}
}

func (p *xmlParser) parseDoctype() Node {
	openStart := p.Pos
	p.AdvanceN(len("<!doctype"))
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset
	p.skipDoctypeBody()

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.Peek() == '>' {
		p.Advance()
	} else {
		p.addError(p.Pos, "expected > to close DOCTYPE")
	}

	return &Doctype{Open: open, Text: text, Close: Span{Start: closeStart, End: p.Pos}}
}

// skipDoctypeBody moves the cursor to the `>` that closes the DOCTYPE. The
// internal subset between `[` and `]` holds declarations with their own `>`,
// and a quoted literal or a comment may hold `]` or `>` as text, so none of
// those end it.
func (p *xmlParser) skipDoctypeBody() {
	inSubset := false

	for !p.Eof() {
		switch c := p.Peek(); {
		case c == '"' || c == '\'':
			p.Advance()

			for !p.Eof() && p.Peek() != c {
				p.Advance()
			}
		case inSubset && p.StartsWith("<!--"):
			for !p.Eof() && !p.StartsWith("-->") {
				p.Advance()
			}
		case c == '[':
			inSubset = true
		case c == ']':
			inSubset = false
		case c == '>' && !inSubset:
			return
		}

		p.Advance()
	}
}

func (p *xmlParser) parseProcessingInstruction() Node {
	openStart := p.Pos
	p.AdvanceN(len("<?"))
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith("?>") {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.StartsWith("?>") {
		p.AdvanceN(2)
	} else {
		p.addError(p.Pos, "expected ?> to close processing instruction")
	}

	return &PI{Open: open, Text: text, Close: Span{Start: closeStart, End: p.Pos}}
}

func (p *xmlParser) parseElement(stack [][]byte) Node {
	p.depth++
	defer func() { p.depth-- }()

	if p.depth > maxNestingDepth {
		p.exceedDepth()

		return nil
	}

	openStart := p.Pos
	p.Advance()
	open := Span{Start: openStart, End: p.Pos}

	name := p.readName()
	attrs := p.readAttrs()

	openCloseStart := p.Pos
	selfClose := false

	switch {
	case p.Peek() == '/' && p.PeekAt(1) == '>':
		p.AdvanceN(2)
		selfClose = true
	case p.Peek() == '>':
		p.Advance()
	default:
		p.addError(p.Pos, "expected > or /> to close element tag")
	}

	el := &Element{
		Open: open, Name: name,
		Attributes: attrs,
		OpenClose:  Span{Start: openCloseStart, End: p.Pos},
		SelfClose:  selfClose,
	}

	if selfClose {
		return el
	}

	el.Children = p.parseNodes(append(stack, name))

	if !p.matchesCloseTag(name) {
		p.addError(openStart, "unclosed element")

		return el
	}

	closeOpenStart := p.Pos
	p.AdvanceN(2)
	el.CloseOpen = Span{Start: closeOpenStart, End: p.Pos}
	el.CloseName = p.readName()
	p.skipTagWhitespace()

	closeFinalStart := p.Pos

	if p.Peek() == '>' {
		p.Advance()
	} else {
		p.addError(p.Pos, "expected > to close element end tag")
	}

	el.CloseFinal = Span{Start: closeFinalStart, End: p.Pos}

	return el
}

func (p *xmlParser) parseStrayClose() Node {
	start := p.Pos
	p.addError(start, "unexpected closing tag")

	for !p.Eof() && p.Peek() != '>' {
		p.Advance()
	}

	if p.Peek() == '>' {
		p.Advance()
	}

	return &Text{Start: start, End: p.Pos, Text: p.Src[start.Offset:p.Pos.Offset]}
}

func (p *xmlParser) matchesCloseTag(name []byte) bool {
	off := p.Pos.Offset

	if off+1 >= len(p.Src) || p.Src[off] != '<' || p.Src[off+1] != '/' {
		return false
	}

	start := off + 2
	end := start + len(name)

	if end > len(p.Src) {
		return false
	}

	if !bytes.Equal(p.Src[start:end], name) {
		return false
	}

	for end < len(p.Src) && isSpaceByte(p.Src[end]) {
		end++
	}

	return end < len(p.Src) && p.Src[end] == '>'
}

func (p *xmlParser) readAttrs() []*Attr {
	var attrs []*Attr

	for !p.Eof() {
		p.skipTagWhitespace()

		c := p.Peek()

		if c == '/' || c == '>' || c == 0 {
			break
		}

		a := p.readAttr()

		if a == nil {
			p.addError(p.Pos, "expected an attribute name")
			p.Advance()

			continue
		}

		attrs = append(attrs, a)
	}

	return attrs
}

func (p *xmlParser) readAttr() *Attr {
	start := p.Pos
	name := p.readName()

	if len(name) == 0 {
		p.Pos = start

		return nil
	}

	attr := &Attr{Start: start, Name: name}

	if p.Peek() == '=' {
		p.Advance()
		p.readAttrValue(attr)
	} else {
		p.addError(p.Pos, "expected = after attribute name")
	}

	attr.End = p.Pos

	return attr
}

func (p *xmlParser) readAttrValue(attr *Attr) {
	c := p.Peek()

	if c == '"' || c == '\'' {
		attr.Quote = c
		p.Advance()

		contentOffset := p.Pos.Offset

		for !p.Eof() && p.Peek() != c {
			p.Advance()
		}

		attr.Value = p.Src[contentOffset:p.Pos.Offset]

		if p.Peek() == c {
			p.Advance()
		} else {
			p.addError(p.Pos, "expected matching quote to close attribute value")
		}

		return
	}

	p.addError(p.Pos, "expected quoted attribute value")

	contentOffset := p.Pos.Offset

	for !p.Eof() {
		b := p.Peek()

		if isSpaceByte(b) || b == '/' || b == '>' {
			break
		}

		p.Advance()
	}

	attr.Value = p.Src[contentOffset:p.Pos.Offset]
}

// asciiNameChar is isNameChar for the bytes below utf8.RuneSelf. Names are
// nearly all ASCII, and calling isNameChar per byte, too large to inline,
// cost the whole format 10%.
var asciiNameChar = func() (table [utf8.RuneSelf]bool) {
	for c := range table {
		table[c] = isNameChar(rune(c))
	}

	return table
}()

func (p *xmlParser) readName() []byte {
	start := p.Pos.Offset
	end := start

	for end < len(p.Src) {
		if c := p.Src[end]; c < utf8.RuneSelf {
			if !asciiNameChar[c] {
				break
			}

			end++

			continue
		}

		r, size := utf8.DecodeRune(p.Src[end:])
		if r == utf8.RuneError && size <= 1 || !isNameChar(r) {
			break
		}

		end += size
	}

	p.AdvanceN(end - start)

	return p.Src[start:end]
}

// runeAt decodes the rune n bytes past the cursor. A byte that is not UTF-8
// reads as -1, which no name holds; a U+FFFD written out in full is a valid
// rune.
func (p *xmlParser) runeAt(n int) rune {
	off := min(p.Pos.Offset+n, len(p.Src))

	if off < len(p.Src) && p.Src[off] < utf8.RuneSelf {
		return rune(p.Src[off])
	}

	r, size := utf8.DecodeRune(p.Src[off:])

	if r == utf8.RuneError && size <= 1 {
		return -1
	}

	return r
}

func (p *xmlParser) skipTagWhitespace() {
	for !p.Eof() && isSpaceByte(p.Peek()) {
		p.Advance()
	}
}

// isNameStart is XML 1.0 (fifth edition) production [4] NameStartChar.
func isNameStart(r rune) bool {
	switch {
	case r == ':', r == '_', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		return true
	case r < 0xC0:
		return false
	}

	return r <= 0xD6 || r >= 0xD8 && r <= 0xF6 || r >= 0xF8 && r <= 0x2FF ||
		r >= 0x370 && r <= 0x37D || r >= 0x37F && r <= 0x1FFF || r >= 0x200C && r <= 0x200D ||
		r >= 0x2070 && r <= 0x218F || r >= 0x2C00 && r <= 0x2FEF || r >= 0x3001 && r <= 0xD7FF ||
		r >= 0xF900 && r <= 0xFDCF || r >= 0xFDF0 && r <= 0xFFFD || r >= 0x10000 && r <= 0xEFFFF
}

// isNameChar is production [4a] NameChar.
func isNameChar(r rune) bool {
	return isNameStart(r) || r == '-' || r == '.' || r >= '0' && r <= '9' || r == 0xB7 ||
		r >= 0x300 && r <= 0x36F || r >= 0x203F && r <= 0x2040
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
