package html

import (
	"bytes"
	"slices"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

type htmlParser struct {
	parserpkg.Cursor

	errors []Error

	// closeIndex maps each lowercased tag name to the ascending offsets of its
	// </name> close tags. Built lazily and reused so void/paired disambiguation
	// is a binary search, not a per-element rescan. nil until built.
	closeIndex map[string][]int

	depth         int
	depthExceeded bool
}

// maxNestingDepth caps element recursion: deeper, a pathological file
// exhausts the Go stack, a fatal error no recover can catch.
const maxNestingDepth = 500

func (p *htmlParser) addError(pos Position, msg string) {
	if p.depthExceeded {
		return
	}

	p.errors = append(p.errors, Error{Pos: pos, Msg: msg})
}

func (p *htmlParser) exceedDepth() {
	p.addError(p.Pos, "maximum nesting depth exceeded")
	p.depthExceeded = true
	p.AdvanceN(len(p.Src) - p.Pos.Offset)
}

func (p *htmlParser) parseNodes(stack [][]byte) []Node {
	var nodes []Node

	for !p.Eof() {
		if p.atContainerBoundary(stack) {
			break
		}

		n := p.parseNext(stack)

		if n == nil {
			p.Advance()
			continue
		}

		nodes = append(nodes, n)
	}

	return nodes
}

// atContainerBoundary reports whether the cursor sits at a tag that ends the
// current element: its own close tag, an open ancestor's close tag (current is
// implicitly closed), or a start tag that closes it (StartTagClosesCurrent).
func (p *htmlParser) atContainerBoundary(stack [][]byte) bool {
	if len(stack) == 0 {
		return false
	}

	current := stack[len(stack)-1]

	if p.matchesCloseTag(current) {
		return true
	}

	if p.isTagClose() {
		if slices.ContainsFunc(stack[:len(stack)-1], p.matchesCloseTag) {
			return true
		}
	}

	if p.isTagOpen() && StartTagClosesCurrent(current, p.peekTagName()) {
		return true
	}

	return false
}

func (p *htmlParser) peekTagName() []byte {
	start := p.Pos.Offset + 1
	off := start

	for off < len(p.Src) {
		c := p.Src[off]

		if isNameStart(c) || isDigit(c) || c == '-' || c == '_' || c == '.' || c == ':' {
			off++
			continue
		}

		break
	}

	return p.Src[start:off]
}

func (p *htmlParser) parseNext(stack [][]byte) Node {
	switch {
	case p.StartsWith("<!--"):
		return p.parseComment()
	case p.StartsWithFold("<!doctype"):
		return p.parseDoctype()
	case p.isTagOpen():
		return p.parseElement(stack)
	case p.isTagClose():
		return p.parseStrayClose()
	default:
		return p.parseText()
	}
}

func (p *htmlParser) isTagOpen() bool {
	return p.Peek() == '<' && isNameStart(p.PeekAt(1))
}

func (p *htmlParser) isTagClose() bool {
	return p.Peek() == '<' && p.PeekAt(1) == '/' && isNameStart(p.PeekAt(2))
}

// atTagBoundary reports whether the cursor sits at a real markup start: a tag,
// a comment, or a doctype. A `<` that begins none of these (`<0`, `a < b`) is
// literal text per the HTML spec, so parseText consumes it rather than splitting
// there, which would drop the `<` and strand the preceding whitespace.
func (p *htmlParser) atTagBoundary() bool {
	return p.isTagOpen() || p.isTagClose() ||
		p.StartsWith("<!--") || p.StartsWithFold("<!doctype")
}

func (p *htmlParser) parseText() Node {
	start := p.Pos

	for !p.Eof() && (p.Peek() != '<' || !p.atTagBoundary()) {
		p.Advance()
	}

	if p.Pos.Offset == start.Offset {
		return nil
	}

	return &Text{Start: start, End: p.Pos, Text: p.Src[start.Offset:p.Pos.Offset]}
}

func (p *htmlParser) parseComment() Node {
	openStart := p.Pos
	p.AdvanceN(4)
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
		p.addError(openStart, "unterminated HTML comment")
	}

	return &Comment{
		Open: open, Text: text,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *htmlParser) parseDoctype() Node {
	openStart := p.Pos
	p.AdvanceN(len("<!doctype"))
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && p.Peek() != '>' {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.Peek() == '>' {
		p.Advance()
	} else {
		p.addError(p.Pos, "expected > to close DOCTYPE")
	}

	return &Doctype{
		Open: open, Text: text,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *htmlParser) parseElement(stack [][]byte) Node {
	p.depth++
	defer func() { p.depth-- }()

	if p.depth > maxNestingDepth {
		p.exceedDepth()

		return nil
	}

	openStart := p.Pos
	p.Advance()
	open := Span{Start: openStart, End: p.Pos}

	name := p.readTagName()
	if !p.Eof() && GluedToTagName(p.Peek()) {
		p.addError(p.Pos, "unexpected character after tag name")
	}

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
		Attributes:  attrs,
		OpenClose:   Span{Start: openCloseStart, End: p.Pos},
		SelfClose:   selfClose,
		SlashClosed: selfClose,
	}

	var parentClose []byte
	if len(stack) > 0 {
		parentClose = stack[len(stack)-1]
	}

	// HTML5 void names (<link>, <br>) normally self-close. The exception: when
	// the author wrote a matching </name> in scope (XML/RSS-shaped, as in
	// Blade-templated feeds), the void name is paired; fall through so
	// <link>foo</link> round-trips instead of becoming a void tag plus close.
	if selfClose || (IsVoidElement(name) && !p.hasPairedClose(name, parentClose)) {
		el.SelfClose = true
		return el
	}

	if IsRawTextElement(name) || IsRcdataElement(name) {
		el.Children = p.parseRawContent(name)
	} else {
		el.Children = p.parseNodes(append(stack, name))
	}

	if p.matchesCloseTag(name) {
		closeOpenStart := p.Pos
		p.AdvanceN(2)
		el.CloseOpen = Span{Start: closeOpenStart, End: p.Pos}
		el.CloseName = p.readTagName()
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

	// No close tag: an element with an optional end tag is implicitly closed
	// here (the formatter inserts it); anything else is genuinely unclosed and
	// fails loudly. The body still ends here; see Element.CloseOpen.
	el.CloseOpen = Span{Start: p.Pos, End: p.Pos}

	if !HasOptionalEndTag(name) {
		p.addError(openStart, "unclosed element")
	}

	return el
}

func (p *htmlParser) parseRawContent(name []byte) []Node {
	start := p.Pos

	for !p.Eof() && !p.matchesCloseTag(name) {
		p.Advance()
	}

	if p.Pos.Offset == start.Offset {
		return nil
	}

	return []Node{&Text{Start: start, End: p.Pos, Text: p.Src[start.Offset:p.Pos.Offset]}}
}

func (p *htmlParser) parseStrayClose() Node {
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

func (p *htmlParser) matchesCloseTag(name []byte) bool {
	return p.matchesCloseTagAt(p.Pos.Offset, name)
}

func (p *htmlParser) matchesCloseTagAt(offset int, name []byte) bool {
	if offset+1 >= len(p.Src) || p.Src[offset] != '<' || p.Src[offset+1] != '/' {
		return false
	}

	off := offset + 2
	end := off + len(name)

	if end > len(p.Src) {
		return false
	}

	if !bytes.EqualFold(p.Src[off:end], name) {
		return false
	}

	for end < len(p.Src) && isSpaceByte(p.Src[end]) {
		end++
	}

	return end < len(p.Src) && p.Src[end] == '>'
}

// hasPairedClose reports whether a </name> close tag exists in scope ahead of
// the cursor (scope ends at </parentClose> or EOF). The check is shallow and
// ignores nested <name> opens, because a document never mixes void and
// paired uses of one HTML5 void name. It searches the lazy closeIndex, so a
// void-heavy document is not rescanned per element.
func (p *htmlParser) hasPairedClose(name, parentClose []byte) bool {
	if p.closeIndex == nil {
		p.closeIndex = CloseTagIndex(p.Src)
	}

	off := p.Pos.Offset

	nameOff, ok := FirstCloseAtOrAfter(p.closeIndex[string(bytes.ToLower(name))], off)
	if !ok {
		return false
	}

	if parentClose == nil {
		return true
	}

	parentOff, ok := FirstCloseAtOrAfter(p.closeIndex[string(bytes.ToLower(parentClose))], off)
	if !ok {
		return true
	}

	return nameOff <= parentOff
}

// CloseTagIndex maps each lowercased tag name to the ascending offsets of its
// </name> close tags; the result is never nil. Shared by the HTML and Blade
// parsers for void-vs-paired disambiguation.
func CloseTagIndex(src []byte) map[string][]int {
	index := make(map[string][]int)

	for i := 0; i+1 < len(src); i++ {
		if src[i] != '<' || src[i+1] != '/' {
			continue
		}

		nameStart := i + 2
		end := nameStart

		for end < len(src) && !isSpaceByte(src[end]) && src[end] != '>' {
			end++
		}

		nameEnd := end

		for end < len(src) && isSpaceByte(src[end]) {
			end++
		}

		if nameEnd == nameStart || end >= len(src) || src[end] != '>' {
			continue
		}

		key := string(bytes.ToLower(src[nameStart:nameEnd]))
		index[key] = append(index[key], i)
	}

	return index
}

// FirstCloseAtOrAfter returns the smallest offset in offs that is >= pos; offs
// must be ascending (as CloseTagIndex produces).
func FirstCloseAtOrAfter(offs []int, pos int) (int, bool) {
	i, _ := slices.BinarySearch(offs, pos)
	if i < len(offs) {
		return offs[i], true
	}

	return 0, false
}

func (p *htmlParser) readTagName() []byte {
	startOff := p.Pos.Offset

	for !p.Eof() {
		c := p.Peek()

		if isNameStart(c) || isDigit(c) || c == '-' || c == '_' || c == '.' || c == ':' {
			p.Advance()
			continue
		}

		break
	}

	return p.Src[startOff:p.Pos.Offset]
}

func (p *htmlParser) readAttrs() []*Attr {
	var attrs []*Attr

	for !p.Eof() {
		p.skipTagWhitespace()

		c := p.Peek()

		if c == '/' || c == '>' || c == 0 {
			break
		}

		a := p.readAttr()

		// Nothing here reads as an attribute name: after skipTagWhitespace and
		// the checks above, the only byte AttrNameByte rejects is a stray `=`.
		// Skipping it would discard source bytes.
		if a == nil {
			p.addError(p.Pos, "expected an attribute name")

			return attrs
		}

		attrs = append(attrs, a)
	}

	return attrs
}

// skipToAttrEquals consumes whitespace between an attribute name and an `=`,
// per HTML5's after-attribute-name state, and reports whether one follows.
// Nothing is consumed when it does not, so `<a href class=y>` still reads as two
// attributes rather than swallowing the second as the first one's value.
func (p *htmlParser) skipToAttrEquals() bool {
	save := p.Pos

	for !p.Eof() && isSpaceByte(p.Peek()) {
		p.Advance()
	}

	if p.Peek() == '=' {
		return true
	}

	p.Pos = save

	return false
}

// skipToAttrValue consumes whitespace between an `=` and its value, per HTML5's
// before-attribute-value state, so `href = "x"` is one attribute rather than a
// valueless `href` plus a second attribute made from the value.
//
// It stops short when the run that follows carries a quote character, because
// that run is the head of the next attribute's quoted value (`@if= data-x="a
// b"`): consuming it would swallow that attribute and orphan its closing quote.
func (p *htmlParser) skipToAttrValue() {
	save := p.Pos

	for !p.Eof() && isSpaceByte(p.Peek()) {
		p.Advance()
	}

	if p.Pos.Offset == save.Offset {
		return
	}

	if c := p.Peek(); c == '"' || c == '\'' {
		return
	}

	for i := p.Pos.Offset; i < len(p.Src); i++ {
		if EndsUnquotedValue(p.Src, i) {
			return
		}

		b := p.Src[i]

		if b == '"' || b == '\'' {
			p.Pos = save

			return
		}
	}
}

func (p *htmlParser) readAttr() *Attr {
	start := p.Pos
	bound := false
	shorthand := false

	// In pure HTML there are no Blade directives, so an @name attribute is an
	// Alpine @event binding (@click, @keydown.enter.prevent). The @, name, and
	// .modifier chain form the attribute name; the value is JS, kept verbatim
	// (Bound stays false, so it never routes through the PHP grammar).
	if p.Peek() == '@' && isNameStart(p.PeekAt(1)) {
		p.Advance()
		p.readAttrName()

		attr := &Attr{Start: start, Name: p.Src[start.Offset:p.Pos.Offset]}

		if p.skipToAttrEquals() {
			eqStart := p.Pos
			p.Advance()
			attr.Eq = Span{Start: eqStart, End: p.Pos}
			p.readAttrValue(attr)
		}

		attr.End = p.Pos

		return attr
	}

	if p.Peek() == ':' {
		p.Advance()
		bound = true

		if p.Peek() == '$' {
			p.Advance()
			shorthand = true
		}
	}

	name := p.readAttrName()

	if len(name) == 0 {
		p.Pos = start
		return nil
	}

	attr := &Attr{
		Start: start,
		Bound: bound, Shorthand: shorthand,
		Name: name,
	}

	if shorthand {
		attr.End = p.Pos
		return attr
	}

	if p.skipToAttrEquals() {
		eqStart := p.Pos
		p.Advance()
		attr.Eq = Span{Start: eqStart, End: p.Pos}
		p.readAttrValue(attr)
	}

	attr.End = p.Pos

	return attr
}

func (p *htmlParser) readAttrName() []byte {
	startOff := p.Pos.Offset

	for !p.Eof() {
		c := p.Peek()

		if !AttrNameByte(c) {
			break
		}

		p.Advance()
	}

	return p.Src[startOff:p.Pos.Offset]
}

// AttrNameByte reports whether c continues an attribute name. HTML5's
// attribute-name state appends every character except whitespace, `/`, `=`
// and `>`, which is why `<button a=0 "">` carries an attribute literally
// named `""` and `<div x-data={ open: false }>` one named `}`. NUL is
// rejected too, so it fails loudly instead of joining a name. Exported so
// the Blade parser's name scan applies the same rule.
func AttrNameByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '/', '=', '>', 0:
		return false
	}

	return true
}

// GluedToTagName reports whether c, read right after a tag name, would have
// continued it: HTML5 ends a tag name only at whitespace, `/` or `>`, so a
// browser reads `<span">` as an element named `span"`. Setting the name apart
// would change the page.
func GluedToTagName(c byte) bool {
	return AttrNameByte(c) || c == '='
}

// EndsUnquotedValue reports whether src[i] ends an unquoted attribute value.
// HTML5 keeps every `/` in such a value, but a `/>` is read as the tag's
// self-closing end, as Blade's component compiler reads it, so `href=/docs`
// keeps its slash and `<x-icon name=check/>` stays self-closing.
func EndsUnquotedValue(src []byte, i int) bool {
	switch src[i] {
	case '>':
		return true
	case '/':
		return i+1 < len(src) && src[i+1] == '>'
	}

	return isSpaceByte(src[i])
}

func (p *htmlParser) readAttrValue(attr *Attr) {
	p.skipToAttrValue()

	c := p.Peek()

	if c == '"' || c == '\'' {
		attr.Quote = c
		p.Advance()

		contentOffset := p.Pos.Offset

		for !p.Eof() && p.Peek() != c {
			p.Advance()
		}

		attr.Value = p.Src[contentOffset:p.Pos.Offset]
		attr.ValueStart = contentOffset

		if p.Peek() == c {
			p.Advance()
		} else {
			p.addError(p.Pos, "expected matching quote to close attribute value")
		}
	} else {
		contentOffset := p.Pos.Offset

		for !p.Eof() && !EndsUnquotedValue(p.Src, p.Pos.Offset) {
			p.Advance()
		}
		attr.Value = p.Src[contentOffset:p.Pos.Offset]
		attr.ValueStart = contentOffset
	}
}

func (p *htmlParser) skipTagWhitespace() {
	for !p.Eof() && isSpaceByte(p.Peek()) {
		p.Advance()
	}
}

func isNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}
