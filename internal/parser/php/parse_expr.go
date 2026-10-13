package php

func parseTernary(p *parser, cond Expr) Expr {
	q := p.advance()
	var then Expr

	if p.peekKind() != KindColon {
		then = p.parseExpression(precNone)
	}

	colon := p.expect(KindColon)
	els := p.parseExpression(precTernary - 1)

	return &TernaryExpr{Cond: cond, Question: q, Then: then, Colon: colon, Else: els}
}

// PHP does not parse `0->p`, `0::C` or `0[1]`: the lexer has taken the digits
// and nothing may follow them. Parenthesised they parse and fail only at
// runtime, which is not the formatter's business.
func rejectNumericPostfix(p *parser, operand Expr, op string) {
	switch operand.(type) {
	case *IntExpr, *FloatExpr:
		p.addError(operand.Span().Start, "cannot use "+op+" on a numeric literal")
	}
}

func parseMember(p *parser, object Expr) Expr {
	rejectNumericPostfix(p, object, "->")

	arrow := p.advance()
	prop := p.parseMemberName()

	return &MemberExpr{Object: object, Arrow: arrow, Property: prop}
}

func parseStaticMember(p *parser, class Expr) Expr {
	rejectNumericPostfix(p, class, "::")

	scope := p.advance()
	name := p.parseMemberName()

	return &StaticMemberExpr{Class: class, ScopeRes: scope, Name: name}
}

func (p *parser) parseMemberName() Expr {
	switch p.peekKind() {
	case KindVariable:
		return &VariableExpr{Token: p.advance()}
	case KindClass:
		return &IdentExpr{Token: p.advance()}
	case KindLBrace:
		lb := p.advance()
		expr := p.parseExpression(precNone)
		rb := p.expect(KindRBrace)

		return &DynamicName{LBrace: lb, Expr: expr, RBrace: rb}
	default:
		if isMemberNameToken(p.peekKind()) {
			return &IdentExpr{Token: p.advance()}
		}

		tok := p.advance()
		p.addError(tok.Span.Start, "expected member name")

		return &IdentExpr{Token: tok}
	}
}

// PHP permits most keywords after `->`/`::` and as named-argument labels.
//
// The ordinal range is load-bearing: every keyword Kind sits between
// KindAbstract and KindYieldFrom, so a non-keyword Kind declared inside that
// block silently becomes a legal member name.
func isMemberNameToken(k Kind) bool {
	if k == KindIdent {
		return true
	}

	return k >= KindAbstract && k <= KindYieldFrom
}

func parseSubscript(p *parser, array Expr) Expr {
	rejectNumericPostfix(p, array, "[")

	lb := p.advance()
	var idx Expr

	if p.peekKind() != KindRBracket {
		idx = p.parseExpression(precNone)
	}

	rb := p.expect(KindRBracket)

	return &SubscriptExpr{Array: array, LBracket: lb, Index: idx, RBracket: rb}
}

func parseArrayShort(p *parser) Expr {
	lb := p.advance()
	elements := p.parseArrayElements(KindRBracket)
	rb := p.expect(KindRBracket)

	return &ArrayExpr{Open: lb, Elements: elements, Close: rb}
}

func parseArrayLong(p *parser) Expr {
	kw := p.advance()
	lp := p.expect(KindLParen)
	elements := p.parseArrayElements(KindRParen)
	rp := p.expect(KindRParen)

	return &ArrayExpr{KeywordPrefix: kw, Open: lp, Elements: elements, Close: rp}
}

// `list(...)` is the long form of `[...]` destructuring and shares ArrayExpr,
// so it can hold a skipped element (`list(, $b) = $x`). A bare `list` with no
// `(` is just a name.
func parseListPattern(p *parser) Expr {
	if p.peekKindAt(1) != KindLParen {
		return parseIdent(p)
	}

	return parseArrayLong(p)
}

func (p *parser) parseArrayElements(end Kind) []*ArrayElement {
	return p.parseCommaList(end, nil, p.parseArrayElement)
}

