package php

import "fmt"

func (p *parser) parseFunctionDecl(attrs []*AttributeGroup) Stmt {
	kw := p.expect(KindFunction)
	var byRef Token

	if p.peekKind() == KindAmp {
		byRef = p.advance()
	}

	name := p.expect(KindIdent)
	lp := p.expect(KindLParen)
	params, commas := p.parseParamList()
	rp := p.expect(KindRParen)

	var returnColon Token
	var returnType TypeHint

	if p.peekKind() == KindColon {
		returnColon = p.advance()
		returnType = p.parseType()
	}

	body := p.parseBlockStmt()

	return &FunctionDecl{
		Attributes: attrs,
		Keyword:    kw, ByRef: byRef, Name: name,
		LParen: lp, Params: params, ParamCommas: commas, RParen: rp,
		ReturnColon: returnColon, ReturnType: returnType,
		Body: body,
	}
}

func (p *parser) isFunctionDecl() bool {
	switch p.peekKindAt(1) {
	case KindIdent:
		return true
	case KindAmp:
		return p.peekKindAt(2) == KindIdent
	}

	return false
}

func (p *parser) parseClassStmt(attrs []*AttributeGroup) Stmt {
	var modifiers []Modifier

	for isClassModifier(p.peekKind()) {
		modifiers = append(modifiers, p.parseModifier())
	}

	kw := p.expect(KindClass)
	name := p.expect(KindIdent)

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

	return &ClassStmt{
		Attributes: attrs,
		Modifiers:  modifiers, Keyword: kw, Name: name,
		Extends: extendsKw, Parent: parent,
		Implements:       implementsKw,
		ImplementsNames:  implementsNames,
		ImplementsCommas: implementsCommas,
		LBrace:           lb, Members: members, RBrace: rb,
	}
}

func (p *parser) parseInterfaceStmt(attrs []*AttributeGroup) Stmt {
	kw := p.advance()
	name := p.expect(KindIdent)

	var extendsKw Token
	var extendsNames []*Name
	var extendsCommas []Token

	if p.peekKind() == KindExtends {
		extendsKw = p.advance()
		extendsNames, extendsCommas = p.parseNameList()
	}

	lb := p.expect(KindLBrace)
	members := p.parseClassMembers()
	rb := p.expect(KindRBrace)

	return &InterfaceStmt{
		Attributes: attrs,
		Keyword:    kw, Name: name,
		Extends: extendsKw, ExtendsNames: extendsNames, ExtendsCommas: extendsCommas,
		LBrace: lb, Members: members, RBrace: rb,
	}
}

func (p *parser) parseTraitStmt(attrs []*AttributeGroup) Stmt {
	kw := p.advance()
	name := p.expect(KindIdent)
	lb := p.expect(KindLBrace)
	members := p.parseClassMembers()
	rb := p.expect(KindRBrace)

	return &TraitStmt{
		Attributes: attrs,
		Keyword:    kw, Name: name,
		LBrace: lb, Members: members, RBrace: rb,
	}
}

