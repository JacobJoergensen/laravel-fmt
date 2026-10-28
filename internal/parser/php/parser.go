package php

import (
	"fmt"
	"strings"
)

// Precedence levels, lowest to highest. Higher values bind tighter.
const (
	precNone           = iota
	precLogicalOr      // or
	precLogicalXor     // xor
	precLogicalAnd     // and
	precAssign         // = += -= etc. (right-associative)
	precTernary        // ? :
	precPipeline       // |> (PHP 8.5, left-associative)
	precCoalesce       // ?? (right-associative)
	precOr             // ||
	precAnd            // &&
	precBitOr          // |
	precBitXor         // ^
	precBitAnd         // &
	precEquality       // == != === !== <=>
	precCompare        // < <= > >=
	precConcat         // . (PHP 8+ position)
	precShift          // << >>
	precAdditive       // + -
	precMultiplicative // * / %
	precLogicalNot     // !
	precInstanceof     // instanceof
	precUnary          // - + ~ @ cast ++ --
	precPow            // ** (right-associative)
	precNew            // clone new
	precCall           // () [] -> ::
)

type (
	prefixFn func(p *parser) Expr
	infixFn  func(p *parser, left Expr) Expr
)

type parseRule struct {
	prefix     prefixFn
	infix      infixFn
	prec       int
	rightAssoc bool
}

// rules is the Pratt dispatch table populated in init. It is declared
// without an initializer so that parseExpression (which reads from
// rules) does not form a package-level initialization cycle with the
// infix callbacks (parseTernary, parseBinary, …) stored in the table.
var rules [kindLast]parseRule