func (p *parser) parseArrayElement() *ArrayElement {
	// An empty slot (a `,` with no preceding value) is a skipped element in a
	// destructuring target (`[$a, , $c] = …`); Value stays nil. The caller's
	// trailing-comma break stops a trailing `,` from producing a phantom slot.
	if p.peekKind() == KindComma {
		return &ArrayElement{}
	}

	if p.peekKind() == KindEllipsis {
		ellipsis := p.advance()
		value := p.parseExpression(precNone)

		return &ArrayElement{Spread: true, Ellipsis: ellipsis, Value: value}
	}

	byRef := false

	var amp Token

	if p.peekKind() == KindAmp {
		amp = p.advance()
		byRef = true
	}

	first := p.parseExpression(precNone)

	if p.peekKind() == KindDoubleArrow {
		if byRef {
			p.addError(amp.Span.Start, "array key cannot be taken by reference")
		}

		arrow := p.advance()
		var valueAmp Token

		if p.peekKind() == KindAmp {
			valueAmp = p.advance()
		}

		value := p.parseExpression(precNone)

		return &ArrayElement{
			Key: first, Arrow: arrow, Value: value,
			ByRef: valueAmp.Kind != KindInvalid, Amp: valueAmp,
		}
	}

	return &ArrayElement{Value: first, ByRef: byRef, Amp: amp}
}

func parseNew(p *parser) Expr {
	kw := p.advance()

	if p.isAnonClassStart() {
		return p.parseAnonClass(&kw)
	}

	class := p.parseNewClassExpr()

	var lp, rp Token
	var args []Expr

	if p.peekKind() == KindLParen {
		if p.peekKindAt(1) == KindEllipsis && p.peekKindAt(2) == KindRParen {
			p.addError(kw.Span.Start, "new Class(...) is not valid PHP; first-class callable syntax does not support constructors")
			lp = p.advance()
			p.advance()
			rp = p.advance()
		} else {
			lp = p.advance()
			args = p.parseCallArgs(KindRParen)
			rp = p.expect(KindRParen)
		}
	}

	return &NewExpr{Keyword: kw, Class: class, LParen: lp, Args: args, RParen: rp}
}

// After `new`, an attribute group or class modifier can only come before
// `class`, so either one starts an anonymous class.
func (p *parser) isAnonClassStart() bool {
	switch p.peekKind() {
	case KindClass, KindAttributeOpen:
		return true
	}

	return isClassModifier(p.peekKind())
}

func (p *parser) parseAnonClass(newKw *Token) Expr {
	var attrs []*AttributeGroup

	if p.peekKind() == KindAttributeOpen {
		attrs = p.parseAttributeGroups()
	}

	var modifiers []Modifier

	for isClassModifier(p.peekKind()) {
		modifiers = append(modifiers, p.parseModifier())
	}

	classKw := p.expect(KindClass)

	var lp, rp Token
	var args []Expr

	if p.peekKind() == KindLParen {
		lp = p.advance()
		args = p.parseCallArgs(KindRParen)
		rp = p.expect(KindRParen)
	}

	var extendsKw Token
	var parent *Name

	if p.peekKind() == KindExtends {
		extendsKw = p.advance()
		parent = p.parseName()
	}

	var implementsKw Token
	var implementsNames []*Name
	var implementsCommas []Token

	if p.peekKind() == KindImplements {
		implementsKw = p.advance()
		implementsNames, implementsCommas = p.parseNameList()
	}

	lb := p.expect(KindLBrace)
	members := p.parseClassMembers()
	rb := p.expect(KindRBrace)

	return &AnonClassExpr{
		New:          *newKw,
		Attributes:   attrs,
		Modifiers:    modifiers,
		ClassKeyword: classKw,
		LParen:       lp, Args: args, RParen: rp,
		Extends: extendsKw, Parent: parent,
		Implements:       implementsKw,
		ImplementsNames:  implementsNames,
		ImplementsCommas: implementsCommas,
		LBrace:           lb, Members: members, RBrace: rb,
	}
}

func (p *parser) parseNewClassExpr() Expr {
	var base Expr

	switch p.peekKind() {
	case KindIdent, KindSelf, KindStatic, KindNamespace:
		id := &IdentExpr{Token: p.advance()}
		extendQualifiedIdent(p, id)
		base = id
	case KindBackslash:
		base = parseQualifiedIdent(p)
	case KindVariable:
		base = &VariableExpr{Token: p.advance()}
	case KindLParen:
		// new (expr) / instanceof (expr): a runtime class value. A `(` here
		// is never a cast, and a trailing `(` is the constructor list,
		// handled by the caller.
		lp := p.advance()
		inner := p.parseExpression(precNone)
		rp := p.expect(KindRParen)
		base = &ParenExpr{LParen: lp, Expr: inner, RParen: rp}
	default:
		tok := p.advance()
		p.addError(tok.Span.Start, "expected class name after new")

		return &IdentExpr{Token: tok}
	}

	// The new_variable chain (::$p, [i], ->p) is part of the class reference;
	// the first ( is the constructor list, not a call. Consuming it here keeps
	// new static::$map[K]($x) one new, not (new static)::$map[K]($x).
	for {
		switch p.peekKind() {
		case KindLBracket:
			base = parseSubscript(p, base)
		case KindArrow:
			base = parseMember(p, base)
		case KindScopeRes:
			if p.peekKindAt(1) != KindVariable {
				return base
			}

			base = parseStaticMember(p, base)
		default:
			return base
		}
	}
}

