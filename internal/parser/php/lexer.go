package php

import (
	"bytes"
	"fmt"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

// minBytesPerToken bounds how densely real PHP packs tokens: across 1451 files
// of a Laravel app none had fewer than 3.94 bytes per token (median 5.87).
// Sizing the token slice from it up front replaces append growth, which
// allocated about five times the final slice and was 43% of a format's bytes.
const minBytesPerToken = 4

// Lex scans src into a token stream and a slice of errors. The stream is
// always complete and ends in KindEOF, so callers can keep parsing through
// partial sources.
func Lex(src []byte) ([]Token, []Error) {
	l := &lexer{
		Src: src, Pos: Position{Line: 1, Column: 1},
		tokens: make([]Token, 0, len(src)/minBytesPerToken+1),
	}

	l.run()

	return l.tokens, l.errors
}

// LexBare is like Lex but starts in PHP mode, skipping inline-HTML and
// open-tag handling.
func LexBare(src []byte) ([]Token, []Error) {
	l := &lexer{
		Src: src, Pos: Position{Line: 1, Column: 1},
		tokens: make([]Token, 0, len(src)/minBytesPerToken+1),
		inPHP:  true,
	}

	l.run()

	return l.tokens, l.errors
}

// lexFrontmatter is Lex stopped after the first `?>`, so the stream is the one
// lexing only the block before it would give. The first `?>` in the bytes can
// only come at or before the tag that closes PHP mode, which makes it an upper
// bound to size the tokens from.
func lexFrontmatter(src []byte) ([]Token, []Error, bool) {
	end := bytes.Index(src, []byte("?>"))
	if end < 0 {
		end = len(src)
	}

	l := &lexer{
		Src: src, Pos: Position{Line: 1, Column: 1},
		tokens:        make([]Token, 0, end/minBytesPerToken+1),
		untilCloseTag: true,
	}

	l.run()

	return l.tokens, l.errors, l.closed
}

// LexBareFunc lexes src in PHP mode like LexBare but invokes fn for each token
// and stops as soon as fn returns false, so a caller that needs only a prefix
// avoids lexing the rest. Errors are not reported; callers that need them use
// LexBare.
func LexBareFunc(src []byte, fn func(Token) bool) {
	l := &lexer{
		Src: src, Pos: Position{Line: 1, Column: 1},
		inPHP: true,
		visit: fn,
	}

	l.run()
}

// CloseTagOffset returns where the first `?>` that ends PHP mode starts in src,
// bare PHP after an open tag, or -1 when PHP runs to the end. It is PHP's rule,
// not a byte search: a `?>` inside a string, a heredoc or a block comment ends
// nothing, and one after a line comment ends both.
func CloseTagOffset(src []byte) int {
	at := -1

	LexBareFunc(src, func(tok Token) bool {
		if tok.Kind == KindCloseTag {
			at = tok.Span.Start.Offset

			return false
		}

		return true
	})

	return at
}

type lexer struct {
	parserpkg.Cursor

	tokens []Token
	errors []Error
	trivia []Trivia
	inPHP  bool

	visit func(Token) bool
	stop  bool
	last  Kind

	untilCloseTag bool
	closed        bool
}

func (l *lexer) run() {
	for !l.Eof() && !l.stop && !l.closed {
		if !l.inPHP {
			l.scanInlineHTML()

			if l.Eof() {
				break
			}

			l.consumeOpenTag()
			continue
		}

		l.scanPHP()
	}

	l.emit(KindEOF, l.Pos, l.Pos, nil)
}

func (l *lexer) emit(kind Kind, start, end Position, text []byte) {
	if l.stop {
		return
	}

	tok := Token{
		Kind:    kind,
		Span:    Span{Start: start, End: end},
		Text:    text,
		Leading: l.trivia,
	}

	l.trivia = nil
	l.last = kind

	if l.visit != nil {
		if !l.visit(tok) {
			l.stop = true
		}

		return
	}

	l.tokens = append(l.tokens, tok)
}

func (l *lexer) addTrivia(kind TriviaKind, start, end Position, text []byte) {
	l.trivia = append(l.trivia, Trivia{
		Kind: kind,
		Span: Span{Start: start, End: end},
		Text: text,
	})
}

func (l *lexer) addError(pos Position, msg string) {
	l.errors = append(l.errors, Error{Pos: pos, Msg: msg})
}

func (l *lexer) scanInlineHTML() {
	start := l.Pos

	for !l.Eof() {
		if l.Peek() == '<' && (l.atPHPOpenTag() || l.atShortEchoTag()) {
			break
		}

		l.Advance()
	}

	if l.Pos.Offset > start.Offset {
		l.emit(KindInlineHTML, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
	}
}

func (l *lexer) atPHPOpenTag() bool {
	if !l.StartsWithFold("<?php") {
		return false
	}

	if l.Pos.Offset+5 >= len(l.Src) {
		return true
	}

	// PHP's scanner opens only on `<?php` followed by a space, tab or line
	// break; a form feed after it leaves the text inline HTML.
	switch l.Src[l.Pos.Offset+5] {
	case ' ', '\t', '\n', '\r':
		return true
	}

	return false
}

func (l *lexer) atShortEchoTag() bool {
	return l.Peek() == '<' && l.PeekAt(1) == '?' && l.PeekAt(2) == '='
}

func (l *lexer) consumeOpenTag() {
	start := l.Pos

	switch {
	case l.atPHPOpenTag():
		l.AdvanceN(5)
		l.emit(KindOpenTag, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
	case l.atShortEchoTag():
		l.AdvanceN(3)
		l.emit(KindOpenTagEcho, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
	}

	l.inPHP = true
}

func (l *lexer) scanPHP() {
	l.scanTrivia()

	if l.Eof() {
		return
	}

	start := l.Pos
	c := l.Peek()

	switch {
	case c == '?' && l.PeekAt(1) == '>':
		l.AdvanceN(2)
		l.emit(KindCloseTag, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
		l.inPHP = false

		// The newline after the tag belongs to what follows, so a frontmatter
		// lex ends here, where lexing the block alone would.
		if l.untilCloseTag {
			l.closed = true

			return
		}

		// PHP consumes a single newline immediately following ?>; keep the
		// bytes as trivia so the token stream stays lossless.
		nlStart := l.Pos

		if l.Peek() == '\r' {
			l.Advance()
		}

		if l.Peek() == '\n' {
			l.Advance()
		}

		if l.Pos.Offset > nlStart.Offset {
			l.addTrivia(TriviaWhitespace, nlStart, l.Pos, l.Src[nlStart.Offset:l.Pos.Offset])
		}
	case c == '$' && IsIdentStart(l.PeekAt(1)):
		l.scanVariable(start)
	case c == '$' && (l.PeekAt(1) == '$' || l.PeekAt(1) == '{'):
		// `$$name` and `${expr}` open with a bare `$`.
		l.Advance()
		l.emit(KindDollar, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
	case IsIdentStart(c):
		l.scanIdentOrKeyword(start)
	case isDigit(c):
		l.scanNumber(start)
	case c == '.' && isDigit(l.PeekAt(1)):
		// A `.` immediately followed by a digit is a leading-dot float
		// literal (`.5`), per PHP's maximal-munch DNUMBER rule, not the
		// concatenation operator.
		l.scanNumber(start)
	case c == '"':
		l.scanDoubleQuotedString(start)
	case c == '\'':
		l.scanSingleQuotedString(start)
	case c == '<' && l.PeekAt(1) == '<' && l.PeekAt(2) == '<':
		l.scanHeredoc(start)
	case c == '`':
		l.scanShellExec(start)
	case c == '#' && l.PeekAt(1) == '[':
		l.AdvanceN(2)
		l.emit(KindAttributeOpen, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
	default:
		l.scanOperator(start)
	}
}

func (l *lexer) scanTrivia() {
	for !l.Eof() {
		c := l.Peek()

		switch {
		case IsWhitespace(c):
			l.scanWhitespace()
		case c == '/' && l.PeekAt(1) == '/':
			l.scanLineComment()
		case c == '#' && l.PeekAt(1) != '[':
			l.scanLineComment()
		case c == '/' && l.PeekAt(1) == '*':
			l.scanBlockComment()
		default:
			return
		}
	}
}

func (l *lexer) scanWhitespace() {
	start := l.Pos

	for !l.Eof() && IsWhitespace(l.Peek()) {
		l.Advance()
	}

	l.addTrivia(TriviaWhitespace, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

func (l *lexer) scanLineComment() {
	start := l.Pos

	for !l.Eof() {
		c := l.Peek()

		// PHP's scanner ends a line comment at CR as well as LF, so a lone CR
		// leaves the rest of the line as code.
		if c == '\n' || c == '\r' {
			break
		}

		// A close tag ends a line comment without consuming the ?>.
		if c == '?' && l.PeekAt(1) == '>' {
			break
		}

		l.Advance()
	}

	l.addTrivia(TriviaLineComment, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

func (l *lexer) scanBlockComment() {
	start := l.Pos
	isDoc := l.PeekAt(2) == '*' && l.PeekAt(3) != '/'

	l.AdvanceN(2)

	terminated := false

	for !l.Eof() {
		if l.Peek() == '*' && l.PeekAt(1) == '/' {
			l.AdvanceN(2)
			terminated = true

			break
		}

		l.Advance()
	}

	if !terminated {
		l.addError(start, "unterminated block comment")
	}

	kind := TriviaBlockComment

	if isDoc {
		kind = TriviaDocComment
	}

	l.addTrivia(kind, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

func (l *lexer) scanIdentOrKeyword(start Position) {
	for !l.Eof() && isIdentCont(l.Peek()) {
		l.Advance()
	}

	text := l.Src[start.Offset:l.Pos.Offset]
	kind := KeywordKind(text)

	if kind == KindYield && l.maybeConsumeYieldFrom() {
		l.emit(KindYieldFrom, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
		return
	}

	// enum is a soft keyword: Rules\Enum, new Enum, Enum::from() are
	// ordinary names. Only a real declaration is the keyword; anything
	// else lexes as an identifier.
	if kind == KindEnum && !l.enumStartsDeclaration() {
		kind = KindIdent
	}

	if bytes.EqualFold(text, haltCompiler) && l.haltIsTopLevelCall() && l.consumeHaltPayload(start) {
		return
	}

	l.emit(kind, start, l.Pos, text)
}

// haltCompiler is spelled case-insensitively, as PHP reads it.
var haltCompiler = []byte("__halt_compiler")

// haltIsTopLevelCall reports whether the name just emitted is the construct
// rather than a member. PHP 7 let reserved words name methods, so
// `$archive->__halt_compiler()` and `Phar::__halt_compiler()` are ordinary calls
// and must not swallow the rest of the file.
func (l *lexer) haltIsTopLevelCall() bool {
	// The name has not been emitted yet, so l.last is the token before it. It
	// is tracked in emit because a visiting lex keeps no token slice.
	switch l.last {
	case KindArrow, KindNullSafeArrow, KindScopeRes, KindFunction, KindConst:
		return false
	}

	return true
}

// consumeHaltPayload swallows everything after a `__halt_compiler();` as one
// opaque token and stops the lexer: those bytes are the archive a PHAR stub
// carries, not PHP, and the formatter puts them back unchanged.
func (l *lexer) consumeHaltPayload(start Position) bool {
	mark := l.Pos
	triviaMark := len(l.trivia)

	// `(`, `)` and `;` are all required: PHP will not compile the call without
	// them, and a close tag does not stand in for the `;` here.
	for _, want := range []byte{'(', ')', ';'} {
		l.scanTrivia()

		if l.Peek() != want {
			l.Pos = mark
			l.trivia = l.trivia[:triviaMark]

			return false
		}

		l.Advance()
	}

	// One token for the whole call, so the parser sees a single statement rather
	// than a name it has no rule for.
	l.emit(KindHaltCompiler, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])

	payload := l.Pos

	for !l.Eof() {
		l.Advance()
	}

	if l.Pos.Offset > payload.Offset {
		l.emit(KindHaltPayload, payload, l.Pos, l.Src[payload.Offset:l.Pos.Offset])
	}

	return true
}

// enumStartsDeclaration reports whether this enum opens a declaration (enum
// <label> then {, :, or implements) rather than a plain identifier. Trivia
// between the parts is skipped to match PHP's lexer.
func (l *lexer) enumStartsDeclaration() bool {
	p := skipTriviaBytes(l.Src, l.Pos.Offset)

	if p >= len(l.Src) || !IsIdentStart(l.Src[p]) {
		return false
	}

	for p < len(l.Src) && isIdentCont(l.Src[p]) {
		p++
	}

	p = skipTriviaBytes(l.Src, p)
	if p >= len(l.Src) {
		return false
	}

	if l.Src[p] == '{' || l.Src[p] == ':' {
		return true
	}

	const implements = "implements"
	end := p + len(implements)

	return end <= len(l.Src) &&
		bytes.EqualFold(l.Src[p:end], []byte(implements)) &&
		(end == len(l.Src) || !isIdentCont(l.Src[end]))
}

// skipTriviaBytes returns the first offset at or after p past any whitespace or
// comment. A raw lookahead scan: unlike scanTrivia it emits nothing.
func skipTriviaBytes(src []byte, p int) int {
	for p < len(src) {
		switch {
		case IsWhitespace(src[p]):
			p++
		case src[p] == '/' && p+1 < len(src) && src[p+1] == '/':
			p += 2
			p = skipToLineEnd(src, p)
		case src[p] == '#' && (p+1 >= len(src) || src[p+1] != '['):
			p++
			p = skipToLineEnd(src, p)
		case src[p] == '/' && p+1 < len(src) && src[p+1] == '*':
			p += 2
			for p+1 < len(src) && (src[p] != '*' || src[p+1] != '/') {
				p++
			}

			p += 2
		default:
			return p
		}
	}

	return p
}

// skipToLineEnd ends a line comment where scanLineComment does: at CR, LF or a
// close tag.
func skipToLineEnd(src []byte, p int) int {
	for p < len(src) && src[p] != '\n' && src[p] != '\r' {
		if src[p] == '?' && p+1 < len(src) && src[p+1] == '>' {
			break
		}

		p++
	}

	return p
}

func (l *lexer) maybeConsumeYieldFrom() bool {
	p := l.Pos.Offset

	for p < len(l.Src) && IsWhitespace(l.Src[p]) {
		p++
	}

	if p+4 > len(l.Src) {
		return false
	}

	if !bytes.EqualFold(l.Src[p:p+4], []byte("from")) {
		return false
	}

	if p+4 < len(l.Src) && isIdentCont(l.Src[p+4]) {
		return false
	}

	for l.Pos.Offset < p+4 {
		l.Advance()
	}

	return true
}

func (l *lexer) scanVariable(start Position) {
	l.Advance()

	for !l.Eof() && isIdentCont(l.Peek()) {
		l.Advance()
	}

	l.emit(KindVariable, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

func (l *lexer) scanNumber(start Position) {
	isFloat := false

	if l.Peek() == '0' {
		switch l.PeekAt(1) {
		case 'x', 'X':
			l.AdvanceN(2)
			l.consumeDigits(isHexDigit)
			l.emit(KindInt, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
			return
		case 'b', 'B':
			l.AdvanceN(2)
			l.consumeDigits(isBinDigit)
			l.emit(KindInt, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
			return
		case 'o', 'O':
			l.AdvanceN(2)
			l.consumeDigits(isOctDigit)
			l.emit(KindInt, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
			return
		}
	}

	l.consumeDigits(isDigit)

	// PHP's DNUM makes the fraction optional, so `1.` is a float and maximal
	// munch takes the dot even with no digit after it. Lexing it as an integer
	// plus concatenation turns `1. + 2` from 3.0 into the string '12'.
	if l.Peek() == '.' {
		isFloat = true
		l.Advance()
		l.consumeDigits(isDigit)
	}

	if c := l.Peek(); c == 'e' || c == 'E' {
		next := l.PeekAt(1)
		exponentOffset := 1

		if next == '+' || next == '-' {
			next = l.PeekAt(2)
			exponentOffset = 2
		}

		if isDigit(next) {
			isFloat = true
			l.AdvanceN(exponentOffset)
			l.consumeDigits(isDigit)
		}
	}

	kind := KindInt

	if isFloat {
		kind = KindFloat
	}

	l.emit(kind, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

func (l *lexer) consumeDigits(accept func(byte) bool) {
	for !l.Eof() {
		c := l.Peek()

		if accept(c) {
			l.Advance()
			continue
		}

		if c == '_' && accept(l.PeekAt(1)) {
			l.Advance()
			continue
		}

		break
	}
}

func (l *lexer) scanSingleQuotedString(start Position) {
	l.Advance()

	for !l.Eof() {
		c := l.Peek()

		if c == '\\' {
			l.Advance()

			if !l.Eof() {
				l.Advance()
			}

			continue
		}

		if c == '\'' {
			l.Advance()
			l.emit(KindString, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
			return
		}

		l.Advance()
	}

	l.addError(start, "unterminated single-quoted string")
	l.emit(KindString, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

// scanDoubleQuotedString emits the entire double-quoted string as one
// KindString token. Interpolation internals are preserved verbatim in Text;
// the formatter never rewrites inside a string literal.
func (l *lexer) scanDoubleQuotedString(start Position) {
	l.Advance()

	for !l.Eof() {
		c := l.Peek()

		if c == '\\' {
			l.Advance()

			if !l.Eof() {
				l.Advance()
			}

			continue
		}

		if c == '"' {
			l.Advance()
			l.emit(KindString, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
			return
		}

		l.Advance()
	}

	l.addError(start, "unterminated double-quoted string")
	l.emit(KindString, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

// scanShellExec emits the whole backtick literal as one KindShellExec token; a
// backslash escapes the next byte, so an escaped backtick does not end it.
func (l *lexer) scanShellExec(start Position) {
	l.Advance()

	for !l.Eof() {
		c := l.Peek()

		if c == '\\' {
			l.Advance()

			if !l.Eof() {
				l.Advance()
			}

			continue
		}

		if c == '`' {
			l.Advance()
			l.emit(KindShellExec, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])

			return
		}

		l.Advance()
	}

	l.addError(start, "unterminated shell exec literal")
	l.emit(KindShellExec, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

// scanHeredoc emits a heredoc or nowdoc literal as a single opaque KindString
// token. The body runs until a line whose leading whitespace is followed by
// the label and a non-identifier byte; PHP 7.3+ flexible indentation is
// tolerated.
func (l *lexer) scanHeredoc(start Position) {
	l.AdvanceN(3)

	for l.Peek() == ' ' || l.Peek() == '\t' {
		l.Advance()
	}

	quote := byte(0)

	if c := l.Peek(); c == '\'' || c == '"' {
		quote = c
		l.Advance()
	}

	if !IsIdentStart(l.Peek()) {
		l.addError(start, "expected heredoc or nowdoc label")
		l.emit(KindInvalid, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
		return
	}

	labelStart := l.Pos.Offset

	for isIdentCont(l.Peek()) {
		l.Advance()
	}

	label := l.Src[labelStart:l.Pos.Offset]

	if quote != 0 {
		if l.Peek() == quote {
			l.Advance()
		} else {
			l.addError(l.Pos, "expected matching closing quote after heredoc label")
		}
	}

	// PHP's heredoc newline is CRLF, LF or a lone CR:
	// `<<<A` CR `hello` CR `A;` compiles.
	if !l.skipHeredocNewline() {
		l.addError(l.Pos, "expected newline after heredoc label")
	}

	for !l.Eof() {
		lineStart := l.Pos

		for l.Peek() == ' ' || l.Peek() == '\t' {
			l.Advance()
		}

		if l.hasClosingLabel(label) {
			l.AdvanceN(len(label))
			l.emit(KindString, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
			return
		}

		l.Pos = lineStart

		for !l.Eof() && l.Peek() != '\n' && l.Peek() != '\r' {
			l.Advance()
		}

		l.skipHeredocNewline()
	}

	l.addError(start, "unterminated heredoc or nowdoc")
	l.emit(KindString, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
}

func (l *lexer) skipHeredocNewline() bool {
	switch l.Peek() {
	case '\r':
		l.Advance()

		if l.Peek() == '\n' {
			l.Advance()
		}

		return true
	case '\n':
		l.Advance()

		return true
	}

	return false
}

// hasClosingLabel reports whether the position begins with label followed by a
// non-identifier byte, the PHP rule that stops a label embedded in a longer
// identifier from terminating the body.
func (l *lexer) hasClosingLabel(label []byte) bool {
	end := l.Pos.Offset + len(label)

	if end > len(l.Src) {
		return false
	}

	for i := range label {
		if l.Src[l.Pos.Offset+i] != label[i] {
			return false
		}
	}

	next := byte(0)

	if end < len(l.Src) {
		next = l.Src[end]
	}

	return !isIdentCont(next)
}

func (l *lexer) scanOperator(start Position) {
	c := l.Peek()

	emit := func(kind Kind, n int) {
		l.AdvanceN(n)
		l.emit(kind, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
	}

	switch c {
	case '+':
		switch l.PeekAt(1) {
		case '+':
			emit(KindPlusPlus, 2)
		case '=':
			emit(KindPlusAssign, 2)
		default:
			emit(KindPlus, 1)
		}
	case '-':
		switch l.PeekAt(1) {
		case '-':
			emit(KindMinusMinus, 2)
		case '=':
			emit(KindMinusAssign, 2)
		case '>':
			emit(KindArrow, 2)
		default:
			emit(KindMinus, 1)
		}
	case '*':
		if l.PeekAt(1) == '*' {
			if l.PeekAt(2) == '=' {
				emit(KindStarStarAssign, 3)
				return
			}

			emit(KindStarStar, 2)
			return
		}

		if l.PeekAt(1) == '=' {
			emit(KindStarAssign, 2)
			return
		}

		emit(KindStar, 1)
	case '/':
		if l.PeekAt(1) == '=' {
			emit(KindSlashAssign, 2)
			return
		}

		emit(KindSlash, 1)
	case '%':
		if l.PeekAt(1) == '=' {
			emit(KindPercentAssign, 2)
			return
		}

		emit(KindPercent, 1)
	case '.':
		if l.PeekAt(1) == '.' && l.PeekAt(2) == '.' {
			emit(KindEllipsis, 3)
			return
		}

		if l.PeekAt(1) == '=' {
			emit(KindDotAssign, 2)
			return
		}

		emit(KindDot, 1)
	case '&':
		switch l.PeekAt(1) {
		case '&':
			emit(KindAmpAmp, 2)
		case '=':
			emit(KindAmpAssign, 2)
		default:
			emit(KindAmp, 1)
		}
	case '|':
		switch l.PeekAt(1) {
		case '|':
			emit(KindPipePipe, 2)
		case '=':
			emit(KindPipeAssign, 2)
		case '>':
			emit(KindPipeline, 2)
		default:
			emit(KindPipe, 1)
		}
	case '^':
		if l.PeekAt(1) == '=' {
			emit(KindCaretAssign, 2)
			return
		}

		emit(KindCaret, 1)
	case '~':
		emit(KindTilde, 1)
	case '!':
		if l.PeekAt(1) == '=' && l.PeekAt(2) == '=' {
			emit(KindBangEqEq, 3)
			return
		}

		if l.PeekAt(1) == '=' {
			emit(KindBangEq, 2)
			return
		}

		emit(KindBang, 1)
	case '@':
		emit(KindAt, 1)
	case '=':
		if l.PeekAt(1) == '=' && l.PeekAt(2) == '=' {
			emit(KindEqEqEq, 3)
			return
		}

		if l.PeekAt(1) == '=' {
			emit(KindEqEq, 2)
			return
		}

		if l.PeekAt(1) == '>' {
			emit(KindDoubleArrow, 2)
			return
		}

		emit(KindEq, 1)
	case '<':
		if l.PeekAt(1) == '=' && l.PeekAt(2) == '>' {
			emit(KindSpaceship, 3)
			return
		}

		if l.PeekAt(1) == '=' {
			emit(KindLtEq, 2)
			return
		}

		if l.PeekAt(1) == '<' {
			if l.PeekAt(2) == '=' {
				emit(KindShlAssign, 3)
				return
			}

			emit(KindShl, 2)
			return
		}

		emit(KindLt, 1)
	case '>':
		if l.PeekAt(1) == '=' {
			emit(KindGtEq, 2)
			return
		}

		if l.PeekAt(1) == '>' {
			if l.PeekAt(2) == '=' {
				emit(KindShrAssign, 3)
				return
			}

			emit(KindShr, 2)
			return
		}

		emit(KindGt, 1)
	case '?':
		if l.PeekAt(1) == '?' {
			if l.PeekAt(2) == '=' {
				emit(KindNullCoalesceAssign, 3)
				return
			}

			emit(KindNullCoalesce, 2)
			return
		}

		if l.PeekAt(1) == '-' && l.PeekAt(2) == '>' {
			emit(KindNullSafeArrow, 3)
			return
		}

		emit(KindQuestion, 1)
	case ':':
		if l.PeekAt(1) == ':' {
			emit(KindScopeRes, 2)
			return
		}

		emit(KindColon, 1)
	case '(':
		emit(KindLParen, 1)
	case ')':
		emit(KindRParen, 1)
	case '{':
		emit(KindLBrace, 1)
	case '}':
		emit(KindRBrace, 1)
	case '[':
		emit(KindLBracket, 1)
	case ']':
		emit(KindRBracket, 1)
	case ',':
		emit(KindComma, 1)
	case ';':
		emit(KindSemicolon, 1)
	case '\\':
		emit(KindBackslash, 1)
	default:
		l.Advance()
		l.emit(KindInvalid, start, l.Pos, l.Src[start.Offset:l.Pos.Offset])
		l.addError(start, fmt.Sprintf("unexpected character %q", c))
	}
}

// IsWhitespace reports whether b is PHP inter-token whitespace: the bytes the
// lexer turns into trivia and the emitter therefore never reproduces. Exported
// so a formatter measuring a raw source span can discount exactly the bytes that
// will not survive; a narrower class there charges width to a byte the output
// drops, and the next pass, measuring without it, decides differently.
func IsWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

// IsIdentStart reports whether b can start a PHP identifier: an ASCII letter,
// underscore, or any byte >= 0x80 (the lead bytes of a non-ASCII UTF-8
// identifier). It is exported so the formatter detects `$name` interpolation
// by the same rule the lexer uses.
func IsIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || b >= 0x80
}

func isIdentCont(b byte) bool {
	return IsIdentStart(b) || (b >= '0' && b <= '9')
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isHexDigit(b byte) bool {
	return isDigit(b) || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func isOctDigit(b byte) bool {
	return b >= '0' && b <= '7'
}

func isBinDigit(b byte) bool {
	return b == '0' || b == '1'
}