func init() {
	rules[KindInt] = parseRule{prefix: parseNumberLiteral}
	rules[KindFloat] = parseRule{prefix: parseNumberLiteral}
	rules[KindString] = parseRule{prefix: parseStringLiteral}
	rules[KindShellExec] = parseRule{prefix: parseShellExec}
	rules[KindVariable] = parseRule{prefix: parseVariable}
	rules[KindDollar] = parseRule{prefix: parseVarVar}
	rules[KindIdent] = parseRule{prefix: parseIdent}
	rules[KindSelf] = parseRule{prefix: parseIdent}
	rules[KindEmpty] = parseRule{prefix: parseIdent}
	rules[KindList] = parseRule{prefix: parseListPattern}
	rules[KindNamespace] = parseRule{prefix: parseIdent}
	rules[KindBackslash] = parseRule{prefix: parseQualifiedIdent}
	rules[KindStatic] = parseRule{prefix: parseStaticPrefix}
	rules[KindFunction] = parseRule{prefix: parseClosure}
	rules[KindFn] = parseRule{prefix: parseArrowFn}
	rules[KindAttributeOpen] = parseRule{prefix: parseAttributedClosure}
	rules[KindTrue] = parseRule{prefix: parseBoolLiteral}
	rules[KindFalse] = parseRule{prefix: parseBoolLiteral}
	rules[KindNull] = parseRule{prefix: parseNullLiteral}
	rules[KindLParen] = parseRule{prefix: parseParenOrCast, infix: parseCall, prec: precCall}
	rules[KindLBracket] = parseRule{prefix: parseArrayShort, infix: parseSubscript, prec: precCall}
	rules[KindArray] = parseRule{prefix: parseArrayLong}

	rules[KindBang] = parseRule{prefix: parseLogicalNot}
	rules[KindTilde] = parseRule{prefix: parseUnary}
	rules[KindAt] = parseRule{prefix: parseUnary}

	rules[KindPlus] = parseRule{prefix: parseUnary, infix: parseBinary, prec: precAdditive}
	rules[KindMinus] = parseRule{prefix: parseUnary, infix: parseBinary, prec: precAdditive}

	rules[KindPlusPlus] = parseRule{prefix: parseUpdatePrefix, infix: parseUpdatePostfix, prec: precCall}
	rules[KindMinusMinus] = parseRule{prefix: parseUpdatePrefix, infix: parseUpdatePostfix, prec: precCall}

	rules[KindStar] = parseRule{infix: parseBinary, prec: precMultiplicative}
	rules[KindSlash] = parseRule{infix: parseBinary, prec: precMultiplicative}
	rules[KindPercent] = parseRule{infix: parseBinary, prec: precMultiplicative}
	rules[KindStarStar] = parseRule{infix: parseBinary, prec: precPow, rightAssoc: true}
	rules[KindDot] = parseRule{infix: parseBinary, prec: precConcat}
	rules[KindShl] = parseRule{infix: parseBinary, prec: precShift}
	rules[KindShr] = parseRule{infix: parseBinary, prec: precShift}

	for _, k := range []Kind{KindLt, KindGt, KindLtEq, KindGtEq} {
		rules[k] = parseRule{infix: parseBinary, prec: precCompare}
	}

	for _, k := range []Kind{KindEqEq, KindBangEq, KindEqEqEq, KindBangEqEq, KindSpaceship} {
		rules[k] = parseRule{infix: parseBinary, prec: precEquality}
	}

	rules[KindAmp] = parseRule{infix: parseBinary, prec: precBitAnd}
	rules[KindCaret] = parseRule{infix: parseBinary, prec: precBitXor}
	rules[KindPipe] = parseRule{infix: parseBinary, prec: precBitOr}
	rules[KindAmpAmp] = parseRule{infix: parseBinary, prec: precAnd}
	rules[KindPipePipe] = parseRule{infix: parseBinary, prec: precOr}
	rules[KindNullCoalesce] = parseRule{infix: parseBinary, prec: precCoalesce, rightAssoc: true}
	rules[KindPipeline] = parseRule{infix: parseBinary, prec: precPipeline}

	assignOps := []Kind{
		KindEq, KindPlusAssign, KindMinusAssign, KindStarAssign, KindSlashAssign,
		KindPercentAssign, KindStarStarAssign, KindDotAssign, KindAmpAssign,
		KindPipeAssign, KindCaretAssign, KindShlAssign, KindShrAssign, KindNullCoalesceAssign,
	}

	for _, k := range assignOps {
		rules[k] = parseRule{infix: parseAssign, prec: precAssign, rightAssoc: true}
	}

	rules[KindAnd] = parseRule{infix: parseBinary, prec: precLogicalAnd}
	rules[KindOr] = parseRule{infix: parseBinary, prec: precLogicalOr}
	rules[KindXor] = parseRule{infix: parseBinary, prec: precLogicalXor}

	rules[KindArrow] = parseRule{infix: parseMember, prec: precCall}
	rules[KindNullSafeArrow] = parseRule{infix: parseMember, prec: precCall}
	rules[KindScopeRes] = parseRule{infix: parseStaticMember, prec: precCall}

	rules[KindInstanceOf] = parseRule{infix: parseInstanceof, prec: precInstanceof}
	rules[KindQuestion] = parseRule{infix: parseTernary, prec: precTernary, rightAssoc: true}

	rules[KindNew] = parseRule{prefix: parseNew}
	rules[KindClone] = parseRule{prefix: parseClone}
	rules[KindPrint] = parseRule{prefix: parsePrint}
	rules[KindInclude] = parseRule{prefix: parseIncludeExpr}
	rules[KindIncludeOnce] = parseRule{prefix: parseIncludeExpr}
	rules[KindRequire] = parseRule{prefix: parseIncludeExpr}
	rules[KindRequireOnce] = parseRule{prefix: parseIncludeExpr}
	rules[KindThrow] = parseRule{prefix: parseThrowExpr}
	rules[KindYield] = parseRule{prefix: parseYield}
	rules[KindYieldFrom] = parseRule{prefix: parseYieldFrom}
	rules[KindMatch] = parseRule{prefix: parseMatch}
}

type parser struct {
	tokens []Token
	pos    int
	errors []Error
	depth  int
	// depthExceeded reports the depth cap once; per token, a 400 KB file gave
	// 200,000 errors.
	depthExceeded bool
	// fileEOF is set when EOF is the end of a whole file. It does not end a
	// statement there (`<?php 0` fails php -l), where in a bare fragment it
	// stands for the `?>` its host compiles around it.
	fileEOF bool
}

