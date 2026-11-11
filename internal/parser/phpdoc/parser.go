package phpdoc

import "bytes"

var (
	kwCovariant     = []byte("covariant")
	kwContravariant = []byte("contravariant")
	kwIs            = []byte("is")
	kwNot           = []byte("not")
)

var callableKeywords = [][]byte{
	[]byte("callable"),
	[]byte("closure"),
	[]byte("pure-callable"),
	[]byte("pure-closure"),
	[]byte("static-closure"),
}

type typeParser struct {
	toks       []token
	pos        int
	depth      int
	failed     bool
	psalmSplit bool
}

func (p *typeParser) kindAt(n int) tokKind {
	i := p.pos + n

	if i >= len(p.toks) {
		return tokEOF
	}

	return p.toks[i].kind
}

func (p *typeParser) at(k tokKind) bool { return p.kindAt(0) == k }

func (p *typeParser) atWord(w []byte) bool {
	return p.at(tokIdent) && bytes.Equal(p.toks[p.pos].raw, w)
}

func (p *typeParser) spaced() bool { return p.toks[p.pos].gap > 0 }

// Outside every bracket Psalm ends a type at a space that PHPStan reads past,
// so there a spaced token ends the type and both tools keep reading the text
// as written. Inside brackets Psalm ignores spaces and PHPStan's rules apply.
func (p *typeParser) psalmTopLevel() bool { return p.psalmSplit && p.depth == 0 }

func (p *typeParser) joins() bool { return !p.psalmTopLevel() || !p.spaced() }

// At the top level Psalm reads past one space on each side of `|` or `&`, as
// in `A | B` or `A| B`, but ends the type at `A |B` or a wider gap.
func (p *typeParser) joinsOperator() bool {
	if !p.psalmTopLevel() {
		return true
	}

	before, after := p.toks[p.pos].gap, p.toks[p.pos+1].gap

	if before == 0 {
		return after <= 1
	}

	return before == 1 && after == 1
}

func (p *typeParser) joinsReturn() bool {
	return !p.psalmTopLevel() || (p.toks[p.pos].gap <= 1 && p.toks[p.pos+1].gap <= 1)
}

func (p *typeParser) advance() token {
	t := p.toks[p.pos]

	if t.kind == tokEOF {
		return t
	}

	p.pos++

	switch t.kind {
	case tokLParen, tokLAngle, tokLBrace, tokLBracket:
		p.depth++
	case tokRParen, tokRAngle, tokRBrace, tokRBracket:
		p.depth--
	default:
	}

	return t
}

func (p *typeParser) fail() { p.failed = true }

func (p *typeParser) parseType() Type {
	return p.parseConditional()
}

func (p *typeParser) parseConditional() Type {
	subject := p.parseUnion()

	// PHPStan reads a conditional only inside brackets; at the top level
	// `T is int ? A : B` is the type T and a description.
	if !p.atWord(kwIs) || p.depth == 0 {
		return subject
	}

	p.advance()
	negated := p.atWord(kwNot)

	if negated {
		p.advance()
	}

	target := p.parseUnion()

	if !p.at(tokQuestion) {
		p.fail()

		return subject
	}

	p.advance()
	ifType := p.parseType()

	if !p.at(tokColon) {
		p.fail()

		return subject
	}

	p.advance()

	return &ConditionalType{Subject: subject, Negated: negated, Target: target, If: ifType, Else: p.parseType()}
}

func (p *typeParser) parseUnion() Type {
	first := p.parseIntersection()

	if !p.atUnion() {
		return first
	}

	members := []Type{first}

	for p.atUnion() {
		p.advance()
		members = append(members, p.parseIntersection())
	}

	return &UnionType{Members: members}
}

func (p *typeParser) atUnion() bool {
	return p.at(tokPipe) && p.joinsOperator()
}

func (p *typeParser) parseIntersection() Type {
	first := p.parsePrefix()

	if !p.atIntersection() {
		return first
	}

	members := []Type{first}

	for p.atIntersection() {
		p.advance()
		members = append(members, p.parsePrefix())
	}

	return &IntersectionType{Members: members}
}

// An `&` before a $variable is a by-reference marker in a callable parameter,
// not an intersection.
func (p *typeParser) atIntersection() bool {
	return p.at(tokAmp) && p.kindAt(1) != tokVar && p.joinsOperator()
}

func (p *typeParser) parsePrefix() Type {
	if p.at(tokQuestion) {
		p.advance()

		if !p.joins() {
			p.fail()
		}

		return &NullableType{Inner: p.parsePrefix()}
	}

	return p.parsePostfix()
}

func (p *typeParser) parsePostfix() Type {
	t := p.parseAtom()

	// After a space PHPStan attaches only `[]`; `string [optional]` is a type
	// and a description.
	for p.at(tokLBracket) && (!p.spaced() || (!p.psalmTopLevel() && p.kindAt(1) == tokRBracket)) {
		p.advance()

		if p.at(tokRBracket) {
			p.advance()
			t = &ArraySuffixType{Inner: t}

			continue
		}

		off := p.parseType()

		if !p.at(tokRBracket) {
			p.fail()

			return t
		}

		p.advance()
		t = &OffsetAccessType{Inner: t, Offset: off}
	}

	return t
}

func (p *typeParser) parseAtom() Type {
	switch p.kindAt(0) {
	case tokLParen:
		p.advance()
		inner := p.parseType()

		if !p.at(tokRParen) {
			p.fail()

			return inner
		}

		p.advance()

		return &ParenType{Inner: inner}
	case tokIdent, tokVar:
		return p.parseNamed()
	case tokInt, tokFloat, tokString:
		return &LiteralType{Raw: p.advance().raw}
	default:
		p.fail()

		if p.at(tokEOF) {
			return &Invalid{}
		}

		return &Invalid{Raw: p.advance().raw}
	}
}