func (p *parser) parseEnumStmt(attrs []*AttributeGroup) Stmt {
	kw := p.advance()
	name := p.expect(KindIdent)

	var backingColon Token
	var backingType TypeHint

	if p.peekKind() == KindColon {
		backingColon = p.advance()
		backingType = p.parseType()
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

	return &EnumStmt{
		Attributes: attrs,
		Keyword:    kw, Name: name,
		BackingColon: backingColon, BackingType: backingType,
		Implements:       implementsKw,
		ImplementsNames:  implementsNames,
		ImplementsCommas: implementsCommas,
		LBrace:           lb, Members: members, RBrace: rb,
	}
}

func isClassModifier(k Kind) bool {
	switch k {
	case KindAbstract, KindFinal, KindReadonly:
		return true
	}

	return false
}

func isMemberModifier(k Kind) bool {
	switch k {
	case KindPublic, KindProtected, KindPrivate, KindStatic,
		KindFinal, KindAbstract, KindReadonly, KindVar:
		return true
	}

	return false
}

func isVisibilityModifier(k Kind) bool {
	switch k {
	case KindPublic, KindProtected, KindPrivate:
		return true
	}

	return false
}

func (p *parser) parseModifier() Modifier {
	kw := p.advance()

	if !isVisibilityModifier(kw.Kind) || p.peekKind() != KindLParen {
		return Modifier{Keyword: kw}
	}

	lp := p.advance()
	set := p.expect(KindIdent)
	rp := p.expect(KindRParen)

	return Modifier{Keyword: kw, LParen: lp, Set: set, RParen: rp}
}

func (p *parser) parseClassMembers() []ClassMember {
	var members []ClassMember

	for p.peekKind() != KindRBrace && p.peekKind() != KindEOF {
		start := p.pos
		m := p.parseClassMember()

		if m != nil {
			members = append(members, m)
		}

		p.guardProgress(start)
	}

	return members
}

func (p *parser) parseClassMember() ClassMember {
	if p.peekKind() == KindUse {
		return p.parseTraitUseStmt()
	}

	var attrs []*AttributeGroup

	if p.peekKind() == KindAttributeOpen {
		attrs = p.parseAttributeGroups()
	}

	var modifiers []Modifier

	for isMemberModifier(p.peekKind()) {
		modifiers = append(modifiers, p.parseModifier())
	}

	switch p.peekKind() {
	case KindFunction:
		return p.parseMethodDecl(attrs, modifiers)
	case KindConst:
		return p.parseClassConstDecl(attrs, modifiers)
	case KindCase:
		return p.parseEnumCase(attrs)
	default:
		// PHP requires a modifier, `var` at least, before a property.
		if len(modifiers) == 0 {
			tok := p.peek()
			p.addError(tok.Span.Start, fmt.Sprintf("expected a modifier or %s, got %s", KindFunction, tok.Kind))
		}

		return p.parsePropertyDecl(attrs, modifiers)
	}
}

func (p *parser) parsePropertyDecl(attrs []*AttributeGroup, modifiers []Modifier) ClassMember {
	var typ TypeHint

	if p.peekKind() != KindVariable {
		typ = p.parseType()
	}

	items := []*PropertyItem{p.parsePropertyItem()}
	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())
		items = append(items, p.parsePropertyItem())
	}

	var lb, rb, semi Token
	var hooks []*PropertyHook

	if p.peekKind() == KindLBrace {
		lb, hooks, rb = p.parsePropertyHooks()
	} else {
		semi = p.expectMemberTerminator()
	}

	return &PropertyDecl{
		Attributes: attrs,
		Modifiers:  modifiers, Type: typ,
		Items: items, Commas: commas,
		LBrace: lb, Hooks: hooks, RBrace: rb,
		Semicolon: semi,
	}
}

// parsePropertyHooks rejects an empty hook list, as PHP does: printed with no
// hooks it would read as a plain property, turning a fatal error into valid
// code.
func (p *parser) parsePropertyHooks() (Token, []*PropertyHook, Token) {
	lb := p.advance()

	var hooks []*PropertyHook

	for p.peekKind() != KindRBrace && p.peekKind() != KindEOF {
		start := p.pos
		hooks = append(hooks, p.parsePropertyHook())
		p.guardProgress(start)
	}

	if len(hooks) == 0 {
		p.addError(lb.Span.Start, "property hook list must not be empty")
	}

	return lb, hooks, p.expect(KindRBrace)
}