// parseClone handles `clone expr` and the PHP 8.5 call form `clone(arg, mods)`.
// A `(` right after the keyword selects the call form even for `clone ($x)`,
// which is semantically identical. `clone(...)` is rejected: clone is an
// operator, not a callable.
func parseClone(p *parser) Expr {
	kw := p.advance()

	if p.peekKind() == KindLParen {
		lp := p.advance()

		if p.peekKind() == KindEllipsis && p.peekKindAt(1) == KindRParen {
			p.addError(kw.Span.Start, "clone(...) is not valid PHP; first-class callable syntax does not support clone")
			p.advance()
			rp := p.advance()

			return &CloneExpr{Keyword: kw, LParen: lp, RParen: rp}
		}

		args := p.parseCallArgs(KindRParen)
		rp := p.expect(KindRParen)

		return &CloneExpr{Keyword: kw, LParen: lp, Args: args, RParen: rp}
	}

	expr := p.parseExpression(precNew)

	return &CloneExpr{Keyword: kw, Expr: expr}
}

func parseInstanceof(p *parser, left Expr) Expr {
	kw := p.advance()
	class := p.parseNewClassExpr()

	return &InstanceofExpr{Left: left, Keyword: kw, Class: class}
}

func parsePrint(p *parser) Expr {
	kw := p.advance()
	expr := p.parseExpression(precAssign)

	return &PrintExpr{Keyword: kw, Expr: expr}
}

func parseIncludeExpr(p *parser) Expr {
	kw := p.advance()
	expr := p.parseExpression(precAssign)

	return &IncludeExpr{Keyword: kw, Expr: expr}
}

func parseThrowExpr(p *parser) Expr {
	kw := p.advance()
	expr := p.parseExpression(precAssign)

	return &ThrowExpr{Keyword: kw, Expr: expr}
}

func parseYield(p *parser) Expr {
	kw := p.advance()

	if rules[p.peekKind()].prefix == nil {
		return &YieldExpr{Keyword: kw}
	}

	first := p.parseExpression(precAssign)

	if p.peekKind() == KindDoubleArrow {
		arrow := p.advance()
		value := p.parseExpression(precAssign)

		return &YieldExpr{Keyword: kw, Key: first, Arrow: arrow, Value: value}
	}

	return &YieldExpr{Keyword: kw, Value: first}
}

func parseYieldFrom(p *parser) Expr {
	kw := p.advance()
	expr := p.parseExpression(precAssign)

	return &YieldFromExpr{Keyword: kw, Expr: expr}
}

func parseMatch(p *parser) Expr {
	kw := p.advance()
	lp := p.expect(KindLParen)
	subject := p.parseExpression(precNone)
	rp := p.expect(KindRParen)
	lb := p.expect(KindLBrace)

	arms := p.parseCommaList(KindRBrace, nil, p.parseMatchArm)

	rb := p.expect(KindRBrace)

	return &MatchExpr{
		Keyword: kw, LParen: lp, Subject: subject, RParen: rp,
		LBrace: lb, Arms: arms, RBrace: rb,
	}
}

func (p *parser) parseMatchArm() *MatchArm {
	if p.peekKind() == KindDefault {
		def := p.advance()

		if p.peekKind() == KindComma && p.peekKindAt(1) == KindDoubleArrow {
			p.advance()
		}

		arrow := p.expect(KindDoubleArrow)
		result := p.parseExpression(precNone)

		return &MatchArm{
			IsDefault: true, DefaultKeyword: def,
			Arrow: arrow, Result: result,
		}
	}

	conds := []Expr{p.parseExpression(precNone)}

	for p.peekKind() == KindComma && p.peekKindAt(1) != KindDoubleArrow {
		p.advance()
		conds = append(conds, p.parseExpression(precNone))
	}

	if p.peekKind() == KindComma && p.peekKindAt(1) == KindDoubleArrow {
		p.advance()
	}

	arrow := p.expect(KindDoubleArrow)
	result := p.parseExpression(precNone)

	return &MatchArm{Conds: conds, Arrow: arrow, Result: result}
}