// maxNestingDepth caps expression recursion so a pathological file (a long run
// of `[` or `(`) returns a located error instead of dying on a Go stack
// overflow, which is a fatal error no recover can catch and which takes every
// other file in the run down with it. The deepest bracket nesting measured
// across 60,636 real PHP files is 52, so this is three orders of magnitude of
// headroom over anything a person writes.
const maxNestingDepth = 500

func (p *parser) peek() Token {
	return p.tokens[p.pos]
}

func (p *parser) peekKind() Kind {
	return p.tokens[p.pos].Kind
}

func (p *parser) peekKindAt(n int) Kind {
	if p.pos+n >= len(p.tokens) {
		return KindEOF
	}

	return p.tokens[p.pos+n].Kind
}

func (p *parser) advance() Token {
	tok := p.tokens[p.pos]

	if tok.Kind != KindEOF {
		p.pos++
	}

	return tok
}

// expect consumes a token of the given kind. On mismatch it records an error
// and returns a synthetic zero-span token at the current position, so callers
// always get a shaped tree.
func (p *parser) expect(kind Kind) Token {
	tok := p.peek()

	if tok.Kind != kind {
		p.addError(tok.Span.Start, fmt.Sprintf("expected %s, got %s", kind, tok.Kind))

		return Token{Kind: kind, Span: Span{Start: tok.Span.Start, End: tok.Span.Start}}
	}

	return p.advance()
}

// maxErrors bounds what one file can report. Past the first few, errors on a
// pathological file are cascade noise from the same root, each printed with
// its own source snippet.
const maxErrors = 50

func (p *parser) addError(pos Position, msg string) {
	// Past the depth cap every bracket still open reports itself unclosed.
	if p.depthExceeded || len(p.errors) >= maxErrors {
		return
	}

	p.errors = append(p.errors, Error{Pos: pos, Msg: msg})
}

// guardProgress keeps a recovery loop from spinning on a token no sub-parser
// can consume: when an iteration made no progress it records a diagnostic and
// skips a token, so every `for peek != closer && peek != EOF` loop reaches EOF
// instead of allocating forever on malformed input. The recorded error makes
// the formatter bail, so the skipped token never yields wrong output.
func (p *parser) guardProgress(start int) {
	if p.pos != start {
		return
	}

	tok := p.peek()
	p.addError(tok.Span.Start, fmt.Sprintf("unexpected %s", tok.Kind))
	p.advance()
}

func (p *parser) parseFile() []Stmt {
	var stmts []Stmt

	for p.peekKind() != KindEOF {
		stmts = append(stmts, p.parseStmtOrTag())
	}

	return stmts
}

// parseStmtOrTag dispatches one statement, accepting the inline-HTML and tag
// tokens a ?> interruption produces at every nesting level, not just the file
// top.
func (p *parser) parseStmtOrTag() Stmt {
	switch p.peekKind() {
	case KindHaltCompiler:
		return p.parseHaltCompilerStmt()
	case KindInlineHTML:
		return &InlineHTMLStmt{Token: p.advance()}
	case KindOpenTag, KindCloseTag:
		return &TagStmt{Token: p.advance()}
	case KindOpenTagEcho:
		return p.parseShortEchoStmt()
	default:
		return p.parseStmt()
	}
}