func (p *parser) parsePropertyHook() *PropertyHook {
	var attrs []*AttributeGroup

	if p.peekKind() == KindAttributeOpen {
		attrs = p.parseAttributeGroups()
	}

	var final, ref Token

	if p.peekKind() == KindFinal {
		final = p.advance()
	}

	if p.peekKind() == KindAmp {
		ref = p.advance()
	}

	name := p.expect(KindIdent)

	var lp, rp Token
	var params []*Param
	var paramCommas []Token

	if p.peekKind() == KindLParen {
		lp = p.advance()
		params, paramCommas = p.parseParamList()
		rp = p.expect(KindRParen)
	}

	hook := &PropertyHook{
		Attributes: attrs, Final: final, Ref: ref, Name: name,
		LParen: lp, Params: params, ParamCommas: paramCommas, RParen: rp,
	}

	switch p.peekKind() {
	case KindDoubleArrow:
		hook.Arrow = p.advance()
		hook.Expr = p.parseExpression(precNone)
		hook.Semicolon = p.expectMemberTerminator()
	case KindLBrace:
		hook.Body = p.parseBlockStmt()
	case KindSemicolon:
		hook.Semicolon = p.advance()
	default:
		tok := p.peek()
		p.addError(tok.Span.Start, "expected '=>', '{', or ';' after property hook")
		hook.Semicolon = p.expectMemberTerminator()
	}

	return hook
}

func (p *parser) parsePropertyItem() *PropertyItem {
	variable := p.expect(KindVariable)

	var eq Token
	var def Expr

	if p.peekKind() == KindEq {
		eq = p.advance()
		def = p.parseExpression(precNone)
	}

	return &PropertyItem{Var: variable, Eq: eq, Default: def}
}

func (p *parser) parseMethodDecl(attrs []*AttributeGroup, modifiers []Modifier) ClassMember {
	kw := p.expect(KindFunction)
	var byRef Token

	if p.peekKind() == KindAmp {
		byRef = p.advance()
	}

	name := p.parseMethodNameToken()
	lp := p.expect(KindLParen)
	params, commas := p.parseParamList()
	rp := p.expect(KindRParen)

	var returnColon Token
	var returnType TypeHint

	if p.peekKind() == KindColon {
		returnColon = p.advance()
		returnType = p.parseType()
	}

	var body *BlockStmt
	var semi Token

	if p.peekKind() == KindLBrace {
		body = p.parseBlockStmt()
	} else {
		semi = p.expectMemberTerminator()
	}

	return &MethodDecl{
		Attributes: attrs,
		Modifiers:  modifiers, Keyword: kw,
		ByRef: byRef, Name: name,
		LParen: lp, Params: params, ParamCommas: commas, RParen: rp,
		ReturnColon: returnColon, ReturnType: returnType,
		Body: body, Semicolon: semi,
	}
}

// parseMethodNameToken accepts an identifier or any keyword: PHP permits
// reserved words as method names.
func (p *parser) parseMethodNameToken() Token {
	if isMemberNameToken(p.peekKind()) {
		return p.advance()
	}

	tok := p.peek()
	p.addError(tok.Span.Start, "expected method name")

	return p.advance()
}

func (p *parser) parseParamList() ([]*Param, []Token) {
	var commas []Token
	params := p.parseCommaList(KindRParen, &commas, p.parseParam)

	return params, commas
}

func (p *parser) parseParam() *Param {
	var attrs []*AttributeGroup

	if p.peekKind() == KindAttributeOpen {
		attrs = p.parseAttributeGroups()
	}

	var modifiers []Modifier

	for isMemberModifier(p.peekKind()) {
		modifiers = append(modifiers, p.parseModifier())
	}

	var typ TypeHint

	if p.peekKind() != KindVariable && p.peekKind() != KindAmp && p.peekKind() != KindEllipsis {
		typ = p.parseType()
	}

	var byRef Token

	if p.peekKind() == KindAmp {
		byRef = p.advance()
	}

	var variadic Token

	if p.peekKind() == KindEllipsis {
		variadic = p.advance()
	}

	variable := p.expect(KindVariable)

	var eq Token
	var def Expr

	if p.peekKind() == KindEq {
		eq = p.advance()
		def = p.parseExpression(precNone)
	}

	var lb, rb Token
	var hooks []*PropertyHook

	if p.peekKind() == KindLBrace {
		lb, hooks, rb = p.parsePropertyHooks()
	}

	return &Param{
		Attributes: attrs,
		Modifiers:  modifiers,
		Type:       typ,
		ByRef:      byRef,
		Variadic:   variadic,
		Var:        variable,
		Eq:         eq,
		Default:    def,
		LBrace:     lb,
		Hooks:      hooks,
		RBrace:     rb,
	}
}

