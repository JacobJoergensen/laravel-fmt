package php

import "slices"

func (p *parser) parseIfStmt() Stmt {
	kw := p.advance()
	lp := p.expect(KindLParen)
	cond := p.parseExpression(precNone)
	rp := p.expect(KindRParen)

	if p.peekKind() == KindColon {
		return p.parseAltIfStmt(&kw, &lp, cond, &rp)
	}

	then := p.parseStmt()

	var elseIfs []*ElseIfClause

	for p.peekKind() == KindElseIf {
		elseIfs = append(elseIfs, p.parseElseIfClause())
	}

	var els *ElseClause

	if p.peekKind() == KindElse {
		elsKw := p.advance()
		elsBody := p.parseStmt()
		els = &ElseClause{Keyword: elsKw, Body: elsBody}
	}

	return &IfStmt{
		Keyword: kw, LParen: lp, Cond: cond, RParen: rp,
		Then: then, ElseIfs: elseIfs, Else: els,
	}
}

func (p *parser) parseAltIfStmt(kw, lp *Token, cond Expr, rp *Token) Stmt {
	colon := p.advance()
	body := p.parseStmtListUntil(KindElseIf, KindElse, KindEndIf)

	var elseIfs []*ElseIfClause

	for p.peekKind() == KindElseIf {
		elseIfs = append(elseIfs, p.parseAltElseIfClause())
	}

	var els *ElseClause

	if p.peekKind() == KindElse {
		els = p.parseAltElseClause()
	}

	endKw := p.expect(KindEndIf)
	endSemi := p.expectVerbatimStmtTerminator()

	return &IfStmt{
		Keyword: *kw, LParen: *lp, Cond: cond, RParen: *rp,
		Colon: colon, AltBody: body,
		ElseIfs: elseIfs, Else: els,
		EndKeyword: endKw, EndSemi: endSemi,
	}
}

func (p *parser) parseElseIfClause() *ElseIfClause {
	kw := p.advance()
	lp := p.expect(KindLParen)
	cond := p.parseExpression(precNone)
	rp := p.expect(KindRParen)
	body := p.parseStmt()

	return &ElseIfClause{Keyword: kw, LParen: lp, Cond: cond, RParen: rp, Body: body}
}

func (p *parser) parseAltElseIfClause() *ElseIfClause {
	kw := p.advance()
	lp := p.expect(KindLParen)
	cond := p.parseExpression(precNone)
	rp := p.expect(KindRParen)
	colon := p.expect(KindColon)
	body := p.parseStmtListUntil(KindElseIf, KindElse, KindEndIf)

	return &ElseIfClause{
		Keyword: kw, LParen: lp, Cond: cond, RParen: rp,
		Colon: colon, AltBody: body,
	}
}

func (p *parser) parseAltElseClause() *ElseClause {
	kw := p.advance()
	colon := p.expect(KindColon)
	body := p.parseStmtListUntil(KindEndIf)

	return &ElseClause{Keyword: kw, Colon: colon, AltBody: body}
}

// Statements go through parseStmtOrTag, so a `?> html <?php` run inside an
// alternative-syntax body is accepted.
func (p *parser) parseStmtListUntil(terms ...Kind) []Stmt {
	var stmts []Stmt

	for {
		k := p.peekKind()

		if k == KindEOF || slices.Contains(terms, k) {
			return stmts
		}

		start := p.pos
		stmts = append(stmts, p.parseStmtOrTag())
		p.guardProgress(start)
	}
}

func (p *parser) parseWhileStmt() Stmt {
	kw := p.advance()
	lp := p.expect(KindLParen)
	cond := p.parseExpression(precNone)
	rp := p.expect(KindRParen)

	if p.peekKind() == KindColon {
		colon := p.advance()
		body := p.parseStmtListUntil(KindEndWhile)
		endKw := p.expect(KindEndWhile)
		endSemi := p.expectVerbatimStmtTerminator()

		return &WhileStmt{
			Keyword: kw, LParen: lp, Cond: cond, RParen: rp,
			Colon: colon, AltBody: body,
			EndKeyword: endKw, EndSemi: endSemi,
		}
	}

	body := p.parseStmt()

	return &WhileStmt{Keyword: kw, LParen: lp, Cond: cond, RParen: rp, Body: body}
}

