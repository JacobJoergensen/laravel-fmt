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

	return document.Concat(sig, b.signatureBlock(b.cfg.PHP.FunctionBraceStyle, paramsID, n.Body))
}

// signatureBlock is braceBlock for a function or method, where the brace hugs
// a signature that wrapped (PHP-CS-Fixer's
// next_line_unless_newline_at_signature_end). paramsID is the group holding
// the parameter list, or 0 when no list could break. always-next-line is the
// style that never hugs.
func (b *builder) signatureBlock(style string, paramsID int32, blk *parser.BlockStmt) *document.Doc {
	if style != config.BraceStyleNextLine || paramsID == 0 {
		return b.braceBlock(style, blk)
	}

	if blk == nil || (len(blk.Stmts) == 0 && b.danglingComments(blk.RBrace) == nil) {
		return document.Text(" {}")
	}

	lead := document.IfBreakOf(paramsID, document.Text(" "), document.HardLine())

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
func (b *builder) paramParts(ps []*parser.Param, rparen parser.Token) ([]*document.Doc, *document.Doc, bool) {
	docs := make([]*document.Doc, len(ps))
	for i, p := range ps {
		docs[i] = b.param(p)
	}

	comment, breaks := b.trailingParenComment(rparen)

	// Promoted properties go one per line, as in the Laravel docs and
	// PHP-CS-Fixer's multiline_promoted_properties.
	breaks = breaks || b.authorBroke(paramStart(ps[0])) || slices.ContainsFunc(ps, isPromoted)

	return docs, comment, breaks
}

func isPromoted(p *parser.Param) bool {
	return len(p.Modifiers) > 0
}

func (b *builder) param(p *parser.Param) *document.Doc {
	parts := make([]*document.Doc, 0, 6+len(p.Attributes)*2)

	// A parameter is not an Expr, so exprLeadComments never sees its leading
	// comments.
	if lead := b.leadCommentsAt(paramStart(p)); lead != nil {
		parts = append(parts, lead)
	}

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

	return document.Concat(document.Concat(head...), b.braceMembers(b.cfg.PHP.ClasslikeBraceStyle, n.Members, n.RBrace))
}

func (b *builder) anonClass(n *parser.AnonClassExpr) *document.Doc {
	head := make([]*document.Doc, 0, 9)
	head = append(head, document.Text("new "))

	for _, g := range n.Attributes {
		head = append(head, b.attributeGroup(g), document.Text(" "))
	}

	head = append(head, b.modifiers(n.Modifiers), document.Text("class"))

	if has(n.LParen) {
		head = append(head, b.argList(n.Args, withTrailingComma))
	}

	if n.Parent != nil {
		head = append(head, document.Text(" extends "), b.name(n.Parent))
	}

	if len(n.ImplementsNames) > 0 {
		head = append(head, document.Text(" implements "), b.joinNames(n.ImplementsNames))
	}

	return document.Concat(document.Concat(head...), b.braceMembers(b.cfg.PHP.AnonClassBraceStyle, n.Members, n.RBrace))
}

func (b *builder) interfaceStmt(n *parser.InterfaceStmt) *document.Doc {
	head := document.Concat(b.attributes(n.Attributes), document.Text("interface "), b.tok(n.Name))

	if len(n.ExtendsNames) > 0 {
		head = document.Concat(head, document.Text(" extends "), b.joinNames(n.ExtendsNames))
	}

	return document.Concat(head, b.braceMembers(b.cfg.PHP.ClasslikeBraceStyle, n.Members, n.RBrace))
}

func (b *builder) traitStmt(n *parser.TraitStmt) *document.Doc {
	head := document.Concat(b.attributes(n.Attributes), document.Text("trait "), b.tok(n.Name))

	return document.Concat(head, b.braceMembers(b.cfg.PHP.ClasslikeBraceStyle, n.Members, n.RBrace))
}

func (b *builder) enumStmt(n *parser.EnumStmt) *document.Doc {
	head := document.Concat(b.attributes(n.Attributes), document.Text("enum "), b.tok(n.Name))

	if has(n.BackingColon) {
		head = document.Concat(head, document.Text(": "), b.typeHint(n.BackingType))
	}

	if len(n.ImplementsNames) > 0 {
		head = document.Concat(head, document.Text(" implements "), b.joinNames(n.ImplementsNames))
	}

	return document.Concat(head, b.braceMembers(b.cfg.PHP.ClasslikeBraceStyle, n.Members, n.RBrace))
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

func (b *builder) braceMembers(style string, members []parser.ClassMember, rbrace parser.Token) *document.Doc {
	if len(members) == 0 {
		if dangling := b.danglingComments(rbrace); dangling != nil {
			return b.braceBody(style, dangling)
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

	return document.Concat(sig, b.signatureBlock(b.cfg.PHP.MethodBraceStyle, paramsID, n.Body))
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
