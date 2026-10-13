package php

import (
	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

func (b *builder) block(blk *parser.BlockStmt) *document.Doc {
	if blk == nil {
		return document.Text("{}")
	}

	if len(blk.Stmts) == 0 {
		if dangling := b.danglingComments(blk.RBrace); dangling != nil {
			return dangling
		}

		return document.Text("{}")
	}

	b.depth++
	body := b.stmtList(blk.Stmts)
	b.depth--

	if tail := b.bodyTailComments(blk.RBrace); tail != nil {
		body = document.Concat(body, tail)
	}

	return document.Concat(document.Text("{"), document.Indent(body), document.HardLine(), document.Text("}"))
}

func (b *builder) stmtList(stmts []parser.Stmt) *document.Doc {
	stmts = b.preprocessUseRuns(stmts)

	spans := make([]parser.Span, len(stmts))
	docs := make([]*document.Doc, len(stmts))

	for i, s := range stmts {
		spans[i] = s.Span()
		docs[i] = b.stmt(s)
	}

	return b.buildSequence(spans, docs, b.stmtForceBlank(stmts), true)
}

func (b *builder) stmtAsBlock(s parser.Stmt) *document.Doc {
	if blk, ok := s.(*parser.BlockStmt); ok {
		return b.controlBlock(blk)
	}

	b.depth++
	body := b.stmt(s)
	b.depth--

	return document.Concat(document.Text("{"), document.Indent(document.Concat(document.HardLine(), body)), document.HardLine(), document.Text("}"))
}

func (b *builder) braceBody(style string, body *document.Doc) *document.Doc {
	if style == config.BraceStyleNextLine || style == config.BraceStyleAlwaysNextLine {
		return document.Concat(document.HardLine(), body)
	}

	return document.Concat(document.Text(" "), body)
}

func (b *builder) braceBlock(style string, blk *parser.BlockStmt) *document.Doc {
	if blk == nil {
		return document.Text(" {}")
	}

	if len(blk.Stmts) == 0 {
		if dangling := b.danglingComments(blk.RBrace); dangling != nil {
			return b.braceBody(style, dangling)
		}

		return document.Text(" {}")
	}

	return b.braceBody(style, b.block(blk))
}

func (b *builder) controlBrace(body *document.Doc) *document.Doc {
	return b.braceBody(b.cfg.PHP.ControlBraceStyle, body)
}

func (b *builder) controlLead() *document.Doc {
	if b.cfg.PHP.ControlBraceStyle == config.BraceStyleSameLine {
		return document.Text(" ")
	}

	return document.HardLine()
}

// A bare `;` body stays on the header: in braces it is an empty statement,
// which Pint removes.
func (b *builder) controlBody(s parser.Stmt) *document.Doc {
	if _, ok := s.(*parser.EmptyStmt); ok {
		return document.Text(";")
	}

	return b.controlBrace(b.stmtAsBlock(s))
}

func (b *builder) leadAfter(body parser.Stmt) *document.Doc {
	if _, ok := body.(*parser.EmptyStmt); ok {
		return document.HardLine()
	}

	return b.controlLead()
}

// controlBlock is block for a control structure, whose empty body stays on two
// lines; only class-likes and functions collapse to `{}`.
func (b *builder) controlBlock(blk *parser.BlockStmt) *document.Doc {
	if blk != nil && len(blk.Stmts) == 0 && b.danglingComments(blk.RBrace) == nil {
		return document.Concat(document.Text("{"), document.HardLine(), document.Text("}"))
	}

	return b.block(blk)
}

// header builds `keyword (cond)`. When the header is too wide, cond moves onto
// its own indented line, unless subject hugs (HeaderHugs) and breaks inside
// its own brackets instead. A for header passes a nil subject.
func (b *builder) header(keyword string, cond *document.Doc, subject parser.Expr, rparen parser.Token) *document.Doc {
	comment, breaks := b.trailingParenComment(rparen)
	if comment == nil && HeaderHugs(subject) {
		return document.Concat(document.Text(keyword+" ("), cond, document.Text(")"))
	}

	if comment != nil {
		cond = document.Concat(cond, comment)
	}

	line := document.SoftLine
	if breaks {
		line = document.HardLine
	}

	// A condition that stays on the header line while breaking inside (a hugged
	// closure or array) continues that line, so it takes the indent only when
	// the header itself breaks.
	id := b.nextGroupID()
	parts := []*document.Doc{
		document.Text(keyword + " ("),
		document.IfBreakOf(id, document.Indent(document.Concat(line(), cond)), cond),
		line(),
		document.Text(")"),
	}

	// A break inside IfBreakOf does not propagate; restate it.
	if document.HasForcedBreak(cond) {
		parts = append(parts, document.BreakParent())
	}

	return document.GroupID(id, document.Concat(parts...))
}

// HeaderHugs reports whether a control header's subject opens its own bracket on
// the header line, `foreach ([` or `if (! in_array(`, rather than moving onto an
// indented line of its own. A leading `!` and a plain `$x =` are looked through;
// a binary or a fluent chain has no bracket to open, so it does not hug. A
// property path ending in one call (`$this->repo->find(…)`) is a call, not a
// chain, even though isHuggableArg counts its property links as chain steps.
func HeaderHugs(subject parser.Expr) bool {
	for {
		switch n := subject.(type) {
		case nil:
			return false
		case *parser.UnaryExpr:
			if n.Op.Kind != parser.KindBang {
				return false
			}

			subject = n.Expr
		case *parser.AssignExpr:
			if n.Op.Kind != parser.KindEq || n.ByRef {
				return false
			}

			subject = n.Right
		case *parser.CallExpr:
			if chainCallCount(n) > 1 || len(n.Args) == 0 {
				return false
			}

			// A lone function literal breaks cleanly onto its own line here, though
			// isHuggableArg keeps it from pulling an outer argument list open.
			last := n.Args[len(n.Args)-1]

			return len(n.Args) >= 2 || isHuggableArg(last) || isBlockLikeArg(last)
		default:
			return isHuggableArg(subject)
		}
	}
}

func chainCallCount(call *parser.CallExpr) int {
	count := 0

	for _, step := range collectChainSpine(call) {
		if step.call != nil {
			count++
		}
	}

	return count
}

// condition builds a control structure's condition without the binary's own
// continuation indent: the header already indents it, and PSR-12 wants one level.
func (b *builder) condition(e parser.Expr) *document.Doc {
	if n, ok := e.(*parser.BinaryExpr); ok {
		return b.binaryIndented(n, false, false)
	}

	return b.expr(e)
}

func (b *builder) ifStmt(n *parser.IfStmt) *document.Doc {
	if has(n.Colon) {
		return b.verbatim(n.Span())
	}

	parts := []*document.Doc{b.header("if", b.condition(n.Cond), n.Cond, n.RParen), b.controlBody(n.Then)}
	prev := n.Then

	for _, ei := range n.ElseIfs {
		parts = append(parts, b.leadAfter(prev), b.header("elseif", b.condition(ei.Cond), ei.Cond, ei.RParen), b.controlBody(ei.Body))
		prev = ei.Body
	}

	if n.Else != nil {
		parts = append(parts, b.leadAfter(prev), document.Text("else"), b.controlBody(n.Else.Body))
	}

	return document.Concat(parts...)
}

func (b *builder) whileStmt(n *parser.WhileStmt) *document.Doc {
	if has(n.Colon) {
		return b.verbatim(n.Span())
	}

	return document.Concat(b.header("while", b.condition(n.Cond), n.Cond, n.RParen), b.controlBody(n.Body))
}

func (b *builder) doWhileStmt(n *parser.DoWhileStmt) *document.Doc {
	return document.Concat(
		document.Text("do"), b.controlBody(n.Body), b.leadAfter(n.Body),
		b.header("while", b.condition(n.Cond), n.Cond, n.RParen), document.Text(";"),
	)
}

func (b *builder) forStmt(n *parser.ForStmt) *document.Doc {
	if has(n.Colon) {
		return b.verbatim(n.Span())
	}

	return document.Concat(b.header("for", b.forClause(n), nil, n.RParen), b.controlBody(n.Body))
}

// forClause builds what a for's parentheses hold, its three `;`-separated
// expression lists. A Blade @for carries the same clause. Too wide for its
// line, the clause puts each list on its own line (PSR-12 5.4) before any
// expression inside one breaks. An empty last list leaves no space before
// the `)`: `for (;;)` prints as `for (; ;)`.
func (b *builder) forClause(n *parser.ForStmt) *document.Doc {
	parts := []*document.Doc{
		b.exprList(n.Init), document.Text(";"), document.Line(),
		b.exprList(n.Cond), document.Text(";"),
	}

	if len(n.Post) > 0 {
		parts = append(parts, document.Line(), b.exprList(n.Post))
	}

	return document.Group(document.Concat(parts...))
}

func (b *builder) foreachStmt(n *parser.ForEachStmt) *document.Doc {
	if has(n.Colon) {
		return b.verbatim(n.Span())
	}

	return document.Concat(b.header("foreach", b.foreachClause(n), n.Expr, n.RParen), b.controlBody(n.Body))
}

// foreachClause builds what a foreach's parentheses hold, `$items as $k => $v`.
// A Blade @foreach carries the same clause.
func (b *builder) foreachClause(n *parser.ForEachStmt) *document.Doc {
	clause := make([]*document.Doc, 0, 5)
	clause = append(clause, b.expr(n.Expr), document.Text(" as "))

	if n.Key != nil {
		clause = append(clause, b.expr(n.Key), document.Text(" => "))
	}

	if n.ByRef {
		clause = append(clause, document.Text("&"))
	}

	clause = append(clause, b.expr(n.Value))

	return document.Concat(clause...)
}

func (b *builder) switchStmt(n *parser.SwitchStmt) *document.Doc {
	if has(n.Colon) {
		return b.verbatim(n.Span())
	}

	cases := make([]*document.Doc, 0, len(n.Cases)*2)

	savedStmt := b.stmtStart

	for _, c := range n.Cases {
		// A case is neither a statement nor an expression, so its keyword
		// offset owns the comments above it, which also keeps the case's own
		// expression from claiming them.
		offset := c.Keyword.Span.Start.Offset
		lead := b.allComments(offset)
		b.stmtStart = offset

		head := document.Text("default:")
		if !c.IsDefault {
			head = document.Concat(document.Text("case "), b.expr(c.Expr), document.Text(":"))
		}

		b.stmtStart = savedStmt

		for _, comment := range lead {
			cases = append(cases, document.HardLine(), comment)
		}

		body := head
		if len(c.Stmts) > 0 {
			body = document.Concat(head, document.Indent(b.stmtList(c.Stmts)))
		}

		cases = append(cases, document.HardLine(), body)
	}

	body := document.Concat(document.Text("{"), document.Indent(document.Concat(cases...)), document.HardLine(), document.Text("}"))

	return document.Concat(b.header("switch", b.condition(n.Expr), n.Expr, n.RParen), b.controlBrace(body))
}

func (b *builder) tryStmt(n *parser.TryStmt) *document.Doc {
	parts := []*document.Doc{document.Text("try"), b.controlBrace(b.controlBlock(n.Body))}

	for _, c := range n.Catches {
		types := make([]*document.Doc, len(c.Types))
		for i, t := range c.Types {
			types[i] = b.expr(t)
		}

		catch := document.Concat(b.controlLead(), document.Text("catch ("), document.Join(document.Text("|"), types))
		if c.Var != nil {
			catch = document.Concat(catch, document.Text(" "), b.tok(c.Var.Token))
		}

		catch, breaks := b.withTrailingParenComment(catch, c.RParen)

		closer := document.Text(")")
		if breaks {
			closer = document.Concat(document.HardLine(), closer)
		}

		parts = append(parts, catch, closer, b.controlBrace(b.controlBlock(c.Body)))
	}

	if n.Finally != nil {
		parts = append(parts, b.controlLead(), document.Text("finally"), b.controlBrace(b.controlBlock(n.Finally.Body)))
	}

	return document.Concat(parts...)
}

func (b *builder) jump(keyword string, level parser.Expr) *document.Doc {
	if level == nil {
		return document.Text(keyword + ";")
	}

	return document.Concat(document.Text(keyword+" "), b.expr(level), document.Text(";"))
}

func (b *builder) declareStmt(n *parser.DeclareStmt) *document.Doc {
	if has(n.Colon) {
		return b.verbatim(n.Span())
	}

	dirs := make([]*document.Doc, len(n.Directives))
	for i, d := range n.Directives {
		dirs[i] = document.Concat(b.tok(d.Name), document.Text("="), b.expr(d.Value))
	}

	head := document.Concat(document.Text("declare("), document.Join(document.Text(", "), dirs), document.Text(")"))

	if n.Body != nil {
		return document.Concat(head, document.Text(" "), b.stmtAsBlock(n.Body))
	}

	return document.Concat(head, document.Text(";"))
}

func (b *builder) namespaceStmt(n *parser.NamespaceStmt) *document.Doc {
	head := document.Text("namespace")
	if n.Name != nil {
		head = document.Concat(head, document.Text(" "), b.name(n.Name))
	}

	if n.Body != nil {
		return document.Concat(head, document.Text(" "), b.block(n.Body))
	}

	return document.Concat(head, document.Text(";"))
}

func (b *builder) constStmt(keyword string, items []*parser.ConstItem) *document.Doc {
	docs := make([]*document.Doc, len(items))
	for i, it := range items {
		docs[i] = document.Concat(b.tok(it.Name), document.Text(" = "), b.expr(it.Value))
	}

	return document.Concat(document.Text(keyword+" "), document.Join(document.Text(", "), docs), document.Text(";"))
}

func (b *builder) staticVarStmt(n *parser.StaticVarStmt) *document.Doc {
	docs := make([]*document.Doc, len(n.Vars))
	for i, v := range n.Vars {
		item := b.tok(v.Var)
		if v.Default != nil {
			item = document.Concat(item, document.Text(" = "), b.expr(v.Default))
		}

		docs[i] = item
	}

	return document.Concat(document.Text("static "), document.Join(document.Text(", "), docs), document.Text(";"))
}