func (p *parser) parseDoWhileStmt() Stmt {
	doKw := p.advance()
	body := p.parseStmt()
	whileKw := p.expect(KindWhile)
	lp := p.expect(KindLParen)
	cond := p.parseExpression(precNone)
	rp := p.expect(KindRParen)
	semi := p.expectStmtTerminator()

	return &DoWhileStmt{
		DoKeyword: doKw, Body: body,
		WhileKeyword: whileKw, LParen: lp, Cond: cond, RParen: rp,
		Semicolon: semi,
	}
}

func (p *parser) parseForStmt() Stmt {
	kw := p.advance()
	lp := p.expect(KindLParen)
	init := p.parseExprList(KindSemicolon)
	semi1 := p.expect(KindSemicolon)
	cond := p.parseExprList(KindSemicolon)
	semi2 := p.expect(KindSemicolon)
	post := p.parseExprList(KindRParen)
	rp := p.expect(KindRParen)

	if p.peekKind() == KindColon {
		colon := p.advance()
		body := p.parseStmtListUntil(KindEndFor)
		endKw := p.expect(KindEndFor)
		endSemi := p.expectVerbatimStmtTerminator()

		return &ForStmt{
			Keyword: kw, LParen: lp,
			Init: init, Semi1: semi1,
			Cond: cond, Semi2: semi2,
			Post: post, RParen: rp,
			Colon: colon, AltBody: body,
			EndKeyword: endKw, EndSemi: endSemi,
		}
	}

	body := p.parseStmt()

	return &ForStmt{
		Keyword: kw, LParen: lp,
		Init: init, Semi1: semi1,
		Cond: cond, Semi2: semi2,
		Post: post, RParen: rp,
		Body: body,
	}
}

func (p *parser) parseExprList(end Kind) []Expr {
	if p.peekKind() == end {
		return nil
	}

	exprs := []Expr{p.parseExpression(precNone)}

	for p.peekKind() == KindComma {
		p.advance()
		exprs = append(exprs, p.parseExpression(precNone))
	}

	return exprs
}

func (p *parser) parseForEachStmt() Stmt {
	kw := p.advance()
	lp := p.expect(KindLParen)
	expr := p.parseExpression(precNone)
	as := p.expect(KindAs)
	byRef := false

	if p.peekKind() == KindAmp {
		p.advance()
		byRef = true
	}

	first := p.parseExpression(precNone)

	var key Expr
	var arrow Token

	value := first

	if p.peekKind() == KindDoubleArrow {
		arrow = p.advance()
		key = first

		if byRef {
			p.addError(first.Span().Start, "foreach key cannot be taken by reference")
		}

		byRef = false

		if p.peekKind() == KindAmp {
			p.advance()
			byRef = true
		}

		value = p.parseExpression(precNone)
	}

	rp := p.expect(KindRParen)

	if p.peekKind() == KindColon {
		colon := p.advance()
		body := p.parseStmtListUntil(KindEndForEach)
		endKw := p.expect(KindEndForEach)
		endSemi := p.expectVerbatimStmtTerminator()

		return &ForEachStmt{
			Keyword: kw, LParen: lp,
			Expr: expr, As: as,
			Key: key, Arrow: arrow,
			ByRef: byRef, Value: value,
			RParen: rp,
			Colon:  colon, AltBody: body,
			EndKeyword: endKw, EndSemi: endSemi,
		}
	}

	body := p.parseStmt()

	return &ForEachStmt{
		Keyword: kw, LParen: lp,
		Expr: expr, As: as,
		Key: key, Arrow: arrow,
		ByRef: byRef, Value: value,
		RParen: rp, Body: body,
	}
}