func parseStaticPrefix(p *parser) Expr {
	switch p.peekKindAt(1) {
	case KindFunction:
		static := p.advance()

		return p.finishClosure(&static)
	case KindFn:
		static := p.advance()

		return p.finishArrowFn(&static)
	}

	return parseIdent(p)
}

// PHP allows `#[…]` groups before a closure or an arrow function, the only
// expressions that take them.
func parseAttributedClosure(p *parser) Expr {
	groups := p.parseAttributeGroups()

	var fn Expr

	switch {
	case p.peekKind() == KindFunction:
		fn = p.finishClosure(nil)
	case p.peekKind() == KindFn:
		fn = p.finishArrowFn(nil)
	case p.peekKind() == KindStatic && (p.peekKindAt(1) == KindFunction || p.peekKindAt(1) == KindFn):
		fn = parseStaticPrefix(p)
	default:
		tok := p.peek()
		p.addError(tok.Span.Start, "expected a closure or arrow function after attributes")

		return &IdentExpr{Token: tok}
	}

	switch n := fn.(type) {
	case *ClosureExpr:
		n.Attributes = groups
	case *ArrowFnExpr:
		n.Attributes = groups
	}

	return fn
}

func parseClosure(p *parser) Expr {
	return p.finishClosure(nil)
}

func parseArrowFn(p *parser) Expr {
	return p.finishArrowFn(nil)
}

func (p *parser) finishClosure(static *Token) Expr {
	kw := p.expect(KindFunction)
	var byRef Token

	if p.peekKind() == KindAmp {
		byRef = p.advance()
	}

	lp := p.expect(KindLParen)
	params, commas := p.parseParamList()
	rp := p.expect(KindRParen)

	var useKw, useLp, useRp Token
	var uses []*ClosureUse
	var useCommas []Token

	if p.peekKind() == KindUse {
		useKw = p.advance()
		useLp = p.expect(KindLParen)
		uses, useCommas = p.parseClosureUseList()
		useRp = p.expect(KindRParen)
	}

	var returnColon Token
	var returnType TypeHint

	if p.peekKind() == KindColon {
		returnColon = p.advance()
		returnType = p.parseType()
	}

	body := p.parseBlockStmt()

	var staticTok Token
	if static != nil {
		staticTok = *static
	}

	return &ClosureExpr{
		Static: staticTok, Keyword: kw, ByRef: byRef,
		LParen: lp, Params: params, ParamCommas: commas, RParen: rp,
		UseKeyword: useKw, UseLParen: useLp,
		Uses: uses, UseCommas: useCommas, UseRParen: useRp,
		ReturnColon: returnColon, ReturnType: returnType,
		Body: body,
	}
}

func (p *parser) finishArrowFn(static *Token) Expr {
	kw := p.expect(KindFn)
	var byRef Token

	if p.peekKind() == KindAmp {
		byRef = p.advance()
	}

	lp := p.expect(KindLParen)
	params, commas := p.parseParamList()
	rp := p.expect(KindRParen)

	var returnColon Token
	var returnType TypeHint

	if p.peekKind() == KindColon {
		returnColon = p.advance()
		returnType = p.parseType()
	}

	arrow := p.expect(KindDoubleArrow)
	body := p.parseExpression(precNone)

	var staticTok Token
	if static != nil {
		staticTok = *static
	}

	return &ArrowFnExpr{
		Static: staticTok, Keyword: kw, ByRef: byRef,
		LParen: lp, Params: params, ParamCommas: commas, RParen: rp,
		ReturnColon: returnColon, ReturnType: returnType,
		Arrow: arrow, Body: body,
	}
}

func (p *parser) parseClosureUseList() ([]*ClosureUse, []Token) {
	var commas []Token
	uses := p.parseCommaList(KindRParen, &commas, p.parseClosureUse)

	return uses, commas
}

func (p *parser) parseClosureUse() *ClosureUse {
	var byRef Token

	if p.peekKind() == KindAmp {
		byRef = p.advance()
	}

	variable := p.expect(KindVariable)

	return &ClosureUse{ByRef: byRef, Var: variable}
}
