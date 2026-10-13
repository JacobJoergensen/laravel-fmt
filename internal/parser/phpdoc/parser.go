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
	toks   []token
	pos    int
	failed bool
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

func (p *typeParser) advance() token {
	t := p.toks[p.pos]

	if t.kind != tokEOF {
		p.pos++
	}

	return t
}

func (p *typeParser) fail() { p.failed = true }

func (p *typeParser) parseType() Type {
	return p.parseConditional()
}

func (p *typeParser) parseConditional() Type {
	subject := p.parseUnion()

	if !p.atWord(kwIs) {
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

	if !p.at(tokPipe) {
		return first
	}

	members := []Type{first}

	for p.at(tokPipe) {
		p.advance()
		members = append(members, p.parseIntersection())
	}

	return &UnionType{Members: members}
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
	return p.at(tokAmp) && p.kindAt(1) != tokVar
}

func (p *typeParser) parsePrefix() Type {
	if p.at(tokQuestion) {
		p.advance()

		return &NullableType{Inner: p.parsePrefix()}
	}

	return p.parsePostfix()
}

func (p *typeParser) parsePostfix() Type {
	t := p.parseAtom()

	for p.at(tokLBracket) {
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
	case p.at(tokDoubleColon):
		return p.parseConstFetch(name.raw)
	case p.at(tokLAngle):
		return p.parseGeneric(&IdentType{Name: name.raw})
	case p.at(tokLBrace):
		return p.parseShape(name.raw)
	case p.at(tokLParen) && isCallableKeyword(name.raw):
		return p.parseCallable(name.raw)
	default:
		return &IdentType{Name: name.raw}
	}
}

func (p *typeParser) parseConstFetch(class []byte) Type {
	p.advance()

	var member []byte

	if p.at(tokIdent) {
		member = append(member, p.advance().raw...)
	}

	if p.at(tokStar) {
		member = append(member, p.advance().raw...)
	}

	if len(member) == 0 {
		p.fail()

		return &IdentType{Name: class}
	}

	return &ConstFetchType{Class: class, Member: member}
}

func (p *typeParser) parseCallable(keyword []byte) Type {
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

	if p.at(tokColon) {
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