func (p *parser) parseSwitchStmt() Stmt {
	kw := p.advance()
	lp := p.expect(KindLParen)
	expr := p.parseExpression(precNone)
	rp := p.expect(KindRParen)

	if p.peekKind() == KindColon {
		colon := p.advance()

		var cases []*SwitchCase

		for p.peekKind() != KindEndSwitch && p.peekKind() != KindEOF {
			start := p.pos
			cases = append(cases, p.parseSwitchCase(KindEndSwitch))
			p.guardProgress(start)
		}

		endKw := p.expect(KindEndSwitch)
		endSemi := p.expectVerbatimStmtTerminator()

		return &SwitchStmt{
			Keyword: kw, LParen: lp, Expr: expr, RParen: rp,
			Colon: colon, Cases: cases,
			EndKeyword: endKw, EndSemi: endSemi,
		}
	}

	lb := p.expect(KindLBrace)

	var cases []*SwitchCase

	for p.peekKind() != KindRBrace && p.peekKind() != KindEOF {
		start := p.pos
		cases = append(cases, p.parseSwitchCase(KindRBrace))
		p.guardProgress(start)
	}

	rb := p.expect(KindRBrace)

	return &SwitchStmt{
		Keyword: kw, LParen: lp, Expr: expr, RParen: rp,
		LBrace: lb, Cases: cases, RBrace: rb,
	}
}

func (p *parser) parseSwitchCase(terminator Kind) *SwitchCase {
	c := &SwitchCase{}

	switch p.peekKind() {
	case KindCase:
		c.Keyword = p.advance()
		c.Expr = p.parseExpression(precNone)
	case KindDefault:
		c.Keyword = p.advance()
		c.IsDefault = true
	default:
		tok := p.advance()
		p.addError(tok.Span.Start, "expected 'case' or 'default'")
		c.Keyword = tok
	}

	// PHP permits either ':' or ';' as the case terminator.
	switch p.peekKind() {
	case KindColon, KindSemicolon:
		c.Colon = p.advance()
	default:
		c.Colon = p.expect(KindColon)
	}

	for !isSwitchCaseBoundary(p.peekKind(), terminator) {
		c.Stmts = append(c.Stmts, p.parseStmtOrTag())
	}

	return c
}

func isSwitchCaseBoundary(k, terminator Kind) bool {
	return k == KindCase || k == KindDefault || k == terminator || k == KindEOF
}

func (p *parser) parseTryStmt() Stmt {
	kw := p.advance()
	body := p.parseBlockStmt()

	var catches []*CatchClause

	for p.peekKind() == KindCatch {
		catches = append(catches, p.parseCatchClause())
	}

	var finally *FinallyClause

	if p.peekKind() == KindFinally {
		finKw := p.advance()
		finBody := p.parseBlockStmt()
		finally = &FinallyClause{Keyword: finKw, Body: finBody}
	}

	return &TryStmt{Keyword: kw, Body: body, Catches: catches, Finally: finally}
}

func (p *parser) parseCatchClause() *CatchClause {
	kw := p.advance()
	lp := p.expect(KindLParen)
	types := []Expr{p.parseCatchType()}

	var pipes []Token

	for p.peekKind() == KindPipe {
		pipes = append(pipes, p.advance())
		types = append(types, p.parseCatchType())
	}

	var variable *VariableExpr

	if p.peekKind() == KindVariable {
		variable = &VariableExpr{Token: p.advance()}
	}

	rp := p.expect(KindRParen)
	body := p.parseBlockStmt()

	return &CatchClause{
		Keyword: kw, LParen: lp,
		Types: types, Pipes: pipes,
		Var: variable, RParen: rp,
		Body: body,
	}
}

func (p *parser) parseCatchType() Expr {
	switch p.peekKind() {
	case KindIdent, KindSelf, KindStatic:
		expr := &IdentExpr{Token: p.advance()}
		extendQualifiedIdent(p, expr)

		return expr
	case KindBackslash:
		return parseQualifiedIdent(p)
	default:
		tok := p.advance()
		p.addError(tok.Span.Start, "expected exception type")

		return &IdentExpr{Token: tok}
	}
}