func (p *parser) parseStmt() Stmt {
	switch p.peekKind() {
	case KindLBrace:
		return p.parseBlockStmt()
	case KindEcho:
		return p.parseEchoStmt()
	case KindReturn:
		return p.parseReturnStmt()
	case KindIf:
		return p.parseIfStmt()
	case KindWhile:
		return p.parseWhileStmt()
	case KindDo:
		return p.parseDoWhileStmt()
	case KindFor:
		return p.parseForStmt()
	case KindForEach:
		return p.parseForEachStmt()
	case KindSwitch:
		return p.parseSwitchStmt()
	case KindTry:
		return p.parseTryStmt()
	case KindBreak:
		return p.parseBreakStmt()
	case KindContinue:
		return p.parseContinueStmt()
	case KindGoto:
		return p.parseGotoStmt()
	case KindConst:
		return p.parseConstStmt()
	case KindGlobal:
		return p.parseGlobalStmt()
	case KindUnset:
		return p.parseUnsetStmt()
	case KindStatic:
		if p.peekKindAt(1) == KindVariable {
			return p.parseStaticVarStmt()
		}

		return p.parseExpressionStmt()
	case KindNamespace:
		// `namespace\Foo…` (the keyword immediately followed by `\`) is a
		// relative-name expression statement, not a namespace declaration;
		// a real declaration is `namespace App;` / `namespace App { }`.
		if p.peekKindAt(1) == KindBackslash {
			return p.parseExpressionStmt()
		}

		return p.parseNamespaceStmt()
	case KindUse:
		return p.parseUseStmt()
	case KindDeclare:
		return p.parseDeclareStmt()
	case KindClass, KindAbstract, KindFinal, KindReadonly:
		return p.parseClassStmt(nil)
	case KindInterface:
		return p.parseInterfaceStmt(nil)
	case KindTrait:
		return p.parseTraitStmt(nil)
	case KindEnum:
		return p.parseEnumStmt(nil)
	case KindFunction:
		if p.isFunctionDecl() {
			return p.parseFunctionDecl(nil)
		}

		return p.parseExpressionStmt()
	case KindAttributeOpen:
		return p.parseAttributedStmt()
	case KindSemicolon:
		return &EmptyStmt{Semicolon: p.advance()}
	default:
		if p.peekKind() == KindIdent && p.peekKindAt(1) == KindColon {
			return p.parseLabelStmt()
		}

		return p.parseExpressionStmt()
	}
}

func (p *parser) parseBlockStmt() *BlockStmt {
	lb := p.expect(KindLBrace)
	var stmts []Stmt

	for p.peekKind() != KindRBrace && p.peekKind() != KindEOF {
		start := p.pos
		stmts = append(stmts, p.parseStmtOrTag())
		p.guardProgress(start)
	}

	rb := p.expect(KindRBrace)

	return &BlockStmt{LBrace: lb, Stmts: stmts, RBrace: rb}
}

func (p *parser) parseEchoStmt() Stmt {
	kw := p.advance()
	exprs := []Expr{p.parseExpression(precNone)}

	for p.peekKind() == KindComma {
		p.advance()
		exprs = append(exprs, p.parseExpression(precNone))
	}

	semi := p.expectStmtTerminator()

	return &EchoStmt{Keyword: kw, Exprs: exprs, Semicolon: semi}
}

func (p *parser) parseShortEchoStmt() *ShortEchoStmt {
	open := p.advance()
	stmt := &ShortEchoStmt{OpenTag: open}

	stmt.Exprs = append(stmt.Exprs, p.parseExpression(precNone))

	for p.peekKind() == KindComma {
		stmt.Commas = append(stmt.Commas, p.advance())
		stmt.Exprs = append(stmt.Exprs, p.parseExpression(precNone))
	}

	stmt.Semicolon = p.expectStmtTerminator()

	return stmt
}

func (p *parser) parseReturnStmt() Stmt {
	kw := p.advance()
	var expr Expr

	if !p.atStmtTerminator() {
		expr = p.parseExpression(precNone)
	}

	semi := p.expectStmtTerminator()

	return &ReturnStmt{Keyword: kw, Expr: expr, Semicolon: semi}
}

func (p *parser) parseExpressionStmt() Stmt {
	expr := p.parseExpression(precNone)
	semi := p.expectStmtTerminator()

	return &ExpressionStmt{Expr: expr, Semicolon: semi}
}

// atStmtTerminator reports whether the next token ends the current statement.
// It pairs with expectStmtTerminator so a statement whose expression is optional
// reads the same set of terminators that consuming one accepts.
func (p *parser) atStmtTerminator() bool {
	switch p.peekKind() {
	case KindSemicolon, KindCloseTag, KindEOF:
		return true
	}

	return false
}

