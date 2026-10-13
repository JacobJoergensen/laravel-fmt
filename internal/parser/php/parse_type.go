package php

func (p *parser) parseType() TypeHint {
	if p.peekKind() == KindQuestion {
		q := p.advance()
		inner := p.parseSingleType()
		p.rejectParenType(inner)

		return &NullableType{Question: q, Type: inner}
	}

	first := p.parseSingleType()

	if p.peekKind() == KindPipe && isTypeStart(p.peekKindAt(1)) {
		types := []TypeHint{first}
		var pipes []Token

		for p.peekKind() == KindPipe && isTypeStart(p.peekKindAt(1)) {
			pipes = append(pipes, p.advance())
			types = append(types, p.parseSingleType())
		}

		return &UnionType{Types: types, Pipes: pipes}
	}

	p.rejectParenType(first)

	if p.peekKind() == KindAmp && isTypeStart(p.peekKindAt(1)) {
		types := []TypeHint{first}
		var amps []Token

		for p.peekKind() == KindAmp && isTypeStart(p.peekKindAt(1)) {
			amps = append(amps, p.advance())
			member := p.parseSingleType()
			p.rejectParenType(member)
			types = append(types, member)
		}

		return &IntersectionType{Types: types, Amps: amps}
	}

	return first
}

// PHP's grammar allows a parenthesized type only as a member of a union.
func (p *parser) rejectParenType(t TypeHint) {
	if paren, ok := t.(*ParenType); ok {
		p.addError(paren.LParen.Span.Start, "parenthesized type outside a union")
	}
}

func (p *parser) parseSingleType() TypeHint {
	switch p.peekKind() {
	case KindArray, KindCallable, KindSelf, KindStatic, KindTrue, KindFalse, KindNull:
		return &KeywordType{Token: p.advance()}
	case KindIdent, KindBackslash, KindNamespace:
		return &NamedType{Name: p.parseName()}
	case KindLParen:
		return p.parseParenType()
	default:
		tok := p.advance()
		p.addError(tok.Span.Start, "expected type")

		return &KeywordType{Token: tok}
	}
}

func (p *parser) parseParenType() TypeHint {
	lp := p.advance()
	inner := p.parseType()

	if _, ok := inner.(*IntersectionType); !ok {
		p.addError(lp.Span.Start, "expected an intersection type in parentheses")
	}

	rp := p.expect(KindRParen)

	return &ParenType{LParen: lp, Type: inner, RParen: rp}
}

func isTypeStart(k Kind) bool {
	switch k {
	case KindQuestion, KindIdent, KindBackslash, KindNamespace,
		KindArray, KindCallable, KindSelf, KindStatic,
		KindTrue, KindFalse, KindNull, KindLParen:
		return true
	}

	return false
}
