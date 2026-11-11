package php

import (
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

func (b *builder) functionDecl(n *parser.FunctionDecl) *document.Doc {
	byRef := document.Text("")
	if has(n.ByRef) {
		byRef = document.Text("&")
	}

	params, paramsID := b.paramsID(n.Params, n.RParen)

	sig := document.Concat(
		b.attributes(n.Attributes), document.Text("function "), byRef, b.tok(n.Name), params,
		b.returnType(n.ReturnColon, n.ReturnType),
	)

	return document.Concat(sig, b.signatureBlock(b.cfg.PHP.FunctionBraceStyle, b.cfg.PHP.InlineEmptyFunctionBodies, paramsID, n.Body))
}

// signatureBlock is braceBlock for a function or method, where the brace hugs
// a signature that wrapped (PHP-CS-Fixer's
// next_line_unless_newline_at_signature_end). paramsID is the group holding
// the parameter list, or 0 when no list could break. always-next-line is the
// style that never hugs.
func (b *builder) signatureBlock(style string, inlineEmpty bool, paramsID int32, blk *parser.BlockStmt) *document.Doc {
	if style != config.BraceStyleNextLine || paramsID == 0 {
		return b.braceBlock(style, blk, inlineEmpty)
	}

	lead := document.IfBreakOf(paramsID, document.Text(" "), document.HardLine())

	if blk == nil || (len(blk.Stmts) == 0 && b.danglingComments(blk.RBrace) == nil) {
		if blk == nil || inlineEmpty {
			return document.Text(" {}")
		}

		return document.Concat(lead, emptyBracesOnTwoLines())
	}

	if len(blk.Stmts) == 0 {
		return document.Concat(lead, b.danglingComments(blk.RBrace))
	}

	return document.Concat(lead, b.block(blk))
}

func (b *builder) params(ps []*parser.Param, rparen parser.Token) *document.Doc {
	doc, _ := b.paramsID(ps, rparen)

	return doc
}

// paramsID is params plus the id of the group that wraps the list, so a caller
// can ask whether the signature wrapped. The id is 0 when there is no list to
// break, which reads as "did not break".
func (b *builder) paramsID(ps []*parser.Param, rparen parser.Token) (*document.Doc, int32) {
	if len(ps) == 0 {
		if dangling := b.emptyListComments(rparen); dangling != nil {
			return dangling, 0
		}

		return document.Text("()"), 0
	}

	docs, comment, breaks := b.paramParts(ps, rparen)
	id := b.nextGroupID()

	return delimitedListComment("(", docs, withTrailingComma, id, comment, breaks), id
}

// paramParts builds a non-empty parameter list's pieces: each parameter, the
// comment trailing its `)`, and whether the list must break.
//
// Every comment keeps its side of each comma: PHP 8.6 gives a parameter the
// last doc comment read before the token after it, so `$a /** x */,` documents
// $a while `$a, /** x */` documents the next parameter or, before `)`, nothing.
func (b *builder) paramParts(ps []*parser.Param, rparen parser.Token) ([]*document.Doc, *document.Doc, bool) {
	docs := make([]*document.Doc, len(ps))
	commented := false
	var next parser.Token
	var moved []parser.Trivia

	for i, p := range ps {
		lead, trail := b.paramLeadComments(moved, paramStart(p), i == 0)
		if trail != nil {
			docs[i-1] = document.Concat(docs[i-1], document.LineSuffix(trail), document.BreakParent())
			commented = true
		}

		next = b.tokenFrom(p.Span().End.Offset)

		var glued *document.Doc
		glued, moved = b.paramCommaComments(next)
		docs[i] = document.Concat(lead, b.param(p), glued)
	}

	comment, breaks := b.trailingParenComment(rparen)

	// Line comments moved past a trailing comma have no parameter after them,
	// so they lead the comments before `)`.
	if len(moved) > 0 {
		comment, breaks = b.movedBeforeParen(moved, rparen), true
	}

	// Promoted properties go one per line, as in the Laravel docs and
	// PHP-CS-Fixer's multiline_promoted_properties. A comment after an authored
	// trailing comma keeps the list broken, since the flat form drops the comma.
	breaks = breaks || commented || comment != nil && next.Kind == parser.KindComma ||
		b.authorBroke(paramStart(ps[0])) || slices.ContainsFunc(ps, isPromoted)

	return docs, comment, breaks
}

func (b *builder) movedBeforeParen(moved []parser.Trivia, rparen parser.Token) *document.Doc {
	parts := make([]*document.Doc, 0, len(moved)*2)
	parts = append(parts, b.trailingComment(moved[0]))

	for _, tr := range moved[1:] {
		parts = append(parts, document.HardLine(), b.commentVerbatim(tr))
	}

	for _, c := range b.allComments(rparen.Span.Start.Offset) {
		parts = append(parts, document.HardLine(), c)
	}

	return document.Concat(parts...)
}

// paramLeadComments places the comments before a parameter in source order.
// Once one ends its line, every later one goes above the parameter, so none
// lands behind a line comment.
func (b *builder) paramLeadComments(moved []parser.Trivia, offset int, first bool) (lead, trail *document.Doc) {
	var leads []*document.Doc
	ownLine := first

	place := func(tr parser.Trivia, ends bool) {
		switch {
		case !ends:
			leads = append(leads, b.inlineComment(tr))
		case ownLine || len(leads) > 0:
			leads = append(leads, b.leadComment(tr), document.HardLine())
		default:
			trail = b.trailingComment(tr)
		}

		ownLine = ownLine || ends
	}

	for _, tr := range moved {
		place(tr, true)
	}

	trivia := b.leadingAt(offset)

	for i, tr := range trivia {
		if tr.Kind == parser.TriviaWhitespace {
			ownLine = ownLine || triviaLineBreaks(tr.Text) > 0

			continue
		}

		if !isCommentTrivia(tr.Kind) || b.emitted[tr.Span.Start.Offset] {
			continue
		}

		b.claim(tr)
		place(tr, endsLine(trivia, i))
	}

	return document.Concat(leads...), trail
}

// paramCommaComments keeps the comments before a comma or `)` in front of it,
// except line comments after the last block comment, which the comma would
// fall inside: before a comma they come back as moved, before `)`
// trailingParenComment takes them.
func (b *builder) paramCommaComments(next parser.Token) (*document.Doc, []parser.Trivia) {
	if next.Kind != parser.KindComma && next.Kind != parser.KindRParen {
		return document.Text(""), nil
	}

	trivia := b.leadingAt(next.Span.Start.Offset)
	lastBlock := -1

	for i, tr := range trivia {
		if isCommentTrivia(tr.Kind) && tr.Kind != parser.TriviaLineComment && !b.emitted[tr.Span.Start.Offset] {
			lastBlock = i
		}
	}

	var parts []*document.Doc
	var moved []parser.Trivia
	newline := false

	for i, tr := range trivia {
		if tr.Kind == parser.TriviaWhitespace {
			newline = newline || triviaLineBreaks(tr.Text) > 0

			continue
		}

		if !isCommentTrivia(tr.Kind) || b.emitted[tr.Span.Start.Offset] {
			continue
		}

		if i > lastBlock {
			if next.Kind == parser.KindComma {
				b.claim(tr)
				moved = append(moved, tr)
			}

			continue
		}

		b.claim(tr)

		switch {
		case newline:
			parts = append(parts, document.HardLine(), b.commentVerbatim(tr))
		case tr.Kind == parser.TriviaLineComment:
			parts = append(parts, b.trailingComment(tr))
		default:
			parts = append(parts, document.Text(" "), b.commentVerbatim(tr))
		}

		newline = tr.Kind == parser.TriviaLineComment
	}

	return document.Concat(parts...), moved
}

func endsLine(trivia []parser.Trivia, i int) bool {
	if trivia[i].Kind == parser.TriviaLineComment {
		return true
	}

	return i+1 < len(trivia) && trivia[i+1].Kind == parser.TriviaWhitespace && triviaLineBreaks(trivia[i+1].Text) > 0
}

func isPromoted(p *parser.Param) bool {
	return len(p.Modifiers) > 0
}

func (b *builder) param(p *parser.Param) *document.Doc {
	parts := make([]*document.Doc, 0, 6+len(p.Attributes)*2)

	for _, g := range p.Attributes {
		parts = append(parts, b.attributeGroup(g), document.Line())
	}

	parts = append(parts, b.modifiers(p.Modifiers))

	if p.Type != nil {
		parts = append(parts, b.typeHint(p.Type), document.Text(" "))
	}

	if has(p.ByRef) {
		parts = append(parts, document.Text("&"))
	}

	if has(p.Variadic) {
		parts = append(parts, document.Text("..."))
	}

	parts = append(parts, b.tok(p.Var))

	if p.Default != nil {
		parts = append(parts, document.Text(" = "), b.expr(p.Default))
	}

	if len(p.Hooks) > 0 {
		parts = append(parts, b.propertyHooks(p.Hooks))
	}

	return document.Concat(parts...)
}

func paramStart(p *parser.Param) int {
	if len(p.Attributes) > 0 {
		return p.Attributes[0].Span().Start.Offset
	}

	if len(p.Modifiers) > 0 {
		return p.Modifiers[0].Keyword.Span.Start.Offset
	}

	if p.Type != nil {
		return p.Type.Span().Start.Offset
	}

	for _, tok := range [...]parser.Token{p.ByRef, p.Variadic, p.Var} {
		if has(tok) {
			return tok.Span.Start.Offset
		}
	}

	return -1
}

func (b *builder) returnType(colon parser.Token, t parser.TypeHint) *document.Doc {
	if !has(colon) || t == nil {
		return document.Text("")
	}

	return document.Concat(document.Text(": "), b.typeHint(t))
}

func (b *builder) modifiers(mods []parser.Modifier) *document.Doc {
	if len(mods) == 0 {
		return document.Text("")
	}

	parts := make([]*document.Doc, 0, len(mods))

	appendMod := func(m *parser.Modifier) {
		mod := document.Text(strings.ToLower(string(m.Keyword.Text)))
		if has(m.RParen) {
			mod = document.Concat(mod, document.Text("("), b.tok(m.Set), document.Text(")"))
		}

		parts = append(parts, mod, document.Text(" "))
	}

	// A comment between modifiers cannot be carried across a reorder or a lowercase
	// rewrite, so emit the whole run verbatim: source order and the comment are
	// preserved, and the result is a stable fixed point.
	if modifiersHaveInnerComment(mods) {
		first := mods[0].Keyword.Span.Start.Offset
		lastMod := &mods[len(mods)-1]

		last := lastMod.Keyword.Span.End.Offset
		if has(lastMod.RParen) {
			last = lastMod.RParen.Span.End.Offset
		}

		return document.Concat(document.Text(string(b.src[first:last])), document.Text(" "))
	}

	for rank := 0; rank <= modifierRankMax; rank++ {
		for i := range mods {
			if modifierRank(&mods[i]) == rank {
				appendMod(&mods[i])
			}
		}
	}

	return document.Concat(parts...)
}

// modifierRank orders member modifiers per PER Coding Style 3.0 §4.6:
// abstract|final, visibility, set-visibility, static, readonly. Unknown
// keywords sort last in source order, keeping the comparator total.
func modifierRank(m *parser.Modifier) int {
	switch strings.ToLower(string(m.Keyword.Text)) {
	case "abstract", "final":
		return 0
	case "public", "protected", "private":
		if m.RParen.Kind != parser.KindInvalid { // `(set)` asymmetric visibility
			return 2
		}

		return 1
	case "static":
		return 3
	case "readonly":
		return 4
	default:
		return 5
	}
}

const modifierRankMax = 5

func modifiersHaveInnerComment(mods []parser.Modifier) bool {
	for i := 1; i < len(mods); i++ {
		for _, tr := range mods[i].Keyword.Leading {
			if tr.Kind != parser.TriviaWhitespace {
				return true
			}
		}
	}

	return false
}

func (b *builder) attributes(groups []*parser.AttributeGroup) *document.Doc {
	if len(groups) == 0 {
		return document.Text("")
	}

	parts := make([]*document.Doc, 0, len(groups)*2)
	for _, g := range groups {
		parts = append(parts, b.attributeGroup(g), document.HardLine())
	}

	return document.Concat(parts...)
}

// inlineAttributes renders the attribute groups of a closure or arrow function,
// which stay on its line: `#[Pure] fn ($x) => $x * 2`.
func (b *builder) inlineAttributes(groups []*parser.AttributeGroup) []*document.Doc {
	parts := make([]*document.Doc, 0, len(groups)*2)
	for _, g := range groups {
		parts = append(parts, b.attributeGroup(g), document.Text(" "))
	}

	return parts
}

// attributeGroup renders `#[...]` with no trailing break, so callers control
// whether it sits inline or on its own line.
func (b *builder) attributeGroup(g *parser.AttributeGroup) *document.Doc {
	attrs := make([]*document.Doc, len(g.Attributes))
	for i, a := range g.Attributes {
		attr := b.name(a.Name)
		if has(a.RParen) {
			attr = document.Concat(attr, b.argList(a.Args, withTrailingComma))
		}

		attrs[i] = attr
	}

	return document.Concat(document.Text("#["), document.Join(document.Text(", "), attrs), document.Text("]"))
}

func (b *builder) classStmt(n *parser.ClassStmt) *document.Doc {
	head := make([]*document.Doc, 0, 9)
	head = append(head, b.attributes(n.Attributes), b.modifiers(n.Modifiers), document.Text("class "), b.tok(n.Name))

	if n.Parent != nil {
		head = append(head, document.Text(" extends "), b.name(n.Parent))
	}

	if len(n.ImplementsNames) > 0 {
		head = append(head, document.Text(" implements "), b.joinNames(n.ImplementsNames))
	}

	return document.Concat(document.Concat(head...), b.braceMembers(b.cfg.PHP.ClasslikeBraceStyle, n.Members, n.RBrace, b.cfg.PHP.InlineEmptyClassBodies))
}

func (b *builder) anonClass(n *parser.AnonClassExpr) *document.Doc {
	head := make([]*document.Doc, 0, 9)
	head = append(head, document.Text("new "))

	for _, g := range n.Attributes {
		head = append(head, b.attributeGroup(g), document.Text(" "))
	}

	head = append(head, b.modifiers(n.Modifiers), document.Text("class"))

	if has(n.LParen) {
		if b.cfg.PHP.AnonClassParenSpace {
			head = append(head, document.Text(" "))
		}

		head = append(head, b.argList(n.Args, withTrailingComma))
	}

	if n.Parent != nil {
		head = append(head, document.Text(" extends "), b.name(n.Parent))
	}

	if len(n.ImplementsNames) > 0 {
		head = append(head, document.Text(" implements "), b.joinNames(n.ImplementsNames))
	}

	return document.Concat(document.Concat(head...), b.braceMembers(b.cfg.PHP.AnonClassBraceStyle, n.Members, n.RBrace, b.cfg.PHP.InlineEmptyAnonClassBodies))
}

func (b *builder) interfaceStmt(n *parser.InterfaceStmt) *document.Doc {
	head := document.Concat(b.attributes(n.Attributes), document.Text("interface "), b.tok(n.Name))

	if len(n.ExtendsNames) > 0 {
		head = document.Concat(head, document.Text(" extends "), b.joinNames(n.ExtendsNames))
	}

	return document.Concat(head, b.braceMembers(b.cfg.PHP.ClasslikeBraceStyle, n.Members, n.RBrace, b.cfg.PHP.InlineEmptyClassBodies))
}

func (b *builder) traitStmt(n *parser.TraitStmt) *document.Doc {
	head := document.Concat(b.attributes(n.Attributes), document.Text("trait "), b.tok(n.Name))

	return document.Concat(head, b.braceMembers(b.cfg.PHP.ClasslikeBraceStyle, n.Members, n.RBrace, b.cfg.PHP.InlineEmptyClassBodies))
}

func (b *builder) enumStmt(n *parser.EnumStmt) *document.Doc {
	head := document.Concat(b.attributes(n.Attributes), document.Text("enum "), b.tok(n.Name))

	if has(n.BackingColon) {
		head = document.Concat(head, document.Text(": "), b.typeHint(n.BackingType))
	}

	if len(n.ImplementsNames) > 0 {
		head = document.Concat(head, document.Text(" implements "), b.joinNames(n.ImplementsNames))
	}

	return document.Concat(head, b.braceMembers(b.cfg.PHP.ClasslikeBraceStyle, n.Members, n.RBrace, b.cfg.PHP.InlineEmptyClassBodies))
}

func (b *builder) joinNames(names []*parser.Name) *document.Doc {
	docs := make([]*document.Doc, len(names))
	for i, nm := range names {
		docs[i] = b.name(nm)
	}

	return document.Join(document.Text(", "), docs)
}

func (b *builder) classBody(members []parser.ClassMember, rbrace parser.Token) *document.Doc {
	if len(members) == 0 {
		return document.Text("{}")
	}

	spans := make([]parser.Span, len(members))
	docs := make([]*document.Doc, len(members))

	b.depth++
	for i, m := range members {
		spans[i] = m.Span()
		docs[i] = b.classMember(m)
	}
	b.depth--

	body := b.buildSequence(spans, docs, b.memberForceBlank(members), true)

	if tail := b.bodyTailComments(rbrace); tail != nil {
		body = document.Concat(body, tail)
	}

	return document.Concat(document.Text("{"), document.Indent(body), document.HardLine(), document.Text("}"))
}

func (b *builder) braceMembers(style string, members []parser.ClassMember, rbrace parser.Token, inlineEmpty bool) *document.Doc {
	if len(members) == 0 {
		if dangling := b.danglingComments(rbrace); dangling != nil {
			return b.braceBody(style, dangling)
		}

		if !inlineEmpty {
			return b.braceBody(style, emptyBracesOnTwoLines())
		}

		return document.Text(" {}")
	}

	return b.braceBody(style, b.classBody(members, rbrace))
}

func (b *builder) classMember(m parser.ClassMember) *document.Doc {
	return b.keepComments(m.Span(), b.layoutClassMember(m), m)
}

func (b *builder) layoutClassMember(m parser.ClassMember) *document.Doc {
	switch n := m.(type) {
	case *parser.MethodDecl:
		return b.methodDecl(n)
	case *parser.PropertyDecl:
		return b.propertyDecl(n)
	case *parser.ClassConstDecl:
		return document.Concat(b.attributes(n.Attributes), b.classConstDecl(n))
	case *parser.EnumCase:
		return document.Concat(b.attributes(n.Attributes), b.enumCase(n))
	case *parser.TraitUseStmt:
		return b.traitUseStmt(n)
	}

	return b.verbatim(m.Span())
}

func (b *builder) methodDecl(n *parser.MethodDecl) *document.Doc {
	byRef := document.Text("")
	if has(n.ByRef) {
		byRef = document.Text("&")
	}

	params, paramsID := b.paramsID(n.Params, n.RParen)

	sig := document.Concat(
		b.attributes(n.Attributes), b.modifiers(n.Modifiers), document.Text("function "), byRef, b.tok(n.Name),
		params, b.returnType(n.ReturnColon, n.ReturnType),
	)

	if n.Body == nil {
		return document.Concat(sig, document.Text(";"))
	}

	return document.Concat(sig, b.signatureBlock(b.cfg.PHP.MethodBraceStyle, b.cfg.PHP.InlineEmptyMethodBodies, paramsID, n.Body))
}

func (b *builder) propertyDecl(n *parser.PropertyDecl) *document.Doc {
	parts := make([]*document.Doc, 0, 5)
	parts = append(parts, b.attributes(n.Attributes), b.modifiers(n.Modifiers))

	if n.Type != nil {
		parts = append(parts, b.typeHint(n.Type), document.Text(" "))
	}

	items := make([]*document.Doc, len(n.Items))
	for i, it := range n.Items {
		item := b.tok(it.Var)
		if it.Default != nil {
			item = document.Concat(item, document.Text(" = "), b.expr(it.Default))
		}

		items[i] = item
	}

	parts = append(parts, document.Join(document.Text(", "), items))

	if len(n.Hooks) > 0 {
		parts = append(parts, b.propertyHooks(n.Hooks))
	} else {
		parts = append(parts, document.Text(";"))
	}

	return document.Concat(parts...)
}

func (b *builder) propertyHooks(hooks []*parser.PropertyHook) *document.Doc {
	if allAbstractHooks(hooks) {
		parts := make([]*document.Doc, 0, len(hooks)+2)
		parts = append(parts, document.Text(" {"))

		for _, h := range hooks {
			parts = append(parts, document.Text(" "), b.propertyHook(h))
		}

		parts = append(parts, document.Text(" }"))

		return document.Concat(parts...)
	}

	inner := make([]*document.Doc, 0, len(hooks)*2)
	for _, h := range hooks {
		inner = append(inner, document.HardLine(), b.propertyHook(h))
	}

	return document.Concat(document.Text(" {"), document.Indent(document.Concat(inner...)), document.HardLine(), document.Text("}"))
}

// allAbstractHooks reports whether every hook is abstract (a bare `get;`/`set;`
// with no body, expression, or attribute). Such a list collapses onto the
// property line (PER-CS 4.10); any concrete or attributed hook keeps it expanded.
func allAbstractHooks(hooks []*parser.PropertyHook) bool {
	for _, h := range hooks {
		if h.Body != nil || h.Expr != nil || len(h.Attributes) > 0 {
			return false
		}
	}

	return true
}

func (b *builder) propertyHook(h *parser.PropertyHook) *document.Doc {
	parts := make([]*document.Doc, 0, 7)
	parts = append(parts, b.attributes(h.Attributes))

	if has(h.Final) {
		parts = append(parts, document.Text("final "))
	}

	if has(h.Ref) {
		parts = append(parts, document.Text("&"))
	}

	parts = append(parts, b.tok(h.Name))

	if has(h.LParen) {
		parts = append(parts, b.params(h.Params, h.RParen))
	}

	switch {
	case h.Body != nil:
		parts = append(parts, b.braceBody(config.BraceStyleSameLine, b.block(h.Body)))
	case h.Expr != nil:
		parts = append(parts, document.Text(" => "), b.expr(h.Expr), document.Text(";"))
	default:
		parts = append(parts, document.Text(";"))
	}

	return document.Concat(parts...)
}

func (b *builder) classConstDecl(n *parser.ClassConstDecl) *document.Doc {
	docs := make([]*document.Doc, len(n.Items))
	for i, it := range n.Items {
		docs[i] = document.Concat(b.tok(it.Name), document.Text(" = "), b.expr(it.Value))
	}

	head := document.Concat(b.modifiers(n.Modifiers), document.Text("const "))
	if n.Type != nil {
		head = document.Concat(b.modifiers(n.Modifiers), document.Text("const "), b.typeHint(n.Type), document.Text(" "))
	}

	return document.Concat(head, document.Join(document.Text(", "), docs), document.Text(";"))
}

func (b *builder) enumCase(n *parser.EnumCase) *document.Doc {
	head := document.Concat(document.Text("case "), b.tok(n.Name))
	if n.Value != nil {
		head = document.Concat(head, document.Text(" = "), b.expr(n.Value))
	}

	return document.Concat(head, document.Text(";"))
}

func (b *builder) traitUseStmt(n *parser.TraitUseStmt) *document.Doc {
	names := make([]*document.Doc, len(n.Traits))
	for i, t := range n.Traits {
		names[i] = b.name(t)
	}

	if !has(n.LBrace) && b.cfg.PHP.SplitTraitUses {
		parts := make([]*document.Doc, 0, len(names)*3)
		for i, nm := range names {
			if i > 0 {
				parts = append(parts, document.HardLine())
			}

			parts = append(parts, document.Text("use "), nm, document.Text(";"))
		}

		return document.Concat(parts...)
	}

	use := document.Concat(document.Text("use "), document.Join(document.Text(", "), names))

	if !has(n.LBrace) {
		return document.Concat(use, document.Text(";"))
	}

	return document.Concat(use, document.Text(" "), b.verbatim(adaptationSpan(n)))
}

func adaptationSpan(n *parser.TraitUseStmt) parser.Span {
	return parser.Span{Start: n.LBrace.Span.Start, End: n.RBrace.Span.End}
}