// expectMemberTerminator is expectStmtTerminator for a class member, where a
// `?>` is not a statement boundary the file keeps but the first half of a
// `?><?php` pair. PHP allows a class body to output nothing, so the pair can
// carry nothing between it and means nothing of its own: it is consumed here and
// the builder writes the `;` it stood in for. A close tag with anything else
// after it is left in place, which is how PHP rejects it too.
func (p *parser) expectMemberTerminator() Token {
	if p.peekKind() == KindCloseTag && p.pos+1 < len(p.tokens) &&
		p.tokens[p.pos+1].Kind == KindOpenTag {
		semi := p.syntheticSemicolon()

		p.advance()
		p.advance()

		return semi
	}

	return p.expectStmtTerminator()
}

// expectVerbatimStmtTerminator is expectStmtTerminator for a statement the
// formatter reproduces byte for byte: an alternative-syntax block carries its
// own `endif`, so a terminator the parser only synthesized would never reach the
// output and the emission would not parse. A `?>` is still one, since it stays
// in the source the block is copied from; EOF is not, and php -l agrees.
func (p *parser) expectVerbatimStmtTerminator() Token {
	if p.peekKind() == KindEOF {
		tok := p.peek()
		p.addError(tok.Span.Start, fmt.Sprintf("expected %s, got %s", KindSemicolon, tok.Kind))

		return p.syntheticSemicolon()
	}

	return p.expectStmtTerminator()
}

// expectStmtTerminator consumes a `;`, or at a `?>` close tag or EOF returns a
// synthetic zero-span semicolon without consuming the lookahead. PHP ends a
// statement on any of the three, and the `?>` must survive as a TagStmt for
// parseFile. Any other token records the usual "expected ;" error.
func (p *parser) expectStmtTerminator() Token {
	tok := p.peek()

	switch {
	case tok.Kind == KindSemicolon:
		return p.advance()
	case tok.Kind == KindCloseTag, tok.Kind == KindEOF && !p.fileEOF:
		return p.syntheticSemicolon()
	}

	p.addError(tok.Span.Start, fmt.Sprintf("expected %s, got %s", KindSemicolon, tok.Kind))

	return p.syntheticSemicolon()
}

// syntheticSemicolon anchors its zero-width `;` at the end of the last consumed
// token, not at the lookahead: an alternative-syntax block is copied from its
// span, and a comment between `endforeach` and a `?>` would otherwise land in
// the copy and also stay trivia, printing twice.
func (p *parser) syntheticSemicolon() Token {
	at := p.peek().Span.Start

	if p.pos > 0 {
		at = p.tokens[p.pos-1].Span.End
	}

	return Token{Kind: KindSemicolon, Span: Span{Start: at, End: at}}
}

// parseExpression is the Pratt entry point. minPrec is the lowest infix
// precedence that will be consumed; use precNone at the outermost call.
func (p *parser) parseExpression(minPrec int) Expr {
	p.depth++
	defer func() { p.depth-- }()

	tok := p.peek()

	if p.depth > maxNestingDepth {
		if !p.depthExceeded {
			p.addError(tok.Span.Start, "maximum nesting depth exceeded")
			p.depthExceeded = true
		}

		p.advance()

		return &IdentExpr{Token: tok}
	}

	rule := rules[tok.Kind]

	if rule.prefix == nil {
		p.addError(tok.Span.Start, fmt.Sprintf("unexpected %s", tok.Kind))
		p.advance()

		return &IdentExpr{Token: tok}
	}

	left := rule.prefix(p)

	for {
		cur := p.peek()
		next := rules[cur.Kind]

		if next.infix == nil || next.prec <= minPrec {
			break
		}

		left = next.infix(p, left)
	}

	return left
}

func parseNumberLiteral(p *parser) Expr {
	tok := p.advance()

	if tok.Kind == KindFloat {
		return &FloatExpr{Token: tok}
	}

	return &IntExpr{Token: tok}
}

func parseStringLiteral(p *parser) Expr {
	return &StringExpr{Token: p.advance()}
}

func parseShellExec(p *parser) Expr {
	return &ShellExecExpr{Token: p.advance()}
}

func parseVariable(p *parser) Expr {
	return &VariableExpr{Token: p.advance()}
}

