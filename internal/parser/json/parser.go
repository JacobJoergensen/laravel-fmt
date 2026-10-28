package json

import (
	"bytes"
	"fmt"
	"slices"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

type jsonParser struct {
	parserpkg.Cursor

	errors        []Error
	depth         int
	depthExceeded bool

	// jsonc enables comments and trailing commas (tsconfig.json and friends).
	// Strict JSON keeps every comment/trailing-comma path inert.
	jsonc        bool
	fileLeading  []Comment
	rootTrailing *Comment
	fileTrailing []Comment
}

// maxNestingDepth caps array/object recursion so a pathological document (a
// multi-megabyte run of `[`) returns a located error instead of crashing with
// a stack overflow.
const maxNestingDepth = 500

func (p *jsonParser) addError(pos Position, msg string) {
	if p.depthExceeded {
		return
	}

	p.errors = append(p.errors, Error{Pos: pos, Msg: msg})
}

func (p *jsonParser) exceedDepth() {
	p.addError(p.Pos, "maximum nesting depth exceeded")
	p.depthExceeded = true
	p.AdvanceN(len(p.Src) - p.Pos.Offset)
}

func (p *jsonParser) parseDocument() Value {
	p.fileLeading, _ = p.leadingComments()

	if p.Eof() {
		return nil
	}

	root := p.parseValue()

	p.rootTrailing = p.sameLineComment()
	p.fileTrailing, _ = p.leadingComments()

	if !p.Eof() {
		p.addError(p.Pos, "unexpected trailing content after root value")
	}

	return root
}

// scanComment consumes a `//` (to end of line, its newline left in place) or
// `/* */` comment when the cursor is at one, returning it with its delimiters
// and a line comment's trailing whitespace trimmed. It only fires in JSONC
// mode.
func (p *jsonParser) scanComment() (Comment, bool) {
	if !p.jsonc || p.Eof() || p.Peek() != '/' {
		return Comment{}, false
	}

	switch {
	case p.StartsWith("//"):
		start := p.Pos

		for !p.Eof() && p.Peek() != '\n' {
			p.Advance()
		}

		text := bytes.TrimRight(p.Src[start.Offset:p.Pos.Offset], " \t\r")

		return Comment{Range: Span{Start: start, End: p.Pos}, Text: text}, true
	case p.StartsWith("/*"):
		start := p.Pos
		p.AdvanceN(2)

		for !p.Eof() && !p.StartsWith("*/") {
			p.Advance()
		}

		if p.StartsWith("*/") {
			p.AdvanceN(2)
		} else {
			p.addError(start, "unterminated block comment")
		}

		return Comment{Range: Span{Start: start, End: p.Pos}, Text: p.Src[start.Offset:p.Pos.Offset]}, true
	}

	return Comment{}, false
}

// leadingComments consumes whitespace (including newlines) and any own-line
// comments before the next token, returning the comments in order. In strict mode
// it is exactly skipWhitespace and returns no comments.
func (p *jsonParser) leadingComments() ([]Comment, bool) {
	if !p.jsonc {
		return nil, p.skipWhitespace()
	}

	var out []Comment

	for {
		blank := p.skipWhitespace()

		c, ok := p.scanComment()
		if !ok {
			return out, blank
		}

		c.BlankBefore = blank
		out = append(out, c)
	}
}

// sameLineComment consumes inline whitespace (spaces and tabs, never a newline)
// and returns a comment that begins on the current line, or nil. It is how a
// trailing `// note` stays attached to the value or comma it follows.
func (p *jsonParser) sameLineComment() *Comment {
	if !p.jsonc {
		return nil
	}

	for !p.Eof() && (p.Peek() == ' ' || p.Peek() == '\t') {
		p.Advance()
	}

	if c, ok := p.scanComment(); ok {
		return &c
	}

	return nil
}

func (p *jsonParser) parseValue() Value {
	p.skipWhitespace()

	if p.Eof() {
		p.addError(p.Pos, "expected JSON value")

		return nil
	}

	p.depth++
	defer func() { p.depth-- }()

	if p.depth > maxNestingDepth {
		p.exceedDepth()

		return nil
	}

	c := p.Peek()

	switch {
	case c == '{':
		return p.parseObject()
	case c == '[':
		return p.parseArray()
	case c == '"':
		return p.parseString()
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	case c == 't' || c == 'f':
		return p.parseBool()
	case c == 'n':
		return p.parseNull()
	}

	p.addError(p.Pos, fmt.Sprintf("unexpected character %q", c))
	p.Advance()

	return nil
}

func (p *jsonParser) parseObject() *Object {
	start := p.Pos
	p.Advance()
	obj := &Object{}

	lead, blank := p.leadingComments()

	if !p.Eof() && p.Peek() == '}' {
		obj.Dangling = lead
		p.Advance()
		obj.Range = Span{Start: start, End: p.Pos}

		return obj
	}

	for {
		if p.Eof() {
			p.addError(p.Pos, "expected } to close object")
			obj.Dangling = lead
			obj.Range = Span{Start: start, End: p.Pos}

			return obj
		}

		if p.Peek() != '"' {
			p.addError(p.Pos, "expected string key in object")
			obj.Range = Span{Start: start, End: p.Pos}

			return obj
		}

		key := p.parseString()
		beforeColon, _ := p.leadingComments()

		if p.Eof() || p.Peek() != ':' {
			p.addError(p.Pos, "expected : after object key")
			obj.Range = Span{Start: start, End: p.Pos}

			return obj
		}

		p.Advance()
		afterColon, _ := p.leadingComments()
		val := p.parseValue()
		member := &Member{
			Key:         key,
			Value:       val,
			Leading:     lead,
			BeforeValue: slices.Concat(beforeColon, afterColon),
			BlankBefore: blank,
		}
		member.Trailing = p.sameLineComment()
		obj.Members = append(obj.Members, member)

		mid, _ := p.leadingComments()

		if p.Eof() {
			p.addError(p.Pos, "expected } to close object")
			obj.Dangling = mid
			obj.Range = Span{Start: start, End: p.Pos}

			return obj
		}

		switch p.Peek() {
		case ',':
			p.Advance()

			if tc := p.sameLineComment(); tc != nil {
				if member.Trailing == nil {
					member.Trailing = tc
				} else {
					mid = append(mid, *tc)
				}
			}

			next, nextBlank := p.leadingComments()
			lead, blank = slices.Concat(mid, next), nextBlank

			if !p.Eof() && p.Peek() == '}' {
				if !p.jsonc {
					p.addError(p.Pos, "trailing comma not permitted in object")
				}

				obj.Dangling = lead
				p.Advance()
				obj.Range = Span{Start: start, End: p.Pos}

				return obj
			}
		case '}':
			obj.Dangling = mid
			p.Advance()
			obj.Range = Span{Start: start, End: p.Pos}

			return obj
		default:
			p.addError(p.Pos, "expected , or } in object")
			obj.Range = Span{Start: start, End: p.Pos}

			return obj
		}
	}
}

func (p *jsonParser) parseArray() *Array {
	start := p.Pos
	p.Advance()
	arr := &Array{}

	lead, blank := p.leadingComments()

	if !p.Eof() && p.Peek() == ']' {
		arr.Dangling = lead
		p.Advance()
		arr.Range = Span{Start: start, End: p.Pos}

		return arr
	}

	for {
		val := p.parseValue()
		element := &Element{Value: val, Leading: lead, BlankBefore: blank}
		element.Trailing = p.sameLineComment()
		arr.Elements = append(arr.Elements, element)

		mid, _ := p.leadingComments()

		if p.Eof() {
			p.addError(p.Pos, "expected ] to close array")
			arr.Dangling = mid
			arr.Range = Span{Start: start, End: p.Pos}

			return arr
		}

		switch p.Peek() {
		case ',':
			p.Advance()

			if tc := p.sameLineComment(); tc != nil {
				if element.Trailing == nil {
					element.Trailing = tc
				} else {
					mid = append(mid, *tc)
				}
			}

			next, nextBlank := p.leadingComments()
			lead, blank = slices.Concat(mid, next), nextBlank

			if !p.Eof() && p.Peek() == ']' {
				if !p.jsonc {
					p.addError(p.Pos, "trailing comma not permitted in array")
				}

				arr.Dangling = lead
				p.Advance()
				arr.Range = Span{Start: start, End: p.Pos}

				return arr
			}
		case ']':
			arr.Dangling = mid
			p.Advance()
			arr.Range = Span{Start: start, End: p.Pos}

			return arr
		default:
			p.addError(p.Pos, "expected , or ] in array")
			arr.Range = Span{Start: start, End: p.Pos}

			return arr
		}
	}
}

func (p *jsonParser) parseString() *String {
	start := p.Pos
	p.Advance()
	rawOff := p.Pos.Offset

	for !p.Eof() {
		c := p.Peek()

		switch {
		case c == '"':
			raw := p.Src[rawOff:p.Pos.Offset]
			p.Advance()

			return &String{
				Range: Span{Start: start, End: p.Pos},
				Raw:   raw,
			}
		case c == '\\':
			p.Advance()

			if p.Eof() {
				p.addError(p.Pos, "unterminated string escape")

				return &String{
					Range: Span{Start: start, End: p.Pos},
					Raw:   p.Src[rawOff:p.Pos.Offset],
				}
			}

			p.parseStringEscape()
		case c == '\n':
			p.addError(p.Pos, "unescaped newline in string")
			p.Advance()
		case c < 0x20:
			p.addError(p.Pos, fmt.Sprintf("unescaped control character %#02x in string", c))
			p.Advance()
		default:
			p.Advance()
		}
	}

	p.addError(p.Pos, "unterminated string")

	return &String{
		Range: Span{Start: start, End: p.Pos},
		Raw:   p.Src[rawOff:p.Pos.Offset],
	}
}

// parseStringEscape consumes the escape whose backslash is already read. RFC
// 8259 permits only " \ / b f n r t and uXXXX (four hex digits); anything else
// is reported.
func (p *jsonParser) parseStringEscape() {
	esc := p.Peek()

	switch esc {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		p.Advance()
	case 'u':
		p.Advance()

		for range 4 {
			if p.Eof() || !isHexDigit(p.Peek()) {
				p.addError(p.Pos, "\\u escape must be followed by four hex digits")

				return
			}

			p.Advance()
		}
	default:
		p.addError(p.Pos, fmt.Sprintf("invalid string escape %q", esc))
		p.Advance()
	}
}

func (p *jsonParser) parseNumber() *Number {
	start := p.Pos
	startOff := p.Pos.Offset

	if p.Peek() == '-' {
		p.Advance()

		if p.Eof() || !isDigit(p.Peek()) {
			p.addError(p.Pos, "expected digit after -")

			return &Number{Range: Span{Start: start, End: p.Pos}, Raw: p.Src[startOff:p.Pos.Offset]}
		}
	}

	if p.Peek() == '0' {
		p.Advance()

		if !p.Eof() && isDigit(p.Peek()) {
			p.addError(p.Pos, "leading zeros not permitted in numbers")
		}
	}

	for !p.Eof() && isDigit(p.Peek()) {
		p.Advance()
	}

	if !p.Eof() && p.Peek() == '.' {
		p.Advance()

		if p.Eof() || !isDigit(p.Peek()) {
			p.addError(p.Pos, "expected digit after .")
		}

		for !p.Eof() && isDigit(p.Peek()) {
			p.Advance()
		}
	}

	if !p.Eof() && (p.Peek() == 'e' || p.Peek() == 'E') {
		p.Advance()

		if !p.Eof() && (p.Peek() == '+' || p.Peek() == '-') {
			p.Advance()
		}

		if p.Eof() || !isDigit(p.Peek()) {
			p.addError(p.Pos, "expected digit in exponent")
		}

		for !p.Eof() && isDigit(p.Peek()) {
			p.Advance()
		}
	}

	return &Number{
		Range: Span{Start: start, End: p.Pos},
		Raw:   p.Src[startOff:p.Pos.Offset],
	}
}

func (p *jsonParser) parseBool() Value {
	start := p.Pos

	if p.StartsWith("true") {
		p.AdvanceN(4)

		return &Bool{Range: Span{Start: start, End: p.Pos}, Value: true}
	}

	if p.StartsWith("false") {
		p.AdvanceN(5)

		return &Bool{Range: Span{Start: start, End: p.Pos}, Value: false}
	}

	p.addError(start, "invalid boolean literal")
	p.Advance()

	return nil
}

func (p *jsonParser) parseNull() Value {
	start := p.Pos

	if p.StartsWith("null") {
		p.AdvanceN(4)

		return &Null{Range: Span{Start: start, End: p.Pos}}
	}

	p.addError(start, "invalid null literal")
	p.Advance()

	return nil
}

// skipWhitespace consumes whitespace and reports whether it held a blank line.
func (p *jsonParser) skipWhitespace() bool {
	breaks := 0

	for !p.Eof() && isJSONWhitespace(p.Peek()) {
		if p.Peek() == '\n' || p.Peek() == '\r' && p.PeekAt(1) != '\n' {
			breaks++
		}

		p.Advance()
	}

	return breaks > 1
}

// isJSONWhitespace reports whether c is one of the four RFC 8259 §2 whitespace
// bytes; JSON excludes the broader set CSS/HTML allow (e.g. \f).
func isJSONWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
