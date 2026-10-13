package alpine

import "bytes"

type kind uint8

const (
	kindIdent kind = iota
	kindNumber
	kindString
	kindTemplate
	kindRegex
	kindComment
	kindPunct
)

type token struct {
	kind  kind
	start int
	end   int
}

// regexKeywords are identifiers after which a `/` opens a regular expression
// literal rather than a division.
var regexKeywords = map[string]bool{
	"await": true, "case": true, "delete": true, "do": true, "else": true,
	"in": true, "instanceof": true, "new": true, "of": true, "return": true,
	"throw": true, "typeof": true, "void": true, "yield": true,
}

type lexer struct {
	src  []byte
	off  int
	toks []token
	prev int
	ok   bool
}

// lex scans src into a token stream. Whitespace between tokens is not
// recorded: every non-whitespace byte belongs to exactly one token, so the
// stream plus the original gaps reproduces src byte-identical. ok is false
// when any construct cannot be tokenized with confidence.
func lex(src []byte) ([]token, bool) {
	l := &lexer{src: src, prev: -1, ok: true}

	for l.ok {
		l.skipSpace()

		if l.off >= len(l.src) {
			break
		}

		l.scanToken()
	}

	if !l.ok {
		return nil, false
	}

	return l.toks, true
}

// MultilineLiterals returns the byte spans of the string and template literals
// in JavaScript source that run across a line break, so the whitespace starting
// each of their inner lines is part of the value. ok is false when src cannot be
// tokenized with confidence (an ambiguous `/`, an unterminated construct, a byte
// JavaScript does not allow), in which case nothing about its literals is known.
func MultilineLiterals(src []byte) (spans [][2]int, ok bool) {
	toks, ok := lex(src)
	if !ok {
		return nil, false
	}

	for _, t := range toks {
		if t.kind != kindString && t.kind != kindTemplate {
			continue
		}

		if bytes.ContainsAny(src[t.start:t.end], "\n\r") {
			spans = append(spans, [2]int{t.start, t.end})
		}
	}

	return spans, true
}

func (l *lexer) skipSpace() {
	for l.off < len(l.src) && isSpace(l.src[l.off]) {
		l.off++
	}
}

func (l *lexer) scanToken() {
	c := l.src[l.off]

	switch {
	case isIdentStart(c):
		l.scanIdent()
	case isDigit(c) || (c == '.' && l.off+1 < len(l.src) && isDigit(l.src[l.off+1])):
		l.scanNumber()
	case c == '\'' || c == '"':
		l.scanString()
	case c == '`':
		l.scanTemplate()
	case c == '/':
		l.scanSlash()
	default:
		l.scanPunct()
	}
}

func (l *lexer) emit(k kind, start int) {
	l.toks = append(l.toks, token{kind: k, start: start, end: l.off})

	if k != kindComment {
		l.prev = len(l.toks) - 1
	}
}

func (l *lexer) fail() {
	l.ok = false
}

func (l *lexer) scanIdent() {
	start := l.off

	for l.off < len(l.src) && isIdentPart(l.src[l.off]) {
		l.off++
	}

	l.emit(kindIdent, start)
}

func (l *lexer) scanNumber() {
	start := l.off

	if l.src[l.off] == '0' && l.off+1 < len(l.src) && isRadixPrefix(l.src[l.off+1]) {
		l.off += 2
		l.scanDigits(isHexDigit)
	} else {
		l.scanDigits(isDigit)

		if l.off < len(l.src) && l.src[l.off] == '.' {
			l.off++
			l.scanDigits(isDigit)
		}

		if l.off < len(l.src) && (l.src[l.off] == 'e' || l.src[l.off] == 'E') {
			l.off++

			if l.off < len(l.src) && (l.src[l.off] == '+' || l.src[l.off] == '-') {
				l.off++
			}

			if l.off >= len(l.src) || !isDigit(l.src[l.off]) {
				l.fail()

				return
			}

			l.scanDigits(isDigit)
		}
	}

	if l.off < len(l.src) && l.src[l.off] == 'n' {
		l.off++
	}

	// A glued identifier byte (`2px`) would re-lex differently once a
	// space is inserted around it, so it is a refusal, not a guess.
	if l.off < len(l.src) && isIdentPart(l.src[l.off]) {
		l.fail()

		return
	}

	l.emit(kindNumber, start)
}

func (l *lexer) scanDigits(valid func(byte) bool) {
	for l.off < len(l.src) && (valid(l.src[l.off]) || l.src[l.off] == '_') {
		l.off++
	}
}

func (l *lexer) scanString() {
	start := l.off
	quote := l.src[l.off]
	l.off++

	for l.off < len(l.src) {
		switch c := l.src[l.off]; c {
		case '\\':
			l.off += 2
		case '\n', '\r':
			l.fail()

			return
		case quote:
			l.off++
			l.emit(kindString, start)

			return
		default:
			l.off++
		}
	}

	l.fail()
}

func (l *lexer) scanTemplate() {
	start := l.off
	l.off++

	for l.off < len(l.src) {
		switch {
		case l.src[l.off] == '\\':
			l.off += 2
		case l.src[l.off] == '`':
			l.off++
			l.emit(kindTemplate, start)

			return
		case l.src[l.off] == '$' && l.off+1 < len(l.src) && l.src[l.off+1] == '{':
			l.off += 2
			l.skipTemplateExpr()

			if !l.ok {
				return
			}
		default:
			l.off++
		}
	}

	l.fail()
}