func (p *parser) parseBreakStmt() Stmt {
	kw := p.advance()
	var level Expr

	if !p.atStmtTerminator() {
		level = p.parseExpression(precNone)
	}

	semi := p.expectStmtTerminator()

	return &BreakStmt{Keyword: kw, Level: level, Semicolon: semi}
}

func (p *parser) parseContinueStmt() Stmt {
	kw := p.advance()
	var level Expr

	if !p.atStmtTerminator() {
		level = p.parseExpression(precNone)
	}

	semi := p.expectStmtTerminator()

	return &ContinueStmt{Keyword: kw, Level: level, Semicolon: semi}
}

func (p *parser) parseGotoStmt() Stmt {
	kw := p.advance()
	label := p.expect(KindIdent)
	semi := p.expectStmtTerminator()

	return &GotoStmt{Keyword: kw, Label: label, Semicolon: semi}
}

// parseLabelStmt consumes two tokens unchecked; the caller confirms the
// identifier-colon shape first.
func (p *parser) parseLabelStmt() Stmt {
	name := p.advance()
	colon := p.advance()

	return &LabelStmt{Name: name, Colon: colon}
}

func (p *parser) parseConstStmt() Stmt {
	kw := p.advance()
	items := []*ConstItem{p.parseConstItem(false)}

	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())
		items = append(items, p.parseConstItem(false))
	}

	semi := p.expectStmtTerminator()

	return &ConstStmt{Keyword: kw, Items: items, Commas: commas, Semicolon: semi}
}

func (p *parser) parseGlobalStmt() Stmt {
	kw := p.advance()
	vars := []Token{p.expect(KindVariable)}

	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())
		vars = append(vars, p.expect(KindVariable))
	}

	semi := p.expectStmtTerminator()

	return &GlobalStmt{Keyword: kw, Vars: vars, Commas: commas, Semicolon: semi}
}

// The caller has checked that a variable follows `static`, which rules out
// static closures and `static::`.
func (p *parser) parseStaticVarStmt() Stmt {
	kw := p.advance()
	items := []*StaticVarItem{p.parseStaticVarItem()}

	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())
		items = append(items, p.parseStaticVarItem())
	}

	semi := p.expectStmtTerminator()

	return &StaticVarStmt{Keyword: kw, Vars: items, Commas: commas, Semicolon: semi}
}

func (p *parser) parseStaticVarItem() *StaticVarItem {
	variable := p.expect(KindVariable)

	var eq Token
	var def Expr

	if p.peekKind() == KindEq {
		eq = p.advance()
		def = p.parseExpression(precNone)
	}

	return &StaticVarItem{Var: variable, Eq: eq, Default: def}
}

func (p *parser) parseUnsetStmt() Stmt {
	kw := p.advance()
	lp := p.expect(KindLParen)
	exprs := []Expr{p.parseExpression(precNone)}

	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())

		if p.peekKind() == KindRParen {
			break
		}

		exprs = append(exprs, p.parseExpression(precNone))
	}

	rp := p.expect(KindRParen)
	semi := p.expectStmtTerminator()

	return &UnsetStmt{
		Keyword: kw, LParen: lp,
		Exprs: exprs, Commas: commas, RParen: rp,
		Semicolon: semi,
	}
}

func (p *parser) parseNamespaceStmt() Stmt {
	kw := p.advance()
	var name *Name

	if p.peekKind() == KindIdent || p.peekKind() == KindBackslash {
		name = p.parseName()
	}

	if p.peekKind() == KindLBrace {
		body := p.parseBlockStmt()

		return &NamespaceStmt{Keyword: kw, Name: name, Body: body}
	}

	semi := p.expectStmtTerminator()

	return &NamespaceStmt{Keyword: kw, Name: name, Semicolon: semi}
}