func (p *parser) parseClassConstDecl(attrs []*AttributeGroup, modifiers []Modifier) ClassMember {
	kw := p.expect(KindConst)
	var typ TypeHint

	if constDeclHasType(p) {
		typ = p.parseType()
	}

	items := []*ConstItem{p.parseConstItem(true)}
	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())
		items = append(items, p.parseConstItem(true))
	}

	semi := p.expectMemberTerminator()

	return &ClassConstDecl{
		Attributes: attrs,
		Modifiers:  modifiers, Keyword: kw, Type: typ,
		Items: items, Commas: commas,
		Semicolon: semi,
	}
}

// A name followed by `=`, `,` or `;` is the constant's own name, not a type,
// even a keyword that could start one (`const ARRAY = …`).
func constDeclHasType(p *parser) bool {
	peek := p.peekKind()

	if isMemberNameToken(peek) {
		switch p.peekKindAt(1) {
		case KindEq, KindComma, KindSemicolon:
			return false
		}
	}

	switch peek {
	case KindQuestion, KindBackslash, KindArray, KindCallable,
		KindSelf, KindStatic, KindTrue, KindFalse, KindNull, KindLParen:
		return true
	}

	return peek == KindIdent
}

func (p *parser) parseConstItem(allowReserved bool) *ConstItem {
	var name Token

	if allowReserved {
		name = p.consumeMemberNameToken()
	} else {
		name = p.expect(KindIdent)
	}

	eq := p.expect(KindEq)
	value := p.parseExpression(precNone)

	return &ConstItem{Name: name, Eq: eq, Value: value}
}

func (p *parser) parseEnumCase(attrs []*AttributeGroup) ClassMember {
	kw := p.expect(KindCase)
	name := p.consumeMemberNameToken()

	var eq Token
	var value Expr

	if p.peekKind() == KindEq {
		eq = p.advance()
		value = p.parseExpression(precNone)
	}

	semi := p.expectMemberTerminator()

	return &EnumCase{
		Attributes: attrs,
		Keyword:    kw, Name: name,
		Eq: eq, Value: value,
		Semicolon: semi,
	}
}

func (p *parser) parseTraitUseStmt() ClassMember {
	kw := p.advance()
	traits := []*Name{p.parseName()}

	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())
		traits = append(traits, p.parseName())
	}

	if p.peekKind() == KindLBrace {
		lb := p.advance()

		var adaptations []TraitAdaptation
		for p.peekKind() != KindRBrace && p.peekKind() != KindEOF {
			start := p.pos
			adaptations = append(adaptations, p.parseTraitAdaptation())
			p.guardProgress(start)
		}

		rb := p.expect(KindRBrace)

		return &TraitUseStmt{
			Keyword: kw, Traits: traits, Commas: commas,
			LBrace: lb, Adaptations: adaptations, RBrace: rb,
		}
	}

	semi := p.expectMemberTerminator()

	return &TraitUseStmt{Keyword: kw, Traits: traits, Commas: commas, Semicolon: semi}
}

func (p *parser) parseTraitAdaptation() TraitAdaptation {
	method := p.parseTraitMethodRef()

	switch p.peekKind() {
	case KindInsteadOf:
		return p.parseTraitInsteadofAdaptation(&method)
	case KindAs:
		return p.parseTraitAliasAdaptation(&method)
	default:
		p.addError(p.peek().Span.Start, "expected `insteadof` or `as`")

		for p.peekKind() != KindSemicolon && p.peekKind() != KindRBrace && p.peekKind() != KindEOF {
			p.advance()
		}

		if p.peekKind() == KindSemicolon {
			p.advance()
		}

		return &TraitAliasAdaptation{Method: method}
	}
}

