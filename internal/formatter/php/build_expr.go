package php

import (
	"bytes"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

func (b *builder) exprList(exprs []parser.Expr) *document.Doc {
	docs := make([]*document.Doc, len(exprs))
	for i, e := range exprs {
		docs[i] = b.expr(e)
	}

	return document.Join(document.Text(", "), docs)
}

func (b *builder) expr(e parser.Expr) *document.Doc {
	if lead := b.exprLeadComments(e); lead != nil {
		return document.Concat(lead, b.exprInner(e))
	}

	return b.exprInner(e)
}

// exprLeadComments claims the comments sitting immediately before an
// expression and returns them as an inline prefix, or nil when there are none,
// so a comment between two tokens of an expression is kept; a `@var` there is
// functional. A statement's, array element's or match arm's own start offset is
// skipped: those comments belong to the sequence, which places them above.
func (b *builder) exprLeadComments(e parser.Expr) *document.Doc {
	offset := e.Span().Start.Offset

	if b.measuring || offset == b.stmtStart || offset == b.elemStart || offset == b.armStart {
		return nil
	}

	return b.leadCommentsAt(offset)
}

// leadCommentsAt claims the unclaimed comments in the leading trivia of the token at
// offset and returns them as an inline prefix, or nil when there are none.
func (b *builder) leadCommentsAt(offset int) *document.Doc {
	var parts []*document.Doc

	for _, tr := range b.leadingAt(offset) {
		if !isCommentTrivia(tr.Kind) || b.emitted[tr.Span.Start.Offset] {
			continue
		}

		b.claim(tr)

		// A line comment runs to the end of the line, so the rest of the expression
		// has to move below it; a block comment stays on the line it was written on.
		if tr.Kind == parser.TriviaLineComment {
			parts = append(parts, b.commentVerbatim(tr), document.HardLine())

			continue
		}

		// A multi-line block comment keeps the line it opened. Pulled onto the line
		// its expression starts, the next pass reads it as a trailing comment and
		// re-homes its continuation lines against a different column, so the file
		// never settles.
		if bytes.ContainsAny(tr.Text, "\n\r") {
			parts = append(parts, document.HardLine(), b.commentVerbatim(tr), document.Text(" "))

			continue
		}

		parts = append(parts, b.commentVerbatim(tr), document.Text(" "))
	}

	if len(parts) == 0 {
		return nil
	}

	return document.Concat(parts...)
}

func (b *builder) claim(tr parser.Trivia) {
	if b.emitted == nil {
		b.emitted = make(map[int]bool)
	}

	b.emitted[tr.Span.Start.Offset] = true
	b.claims = append(b.claims, tr.Span.Start.Offset)
}

// claimMark records how many comments have been claimed, for a caller that may
// throw away the docs it is about to build.
func (b *builder) claimMark() int {
	return len(b.claims)
}

// releaseClaims un-claims everything claimed since mark, so a discarded doc
// gives its comments back for the rebuild.
func (b *builder) releaseClaims(mark int) {
	for _, offset := range b.claims[mark:] {
		delete(b.emitted, offset)
	}

	b.claims = b.claims[:mark]
}

func (b *builder) exprInner(e parser.Expr) *document.Doc {
	// chainStmtLevel marks a statement's own value, the only place a chain may
	// preserve author breaks. Consume it here so it reaches a chain only through
	// transparent wrappers (cast, paren, unary) or an assignment's right side; a
	// value nested in a measured container (match arm, binary operand, argument,
	// closure body) sees it cleared and does not preserve.
	stmtLevel := b.chainStmtLevel
	b.chainStmtLevel = false
	lineKnown := b.lineKnownNext
	b.lineKnownNext = false

	switch n := e.(type) {
	case *parser.IntExpr:
		return b.tok(n.Token)
	case *parser.FloatExpr:
		return b.tok(n.Token)
	case *parser.StringExpr:
		return b.stringLiteral(n.Token.Text)
	case *parser.BoolExpr:
		if n.Value {
			return document.Text("true")
		}

		return document.Text("false")
	case *parser.NullExpr:
		return document.Text("null")
	case *parser.VariableExpr:
		return b.tok(n.Token)
	case *parser.VarVarExpr:
		if n.LBrace.Kind != parser.KindInvalid {
			return document.Concat(document.Text("${"), b.expr(n.Name), document.Text("}"))
		}

		return document.Concat(document.Text("$"), b.expr(n.Name))
	case *parser.IdentExpr:
		return b.ident(n)
	case *parser.UnaryExpr:
		// The `!` operator alone gets a trailing space under cfg.PHP.NotOperatorSpace
		// (Pint not_operator_with_space); other unary operators stay tight. A unary
		// wraps the statement value, so its operand keeps chainStmtLevel.
		b.wrapStmt(stmtLevel, len(n.Op.Text)+1, 0)
		if n.Op.Kind == parser.KindBang && b.cfg.PHP.NotOperatorSpace {
			return document.Concat(b.tok(n.Op), document.Text(" "), b.expr(n.Expr))
		}

		if unaryFuses(n.Op, n.Expr) {
			return document.Concat(b.tok(n.Op), document.Text(" "), b.expr(n.Expr))
		}

		return document.Concat(b.tok(n.Op), b.expr(n.Expr))
	case *parser.BinaryExpr:
		if stmtLevel && n.Op.Kind == parser.KindPipeline {
			if d := b.preservedPipe(n); d != nil {
				return d
			}
		}

		return b.binary(n, stmtLevel)
	case *parser.AssignExpr:
		return b.assign(n, stmtLevel, false)
	case *parser.ParenExpr:
		// A comment that takes the rest of its line puts the content on its own
		// line one level in, off the statement's line, so it is built there.
		if n.RParen.Kind != parser.KindInvalid && b.hasBreakingComment(n.RParen.Span.Start.Offset) {
			b.depth++
			content := b.expr(n.Expr)
			b.depth--

			return b.parenGroup(content, n.RParen)
		}

		b.wrapStmt(stmtLevel, len("("), len(")"))
		return b.parenGroup(b.expr(n.Expr), n.RParen)
	case *parser.CallExpr:
		return b.postfix(n, stmtLevel)
	case *parser.MemberExpr:
		return b.postfix(n, stmtLevel)
	case *parser.StaticMemberExpr:
		return b.postfix(n, stmtLevel)
	case *parser.SubscriptExpr:
		return b.postfix(n, stmtLevel)
	case *parser.NewExpr:
		// An argument-less `new Foo` keeps whatever the author wrote, as `new
		// class` does: dropping the parens would break PHP 8.4's
		// `new Foo()->bar()`, where they are required.
		if !has(n.LParen) {
			return document.Concat(document.Text("new "), b.expr(n.Class))
		}

		// A comment wedged before the `)` has no slot in argList, which owns that
		// paren across several layouts; keep the argument region verbatim so it is
		// not lost, the way a ternary or a cast does.
		if args := (parser.Span{Start: n.LParen.Span.Start, End: n.RParen.Span.End}); b.spanHasComment(args) {
			return document.Concat(document.Text("new "), b.expr(n.Class), b.verbatim(args))
		}

		return document.Concat(document.Text("new "), b.expr(n.Class), b.argList(n.Args, withTrailingComma))
	case *parser.ArrayExpr:
		return b.array(n)
	case *parser.TernaryExpr:
		return b.ternary(n, stmtLevel || lineKnown)
	case *parser.NamedArgExpr:
		return document.Concat(b.tok(n.Name), document.Text(": "), b.expr(n.Value))
	case *parser.SpreadExpr:
		return document.Concat(document.Text("..."), b.expr(n.Expr))
	case *parser.ClosureExpr:
		return b.closure(n)
	case *parser.ArrowFnExpr:
		return b.arrowFn(n)
	case *parser.MatchExpr:
		return b.match(n)
	case *parser.AnonClassExpr:
		return b.anonClass(n)
	case *parser.CastExpr:
		// A cast wraps the statement value (`return (int) $x->a()->b();`).
		b.wrapStmt(stmtLevel, nonSpaceLen(b.src[n.Span().Start.Offset:n.Expr.Span().Start.Offset])+1, 0)
		return b.cast(n)
	case *parser.UpdateExpr:
		if n.Prefix {
			return document.Concat(b.tok(n.Op), b.expr(n.Operand))
		}

		return document.Concat(b.expr(n.Operand), b.tok(n.Op))
	case *parser.ThrowExpr:
		return document.Concat(document.Text("throw "), b.expr(n.Expr))
	case *parser.CloneExpr:
		if n.Expr == nil {
			return document.Concat(document.Text("clone"), b.argList(n.Args, withTrailingComma))
		}

		return document.Concat(document.Text("clone "), b.expr(n.Expr))
	case *parser.PrintExpr:
		return document.Concat(document.Text("print "), b.expr(n.Expr))
	case *parser.InstanceofExpr:
		return document.Concat(b.expr(n.Left), document.Text(" instanceof "), b.expr(n.Class))
	case *parser.YieldExpr:
		return b.yield(n)
	case *parser.DynamicName:
		return document.Concat(document.Text("{"), b.expr(n.Expr), document.Text("}"))
	}

	return b.verbatim(e.Span())
}

func (b *builder) ident(n *parser.IdentExpr) *document.Doc {
	parts := make([]*document.Doc, 0, len(n.Parts)*2+2)

	if n.LeadingBackslash.Kind != parser.KindInvalid {
		parts = append(parts, document.Text("\\"))
	}

	parts = append(parts, b.tok(n.Token))

	for i := range n.Parts {
		parts = append(parts, b.tok(n.Separators[i]), b.tok(n.Parts[i]))
	}

	return document.Concat(parts...)
}

// stringLiteral lays out a string. A heredoc/nowdoc is reindented to the print
// column per cfg.PHP.HeredocIndentation (start_plus_one / same_as_start); `none`
// and every regular string keep their source form (quote normalization aside).
func (b *builder) stringLiteral(text []byte) *document.Doc {
	mode := b.cfg.PHP.HeredocIndentation
	reindent := mode == config.HeredocIndentStartPlusOne || mode == config.HeredocIndentSameAsStart

	if reindent && bytes.HasPrefix(text, []byte("<<<")) {
		// In an embedded fragment the host re-emits the output line by line and
		// normalizes EOLs, so a carriage return in the body is dropped later by
		// stripLineCR (build_embed.go). Strip it before reindent too, or reindent
		// prefixes a would-be-blank line with indent spaces that the CR-strip then
		// strands as trailing whitespace, which collapses on the next pass.
		if b.embed {
			text = stripLineCR(text)
		}

		return document.Heredoc(string(text), mode == config.HeredocIndentStartPlusOne)
	}

	return document.Text(string(normalizeStringLiteral(text, b.cfg.SingleQuote())))
}

// normalizeStringLiteral returns the canonical quoted form under preferSingle. A
// rewrite that would alter escape semantics or touch a heredoc/nowdoc returns the
// original bytes, so formatting is semantics-preserving by default.
func normalizeStringLiteral(text []byte, preferSingle bool) []byte {
	if len(text) < 2 {
		return text
	}

	switch text[0] {
	case '"':
		if !preferSingle {
			return text
		}

		if rewritten, ok := doubleToSingle(text); ok {
			return rewritten
		}

		return text
	case '\'':
		if preferSingle {
			return text
		}

		if rewritten, ok := singleToDouble(text); ok {
			return rewritten
		}

		return text
	default:
		return text
	}
}

// doubleToSingle rewrites a double-quoted literal to single-quoted only when the
// swap is pure form. It refuses (returns false) whenever a content byte would need
// rewriting: an interpolation trigger (`$name`, `{$…}`), any backslash escape but
// `\\`, or a literal `'`.
func doubleToSingle(text []byte) ([]byte, bool) {
	if len(text) < 2 || text[0] != '"' || text[len(text)-1] != '"' {
		return nil, false
	}

	content := text[1 : len(text)-1]
	i := 0

	for i < len(content) {
		b := content[i]

		switch b {
		case '$':
			if i+1 < len(content) && (parser.IsIdentStart(content[i+1]) || content[i+1] == '{') {
				return nil, false
			}

			i++
		case '{':
			if i+1 < len(content) && content[i+1] == '$' {
				return nil, false
			}

			i++
		case '\\':
			if i+1 >= len(content) || content[i+1] != '\\' {
				return nil, false
			}

			i += 2
		case '\'':
			return nil, false
		default:
			i++
		}
	}

	out := make([]byte, 0, len(text))
	out = append(out, '\'')
	out = append(out, content...)
	out = append(out, '\'')

	return out, true
}

// singleToDouble rewrites a single-quoted literal to double-quoted only when the
// swap is pure form. It refuses (returns false) whenever a content byte would need
// rewriting: a `$` that would trigger interpolation, any backslash escape but `\\`,
// or a literal `"`.
func singleToDouble(text []byte) ([]byte, bool) {
	if len(text) < 2 || text[0] != '\'' || text[len(text)-1] != '\'' {
		return nil, false
	}

	content := text[1 : len(text)-1]
	i := 0

	for i < len(content) {
		b := content[i]

		switch b {
		case '$':
			if i+1 < len(content) && (parser.IsIdentStart(content[i+1]) || content[i+1] == '{') {
				return nil, false
			}

			i++
		case '\\':
			if i+1 >= len(content) || content[i+1] != '\\' {
				return nil, false
			}

			i += 2
		case '"':
			return nil, false
		default:
			i++
		}
	}

	out := make([]byte, 0, len(text))
	out = append(out, '"')
	out = append(out, content...)
	out = append(out, '"')

	return out, true
}

func (b *builder) binary(n *parser.BinaryExpr, stmtLevel bool) *document.Doc {
	return b.binaryIndented(n, true, stmtLevel)
}

// stmtLevel: the binary is a statement's own value, so the width after its
// last line is known.
func (b *builder) binaryIndented(n *parser.BinaryExpr, indent, stmtLevel bool) *document.Doc {
	head, tails := flattenBinary(n)

	// A comment wedged between operands (`$a . // note` chains) has no structural
	// slot in a reflowed binary; keep the author's form so it is not lost. A
	// concatenation keeps its authored breaks, so a comment at one has a slot.
	if b.spanHasComment(n.Span()) && !b.concatCommentsAtBreaks(n, head, tails) {
		return b.verbatim(n.Span())
	}

	// Fit-first: the same-precedence run collapses to one line when it fits,
	// else every operand drops to its own indented line with the operator
	// leading (PSR-12). Concatenation is tight unless cfg.PHP.ConcatSpace is
	// set, but a side that would fuse with a numeric literal keeps its space
	// (concatSpaces); every other operator carries a space each side.
	concat := n.Op.Kind == parser.KindDot
	spaced := !concat || b.cfg.PHP.ConcatSpace

	seps := make([]*document.Doc, len(tails))
	ops := make([]string, len(tails))
	befores := make([]bool, len(tails))
	left := head

	for i, t := range tails {
		before, after := spaced, spaced
		if !spaced {
			before, after = concatSpaces(left, t.operand)
		}

		befores[i] = before
		seps[i] = document.SoftLine()

		if before {
			seps[i] = document.Line()
		}

		ops[i] = string(t.op.Text)
		if after {
			ops[i] += " "
		}

		left = t.operand
	}

	// A cast or prefix operator ends on its operand's last line, so the operand
	// decides the glue.
	last := prefixOperand(head)
	headCall, headIsCall := trailingCall(last).(*parser.CallExpr)
	headDoc := b.binaryChainHead(head, indent, stmtLevel && !concat, tails)

	sep := seps[0]

	// A single operator on a head ending in a breakable argument list glues to that
	// list's closing line rather than taking its own, where it would sit under `)`
	// and read as applying to nothing. A fluent chain's operator glues to its last
	// line the same way; on its own line it lands at the link indent and reads as
	// another call (concatenation excluded: its operands are a run of equals).
	// Keyed on the head's group, so the operator keeps a real break opportunity
	// while the head stays flat: a flat head ends in no closer, and PSR-12 puts the
	// operator at the start of the continuation line.
	chainHead := !concat && (headIsCall && len(collectChainSpine(headCall)) >= 1 || readsOffChain(last))

	var (
		glueID   int32
		glueText string
		plain    *document.Doc
	)

	glues := endsOnBreakableCloser(last) || chainHead
	if glues {
		operands := make([]parser.Expr, len(tails))
		for i, t := range tails {
			operands[i] = t.operand
		}

		glues = !b.parenHeadWidens(last, operands)
	}

	if (len(tails) == 1 || !concat) && glues {
		glue := ""
		if befores[0] {
			glue = " "
		}

		id := b.nextGroupID()
		headDoc = document.GroupID(id, headDoc)
		plain = sep
		sep = document.IfBreakOf(id, document.Text(glue), sep)
		seps[0] = sep
		glueID, glueText = id, glue
	}

	inner := make([]*document.Doc, 0, len(tails)*3)
	onlyTail := tails

	// A broken operand prints one level in, so anything inside it that predicts
	// its own width has to measure from there, or a chain in it judges itself
	// against a column it never occupies.
	saved := b.depth

	if indent {
		b.depth++
	}

	indented := func(d *document.Doc) *document.Doc {
		if indent {
			return document.Indent(d)
		}

		return d
	}

	if !concat && glueID != 0 {
		if step := b.chainTail(stmtLevel && chainHead, last, tails, saved+1); step != nil {
			inner = append(inner, document.Text(glueText), step)
		} else {
			headBreaks := document.HasForcedBreak(headDoc)
			inner = append(inner, b.gluedRun(tails, spaced, headBreaks, glueID, glueText, sep, plain, indented)...)
		}

		tails = nil
	}

	for i, t := range tails {
		op, sep := ops[i], seps[i]

		if !concat {
			operand := b.binaryOperand(t.operand, op, stmtLevel && i == len(tails)-1, stmtLevel)
			inner = append(inner, indented(document.Concat(sep, document.Text(op), operand)))

			// A call whose forced break sits in a choice the printer has yet to
			// make (a chain's ConditionalGroup) is hidden from the group, so the
			// run is broken here as it is when the same break propagates.
			if b.hidesForcedBreak(t.operand, operand) {
				inner = append(inner, document.BreakParent())
			}

			continue
		}

		// Concatenation operands are pieces of one string, so they fill: a break
		// comes only where the next piece no longer fits, or where the author put
		// one. A filled line reads back as authored and already fits, so the
		// second pass reproduces it.
		authorBroke := b.concatAuthorBroke(t)

		var lead, trail []*document.Doc
		if authorBroke {
			lead, trail = b.breakComments(t)
		}

		step := document.Concat(document.Text(op), b.expr(t.operand))

		switch {
		case authorBroke:
			if len(trail) > 0 {
				inner = append(inner, document.LineSuffix(document.Concat(trail...)))
			}

			// Grouped like a fill break, so both indent from the same line.
			line := []*document.Doc{document.HardLine()}
			for _, c := range lead {
				line = append(line, c, document.HardLine())
			}

			inner = append(inner, indented(document.Group(document.Concat(append(line, step)...))))
		case document.HasForcedBreak(step):
			// An operand that opens a block (`.match (…) {`) keeps its opener on
			// the line when it fits, its block at that line's indent. Its own
			// group would always break, so the separator is judged alone. The
			// break inside IfBreakOf does not propagate, so it is restated for
			// the enclosing groups. Moved to its own line, the operand is grouped
			// as the author-break path groups it, so its block indents from the
			// line it opens on in both.
			id := b.nextGroupID()
			inner = append(inner,
				indented(document.GroupID(id, sep)),
				document.IfBreakOf(id, indented(document.Group(step)), step),
				document.BreakParent(),
			)
		case glueID != 0:
			// Glued to a broken head's closer, the operand continues that line,
			// so it takes no indent: the block path above prints it the same way
			// once its own break reads back as forced.
			glued := document.Concat(document.Text(glueText), step)
			filled := indented(document.Group(document.Concat(sep, step)))
			inner = append(inner, document.IfBreakOf(glueID, glued, filled))
		default:
			inner = append(inner, indented(document.Group(document.Concat(sep, step))))
		}
	}

	b.depth = saved

	broken := document.Group(document.Concat(headDoc, document.Concat(inner...)))

	// `$a ?? Foo::bar([` keeps the operator on the line that opens a block that
	// breaks anyway, the block breaking below it as a hugged argument does. An
	// operand that fits flat on its own line moves down instead, as a chain's
	// `->m([..])` link does. Only a statement's own value knows the column of
	// that line.
	if stmtLevel && !concat && len(onlyTail) == 1 {
		if width, ok := b.blockOpenerWidth(onlyTail[0].operand); ok {
			op := " " + string(onlyTail[0].op.Text) + " "
			operand := b.expr(onlyTail[0].operand)
			alone := (saved+1)*b.cfg.TabWidth + len(op) + b.oneLineWidth(operand)

			if alone > b.cfg.PrintWidth {
				// The head measures its line up to the block's opener only, as
				// the block breaks below it.
				glued := document.Concat(headDoc, document.Text(op), document.Opener(width, operand))
				width += document.FlatWidth(headDoc, b.cfg.TabWidth) + len(op)

				return document.ConditionalGroup(document.Opener(width, glued), broken)
			}
		}
	}

	return broken
}

func (b *builder) blockOpenerWidth(operand parser.Expr) (int, bool) {
	if arr, ok := operand.(*parser.ArrayExpr); ok {
		return len("["), arrayHasValue(arr.Elements)
	}

	if cast, ok := operand.(*parser.CastExpr); ok {
		return len("(") + len(normalizeCastType(cast.Type.Text)) + len(") ["), castsArray(cast)
	}

	if throw, ok := operand.(*parser.ThrowExpr); ok {
		width, opens := b.blockOpenerWidth(throw.Expr)

		return len("throw ") + width, opens
	}

	// A plain call or `new` with no argument to hug breaks its list right after
	// `(` in every pass, so it opens a block there as an array does; whether it
	// breaks is the caller's fit check. A fluent chain breaks between its links.
	if n, ok := operand.(*parser.NewExpr); ok {
		if len(n.Args) == 0 || slices.ContainsFunc(n.Args, isHuggableArg) {
			return 0, false
		}

		class := b.measureWidth(func() int { return document.FlatWidth(b.expr(n.Class), b.cfg.TabWidth) })

		return len("new ") + class + len("("), true
	}

	call, ok := operand.(*parser.CallExpr)
	if !ok || len(call.Args) == 0 {
		return 0, false
	}

	last := call.Args[len(call.Args)-1]
	if arr, isArray := last.(*parser.ArrayExpr); !isArray || !arrayHasValue(arr.Elements) {
		if len(collectChainSpine(call)) > 0 || slices.ContainsFunc(call.Args, isHuggableArg) {
			return 0, false
		}

		callee := b.measureWidth(func() int { return document.FlatWidth(b.expr(call.Callee), b.cfg.TabWidth) })

		return callee + len("("), true
	}

	hugged := false
	width := b.measureWidth(func() int {
		_, hugged = b.argListHug(call.Args, withTrailingComma)

		width := document.FlatWidth(b.expr(call.Callee), b.cfg.TabWidth) + len("([")
		for _, arg := range call.Args[:len(call.Args)-1] {
			width += document.FlatWidth(b.expr(arg), b.cfg.TabWidth) + len(", ")
		}

		return width
	})

	return width, hugged && width < document.ForcedBreakWidth
}

// chainTail returns a statement's one unbreakable operand when it stays on the
// line of the laddered chain before it, or nil. The operator then has no break
// of its own: a chain that fits only without the tail ladders instead of
// leaving `+ 1;` alone on a line. depth is the ladder's level.
func (b *builder) chainTail(chainStmt bool, last parser.Expr, tails []binaryTail, depth int) *document.Doc {
	if !chainStmt || len(tails) != 1 {
		return nil
	}

	call, ok := last.(*parser.CallExpr)
	if !ok || memberCalls(call) == 0 || callStaysOnHead(call) {
		return nil
	}

	mem, ok := call.Callee.(*parser.MemberExpr)
	if !ok {
		return nil
	}

	step := document.Concat(document.Text(string(tails[0].op.Text)+" "), b.expr(tails[0].operand))
	if document.CanBreak(step) {
		return nil
	}

	// argList consumes a pending noHugNext meant for the real build.
	savedNoHug := b.noHugNext
	b.noHugNext = false

	defer func() { b.noHugNext = savedNoHug }()

	link := b.measureWidth(func() int {
		doc := document.Concat(b.tok(mem.Arrow), b.expr(mem.Property), b.argList(call.Args, withTrailingComma))
		if document.HasForcedBreak(doc) {
			// Broken arguments: the tail rides their closing line instead,
			// as it does once a fitting link breaks there on the first pass.
			closing, _ := document.ClosingLineWidth(doc, b.cfg.TabWidth)

			return closing
		}

		return document.FlatWidth(doc, b.cfg.TabWidth)
	})
	tail := len(" ") + document.FlatWidth(step, b.cfg.TabWidth)

	if depth*b.cfg.TabWidth+link+tail+b.chainStmtTail > b.cfg.PrintWidth {
		return nil
	}

	return step
}

// hidesForcedBreak reports whether doc, a call operand, cannot print on one line
// although its break sits where the group does not see it: in a choice the
// printer has yet to make, such as a chain's ConditionalGroup.
func (b *builder) hidesForcedBreak(operand parser.Expr, doc *document.Doc) bool {
	if _, call := prefixOperand(operand).(*parser.CallExpr); !call {
		return false
	}

	return !document.HasForcedBreak(doc) && document.FlatWidth(doc, b.cfg.TabWidth) >= document.ForcedBreakWidth
}

func (b *builder) gluedRun(tails []binaryTail, spaced, headBreaks bool, glueID int32, glueText string, sep, plain *document.Doc, indented func(*document.Doc) *document.Doc) []*document.Doc {
	steps := make([]*document.Doc, len(tails))
	free := make([]*document.Doc, len(tails))
	fits := true
	forced := false
	freeForced := false

	for i, t := range tails {
		op := string(t.op.Text)
		if spaced {
			op += " "
		}

		steps[i] = document.Concat(document.Text(op), b.expr(t.operand))
		forced = forced || document.HasForcedBreak(steps[i]) || b.hidesForcedBreak(t.operand, steps[i])

		saved := b.noConcatBreaks
		b.noConcatBreaks = true
		free[i] = document.Concat(document.Text(op), b.expr(t.operand))
		b.noConcatBreaks = saved
		freeForced = freeForced || document.HasForcedBreak(free[i]) || b.hidesForcedBreak(t.operand, free[i])

		fits = fits && b.depth*b.cfg.TabWidth+document.FlatWidth(free[i], b.cfg.TabWidth) <= b.cfg.PrintWidth
	}

	if !fits {
		var out []*document.Doc
		if len(steps) == 1 {
			out = []*document.Doc{indented(document.Concat(sep, steps[0]))}
		} else {
			for _, step := range steps {
				out = append(out, indented(document.Concat(plain, step)))
			}
		}

		if forced {
			out = append(out, document.BreakParent())
		}

		return out
	}

	glued := make([]*document.Doc, 0, len(steps)*2)
	moved := make([]*document.Doc, 0, len(steps)*2)
	flat := make([]*document.Doc, 0, len(steps))

	for i, step := range steps {
		glued = append(glued, document.Text(glueText), free[i])
		moved = append(moved, document.HardLine(), step)
		flat = append(flat, indented(document.Concat(plain, step)))
	}

	choice := document.ConditionalGroup(document.Concat(glued...), indented(document.Concat(moved...)))

	// A head that always breaks never prints the flat form, so it is left out:
	// a measurement would read the concatenation breaks it keeps and the glued
	// form drops, which the next pass no longer sees.
	if headBreaks {
		if freeForced {
			return []*document.Doc{choice, document.BreakParent()}
		}

		return []*document.Doc{choice}
	}

	out := []*document.Doc{document.IfBreakOf(glueID, choice, document.Concat(flat...))}

	// A break inside IfBreakOf does not propagate; restate it.
	if forced {
		out = append(out, document.BreakParent())
	}

	return out
}

// concatCommentsAtBreaks reports whether every comment in a concatenation sits
// at a break the author made between two operands, either trailing the operand
// before it on that operand's line or on a line of its own, so each keeps its
// line once the `.` moves to lead the next operand.
func (b *builder) concatCommentsAtBreaks(n *parser.BinaryExpr, head parser.Expr, tails []binaryTail) bool {
	if n.Op.Kind != parser.KindDot || b.noConcatBreaks || b.noAuthorBreaks {
		return false
	}

	atBreaks := 0
	prevEnd := head.Span().End.Offset

	for _, t := range tails {
		beforeOp := b.leadingAt(t.op.Span.Start.Offset)
		afterOp := b.leadingAt(t.operand.Span().Start.Offset)
		count := commentTrivia(beforeOp) + commentTrivia(afterOp)

		if count > 0 {
			if commentTrivia(beforeOp) > 0 && commentTrivia(afterOp) > 0 || !b.concatAuthorBroke(t) {
				return false
			}

			for _, tr := range slices.Concat(beforeOp, afterOp) {
				if isCommentTrivia(tr.Kind) && !b.keepsCommentLine(prevEnd, tr.Span.Start.Offset) {
					return false
				}
			}
		}

		atBreaks += count
		prevEnd = t.operand.Span().End.Offset
	}

	return atBreaks == b.commentsIn(n.Span())
}

func (b *builder) keepsCommentLine(prevEnd, offset int) bool {
	gap := b.src[prevEnd:offset]

	nl := bytes.LastIndexAny(gap, "\n\r")
	if nl < 0 {
		return true
	}

	return len(bytes.TrimLeft(gap[nl+1:], " \t")) == 0
}

func (b *builder) breakComments(t binaryTail) (lead, trail []*document.Doc) {
	offset := t.op.Span.Start.Offset
	if commentTrivia(b.leadingAt(offset)) == 0 {
		offset = t.operand.Span().Start.Offset
	}

	lead, trail = b.splitComments(offset)

	// The operand would otherwise print them again as its own leading comments.
	for _, tr := range b.leadingAt(offset) {
		if isCommentTrivia(tr.Kind) {
			b.claim(tr)
		}
	}

	return lead, trail
}

// A break after the `.` counts too; the operator then leads the next line.
func (b *builder) concatAuthorBroke(t binaryTail) bool {
	if b.noAuthorBreaks || b.noConcatBreaks {
		return false
	}

	return b.newlineBefore(t.op.Span.Start.Offset) || b.newlineBefore(t.operand.Span().Start.Offset)
}

func (b *builder) alignGaps(lefts []*document.Doc, breaksBefore []bool, value func(i int) (width, first int, breaks bool)) []*document.Doc {
	widths := make([]int, len(lefts))
	values := make([]int, len(lefts))
	firsts := make([]int, len(lefts))
	breakable := make([]bool, len(lefts))
	aligned := make([]bool, len(lefts))
	rigid := make([]bool, len(lefts))

	for i, l := range lefts {
		if l == nil {
			continue
		}

		aligned[i] = true
		rigid[i] = !document.CanBreak(l)
		widths[i] = document.FlatWidth(l, b.cfg.TabWidth)

		if widths[i] < document.ForcedBreakWidth {
			values[i], firsts[i], breakable[i] = value(i)
		}
	}

	targets, cuts := b.alignWidths(widths, values, firsts, aligned, breakable, rigid, breaksBefore)
	gaps := make([]*document.Doc, len(lefts))
	scope := b.alignScope

	for i, target := range targets {
		if target < 0 {
			continue
		}

		own := widths[i]
		if own >= document.ForcedBreakWidth {
			own = 0
		}

		gaps[i] = document.AlignGap(scope, max(target-own, 0))

		// The printer reads a run from one scope, so a cut starts a new one.
		if cuts[i] {
			b.alignScope++
			scope = b.alignScope
		}
	}

	return gaps
}

// Runs end where the printer ends them (document.expandAlignGaps): an authored
// break before an entry, a missing left side, an entry printing multi-line.
// Reserving across them would break an entry for padding it never gets.
func (b *builder) alignWidths(widths, values, firsts []int, aligned, breakable, rigid, breaksBefore []bool) (targets []int, cuts []bool) {
	const arrow, comma = len("=> ") + 1, len(",")

	indent := (b.depth + 1) * b.cfg.TabWidth
	flat := make([]bool, len(widths))
	single := make([]bool, len(widths))

	for i := range widths {
		if !aligned[i] || widths[i] >= document.ForcedBreakWidth {
			continue
		}

		fits := indent+widths[i]+arrow+values[i]+comma <= b.cfg.PrintWidth
		flat[i] = widths[i] <= b.cfg.PrintWidth/2 || fits
		single[i] = fits
	}

	targets = make([]int, len(widths))
	cuts = make([]bool, len(widths))

	if b.cfg.PHP.PintAlignment {
		alignAsPint(widths, aligned, targets)

		return targets, cuts
	}

	var run []int

	// An entry that fits only short of the run's padding would break for it, and
	// the break would end the run after it; ending the run there up front keeps
	// the entry flat with the padding it would be printed with anyway.
	endRun := func() {
		// An entry that breaks anyway ends its run. When its first line fits
		// beside its own key but not beside the run's padding, it stands alone
		// and the entries above keep their own column.
		alone := -1

		if n := len(run); n > 1 && !single[run[n-1]] && breakable[run[n-1]] {
			last := run[n-1]
			b.alignRun(run, widths, values, aligned, flat, breakable, targets)

			head := indent + arrow + firsts[last]
			if head+widths[last] <= b.cfg.PrintWidth && head+targets[last] > b.cfg.PrintWidth {
				alone, run = last, run[:n-1]
			}
		}

		rest := run

		for len(rest) > 0 {
			b.alignRun(rest, widths, values, aligned, flat, breakable, targets)

			cut := b.paddingCut(rest, widths, values, breakable, targets)
			if cut < 0 {
				break
			}

			cuts[rest[cut]] = true

			for _, i := range rest[cut+1:] {
				targets[i] = -1
			}

			b.alignRun(rest[:cut+1], widths, values, aligned, flat, breakable, targets)
			rest = rest[cut+1:]
		}

		if alone >= 0 {
			cuts[run[len(run)-1]] = true
			targets[alone] = widths[alone]
		}

		run = run[:0]
	}

	for i := range widths {
		targets[i] = -1

		if !aligned[i] {
			endRun()

			continue
		}

		if breaksBefore[i] || !flat[i] {
			endRun()
		}

		// A left side too wide to align that cannot break takes no gap, so the
		// printer ends the run at it instead of padding its neighbors to its
		// column. One that breaks aligns from its last line.
		if !flat[i] && rigid[i] {
			continue
		}

		run = append(run, i)

		// An entry whose sides both cannot break still prints on one line, so
		// the printer keeps the run going past it.
		if !single[i] && (breakable[i] || !rigid[i]) {
			endRun()
		}
	}

	endRun()
	b.unreserveOverflow(targets, widths, values, breakable)

	return targets, cuts
}

// Pint lays out nothing, it only pads: an entry keeps its unpadded layout and
// the post-print pass pads each run to its widest key, whatever the width.
// Reserving that padding would break values Pint leaves alone.
func alignAsPint(widths []int, aligned []bool, targets []int) {
	for i, w := range widths {
		switch {
		case !aligned[i]:
			targets[i] = -1
		case w >= document.ForcedBreakWidth:
			targets[i] = 0
		default:
			targets[i] = w
		}
	}
}

// An entry whose value cannot break overflows padded, as under Pint. It
// reserves no padding, or the printer would break its key to make room; the
// post-print pass still widens it to the run's column.
func (b *builder) unreserveOverflow(targets, widths, values []int, breakable []bool) {
	const arrow, comma = len("=> ") + 1, len(",")

	indent := (b.depth + 1) * b.cfg.TabWidth

	for i, target := range targets {
		if target >= 0 && !breakable[i] && indent+target+arrow+values[i]+comma > b.cfg.PrintWidth {
			targets[i] = widths[i]
		}
	}
}

func (b *builder) alignRun(run []int, widths, values []int, aligned, flat, breakable []bool, targets []int) {
	if len(run) == 0 {
		return
	}

	member := make([]bool, len(widths))
	for _, i := range run {
		member[i] = aligned[i]
	}

	var widest int

	for {
		widest = 0
		at := -1

		for _, i := range run {
			if member[i] && flat[i] && widths[i] > widest {
				widest, at = widths[i], i
			}
		}

		if widest <= b.cfg.PrintWidth/2 || !b.paddingOverflows(widest, widths, values, member, breakable) {
			break
		}

		member[at] = false
	}

	for _, i := range run {
		if member[i] {
			targets[i] = widest
		}
	}
}

// paddingCut returns the position in run of the first entry, short of the last,
// that fits on one line at its own width but would break at its target, or -1.
// A value that cannot break just overflows padded, as under Pint, and keeps
// its run.
func (b *builder) paddingCut(run []int, widths, values []int, breakable []bool, targets []int) int {
	const arrow, comma = len("=> ") + 1, len(",")

	indent := (b.depth + 1) * b.cfg.TabWidth

	for pos, i := range run[:len(run)-1] {
		if targets[i] < 0 || !breakable[i] {
			continue
		}

		tail := arrow + values[i] + comma
		if indent+widths[i]+tail <= b.cfg.PrintWidth && indent+targets[i]+tail > b.cfg.PrintWidth {
			return pos
		}
	}

	return -1
}

// FlatWidth deliberately reads an author-broken array as flat; this does not.
func (b *builder) oneLineWidth(d *document.Doc) int {
	if document.HasForcedBreak(d) {
		return document.ForcedBreakWidth
	}

	return document.FlatWidth(d, b.cfg.TabWidth)
}

func (b *builder) paddingOverflows(widest int, widths, values []int, aligned, breakable []bool) bool {
	const arrow, comma = len("=> ") + 1, len(",")

	indent := (b.depth + 1) * b.cfg.TabWidth

	for i := range widths {
		if !aligned[i] || !breakable[i] || widths[i] >= widest || widths[i] >= document.ForcedBreakWidth {
			continue
		}

		tail := arrow + values[i] + comma
		if indent+widths[i]+tail <= b.cfg.PrintWidth && indent+widest+tail > b.cfg.PrintWidth {
			return true
		}
	}

	return false
}

// matchResultLeads is the width each arm puts before its result: the head on one
// line, ` => `, and under alignment the most padding it can get (the widest
// head's). A head that may wrap counts at its flat width, an upper bound.
func (b *builder) matchResultLeads(arms []*parser.MatchArm) []int {
	heads := make([]int, len(arms))
	widest := 0

	b.measureWidth(func() int {
		for i, arm := range arms {
			if arm.IsDefault {
				heads[i] = len("default")
			} else {
				for j, c := range arm.Conds {
					if j > 0 {
						heads[i] += len(", ")
					}

					heads[i] += document.FlatWidth(b.expr(c), b.cfg.TabWidth)
				}
			}

			widest = max(widest, heads[i])
		}

		return 0
	})

	leads := make([]int, len(arms))
	for i := range arms {
		leads[i] = heads[i] + len(" => ")
		if b.cfg.PHP.AlignFatArrow {
			leads[i] = widest + len(" => ")
		}
	}

	return leads
}

// preservedPipe keeps a statement's author-broken pipe chain as written, under
// the guards a member chain gets (chainAuthorBroken, chainLayoutFits): two or
// more `|>` the author put on a line of their own, and every authored line
// fitting at the statement's structural depth. It returns nil otherwise, and
// for a chain carrying a comment, which the width path places.
func (b *builder) preservedPipe(n *parser.BinaryExpr) *document.Doc {
	if b.spanHasComment(n.Span()) {
		return nil
	}

	head, tails := flattenBinary(n)

	breaks := make([]bool, len(tails))
	count := 0

	for i, t := range tails {
		breaks[i] = b.newlineBefore(t.op.Span.Start.Offset)
		if breaks[i] {
			count++
		}
	}

	if count < 2 {
		return nil
	}

	// Built as the width path builds it, so a run the formatter broke on width
	// reads back to the same layout when it comes round as authored.
	headDoc := b.binaryHead(head, true)

	b.depth++

	steps := make([]*document.Doc, len(tails))
	for i, t := range tails {
		steps[i] = document.Concat(document.Text(string(t.op.Text)+" "), b.expr(t.operand))
	}

	b.depth--

	base := b.depth * b.cfg.TabWidth
	limit := b.cfg.PrintWidth - base - b.chainStmtLead
	line := []*document.Doc{headDoc}
	parts := []*document.Doc{headDoc}

	for i, step := range steps {
		if !breaks[i] {
			line = append(line, document.Text(" "), step)
			parts = append(parts, document.Text(" "), step)

			continue
		}

		if !b.lineFits(line, limit) {
			return nil
		}

		limit = b.cfg.PrintWidth - base - b.cfg.TabWidth
		line = []*document.Doc{step}
		parts = append(parts, document.Indent(document.Concat(document.HardLine(), step)))
	}

	if !b.lineFits(line, limit-b.chainStmtTail) {
		return nil
	}

	return document.Concat(parts...)
}

// wrapStmt carries the statement-level flag through a wrapper (unary, cast,
// paren) that puts lead before its operand and tail after it. The widths only
// mean something at statement level; anywhere else they stay untouched, or a
// wrapper built while measuring would shift the statement's own reading.
func (b *builder) wrapStmt(stmtLevel bool, lead, tail int) {
	b.chainStmtLevel = stmtLevel

	if stmtLevel {
		b.chainStmtLead += lead
		b.chainStmtTail += tail
	}
}

// binaryHead builds a run's first operand. A tighter binary there breaks one
// level deeper than the run's own operators, so `=== $b` never lines up with
// `&& $c` and read as a sibling of it.
func (b *builder) binaryHead(head parser.Expr, indent bool) *document.Doc {
	// Parentheses only group; a binary inside them still opens a level below
	// the outer operator, or its own continuation reads as a sibling of it.
	inner := head
	for p, ok := inner.(*parser.ParenExpr); ok; p, ok = inner.(*parser.ParenExpr) {
		inner = p.Expr
	}

	if _, nested := inner.(*parser.BinaryExpr); !nested || !indent {
		return b.expr(head)
	}

	b.depth++
	d := b.expr(head)
	b.depth--

	return document.Indent(d)
}

func (b *builder) binaryOperand(operand parser.Expr, op string, last, stmtLevel bool) *document.Doc {
	if !stmtLevel {
		return b.expr(operand)
	}

	savedLead, savedTail := b.chainStmtLead, b.chainStmtTail
	b.chainStmtLead = len(op)

	if !last {
		b.chainStmtTail = 0
	}

	b.preserveChainNext = true

	doc := b.expr(operand)

	b.chainStmtLead, b.chainStmtTail = savedLead, savedTail
	b.preserveChainNext = false

	return doc
}

func readsOffChain(e parser.Expr) bool {
	for {
		mem, ok := e.(*parser.MemberExpr)
		if !ok {
			call, isCall := e.(*parser.CallExpr)

			return isCall && memberCalls(call) > 0
		}

		e = mem.Object
	}
}

func (b *builder) binaryChainHead(head parser.Expr, indent, stmtLevel bool, tails []binaryTail) *document.Doc {
	_, isCall := trailingCall(prefixOperand(head)).(*parser.CallExpr)
	_, isMember := prefixOperand(head).(*parser.MemberExpr)

	if !stmtLevel || len(tails) != 1 || !isCall && !isMember {
		return b.binaryHead(head, indent)
	}

	tail := 0
	breaks := false
	b.measureWidth(func() int {
		operand := b.expr(tails[0].operand)
		breaks = document.HasForcedBreak(operand)
		tail = len(" "+string(tails[0].op.Text)+" ") + document.FlatWidth(operand, b.cfg.TabWidth)

		return 0
	})

	if breaks || tail >= document.ForcedBreakWidth {
		return b.binaryHead(head, indent)
	}

	saved := b.chainStmtTail
	b.chainStmtTail += tail
	b.preserveChainNext = true

	doc := b.binaryHead(head, indent)

	b.chainStmtTail = saved
	b.preserveChainNext = false

	return doc
}

type binaryTail struct {
	op      parser.Token
	operand parser.Expr
}

// flattenBinary collapses a run of one operator into its leftmost operand and the
// following `op operand` segments, stopping at a different-kind child so a
// higher-precedence group stays together. Right-associative operators (`??`, `**`)
// flatten down the right child. Flattening the whole spine here keeps the emit
// linear (per-level recursion would re-measure the remaining chain each time).
func flattenBinary(b *parser.BinaryExpr) (parser.Expr, []binaryTail) {
	if b.Op.Kind == parser.KindNullCoalesce || b.Op.Kind == parser.KindStarStar {
		return flattenBinaryRight(b)
	}

	var tails []binaryTail

	cur := b

	for {
		tails = append(tails, binaryTail{op: cur.Op, operand: cur.Right})

		left, ok := cur.Left.(*parser.BinaryExpr)
		if !ok || left.Op.Kind != b.Op.Kind {
			slices.Reverse(tails)

			return cur.Left, tails
		}

		cur = left
	}
}

func flattenBinaryRight(b *parser.BinaryExpr) (parser.Expr, []binaryTail) {
	var tails []binaryTail

	cur := b

	for {
		right, ok := cur.Right.(*parser.BinaryExpr)
		if ok && right.Op.Kind == b.Op.Kind {
			tails = append(tails, binaryTail{op: cur.Op, operand: right.Left})
			cur = right

			continue
		}

		tails = append(tails, binaryTail{op: cur.Op, operand: cur.Right})

		return b.Left, tails
	}
}

// concatSpaces reports which sides of a tight `.` between left and right need a
// space, so neither neighbor fuses with the operator into a float: `0.` after an
// integer and `.0` before any number. A float on the left already holds its own
// dot, so `1.5.'x'` lexes as written. Pint's concat_space keeps exactly these
// spaces, except that it drops the one before a float, which breaks the code.
func concatSpaces(left, right parser.Expr) (before, after bool) {
	return exprEndsWithInt(left), exprStartsWithNumber(right)
}

func exprEndsWithInt(expr parser.Expr) bool {
	switch x := expr.(type) {
	case *parser.IntExpr:
		return true
	case *parser.BinaryExpr:
		return exprEndsWithInt(x.Right)
	case *parser.AssignExpr:
		return exprEndsWithInt(x.Right)
	case *parser.TernaryExpr:
		return exprEndsWithInt(x.Else)
	case *parser.UnaryExpr:
		return exprEndsWithInt(x.Expr)
	case *parser.SpreadExpr:
		return exprEndsWithInt(x.Expr)
	case *parser.CastExpr:
		// A cast's own tokens are its parentheses, so the literal after them is
		// what a following `.` would fuse with.
		return exprEndsWithInt(x.Expr)
	case *parser.UpdateExpr:
		return x.Prefix && exprEndsWithInt(x.Operand)
	default:
		return false
	}
}

// exprStartsWithNumber reports whether expr's leftmost leaf is a numeric
// literal, so a preceding tight `.` would lex as a float. It descends the
// value/postfix node kinds whose leftmost child can be a literal (e.g. the
// callee of `0()`, the array of `0[1]`); the rest open with a keyword or
// punctuation that cannot start a float.
func exprStartsWithNumber(expr parser.Expr) bool {
	switch x := expr.(type) {
	case *parser.IntExpr, *parser.FloatExpr:
		return true
	case *parser.BinaryExpr:
		return exprStartsWithNumber(x.Left)
	case *parser.AssignExpr:
		return exprStartsWithNumber(x.Left)
	case *parser.TernaryExpr:
		return exprStartsWithNumber(x.Cond)
	case *parser.InstanceofExpr:
		return exprStartsWithNumber(x.Left)
	case *parser.CallExpr:
		return exprStartsWithNumber(x.Callee)
	case *parser.FirstClassCallableExpr:
		return exprStartsWithNumber(x.Callee)
	case *parser.MemberExpr:
		return exprStartsWithNumber(x.Object)
	case *parser.StaticMemberExpr:
		return exprStartsWithNumber(x.Class)
	case *parser.SubscriptExpr:
		return exprStartsWithNumber(x.Array)
	case *parser.UpdateExpr:
		return !x.Prefix && exprStartsWithNumber(x.Operand)
	default:
		return false
	}
}

func prefixOperand(e parser.Expr) parser.Expr {
	for {
		switch n := e.(type) {
		case *parser.CastExpr:
			e = n.Expr
		case *parser.UnaryExpr:
			e = n.Expr
		default:
			return e
		}
	}
}

// Glued to a parenthesised head's `)`, an operand that can break would widen
// the last line and push a break back inside the parentheses.
func (b *builder) parenHeadWidens(last parser.Expr, operands []parser.Expr) bool {
	if _, paren := last.(*parser.ParenExpr); !paren {
		return false
	}

	return slices.ContainsFunc(operands, func(operand parser.Expr) bool {
		breaks := false
		b.measureWidth(func() int {
			breaks = document.CanBreak(b.expr(operand))

			return 0
		})

		return breaks
	})
}

func endsOnBreakableCloser(e parser.Expr) bool {
	for s, ok := e.(*parser.SubscriptExpr); ok; s, ok = e.(*parser.SubscriptExpr) {
		e = s.Array
	}

	switch n := e.(type) {
	case *parser.CallExpr:
		return len(n.Args) > 0
	case *parser.NewExpr:
		return len(n.Args) > 0
	case *parser.ArrayExpr:
		return len(n.Elements) > 0
	case *parser.ParenExpr, *parser.MatchExpr:
		return true
	}

	return false
}

// continued: an enclosing assignment already broke before its operator, so
// this link shares that indent.
func (b *builder) assign(n *parser.AssignExpr, stmtLevel, continued bool) *document.Doc {
	// The right side is the statement's value, so a chain there may preserve its
	// author breaks; the left side is a target, so it builds with the flag cleared.
	left := b.expr(n.Left)

	op := string(n.Op.Text) + " "
	if n.ByRef {
		op += "&"
	}

	flatTarget := !document.CanBreak(left)
	lead := b.chainStmtLead + flatWidth(left, b.cfg.TabWidth) + len(" ") + len(op)

	broken := b.authorBroke(n.Op.Span.Start.Offset) ||
		stmtLevel && flatTarget && b.binaryHeadOverflows(n.Right, lead, op)

	if !broken && b.valueBelowList(n, left) {
		b.chainStmtLevel = stmtLevel
		b.chainStmtLead = 0

		b.depth++
		value := b.assignValue(n.Right)
		b.depth--

		// Flat as measured: where the real column is wider than the build
		// sees (a directive's arguments), a list the printer broke would read
		// back as author-broken and take the other layout.
		list := b.flatDoc(left)

		return document.Concat(list, document.Text(" "+strings.TrimSpace(op)), document.Indent(document.Concat(document.HardLine(), value)))
	}

	if !broken {
		// Only while the target itself stays on one line: a target that can break
		// moves the value off the statement's indent, and a preserved chain would
		// reproduce its breaks against the wrong one.
		b.chainStmtLevel = stmtLevel && flatTarget
		if b.chainStmtLevel {
			b.chainStmtLead = lead
		}

		if _, chained := n.Right.(*parser.AssignExpr); chained && !b.chainStmtLevel {
			saved := b.chainStmtLead
			b.chainStmtLead = lead
			value := b.assignValue(n.Right)
			b.chainStmtLead = saved

			return document.Concat(left, document.Text(" "+op), value)
		}

		return document.Concat(left, document.Text(" "+op), b.assignValue(n.Right))
	}

	b.chainStmtLevel = stmtLevel
	if stmtLevel {
		b.chainStmtLead = len(op)
	}

	if !continued {
		b.depth++
	}

	var value *document.Doc
	if next, ok := n.Right.(*parser.AssignExpr); ok {
		b.chainStmtLevel = false
		value = b.assign(next, stmtLevel, true)
	} else {
		value = b.assignValue(n.Right)
	}

	if continued {
		return document.Concat(left, document.HardLine(), document.Text(op), value)
	}

	b.depth--

	return document.Concat(left, document.Indent(document.Concat(document.HardLine(), document.Text(op), value)))
}

func (b *builder) valueBelowList(n *parser.AssignExpr, left *document.Doc) bool {
	if _, ok := n.Left.(*parser.ArrayExpr); !ok || n.ByRef || document.HasForcedBreak(left) {
		return false
	}

	line := b.depth*b.cfg.TabWidth + b.chainStmtLead + flatWidth(left, b.cfg.TabWidth) + len(" =")
	if line > b.cfg.PrintWidth {
		return false
	}

	opener := b.measureWidth(func() int {
		saved := b.chainStmtLead
		if _, chained := n.Right.(*parser.AssignExpr); chained {
			b.chainStmtLead = line + len(" ") - b.depth*b.cfg.TabWidth
		}

		width, _ := document.BrokenPrefixWidth(b.expr(n.Right), b.cfg.TabWidth)
		b.chainStmtLead = saved

		return width
	})

	return line+len(" ")+opener > b.cfg.PrintWidth
}

func (b *builder) assignValue(e parser.Expr) *document.Doc {
	lead := b.exprLeadComments(e)
	if lead == nil {
		return b.expr(e)
	}

	if !document.HasForcedBreak(lead) {
		return document.Concat(lead, b.expr(e))
	}

	b.depth++
	value := b.expr(e)
	b.depth--

	return document.Indent(document.Concat(lead, value))
}

// A parenthesised or binary head counts, and so does one with no break point
// at all (`$this->items ?? []`), which would otherwise overflow beside the
// operator. A call or chain head has clean break points of its own, and a
// concatenation fills its own lines.
func (b *builder) binaryHeadOverflows(value parser.Expr, lead int, op string) bool {
	run, ok := value.(*parser.BinaryExpr)
	if !ok {
		return false
	}

	head, _ := flattenBinary(run)

	grouped := false

	switch head.(type) {
	case *parser.ParenExpr, *parser.BinaryExpr:
		grouped = true
	}

	if !grouped && run.Op.Kind == parser.KindDot {
		return false
	}

	breakable := false
	width := b.measureWidth(func() int {
		doc := b.expr(head)
		breakable = document.CanBreak(doc)

		return document.FlatWidth(doc, b.cfg.TabWidth)
	})

	if !grouped && breakable {
		return false
	}
	indent := b.depth * b.cfg.TabWidth

	return indent+lead+width > b.cfg.PrintWidth &&
		indent+b.cfg.TabWidth+len(op)+width <= b.cfg.PrintWidth
}

// unaryFuses reports whether writing op straight before operand would join them
// into a longer token. `- -$b` becoming `--$b` turns a read into a pre-decrement
// that writes to $b, and it lints clean, so nothing downstream catches it; `- -0`
// and `- --$b` become `--0` and `---$b`, which PHP will not parse.
//
// Only a repeated sign fuses. `~~`, `-+` and `+-` are not PHP operators, and a
// binary operator before a unary sign is already spaced by the binary printer.
func unaryFuses(op parser.Token, operand parser.Expr) bool {
	var lead parser.Token

	switch n := operand.(type) {
	case *parser.UnaryExpr:
		lead = n.Op
	case *parser.UpdateExpr:
		if !n.Prefix {
			return false
		}

		lead = n.Op
	default:
		return false
	}

	switch op.Kind {
	case parser.KindMinus:
		return lead.Kind == parser.KindMinus || lead.Kind == parser.KindMinusMinus
	case parser.KindPlus:
		return lead.Kind == parser.KindPlus || lead.Kind == parser.KindPlusPlus
	default:
		return false
	}
}

// isDefaultValue reports whether e is a fallback a reader skims past (`[]`,
// `null`, `false`, `0`, an empty string): one that can sit beside a block's
// opener or closer.
func isDefaultValue(e parser.Expr) bool {
	switch v := e.(type) {
	case *parser.ArrayExpr:
		return !arrayHasValue(v.Elements)
	case *parser.IntExpr, *parser.FloatExpr, *parser.NullExpr, *parser.BoolExpr:
		return true
	case *parser.StringExpr:
		return len(v.Token.Text) == len("''")
	}

	return false
}

// lineKnown: the ternary starts a line whose indent the depth gives (a
// statement's value, an array entry's value), so its `? ` line's column is too.
func (b *builder) ternary(n *parser.TernaryExpr, lineKnown bool) *document.Doc {
	// A comment between the condition and a branch has no structural slot once the
	// ternary is reflowed; keep the author's form so it is not lost.
	if b.spanHasComment(n.Span()) {
		return b.verbatim(n.Span())
	}

	cond := b.expr(n.Cond)

	// Fit-first: flat `cond ? then : else` (or elvis `cond ?: else`) when it
	// fits, otherwise `?` and `:` lead their own indented lines (PSR-12).
	// Width-driven, so author breaks are ignored.

	if n.Then == nil {
		other := b.expr(n.Else)
		sep := document.Line()

		last := prefixOperand(n.Cond)
		if endsOnBreakableCloser(last) && !b.parenHeadWidens(last, []parser.Expr{n.Else}) {
			id := b.nextGroupID()
			cond = document.GroupID(id, cond)
			sep = document.IfBreakOf(id, document.Text(" "), sep)
		}

		body := document.Concat(sep, document.Text("?: "), other)

		return document.Group(document.Concat(cond, document.Indent(body)))
	}

	then, other := b.expr(n.Then), b.expr(n.Else)
	broken := document.Group(document.Concat(cond, document.Indent(document.Concat(
		document.Line(), document.Text("? "), then,
		document.Line(), document.Text(": "), other,
	))))

	if !lineKnown {
		return broken
	}

	if isDefaultValue(n.Else) {
		if width, ok := b.branchOpenerWidth(n.Then, then, 0); ok {
			glued := document.Concat(cond, document.Text(" ? "), document.Opener(width, then), document.Text(" : "), other)
			width += document.FlatWidth(cond, b.cfg.TabWidth) + len(" ? ")

			return document.ConditionalGroup(document.Opener(width, glued), broken)
		}
	}

	if isDefaultValue(n.Then) {
		// The else line also carries what ends the value: an entry's `,` or a
		// statement's `;`.
		if width, ok := b.branchOpenerWidth(n.Else, other, len(";")); ok {
			lead := document.Concat(cond, document.Text(" ? "), then, document.Text(" : "))
			glued := document.Concat(lead, document.Opener(width, other))
			width += document.FlatWidth(lead, b.cfg.TabWidth)

			return document.ConditionalGroup(document.Opener(width, glued), broken)
		}
	}

	return broken
}

func (b *builder) branchOpenerWidth(branch parser.Expr, doc *document.Doc, trail int) (int, bool) {
	width, ok := b.blockOpenerWidth(branch)
	if !ok || (b.depth+1)*b.cfg.TabWidth+len("? ")+b.oneLineWidth(doc)+trail <= b.cfg.PrintWidth {
		return 0, false
	}

	return width, true
}

func (b *builder) cast(n *parser.CastExpr) *document.Doc {
	// A comment inside the cast parens (`(/* a */ int)`) has no slot in the
	// normalized `(int)`; keep that region verbatim so it is not lost.
	parens := parser.Span{Start: n.LParen.Span.Start, End: n.RParen.Span.End}
	if b.spanHasComment(parens) {
		return document.Concat(b.verbatim(parens), document.Text(" "), b.expr(n.Expr))
	}

	return document.Concat(document.Text("("), document.Text(normalizeCastType(n.Type.Text)), document.Text(") "), b.parenBlockDoc(n.Expr))
}

// normalizeCastType returns the canonical lowercase cast keyword, choosing the
// short spelling of each alias pair (`integer` to `int`, `double` to `float`);
// PHP evaluates every variant to the same cast.
func normalizeCastType(text []byte) string {
	lower := strings.ToLower(string(text))

	switch lower {
	case "integer":
		return "int"
	case "boolean":
		return "bool"
	case "double":
		return "float"
	case "binary":
		return "string"
	default:
		return lower
	}
}

func (b *builder) closure(n *parser.ClosureExpr) *document.Doc {
	parts := make([]*document.Doc, 0, 8+len(n.Attributes))
	parts = append(parts, b.inlineAttributes(n.Attributes)...)

	if has(n.Static) {
		parts = append(parts, document.Text("static "))
	}

	parts = append(parts, document.Text("function "))

	if has(n.ByRef) {
		parts = append(parts, document.Text("&"))
	}

	parts = append(parts, b.closureParams(n))

	if len(n.Uses) > 0 {
		uses := make([]*document.Doc, len(n.Uses))
		for i, u := range n.Uses {
			ref := document.Text("")
			if has(u.ByRef) {
				ref = document.Text("&")
			}

			uses[i] = document.Concat(ref, b.tok(u.Var))
		}

		// The use list breaks one per line when it overflows, like a parameter
		// list.
		useSpan := parser.Span{Start: n.UseLParen.Span.Start, End: n.UseRParen.Span.End}
		if len(uses) == 1 && !b.spanHasComment(useSpan) {
			parts = append(parts, document.Text(" use ("), uses[0], document.Text(")"))
		} else {
			useComment, useBreaks := b.trailingParenComment(n.UseRParen)
			parts = append(parts, delimitedListComment(" use (", uses, withTrailingComma, 0, useComment, useBreaks))
		}
	}

	parts = append(parts, b.returnType(n.ReturnColon, n.ReturnType), document.Text(" "), b.block(n.Body))

	return document.Concat(parts...)
}

// `(\n    $x,\n)` saves no width, so a single parameter stays on the line.
func (b *builder) closureParams(n *parser.ClosureExpr) *document.Doc {
	span := parser.Span{Start: n.LParen.Span.Start, End: n.RParen.Span.End}
	if len(n.Params) != 1 || b.spanHasComment(span) {
		return b.params(n.Params, n.RParen)
	}

	param := b.param(n.Params[0])
	if document.HasForcedBreak(param) {
		return b.params(n.Params, n.RParen)
	}

	return document.Concat(document.Text("("), b.flatDoc(param), document.Text(")"))
}

func (b *builder) arrowFn(n *parser.ArrowFnExpr) *document.Doc {
	parts := make([]*document.Doc, 0, 6+len(n.Attributes))
	parts = append(parts, b.inlineAttributes(n.Attributes)...)

	if has(n.Static) {
		parts = append(parts, document.Text("static "))
	}

	// `fn` sits tight against `(` (Laravel/PER-CS) unless cfg.PHP.ClosureFnSpace
	// adds a space (Pint closure_fn_spacing); a by-ref `&` always spaces.
	parts = append(parts, document.Text("fn"))

	switch {
	case has(n.ByRef):
		parts = append(parts, document.Text(" &"))
	case b.cfg.PHP.ClosureFnSpace:
		parts = append(parts, document.Text(" "))
	}

	params, broken := b.arrowParams(n.Params, n.RParen)
	returnType := b.returnType(n.ReturnColon, n.ReturnType)
	arrow := document.Text(" => ")

	// The arrow starts no further left than its own indent, so a hug that
	// overflows from there overflows wherever the arrow prints: the call goes
	// one argument per line instead, as a statement's call does (postfix).
	if call, ok := n.Body.(*parser.CallExpr); ok && hugsInsideOut(call) {
		saved := b.chainStmtLead
		b.chainStmtLead = document.FlatWidth(document.Concat(append(slices.Clone(parts), params, returnType, arrow)...), b.cfg.TabWidth)

		if b.staysOnStatementLine(call) && b.hugLineOverflows(call, true) && b.unhuggingHelps(call) {
			b.noHugNext = true
		}

		b.chainStmtLead = saved
	}

	_, b.arrowBodyNext = n.Body.(*parser.CallExpr)
	body := b.expr(n.Body)
	b.arrowBodyNext = false

	flat := document.Concat(append(slices.Clone(parts), params, returnType, arrow, body)...)
	if broken == nil {
		return flat
	}

	// The parameters break only when even the body's narrowest first line
	// cannot follow the signature; the printer judges that at the real column.
	signature := document.Concat(append(slices.Clone(parts), params, returnType, arrow)...)
	first, _ := document.BrokenPrefixWidth(body, b.cfg.TabWidth)
	opener := document.FlatWidth(signature, b.cfg.TabWidth) + first

	wide := document.Concat(append(parts, broken, returnType, arrow, body)...)

	return document.ConditionalGroup(document.Opener(opener, flat), wide)
}

// arrowParams renders an arrow function's parameter list. An arrow's body has
// no brace to end the signature line, so a plain params group's fit check
// scans into the body and would break a compact signature one parameter per
// line to make room for it. The signature stays on one line unless it cannot
// fit a line on its own; broken is the one-per-line alternative, or nil.
func (b *builder) arrowParams(ps []*parser.Param, rparen parser.Token) (params, broken *document.Doc) {
	if len(ps) == 0 {
		return b.params(ps, rparen), nil
	}

	docs, comment, breaks := b.paramParts(ps, rparen)
	grouped := delimitedListComment("(", docs, withTrailingComma, 0, comment, breaks)

	// A list that breaks anyway stays a group, so its lines take the indent
	// they print at; printed to text here they would lose it.
	if breaks || document.FlatWidth(grouped, b.cfg.TabWidth) > b.cfg.PrintWidth-2*b.cfg.TabWidth {
		return grouped, nil
	}

	flat := document.Print(grouped, document.Options{
		PrintWidth: inlineWidth,
		TabWidth:   b.cfg.TabWidth,
		UseTabs:    b.cfg.UseTabs(),
		EOL:        docEOL(b.cfg),
	})

	if len(ps) < 2 {
		return document.Text(string(flat)), nil
	}

	return document.Text(string(flat)), delimitedListComment("(", docs, withTrailingComma, 0, comment, true)
}

func (b *builder) match(n *parser.MatchExpr) *document.Doc {
	if len(n.Arms) == 0 {
		body := b.danglingComments(n.RBrace)
		if body == nil {
			body = document.Text("{}")
		}

		return document.Concat(document.Concat(document.Text("match "), b.parenGroup(b.expr(n.Subject), n.RParen)), document.Text(" "), body)
	}

	heads := make([]*document.Doc, len(n.Arms))
	results := make([]*document.Doc, len(n.Arms))
	below := make([]bool, len(n.Arms))

	b.depth++

	savedArm := b.armStart

	// Arms print one per line, so a result sits like a statement's value: its
	// column is fixed by the head, ` => ` and, when aligned, at most the widest
	// head's padding. That lets an author-broken chain in it keep its shape.
	leads := b.matchResultLeads(n.Arms)

	for i, arm := range n.Arms {
		// The arm list places the comments at an arm's own start offset, so the
		// condition expression must not claim them.
		b.armStart = matchArmStartOffset(arm)
		results[i] = b.stmtExpr(arm.Result, leads[i], len(","))
		heads[i] = b.matchArmHead(arm, results[i])

		// A condition that can break takes the break itself, as Pint lays it out.
		if !document.CanBreak(heads[i]) && b.breaksAfterArrow(arm.Result, heads[i], results[i]) {
			results[i] = document.Group(document.Indent(document.Concat(document.Line(), results[i])))
			below[i] = true
		}
	}

	b.armStart = savedArm
	b.depth--

	// Arms always print one per line, so the gap needs no IfBreak. An arm whose
	// condition wrapped carries its gap on the wrapped line and aligns by that
	// line.
	var gaps []*document.Doc

	if b.cfg.PHP.AlignFatArrow && len(n.Arms) >= 2 {
		b.alignScope++
		offsets := make([]int, len(n.Arms))
		for i, arm := range n.Arms {
			offsets[i] = matchArmStartOffset(arm)
		}

		gaps = b.alignGaps(heads, b.alignBreaks(offsets), func(i int) (int, int, bool) {
			first, _ := document.BrokenPrefixWidth(results[i], b.cfg.TabWidth)

			return b.oneLineWidth(results[i]), first, document.CanBreak(results[i])
		})
	}

	arms := make([]*document.Doc, 0, len(n.Arms)*3)

	for i, arm := range n.Arms {
		offset := matchArmStartOffset(arm)

		if i == 0 {
			arms = append(arms, document.HardLine())

			for _, c := range b.allComments(offset) {
				arms = append(arms, c, document.HardLine())
			}
		} else {
			lead, trail := b.splitComments(offset)

			if len(trail) > 0 {
				arms = append(arms, document.LineSuffix(document.Concat(trail...)))
			}

			arms = append(arms, document.HardLine())

			// Preserve an authored blank line between arms: it is a visual separator
			// that ends a `=>` alignment scope, so dropping it would flip the scope
			// (and thus the padding) on the next pass.
			if b.blankBefore(offset) {
				arms = append(arms, document.HardLine())
			}

			for _, c := range lead {
				arms = append(arms, c, document.HardLine())
			}
		}

		arrow := " => "
		if below[i] {
			arrow = " =>"
		}

		sep := document.Text(arrow)
		if i < len(gaps) && gaps[i] != nil {
			sep = document.Concat(gaps[i], document.Text(arrow[len(" "):]))
		}

		arms = append(arms, document.Concat(heads[i], sep, results[i], document.Text(",")))
	}

	tailLead, tailTrail := b.splitComments(n.RBrace.Span.Start.Offset)

	if len(tailTrail) > 0 {
		arms = append(arms, document.LineSuffix(document.Concat(tailTrail...)))
	}

	for _, c := range tailLead {
		arms = append(arms, document.HardLine(), c)
	}

	return document.Concat(
		document.Concat(document.Text("match "), b.parenGroup(b.expr(n.Subject), n.RParen)), document.Text(" {"),
		document.Indent(document.Concat(arms...)),
		document.HardLine(), document.Text("}"),
	)
}

// matchArmHead builds an arm's condition side. The flat-or-broken choice is made
// against the first line of the result (a preserved chain goes on below it),
// never against the alignment padding, which the layout reserves on its own.
func (b *builder) matchArmHead(arm *parser.MatchArm, result *document.Doc) *document.Doc {
	if arm.IsDefault {
		return document.Text("default")
	}

	resultWidth, _ := document.FlatPrefixWidth(result, b.cfg.TabWidth)

	condDocs := make([]*document.Doc, len(arm.Conds))
	for j, c := range arm.Conds {
		condDocs[j] = b.expr(c)
	}

	if len(arm.Conds) == 1 {
		// A result that cannot break and leaves no column overflows whatever
		// the condition does, so break nothing.
		avail := b.cfg.PrintWidth - b.depth*b.cfg.TabWidth -
			4 - resultWidth - 1

		if avail <= 0 && !document.CanBreak(result) && !document.HasForcedBreak(condDocs[0]) {
			return b.flatDoc(condDocs[0])
		}

		return condDocs[0]
	}

	flat := document.Join(document.Text(", "), condDocs)
	armWidth := b.depth*b.cfg.TabWidth + flatWidth(flat, b.cfg.TabWidth) +
		4 + resultWidth + 1

	// Only a result that can break opens a hug; one kept as written cannot.
	if armWidth > b.cfg.PrintWidth && document.CanBreak(result) {
		opener := b.measureWidth(func() int { return b.hugOpenerWidth(arm.Result) })
		armWidth += opener - resultWidth
	}

	authorBroken := slices.ContainsFunc(arm.Conds[1:], func(c parser.Expr) bool {
		return b.authorBroke(c.Span().Start.Offset)
	})

	if authorBroken || document.HasForcedBreak(flat) || armWidth > b.cfg.PrintWidth {
		return document.Join(document.Concat(document.Text(","), document.HardLine()), condDocs)
	}

	return flat
}

func matchArmStartOffset(a *parser.MatchArm) int {
	if a.IsDefault {
		return a.DefaultKeyword.Span.Start.Offset
	}

	return a.Conds[0].Span().Start.Offset
}

func (b *builder) yield(n *parser.YieldExpr) *document.Doc {
	// A comment wedged inside the yield (`yield /* note */ $x`, or a comment that
	// splits `yield from`) is dropped by the normalized form and can change how the
	// result re-parses; keep the author's form so it round-trips.
	if b.spanHasComment(n.Span()) {
		return b.verbatim(n.Span())
	}

	if n.Value == nil {
		return document.Text("yield")
	}

	if n.Key != nil {
		return document.Concat(document.Text("yield "), b.expr(n.Key), document.Text(" => "), b.expr(n.Value))
	}

	return document.Concat(document.Text("yield "), b.expr(n.Value))
}

// nonSpaceLen counts the bytes of s that are not layout whitespace, so a span
// measured across a source line break yields its flat width.
func nonSpaceLen(s []byte) int {
	n := 0

	for _, c := range s {
		// The lexer's own class, not a second copy of it: `\f` and `\v` are
		// trivia too, so counting them charges width to bytes the output drops.
		if !parser.IsWhitespace(c) {
			n++
		}
	}

	return n
}

// flatSourceWidth measures src[start:end] as its flat form: nonSpaceLen, less
// the trailing commas only a broken list prints, so a list the previous pass
// broke measures as it did before.
func (b *builder) flatSourceWidth(start, end int) int {
	width := nonSpaceLen(b.src[start:end])

	i := sort.Search(len(b.tokens), func(i int) bool { return b.tokens[i].Span.Start.Offset >= start })

	for ; i+1 < len(b.tokens) && b.tokens[i].Span.End.Offset <= end; i++ {
		next := b.tokens[i+1].Kind
		if b.tokens[i].Kind == parser.KindComma && (next == parser.KindRParen || next == parser.KindRBracket) {
			width--
		}
	}

	return width
}

// flatText renders an expression on one line and returns it as literal text, so
// it carries no break points into an enclosing hug.
func (b *builder) flatText(e parser.Expr) *document.Doc {
	return b.flatDoc(b.expr(e))
}

// flatDoc is flatText for a doc that is already built, so measuring it cannot
// re-consume the expression's comments.
func (b *builder) flatDoc(doc *document.Doc) *document.Doc {
	return document.Text(string(document.Print(doc, document.Options{
		PrintWidth: inlineWidth,
		TabWidth:   b.cfg.TabWidth,
		UseTabs:    b.cfg.UseTabs(),
		EOL:        docEOL(b.cfg),
	})))
}

// A key never carries a forced break, so the print is single-line.
//
// WARNING: printing and counting runes is not the same as document.FlatWidth,
// which charges a tab tabWidth columns where this counts it as one. Swapping
// them moves the `=>` column of every aligned array or match arm whose key
// holds a tab.
func flatWidth(d *document.Doc, tabWidth int) int {
	out := document.Print(d, document.Options{PrintWidth: inlineWidth, TabWidth: tabWidth})

	return utf8.RuneCount(out)
}