// skipTemplateExpr consumes a ${...} substitution by lexing real tokens, so
// strings, comments, regexes, and nested templates inside it cannot hide the
// closing brace. The tokens are discarded afterwards: the template stays one
// atomic token.
func (l *lexer) skipTemplateExpr() {
	mark := len(l.toks)
	prev := l.prev
	l.prev = -1
	depth := 0

	for l.ok {
		l.skipSpace()

		if l.off >= len(l.src) {
			l.fail()

			break
		}

		if l.src[l.off] == '}' && depth == 0 {
			l.off++

			break
		}

		l.scanToken()

		if n := len(l.toks); n > mark && l.toks[n-1].kind == kindPunct {
			switch l.src[l.toks[n-1].start] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
	}

	l.toks = l.toks[:mark]
	l.prev = prev
}

func (l *lexer) scanSlash() {
	if l.off+1 < len(l.src) {
		switch l.src[l.off+1] {
		case '/':
			l.scanLineComment()

			return
		case '*':
			l.scanBlockComment()

			return
		}
	}

	switch l.regexPosition() {
	case regexYes:
		l.scanRegex()
	case regexNo:
		l.scanPunct()
	case regexAmbiguous:
		// After `}` a slash is division or a regex depending on whether the
		// brace closed a block or an object literal, and after `)` on whether
		// the paren closed an `if`/`for`/`while` head. Tokens alone cannot
		// tell, and guessing division would space a regex apart
		// (`if (b) /b/.test(b)`), so refuse.
		l.fail()
	}
}

type regexPos uint8

const (
	regexNo regexPos = iota
	regexYes
	regexAmbiguous
)

func (l *lexer) regexPosition() regexPos {
	if l.prev < 0 {
		return regexYes
	}

	t := l.toks[l.prev]

	switch t.kind {
	case kindIdent:
		if regexKeywords[string(l.src[t.start:t.end])] {
			return regexYes
		}

		return regexNo
	case kindPunct:
		switch string(l.src[t.start:t.end]) {
		case "]", "++", "--":
			return regexNo
		case ")", "}":
			return regexAmbiguous
		default:
			return regexYes
		}
	default:
		return regexNo
	}
}

func (l *lexer) scanRegex() {
	start := l.off
	l.off++
	inClass := false

	for l.off < len(l.src) {
		c := l.src[l.off]

		if c == '\\' {
			l.off += 2

			continue
		}

		if c == '\n' || c == '\r' {
			l.fail()

			return
		}

		switch c {
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				l.off++

				for l.off < len(l.src) && isIdentPart(l.src[l.off]) {
					l.off++
				}

				l.emit(kindRegex, start)

				return
			}
		}

		l.off++
	}

	l.fail()
}

func (l *lexer) scanLineComment() {
	start := l.off

	for l.off < len(l.src) && l.src[l.off] != '\n' && l.src[l.off] != '\r' {
		l.off++
	}

	l.emit(kindComment, start)
}

func (l *lexer) scanBlockComment() {
	start := l.off
	l.off += 2

	for l.off+1 < len(l.src) {
		if l.src[l.off] == '*' && l.src[l.off+1] == '/' {
			l.off += 2
			l.emit(kindComment, start)

			return
		}

		l.off++
	}

	l.fail()
}

func (l *lexer) scanPunct() {
	n := punctLen(l.src, l.off)

	if n == 0 {
		l.fail()

		return
	}

	start := l.off
	l.off += n
	l.emit(kindPunct, start)
}

func punctLen(src []byte, i int) int {
	at := func(off int) byte {
		if i+off < len(src) {
			return src[i+off]
		}

		return 0
	}

	switch src[i] {
	case '{', '}', '(', ')', '[', ']', ';', ',', ':', '~':
		return 1
	case '.':
		if at(1) == '.' && at(2) == '.' {
			return 3
		}

		return 1
	case '?':
		if at(1) == '?' {
			if at(2) == '=' {
				return 3
			}

			return 2
		}

		// `?.` followed by a digit is a ternary before a fraction
		// (`x ? .5 : y`), not optional chaining.
		if at(1) == '.' && !isDigit(at(2)) {
			return 2
		}

		return 1
	case '=':
		if at(1) == '=' {
			if at(2) == '=' {
				return 3
			}

			return 2
		}

		if at(1) == '>' {
			return 2
		}

		return 1
	case '!':
		if at(1) == '=' {
			if at(2) == '=' {
				return 3
			}

			return 2
		}

		return 1
	case '+':
		if at(1) == '+' || at(1) == '=' {
			return 2
		}

		return 1
	case '-':
		if at(1) == '-' || at(1) == '=' {
			return 2
		}

		return 1
	case '*':
		if at(1) == '*' {
			if at(2) == '=' {
				return 3
			}

			return 2
		}

		if at(1) == '=' {
			return 2
		}

		return 1
	case '/', '%', '^':
		if at(1) == '=' {
			return 2
		}

		return 1
	case '&':
		if at(1) == '&' {
			if at(2) == '=' {
				return 3
			}

			return 2
		}

		if at(1) == '=' {
			return 2
		}

		return 1
	case '|':
		if at(1) == '|' {
			if at(2) == '=' {
				return 3
			}

			return 2
		}

		if at(1) == '=' {
			return 2
		}

		return 1
	case '<':
		if at(1) == '<' {
			if at(2) == '=' {
				return 3
			}

			return 2
		}

		if at(1) == '=' {
			return 2
		}

		return 1
	case '>':
		if at(1) == '>' {
			if at(2) == '>' {
				if at(3) == '=' {
					return 4
				}

				return 3
			}

			if at(2) == '=' {
				return 3
			}

			return 2
		}

		if at(1) == '=' {
			return 2
		}

		return 1
	}

	return 0
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isRadixPrefix(c byte) bool {
	switch c {
	case 'x', 'X', 'b', 'B', 'o', 'O':
		return true
	default:
		return false
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}