// Postfix (->, [], ()) binds to the whole variable-variable through the
// caller's precedence loop, not to the inner name.
func parseVarVar(p *parser) Expr {
	dollar := p.advance()

	switch p.peekKind() {
	case KindLBrace:
		lb := p.advance()
		inner := p.parseExpression(precNone)
		rb := p.expect(KindRBrace)

		return &VarVarExpr{Dollar: dollar, LBrace: lb, Name: inner, RBrace: rb}
	case KindVariable:
		return &VarVarExpr{Dollar: dollar, Name: &VariableExpr{Token: p.advance()}}
	case KindDollar:
		return &VarVarExpr{Dollar: dollar, Name: parseVarVar(p)}
	default:
		tok := p.peek()
		p.addError(tok.Span.Start, "expected a variable or `{` after `$`")

		return &VarVarExpr{Dollar: dollar, Name: &VariableExpr{Token: p.advance()}}
	}
}

func parseIdent(p *parser) Expr {
	expr := &IdentExpr{Token: p.advance()}
	extendQualifiedIdent(p, expr)

	return expr
}

func parseQualifiedIdent(p *parser) Expr {
	leading := p.advance()

	if !isNameSegment(p.peekKind()) {
		tok := p.peek()
		p.addError(tok.Span.Start, "expected identifier after `\\`")

		return &IdentExpr{LeadingBackslash: leading}
	}

	expr := &IdentExpr{
		LeadingBackslash: leading,
		Token:            p.advance(),
	}
	extendQualifiedIdent(p, expr)

	return expr
}

func extendQualifiedIdent(p *parser, expr *IdentExpr) {
	for p.peekKind() == KindBackslash && isNameSegment(p.peekKindAt(1)) {
		expr.Separators = append(expr.Separators, p.advance())
		expr.Parts = append(expr.Parts, p.advance())
	}
}

// isNameSegment reports whether k can be a namespace-name segment after a `\`.
// PHP lexes a qualified name as one token and applies no per-segment reservation,
// so every keyword (`ReadOnly`, `Match`, `List`, ...) is a legal segment there.
func isNameSegment(k Kind) bool {
	return k == KindIdent || IsKeyword(k)
}

func parseBoolLiteral(p *parser) Expr {
	tok := p.advance()

	return &BoolExpr{Token: tok, Value: tok.Kind == KindTrue}
}

func parseNullLiteral(p *parser) Expr {
	return &NullExpr{Token: p.advance()}
}

func parseUnary(p *parser) Expr {
	op := p.advance()
	operand := p.parseExpression(precUnary)

	return &UnaryExpr{Op: op, Expr: operand}
}

// parseUpdatePrefix parses `++expr` / `--expr`. The operand is parsed at
// precUnary so a chain like `++$obj->prop[0]` reaches through member access and
// subscript without being captured by an outer additive operator.
func parseUpdatePrefix(p *parser) Expr {
	op := p.advance()
	operand := p.parseExpression(precUnary)

	return &UpdateExpr{Op: op, Operand: operand, Prefix: true}
}

// parseUpdatePostfix parses the `expr++` / `expr--` tail. Registered at
// precCall so it binds tighter than every binary operator and runs after
// subscript and member access on the same band.
func parseUpdatePostfix(p *parser, left Expr) Expr {
	op := p.advance()

	return &UpdateExpr{Op: op, Operand: left, Prefix: false}
}

func parseLogicalNot(p *parser) Expr {
	op := p.advance()
	operand := p.parseExpression(precLogicalNot)

	return &UnaryExpr{Op: op, Expr: operand}
}

func parseBinary(p *parser, left Expr) Expr {
	op := p.advance()
	rule := rules[op.Kind]
	minPrec := rule.prec

	if rule.rightAssoc {
		minPrec--
	}

	right := p.parseExpression(minPrec)

	return &BinaryExpr{Left: left, Op: op, Right: right}
}

func parseAssign(p *parser, left Expr) Expr {
	op := p.advance()
	byRef := op.Kind == KindEq && p.peekKind() == KindAmp

	if byRef {
		p.advance()
	}

	if nonLvalue(left) {
		p.addError(left.Span().Start, "cannot assign to this expression")
	}

	right := p.parseExpression(precAssign - 1)

	return &AssignExpr{Left: left, Op: op, Right: right, ByRef: byRef}
}