func (p *parser) parseUseStmt() Stmt {
	kw := p.advance()
	var useKind Token

	if p.peekKind() == KindFunction || p.peekKind() == KindConst {
		useKind = p.advance()
	}

	name := p.parseName()

	if p.peekKind() == KindBackslash && p.peekKindAt(1) == KindLBrace {
		sep := p.advance()
		lb := p.advance()
		items := p.parseUseItems(true)
		rb := p.expect(KindRBrace)
		semi := p.expectStmtTerminator()

		return &UseStmt{
			Keyword: kw, UseKind: useKind,
			Prefix: name, GroupSeparator: sep,
			LBrace: lb, Items: items, RBrace: rb,
			Semicolon: semi,
		}
	}

	first := &UseItem{Name: name}

	if p.peekKind() == KindAs {
		first.AsToken = p.advance()
		first.Alias = p.expect(KindIdent)
	}

	items := []*UseItem{first}

	for p.peekKind() == KindComma {
		p.advance()

		if p.peekKind() == KindSemicolon {
			break
		}

		items = append(items, p.parseUseItem(false))
	}

	semi := p.expectStmtTerminator()

	return &UseStmt{
		Keyword: kw, UseKind: useKind,
		Items: items, Semicolon: semi,
	}
}

func (p *parser) parseUseItems(inGroup bool) []*UseItem {
	return p.parseCommaList(KindRBrace, nil, func() *UseItem {
		return p.parseUseItem(inGroup)
	})
}

func (p *parser) parseUseItem(inGroup bool) *UseItem {
	item := &UseItem{}

	if inGroup && (p.peekKind() == KindFunction || p.peekKind() == KindConst) {
		item.Kind = p.advance()
	}

	item.Name = p.parseName()

	if p.peekKind() == KindAs {
		item.AsToken = p.advance()
		item.Alias = p.expect(KindIdent)
	}

	return item
}

func (p *parser) parseDeclareStmt() Stmt {
	kw := p.advance()
	lp := p.expect(KindLParen)

	directives := []*DeclareDirective{p.parseDeclareDirective()}

	for p.peekKind() == KindComma {
		p.advance()
		directives = append(directives, p.parseDeclareDirective())
	}

	rp := p.expect(KindRParen)

	if p.peekKind() == KindColon {
		colon := p.advance()
		body := p.parseStmtListUntil(KindEndDeclare)
		endKw := p.expect(KindEndDeclare)
		endSemi := p.expectVerbatimStmtTerminator()

		return &DeclareStmt{
			Keyword: kw, LParen: lp, Directives: directives, RParen: rp,
			Colon: colon, AltBody: body,
			EndKeyword: endKw, EndSemi: endSemi,
		}
	}

	// A close tag or EOF ends the statement just as the `;` does, and declare
	// then has no body.
	if p.atStmtTerminator() {
		semi := p.expectStmtTerminator()

		return &DeclareStmt{
			Keyword: kw, LParen: lp, Directives: directives, RParen: rp,
			Semicolon: semi,
		}
	}

	body := p.parseStmt()

	return &DeclareStmt{
		Keyword: kw, LParen: lp, Directives: directives, RParen: rp,
		Body: body,
	}
}

func (p *parser) parseDeclareDirective() *DeclareDirective {
	name := p.expect(KindIdent)
	eq := p.expect(KindEq)
	value := p.parseExpression(precAssign)

	return &DeclareDirective{Name: name, Eq: eq, Value: value}
}

// The loop stops at `\{`, leaving the separator for a grouped use statement.
func (p *parser) parseName() *Name {
	n := &Name{}

	if p.peekKind() == KindBackslash {
		n.LeadingBackslash = p.advance()
	}

	// A leading `namespace` keyword is the relative-namespace prefix
	// (`namespace\Foo`).
	if p.peekKind() == KindIdent || p.peekKind() == KindNamespace {
		n.Parts = append(n.Parts, p.advance())
	} else {
		tok := p.peek()
		p.addError(tok.Span.Start, "expected identifier in qualified name")

		return n
	}

	for p.peekKind() == KindBackslash && isNameSegment(p.peekKindAt(1)) {
		n.Separators = append(n.Separators, p.advance())
		n.Parts = append(n.Parts, p.advance())
	}

	return n
}

func (p *parser) parseNameList() ([]*Name, []Token) {
	names := []*Name{p.parseName()}
	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())
		names = append(names, p.parseName())
	}

	return names, commas
}