func (p *typeParser) parseNamed() Type {
	name := p.advance()

	switch {
	case p.at(tokDoubleColon) && p.joins():
		return p.parseConstFetch(name.raw)
	case p.at(tokLAngle) && p.joins():
		return p.parseGeneric(&IdentType{Name: name.raw})
	case p.at(tokLBrace) && !p.spaced():
		return p.parseShape(name.raw)
	case p.at(tokLParen) && isCallableKeyword(name.raw) && p.joins():
		return p.tryParseCallable(name.raw)
	default:
		return &IdentType{Name: name.raw}
	}
}

// Where Psalm does not end the type, as inside `array<Closure (int): void>`,
// PHPStan takes parentheses after a space as a signature only when a return
// type follows.
func (p *typeParser) tryParseCallable(keyword []byte) Type {
	if !p.spaced() {
		return p.parseCallable(keyword)
	}

	// The rollback restores the flag as found, so a failure earlier in the type
	// still fails it.
	start, depth, failed := p.pos, p.depth, p.failed

	c := p.parseCallable(keyword)
	if !p.failed && c.Return != nil {
		return c
	}

	p.pos, p.depth, p.failed = start, depth, failed

	return &IdentType{Name: keyword}
}

func (p *typeParser) parseConstFetch(class []byte) Type {
	p.advance()

	var member []byte

	if p.at(tokIdent) && p.joins() {
		member = append(member, p.advance().raw...)
	}

	if p.at(tokStar) && p.joins() {
		member = append(member, p.advance().raw...)
	}

	if len(member) == 0 {
		p.fail()

		return &IdentType{Name: class}
	}

	return &ConstFetchType{Class: class, Member: member}
}

func (p *typeParser) parseCallable(keyword []byte) *CallableType {
	p.advance()
	c := &CallableType{Keyword: keyword}

	for !p.at(tokRParen) {
		c.Params = append(c.Params, p.parseCallableParam())

		if p.failed {
			return c
		}

		if !p.at(tokComma) {
			break
		}

		p.advance()
	}

	if !p.at(tokRParen) {
		p.fail()

		return c
	}

	p.advance()

	if p.at(tokColon) && p.joinsReturn() {
		p.advance()
		c.Return = p.parseType()
	}

	return c
}

func (p *typeParser) parseCallableParam() CallableParam {
	param := CallableParam{Type: p.parseType()}

	if p.at(tokAmp) {
		p.advance()
		param.ByRef = true
	}

	if p.at(tokEllipsis) {
		p.advance()
		param.Variadic = true
	}

	if p.at(tokVar) {
		param.Name = p.advance().raw
	}

	if p.at(tokEquals) {
		p.advance()
		param.Optional = true
	}

	return param
}

func isCallableKeyword(name []byte) bool {
	if len(name) > 0 && name[0] == '\\' {
		name = name[1:]
	}

	for _, k := range callableKeywords {
		if bytes.EqualFold(name, k) {
			return true
		}
	}

	return false
}

func (p *typeParser) parseGeneric(base Type) Type {
	p.advance()
	g := &GenericType{Base: base}

	for {
		var variance []byte

		if p.at(tokIdent) {
			w := p.toks[p.pos].raw

			if (bytes.Equal(w, kwCovariant) || bytes.Equal(w, kwContravariant)) && canStartType(p.kindAt(1)) {
				variance = w
				p.advance()
			}
		}

		g.Args = append(g.Args, GenericArg{Variance: variance, Type: p.parseType()})

		if p.failed {
			return g
		}

		if p.at(tokComma) {
			p.advance()

			if p.at(tokRAngle) {
				break
			}

			continue
		}

		break
	}

	if !p.at(tokRAngle) {
		p.fail()

		return g
	}

	p.advance()

	return g
}

func (p *typeParser) parseShape(keyword []byte) Type {
	p.advance()
	sh := &ShapeType{Keyword: keyword, Broken: p.toks[p.pos].newlineBefore}

	if p.at(tokRBrace) {
		p.advance()

		return sh
	}

	for {
		if p.at(tokEllipsis) {
			p.advance()
			sh.HasRest = true

			// PHPStan allows a trailing comma after the rest marker, and a broken
			// shape prints one.
			if p.at(tokComma) {
				p.advance()
			}

			break
		}

		sh.Items = append(sh.Items, p.parseShapeItem())

		if p.failed {
			return sh
		}

		if p.at(tokComma) {
			p.advance()

			if p.at(tokRBrace) || p.at(tokEOF) {
				break
			}

			continue
		}

		break
	}

	if !p.at(tokRBrace) {
		p.fail()

		return sh
	}

	p.advance()

	return sh
}

func (p *typeParser) parseShapeItem() ShapeItem {
	if !p.isShapeKeyAhead() {
		return ShapeItem{Value: p.parseType()}
	}

	key := p.advance()
	optional := false

	if p.at(tokQuestion) {
		p.advance()
		optional = true
	}

	p.advance() // colon, guaranteed by isShapeKeyAhead

	return ShapeItem{Key: key.raw, Optional: optional, Value: p.parseType()}
}

func (p *typeParser) isShapeKeyAhead() bool {
	switch p.kindAt(0) {
	case tokIdent, tokInt, tokString:
	default:
		return false
	}

	if p.kindAt(1) == tokColon {
		return true
	}

	return p.kindAt(1) == tokQuestion && p.kindAt(2) == tokColon
}

func canStartType(k tokKind) bool {
	switch k {
	case tokIdent, tokVar, tokInt, tokFloat, tokString, tokQuestion, tokLParen:
		return true
	default:
		return false
	}
}