// nonLvalue reports whether e can never be an assignment target, so `e = ...`
// is a PHP compile error ("Can't use function return value in write context"
// and kin). Rejecting it keeps the formatter from laying out code that cannot
// run. It is deliberately a blacklist of the certain non-targets, so an unknown
// or edge lvalue is accepted, never wrongly rejected.
func nonLvalue(e Expr) bool {
	switch n := e.(type) {
	case *FirstClassCallableExpr, *CallExpr,
		*IntExpr, *FloatExpr, *StringExpr, *BoolExpr, *NullExpr, *ShellExecExpr,
		*NewExpr, *AnonClassExpr,
		*IdentExpr, // a bare or qualified name is a constant fetch, never a target
		*UpdateExpr:
		return true
	case *UnaryExpr:
		// PHP reads `!$a = 1` as `!($a = 1)`, so what has to be assignable is the
		// operand, not the unary expression this parser builds around it.
		return nonLvalue(n.Expr)
	}

	return false
}

func parseParenOrCast(p *parser) Expr {
	if p.isCast() {
		return p.parseCast()
	}

	lp := p.advance()
	inner := p.parseExpression(precNone)
	rp := p.expect(KindRParen)

	return &ParenExpr{LParen: lp, Expr: inner, RParen: rp}
}

func (p *parser) isCast() bool {
	if p.peekKindAt(2) != KindRParen {
		return false
	}

	typeTok := p.tokens[p.pos+1]

	switch typeTok.Kind {
	case KindArray:
		return true
	case KindIdent:
		return isCastTypeName(string(typeTok.Text))
	}

	return false
}

func (p *parser) parseCast() Expr {
	lp := p.advance()
	typeTok := p.advance()
	rp := p.advance()
	operand := p.parseExpression(precUnary)

	return &CastExpr{LParen: lp, Type: typeTok, RParen: rp, Expr: operand}
}

func isCastTypeName(s string) bool {
	switch strings.ToLower(s) {
	case "int", "integer", "float", "double", "string", "binary", "bool", "boolean", "object", "void":
		return true
	}

	return false
}

func parseCall(p *parser, callee Expr) Expr {
	lp := p.advance()

	if p.peekKind() == KindEllipsis && p.peekKindAt(1) == KindRParen {
		ell := p.advance()
		rp := p.expect(KindRParen)

		return &FirstClassCallableExpr{
			Callee: callee, LParen: lp,
			Ellipsis: ell, RParen: rp,
		}
	}

	args := p.parseCallArgs(KindRParen)
	rp := p.expect(KindRParen)

	return &CallExpr{Callee: callee, LParen: lp, Args: args, RParen: rp}
}

// parseCommaList accepts an empty list and a trailing comma, and fills commas
// when it is non-nil. PHP lists that reject a trailing comma (`global`,
// `const`, `echo`, `declare`, a name list) keep their own loop.
func (p *parser) parseCommaList[T any](end Kind, commas *[]Token, one func() T) []T {
	if p.peekKind() == end {
		return nil
	}

	items := []T{one()}

	for p.peekKind() == KindComma {
		comma := p.advance()

		if commas != nil {
			*commas = append(*commas, comma)
		}

		if p.peekKind() == end {
			break
		}

		items = append(items, one())
	}

	return items
}

func (p *parser) parseCallArgs(end Kind) []Expr {
	return p.parseCommaList(end, nil, p.parseCallArg)
}

func (p *parser) parseCallArg() Expr {
	if p.peekKind() == KindEllipsis {
		ell := p.advance()
		inner := p.parseExpression(precNone)

		return &SpreadExpr{Ellipsis: ell, Expr: inner}
	}

	if isMemberNameToken(p.peekKind()) && p.peekKindAt(1) == KindColon {
		name := p.advance()
		colon := p.advance()
		value := p.parseExpression(precNone)

		return &NamedArgExpr{Name: name, Colon: colon, Value: value}
	}

	return p.parseExpression(precNone)
}

func (p *parser) parseHaltCompilerStmt() Stmt {
	stmt := &HaltCompilerStmt{Token: p.advance()}

	if p.peekKind() == KindHaltPayload {
		stmt.Payload = p.advance()
	}

	return stmt
}