func (p *parser) parseTraitMethodRef() TraitMethodRef {
	next := p.peekKindAt(1)

	if p.peekKind() == KindBackslash || next == KindBackslash || next == KindScopeRes {
		trait := p.parseName()
		sep := p.expect(KindScopeRes)
		methodTok := p.consumeMemberNameToken()

		return TraitMethodRef{Trait: trait, Separator: sep, Method: methodTok}
	}

	return TraitMethodRef{Method: p.consumeMemberNameToken()}
}

func (p *parser) consumeMemberNameToken() Token {
	if isMemberNameToken(p.peekKind()) {
		return p.advance()
	}

	tok := p.advance()
	p.addError(tok.Span.Start, "expected identifier")

	return tok
}

func (p *parser) parseTraitInsteadofAdaptation(method *TraitMethodRef) *TraitInsteadofAdaptation {
	kw := p.advance()
	traits := []*Name{p.parseName()}

	var commas []Token

	for p.peekKind() == KindComma {
		commas = append(commas, p.advance())
		traits = append(traits, p.parseName())
	}

	semi := p.expectMemberTerminator()

	return &TraitInsteadofAdaptation{
		Method:    *method,
		Keyword:   kw,
		Traits:    traits,
		Commas:    commas,
		Semicolon: semi,
	}
}

func (p *parser) parseTraitAliasAdaptation(method *TraitMethodRef) *TraitAliasAdaptation {
	kw := p.advance()

	var visibility Token

	switch p.peekKind() {
	case KindPublic, KindProtected, KindPrivate:
		visibility = p.advance()
	}

	var alias Token

	if isMemberNameToken(p.peekKind()) {
		alias = p.advance()
	}

	semi := p.expectMemberTerminator()

	return &TraitAliasAdaptation{
		Method:     *method,
		Keyword:    kw,
		Visibility: visibility,
		Alias:      alias,
		Semicolon:  semi,
	}
}

func (p *parser) parseAttributedStmt() Stmt {
	attrs := p.parseAttributeGroups()

	switch p.peekKind() {
	case KindClass, KindAbstract, KindFinal, KindReadonly:
		return p.parseClassStmt(attrs)
	case KindInterface:
		return p.parseInterfaceStmt(attrs)
	case KindTrait:
		return p.parseTraitStmt(attrs)
	case KindEnum:
		return p.parseEnumStmt(attrs)
	case KindFunction:
		if p.isFunctionDecl() {
			return p.parseFunctionDecl(attrs)
		}
	}

	tok := p.peek()
	p.addError(tok.Span.Start, "attributes must precede a class, interface, trait, enum, or function declaration")

	return p.parseExpressionStmt()
}

func (p *parser) parseAttributeGroups() []*AttributeGroup {
	var groups []*AttributeGroup

	for p.peekKind() == KindAttributeOpen {
		groups = append(groups, p.parseAttributeGroup())
	}

	return groups
}

func (p *parser) parseAttributeGroup() *AttributeGroup {
	open := p.advance()

	var commas []Token
	attributes := p.parseCommaList(KindRBracket, &commas, p.parseAttribute)

	rb := p.expect(KindRBracket)

	return &AttributeGroup{Open: open, Attributes: attributes, Commas: commas, Close: rb}
}

func (p *parser) parseAttribute() *Attribute {
	name := p.parseName()

	var lp, rp Token
	var args []Expr

	if p.peekKind() == KindLParen {
		lp = p.advance()
		args = p.parseCallArgs(KindRParen)
		rp = p.expect(KindRParen)
	}

	return &Attribute{Name: name, LParen: lp, Args: args, RParen: rp}
}
