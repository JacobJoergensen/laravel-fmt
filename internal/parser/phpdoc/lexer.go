package phpdoc

type tokKind uint8

const (
	tokEOF tokKind = iota
	tokIdent
	tokVar
	tokInt
	tokFloat
	tokString
	tokQuestion
	tokPipe
	tokAmp
	tokLParen
	tokRParen
	tokLAngle
	tokRAngle
	tokLBrace
	tokRBrace
	tokLBracket
	tokRBracket
	tokColon
	tokDoubleColon
	tokComma
	tokEquals
	tokEllipsis
	tokStar
	tokInvalid
)

type token struct {
	kind          tokKind
	raw           []byte
	end           int // byte offset in src immediately after raw
	newlineBefore bool
}

type lexer struct {
	src []byte
	pos int
}

// lex always ends the slice with one tokEOF, so the parser can index the
// current token without a bounds check.
func lex(src []byte) []token {
	l := &lexer{src: src}

	var toks []token

	for {
		newline := l.skipSpace()

		if l.pos >= len(l.src) {
			return append(toks, token{kind: tokEOF, end: l.pos, newlineBefore: newline})
		}

		tok := l.next()
		tok.end = l.pos
		tok.newlineBefore = newline
		toks = append(toks, tok)
	}
}

func (l *lexer) skipSpace() bool {
	newline := false

	for l.pos < len(l.src) {
		switch l.src[l.pos] {
		case ' ', '\t':
			l.pos++
		case '\r', '\n':
			newline = true
			l.pos++
		default:
			return newline
		}
	}

	return newline
}

func (l *lexer) next() token {
	start := l.pos
	c := l.src[l.pos]

	switch {
	case c == '$':
		return l.scanVar(start)
	case isIdentStart(c) || c == '\\':
		return l.scanIdent(start)
	case c >= '0' && c <= '9':
		return l.scanNumber(start)
	case c == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9':
		return l.scanNumber(start)
	case c == '\'' || c == '"':
		return l.scanString(start, c)
	}

	return l.scanPunct(start, c)
}

func (l *lexer) scanIdent(start int) token {
	for l.pos < len(l.src) {
		c := l.src[l.pos]

		switch {
		case isIdentPart(c) || c == '\\':
			l.pos++
		case c == '-' && l.pos+1 < len(l.src) && isIdentStart(l.src[l.pos+1]):
			l.pos++
		default:
			return token{kind: tokIdent, raw: l.src[start:l.pos]}
		}
	}

	return token{kind: tokIdent, raw: l.src[start:l.pos]}
}

func (l *lexer) scanVar(start int) token {
	l.pos++

	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		l.pos++
	}

	return token{kind: tokVar, raw: l.src[start:l.pos]}
}

func (l *lexer) scanNumber(start int) token {
	if l.src[l.pos] == '-' {
		l.pos++
	}

	for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
		l.pos++
	}

	kind := tokInt

	if l.pos+1 < len(l.src) && l.src[l.pos] == '.' && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9' {
		kind = tokFloat
		l.pos++

		for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
			l.pos++
		}
	}

	return token{kind: kind, raw: l.src[start:l.pos]}
}

func (l *lexer) scanString(start int, quote byte) token {
	l.pos++

	for l.pos < len(l.src) {
		c := l.src[l.pos]

		if c == '\\' && l.pos+1 < len(l.src) {
			l.pos += 2

			continue
		}

		if c == quote {
			l.pos++

			break
		}

		l.pos++
	}

	return token{kind: tokString, raw: l.src[start:l.pos]}
}

func (l *lexer) scanPunct(start int, c byte) token {
	single := func(k tokKind) token {
		l.pos++

		return token{kind: k, raw: l.src[start:l.pos]}
	}

	switch c {
	case '?':
		return single(tokQuestion)
	case '|':
		return single(tokPipe)
	case '&':
		return single(tokAmp)
	case '(':
		return single(tokLParen)
	case ')':
		return single(tokRParen)
	case '<':
		return single(tokLAngle)
	case '>':
		return single(tokRAngle)
	case '{':
		return single(tokLBrace)
	case '}':
		return single(tokRBrace)
	case '[':
		return single(tokLBracket)
	case ']':
		return single(tokRBracket)
	case ',':
		return single(tokComma)
	case '=':
		return single(tokEquals)
	case '*':
		return single(tokStar)
	case ':':
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == ':' {
			l.pos += 2

			return token{kind: tokDoubleColon, raw: l.src[start:l.pos]}
		}

		return single(tokColon)
	case '.':
		if l.pos+2 < len(l.src) && l.src[l.pos+1] == '.' && l.src[l.pos+2] == '.' {
			l.pos += 3

			return token{kind: tokEllipsis, raw: l.src[start:l.pos]}
		}
	}

	return single(tokInvalid)
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}
