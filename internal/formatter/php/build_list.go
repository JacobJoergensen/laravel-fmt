package php

import (
	"bytes"
	"slices"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// argList builds `(args)`. A huggable final argument (array, closure, match)
// hugs: the leading args stay on the open-paren line and the last carries the
// break. Otherwise the list is flat, or one argument per indented line with a
// trailing comma.
func (b *builder) argList(args []parser.Expr, trailing trailingComma) *document.Doc {
	doc, _ := b.argListHug(args, trailing)

	return doc
}

func (b *builder) argListHug(args []parser.Expr, trailing trailingComma) (*document.Doc, bool) {
	noHug := b.noHugNext
	linkArgs := b.linkArgsNext
	hugLead := b.hugLeadNext
	b.linkArgsNext = false
	b.noHugNext = false
	b.hugLeadNext = 0

	if len(args) == 0 {
		return document.Text("()"), false
	}

	// A comment before the `)` needs the broken list's slot after the last
	// argument; a hug ends on that argument's own closer and has none.
	rparen := b.argsCloser(args)
	if commentTrivia(b.leadingAt(rparen.Span.Start.Offset)) > 0 && rparen.Kind == parser.KindRParen {
		noHug = true
	} else {
		rparen = parser.Token{}
	}

	// A first argument on its own line is the author choosing one argument per
	// line; Pint's on_multiline: ignore keeps it too. A hug never prints that
	// newline, so hugged output reads back as hugged.
	authorBroken := b.authorBroke(args[0].Span().Start.Offset)

	last := len(args) - 1

	// Last-argument hug: the leading args have no break points, so the hugged
	// argument's own group decides flat vs. broken. An all-named list reads better
	// stacked one per line, so it keeps the broken layout rather than hugging.
	allNamed := len(args) >= 2 && allNamedArgs(args)

	// A sole argument is built once, here, for both the width test and the
	// output: measuring it with a separate build doubles the work at every level
	// of a nested call.
	soleMark := b.claimMark()

	var sole *document.Doc
	if !allNamed && !noHug && !authorBroken && soleArgShape(args) {
		sole = b.expr(args[0])
	}

	soleOverflows := sole != nil && b.soleArgOverflows(sole, linkArgs)
	hugs := isHuggableArg(args[last]) || soleOverflows || b.lastArrowOverflows(args, linkArgs)

	if hugs && !allNamed && !noHug && !authorBroken {
		leading := make([]*document.Doc, last)
		leadingWidth := 0
		leadingSimple := true
		leadingFlattened := false
		arrayFlattened := false
		mark := b.claimMark()

		// Flat, a breakable leading arg cannot split mid-list. Before a call-like
		// hug it would fan out a scalar (call_arrow_tail_stacks).
		flattensLeading := isBlockLikeArg(args[last]) || isArrayArg(args[last]) || wrapsArray(args[last])

		for i := range last {
			leading[i] = b.expr(args[i])

			switch {
			case !document.CanBreak(leading[i]):
			case !flattensLeading || document.HasForcedBreak(leading[i]):
				leadingSimple = false
			default:
				leading[i] = b.flatDoc(leading[i])
				leadingFlattened = true
				arrayFlattened = arrayFlattened || isArrayArg(args[i])
			}

			leadingWidth += document.FlatWidth(leading[i], b.cfg.TabWidth) + len(", ")
		}

		// Behind a flattened leading array, a last array that fits its own line
		// would explode while its sibling stays flat; both go one per line instead.
		var lastDoc *document.Doc
		if arrayFlattened && isArrayArg(args[last]) {
			lastDoc = b.expr(args[last])
			if !document.HasForcedBreak(lastDoc) && !b.soleArgOverflows(lastDoc, linkArgs) {
				leadingSimple = false
			}
		}

		// Hug only when the head that carries the hug fits. That head is the
		// leading args plus the hugged argument's own opener, so count a hugged
		// call's `callee(`: `sprintf(…, $this->wideMethod(` overflows where the
		// leading args alone sit under the budget. A wide head rides past the
		// print width and reads better one argument per line. The opener width
		// comes from the source span, not a doc build, so measuring it cannot
		// re-consume the callee's comments; it counts non-whitespace only, so a
		// callee the formatter already broke across lines (`foo()\n    ->bar(`)
		// measures the same as its flat form and the hug decision does not flip
		// between passes.
		lastOpenerWidth := 0
		if call, ok := args[last].(*parser.CallExpr); ok {
			lastOpenerWidth = b.flatSourceWidth(call.Callee.Span().Start.Offset, call.LParen.Span.End.Offset)
		}

		budget := b.hugHeadBudget()

		// Where the caller knows the column the `(` opens at, the hug's line is
		// measured from there instead of the callee reserve.
		if hugLead > 0 {
			budget = max(b.cfg.PrintWidth-hugLead-len("("), 1)
		}

		// A call or `new` last-argument brings its own `callee(` onto the head and,
		// once hugged, reads inside-out (`f($a, $b, g(` … `))`), so it keeps the
		// conservative half-width budget. An array or heredoc adds only its opener and
		// hugging it is the canonical Laravel shape (`Log::warning('msg', [`), so it
		// is judged against the width actually available.
		if readsInsideOut(args[last]) {
			budget = b.cfg.PrintWidth / 2
		}

		widthOK := isBlockLikeArg(args[last]) || leadingWidth+lastOpenerWidth <= budget

		// An arrow function has no brace of its own, so it hugs only when the line
		// up to its body's opener fits. A body that is a fluent chain opening no
		// block never hugs: its links would hang at the call's indent and read as
		// steps of the call around it.
		lastArg := args[last]
		if n, ok := lastArg.(*parser.NamedArgExpr); ok {
			lastArg = n.Value
		}

		if arrow, ok := lastArg.(*parser.ArrowFnExpr); ok {
			// A body that has to break opens at its first break, not its flat end.
			var opener int

			whole := false

			if soleOverflows {
				opener, _ = document.BrokenPrefixWidth(sole, b.cfg.TabWidth)
			} else {
				opener = b.measureWidth(func() int { return b.hugOpenerWidth(args[last]) })

				// An opener that spans the whole argument leaves nothing to break
				// below it: hugged, the line could only split a list kept whole.
				whole = opener >= b.measureWidth(func() int {
					doc := b.expr(args[last])
					if document.HasForcedBreak(doc) {
						return document.ForcedBreakWidth
					}

					return document.FlatWidth(doc, b.cfg.TabWidth)
				})
			}

			// A binary body opens with its first operand whole: breaking inside it
			// to keep the hug splits a tighter operator.
			if head := b.arrowBinaryHeadWidth(arrow); head > opener {
				opener = head
			}

			widthOK = !hangsChainLinks(arrow.Body) && !whole && leadingWidth+opener <= budget
		}

		if leadingFlattened && widthOK {
			opener := b.measureWidth(func() int { return b.hugOpenerWidth(args[last]) })
			if c, ok := args[last].(*parser.ClosureExpr); ok && shortListClosure(c) == nil {
				opener = b.closureUseOpenerWidth(c, opener)
			}

			widthOK = leadingWidth+opener <= budget
		}

		if leadingSimple && widthOK {
			parts := make([]*document.Doc, 0, len(args)+3)
			parts = append(parts, document.Text("("))

			for i := range last {
				parts = append(parts, leading[i], document.Text(", "))
			}

			if lastDoc == nil {
				lastDoc = sole
			}

			if lastDoc == nil {
				lastDoc = b.expr(args[last])
			}

			parts = append(parts, lastDoc, document.Text(")"))

			return document.Concat(parts...), true
		}

		b.releaseClaims(mark)
	}

	// First-argument hug: a huggable leading argument followed only by short,
	// unbreakable args stays hugged (`f([...], $x, $y)` -> the array wraps, the rest
	// ride the closing line). The leading argument's own group decides flat vs
	// broken against the trailing width, so this is width-driven and idempotent.
	// An arrow function is excluded: it breaks through its `=>` body (a chain hangs
	// one call per line with no closing delimiter), so the trailing args would be
	// stranded after the last body line instead of riding a clean closer. Such a
	// call reads better one argument per line (`array_map(fn ..., $items)`). A
	// call or `new` is excluded too: hugged, its closer leaves `), $more, $args)`
	// trailing a line that reads as the inner call's end, unless that call ends
	// in an array: `]), $more)` closes the array first, as Mago hugs it.
	pair := isArrayPair(args)
	if !allNamed && !noHug && !authorBroken && isHuggableArg(args[0]) && !isArrowFnArg(args[0]) &&
		(!isCallLikeArg(args[0]) || callEndsInArray(args[0])) &&
		(pair || b.trailingArgsHug(args[0], args[1:])) && !b.leadingArgStaysUnhugged(args) {
		trailingArg := b.flatText
		if pair {
			trailingArg = b.expr
		}

		parts := make([]*document.Doc, 0, len(args)+2)
		parts = append(parts, document.Text("("), b.expr(args[0]))

		for i := 1; i < len(args); i++ {
			parts = append(parts, document.Text(", "), trailingArg(args[i]))
		}

		parts = append(parts, document.Text(")"))

		return document.Concat(parts...), false
	}

	// An argument holding a closure or match breaks the list whatever the width,
	// so it is built a level in, where it prints. The test reads the source: a
	// level keyed on a width break would differ between passes.
	if sole != nil && b.holdsBlock(args[0]) {
		b.releaseClaims(soleMark)
		sole = nil
	}

	docs := make([]*document.Doc, len(args))
	for i, a := range args {
		if sole != nil {
			docs[i] = sole

			continue
		}

		if !b.holdsBlock(a) {
			docs[i] = b.listedArg(a)

			continue
		}

		b.depth++
		docs[i] = b.listedArg(a)
		b.depth--
	}

	comment, breaks := b.trailingParenComment(rparen)

	return delimitedListComment("(", docs, trailing, 0, comment, authorBroken || breaks), false
}

func (b *builder) holdsBlock(e parser.Expr) bool {
	span := e.Span()

	i, _ := slices.BinarySearchFunc(b.tokens, span.Start.Offset, func(t parser.Token, offset int) int {
		return t.Span.Start.Offset - offset
	})

	for ; i < len(b.tokens) && b.tokens[i].Span.Start.Offset < span.End.Offset; i++ {
		if k := b.tokens[i].Kind; k == parser.KindFunction || k == parser.KindMatch {
			return true
		}
	}

	return false
}

// argsCloser returns the list's `)`, or the zero token when another delimiter
// closes it.
func (b *builder) argsCloser(args []parser.Expr) parser.Token {
	end := args[len(args)-1].Span().End.Offset

	i, _ := slices.BinarySearchFunc(b.tokens, end, func(t parser.Token, offset int) int {
		return t.Span.Start.Offset - offset
	})

	if i < len(b.tokens) && b.tokens[i].Kind == parser.KindComma {
		i++
	}

	if i < len(b.tokens) && b.tokens[i].Kind == parser.KindRParen {
		return b.tokens[i]
	}

	return parser.Token{}
}

// closureUseOpenerWidth narrows a closure's hug opener, its signature up to the
// body's `{`, to `use (`: a long use list breaks one variable per line below it,
// so the hug's line ends there.
func (b *builder) closureUseOpenerWidth(c *parser.ClosureExpr, signature int) int {
	if len(c.Uses) == 0 {
		return signature
	}

	rest := len(", ")*(len(c.Uses)-1) + len(")") + len(" {")
	for _, u := range c.Uses {
		rest += len(u.ByRef.Text) + len(u.Var.Text)
	}

	if has(c.ReturnColon) {
		span := c.ReturnType.Span()
		rest += len(": ") + b.flatSourceWidth(span.Start.Offset, span.End.Offset)
	}

	return signature - rest
}

// leadingArgIsMinor reports whether a first-argument hug would carry no more
// than the arguments riding its closer; the hug pays only for a payload.
func (b *builder) leadingArgIsMinor(args []parser.Expr) bool {
	limit := b.cfg.PrintWidth - (b.depth+1)*b.cfg.TabWidth - len(",")
	trailing := 0

	for i, a := range args {
		width := b.measureWidth(func() int {
			doc := b.expr(a)
			if document.HasForcedBreak(doc) {
				return limit + 1
			}

			return document.FlatWidth(doc, b.cfg.TabWidth)
		})

		if width > limit {
			return false
		}

		if i > 0 {
			trailing += width
		}
	}

	first := b.measureWidth(func() int { return document.FlatWidth(b.expr(args[0]), b.cfg.TabWidth) })

	return first <= trailing
}

// A bare array hugs only when it breaks anyway, as Mago and Biome print it; a
// call ending in an array (`__('k', [..])`) keeps the payload rule, as Mago hugs it.
func (b *builder) leadingArgStaysUnhugged(args []parser.Expr) bool {
	if isCallLikeArg(args[0]) {
		return b.leadingArgIsMinor(args)
	}

	return b.argsFitTheirOwnLines(args)
}

func (b *builder) argsFitTheirOwnLines(args []parser.Expr) bool {
	limit := b.cfg.PrintWidth - (b.depth+1)*b.cfg.TabWidth - len(",")

	for _, a := range args {
		width := b.measureWidth(func() int {
			doc := b.expr(a)
			if document.HasForcedBreak(doc) {
				return limit + 1
			}

			return document.FlatWidth(doc, b.cfg.TabWidth)
		})

		if width > limit {
			return false
		}
	}

	return true
}

// A binary breaks only once the list has, so it starts its own line and
// continues level with its first operand, as a condition does. Concatenation
// keeps its indent: its pieces fill lines as one string.
func (b *builder) listedArg(a parser.Expr) *document.Doc {
	n, ok := a.(*parser.BinaryExpr)
	if !ok || n.Op.Kind == parser.KindDot {
		return b.expr(a)
	}

	if lead := b.exprLeadComments(a); lead != nil {
		return document.Concat(lead, b.binaryIndented(n, false, false))
	}

	return b.binaryIndented(n, false, false)
}

// hangsChainLinks reports whether e is a chain that opens no block, so a break
// in it falls between its links: two or more steps, or one step on a call head
// (`resolve(X::class)->render(…)`), which breaks before that step.
func hangsChainLinks(e parser.Expr) bool {
	call, ok := e.(*parser.CallExpr)
	if !ok || opensBlock(call) {
		return false
	}

	return len(collectChainSpine(call)) >= 2 || chainHeadCall(call) != nil
}

func opensBlock(e parser.Expr) bool {
	if n, ok := e.(*parser.NamedArgExpr); ok {
		e = n.Value
	}

	switch v := e.(type) {
	case *parser.ArrayExpr:
		return arrayHasValue(v.Elements)
	case *parser.ClosureExpr, *parser.MatchExpr:
		return true
	case *parser.StringExpr:
		return bytes.Contains(v.Token.Text, []byte("\n"))
	case *parser.ArrowFnExpr:
		return opensBlock(v.Body)
	case *parser.CallExpr:
		return len(v.Args) > 0 && opensBlock(v.Args[len(v.Args)-1])
	case *parser.NewExpr:
		return len(v.Args) > 0 && opensBlock(v.Args[len(v.Args)-1])
	}

	return false
}

// allNamedArgs reports whether every entry in args is a NamedArgExpr; the
// structural signal that the call is configuration-shaped and reads better one
// entry per line once two or more carry the label. An empty list returns true;
// callers gate on a count threshold.
func allNamedArgs(args []parser.Expr) bool {
	for _, a := range args {
		if _, ok := a.(*parser.NamedArgExpr); !ok {
			return false
		}
	}

	return true
}

// isCallLikeArg reports whether a hugged last argument opens with its own callee,
// so hugging it nests one call's break inside another's argument list.
func isCallLikeArg(arg parser.Expr) bool {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	switch arg.(type) {
	case *parser.CallExpr, *parser.NewExpr:
		return true
	}

	return false
}

// hugCalleeReserve stands in for the callee that precedes an argument list
// (`$this->log`, `Log::warning`). argList sees only the arguments, so the head it
// is measuring is this much wider than it can observe.
const hugCalleeReserve = 20

// hugHeadBudget is the width a hugged head may occupy: the print width less the
// call's indent and an allowance for the callee.
func (b *builder) hugHeadBudget() int {
	return max(b.cfg.PrintWidth-b.depth*b.cfg.TabWidth-hugCalleeReserve, 1)
}

func (b *builder) trailingArgsHug(lead parser.Expr, trailing []parser.Expr) bool {
	if len(trailing) == 0 {
		return false
	}

	width := b.depth*b.cfg.TabWidth + hugCloserWidth(lead) + len(");")

	for _, a := range trailing {
		if isHuggableArg(a) {
			return false
		}

		w := b.measureWidth(func() int { return document.FlatWidth(b.expr(a), b.cfg.TabWidth) })
		if w >= document.ForcedBreakWidth {
			return false
		}

		width += len(", ") + w
	}

	return width <= b.cfg.PrintWidth
}

// isArrowFnArg reports whether arg is an arrow function (a NamedArgExpr is
// unwrapped first). An arrow breaks through its `=>` body with no closing
// delimiter, so it cannot carry a first-argument hug ahead of trailing args.
func isArrowFnArg(arg parser.Expr) bool {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	_, ok := arg.(*parser.ArrowFnExpr)

	return ok
}

func isArrayPair(args []parser.Expr) bool {
	if len(args) != 2 {
		return false
	}

	for _, a := range args {
		arr, ok := a.(*parser.ArrayExpr)
		if !ok || !arrayHasValue(arr.Elements) {
			return false
		}
	}

	return true
}

func callEndsInArray(arg parser.Expr) bool {
	call, ok := arg.(*parser.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}

	arr, ok := call.Args[len(call.Args)-1].(*parser.ArrayExpr)

	return ok && arrayHasValue(arr.Elements)
}

func readsInsideOut(arg parser.Expr) bool {
	return isCallLikeArg(arg) && !wrapsArray(arg)
}

// A call wrapping only an array (`array_filter([`) hugs as the array does.
func wrapsArray(arg parser.Expr) bool {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	call, ok := arg.(*parser.CallExpr)

	return ok && len(call.Args) == 1 && isArrayArg(call.Args[0])
}

func isArrayArg(arg parser.Expr) bool {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	_, ok := arg.(*parser.ArrayExpr)

	return ok
}

// isHuggableArg reports whether arg can carry an overflowing call's wrap without
// forcing the preceding arguments onto their own lines: a non-empty
// closure/array/match, a multi-line string, or a call/`new` whose own list breaks
// cleanly (2+ args or a huggable last arg). Empties are excluded (nothing to
// break); a NamedArgExpr is unwrapped first.
func isHuggableArg(arg parser.Expr) bool {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	switch v := arg.(type) {
	case *parser.ClosureExpr:
		return v.Body != nil && len(v.Body.Stmts) > 0
	case *parser.CastExpr:
		return castsArray(v)
	case *parser.ArrayExpr:
		return arrayHasValue(v.Elements)
	case *parser.MatchExpr:
		return len(v.Arms) > 0
	case *parser.StringExpr:
		// A heredoc/nowdoc carries its own line structure, so the `<<<'X'` opener
		// belongs on the open-paren line and the closing `X` tight against the `)`.
		// A single-line string has no break point (loneValueArgStaysInline).
		return bytes.Contains(v.Token.Text, []byte("\n"))
	case *parser.NewExpr:
		return len(v.Args) >= 2 || (len(v.Args) > 0 && isHuggableArg(v.Args[len(v.Args)-1]))
	case *parser.ArrowFnExpr:
		return isHuggableArrowBody(v.Body)
	case *parser.BinaryExpr:
		// A concatenation ending in a match ends on the match's block, so it hugs
		// as the match does (`__('key.'.match ($x) {`), as Mago and Pint keep it.
		if v.Op.Kind != parser.KindDot {
			return false
		}

		_, tails := flattenBinary(v)
		m, ok := tails[len(tails)-1].operand.(*parser.MatchExpr)

		return ok && len(m.Arms) > 0
	case *parser.CallExpr:
		// One exception: a multi-step fluent chain (`$q->where(…)->where(…)`) never
		// hugs as a direct argument (no clean early break), so it breaks the
		// surrounding argument list instead.
		if steps := collectChainSpine(v); len(steps) >= 2 {
			return false
		}

		return len(v.Args) >= 2 || (len(v.Args) > 0 && isHuggableArg(v.Args[len(v.Args)-1]))
	}

	return false
}

func soleArgShape(args []parser.Expr) bool {
	if len(args) != 1 {
		return false
	}

	arg := args[0]
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	switch v := arg.(type) {
	case *parser.ArrowFnExpr:
		return !isFluentChain(v.Body)
	case *parser.CallExpr:
		return len(v.Args) > 0 && !isFluentChain(v)
	case *parser.NewExpr:
		return len(v.Args) == 1
	}

	return false
}

// isFluentChain reports whether e, behind any cast, makes two calls along one
// chain (`$q->a()->b()`, `resolve(X)->render()`). A property path
// (`$this->source->items(`) is one call.
func isFluentChain(e parser.Expr) bool {
	if c, ok := e.(*parser.CastExpr); ok {
		e = c.Expr
	}

	call, ok := e.(*parser.CallExpr)
	if !ok {
		return false
	}

	steps := collectChainSpine(call)
	if len(steps) == 0 {
		return false
	}

	calls := 0
	if _, ok := steps[0].member.Object.(*parser.CallExpr); ok {
		calls++
	}

	for _, step := range steps {
		if step.call != nil {
			calls++
		}
	}

	return calls >= 2
}

// lastArrowOverflows reports whether a last arrow argument after others would
// break even on a line of its own, so hugging it costs nothing it saves.
func (b *builder) lastArrowOverflows(args []parser.Expr, linkArgs bool) bool {
	if len(args) < 2 || !isArrowFnArg(args[len(args)-1]) {
		return false
	}

	var overflows bool

	b.measureWidth(func() int {
		overflows = b.soleArgOverflows(b.expr(args[len(args)-1]), linkArgs)

		return 0
	})

	return overflows
}

// 0 when the body is not a binary run.
func (b *builder) arrowBinaryHeadWidth(arrow *parser.ArrowFnExpr) int {
	run, ok := arrow.Body.(*parser.BinaryExpr)
	if !ok || run.Op.Kind == parser.KindDot {
		return 0
	}

	head, _ := flattenBinary(run)

	return b.measureWidth(func() int {
		prefix := document.FlatWidth(b.expr(arrow), b.cfg.TabWidth) - document.FlatWidth(b.expr(arrow.Body), b.cfg.TabWidth)

		return prefix + document.FlatWidth(b.expr(head), b.cfg.TabWidth)
	})
}

func (b *builder) soleArgOverflows(arg *document.Doc, linkArgs bool) bool {
	depth := b.depth + 1
	if linkArgs {
		depth++
	}

	return depth*b.cfg.TabWidth+document.FlatWidth(arg, b.cfg.TabWidth)+len(",") > b.cfg.PrintWidth
}

// isHuggableArrowBody reports whether an arrow function's body wraps cleanly
// enough for the arrow function to be hugged as a call argument. A closure,
// non-empty array, or match body qualifies; a call/`new` body qualifies when its
// argument list breaks (callBodyWraps). A single scalar-argument call does NOT;
// hugging it would fan one bare scalar across lines, which is stricter than
// isHuggableArg's CallExpr arm, because an arrow adds a `fn … =>` prefix.
func isHuggableArrowBody(body parser.Expr) bool {
	switch v := body.(type) {
	case *parser.ClosureExpr:
		return v.Body != nil && len(v.Body.Stmts) > 0
	case *parser.CastExpr:
		return castsArray(v)
	case *parser.ArrayExpr:
		return arrayHasValue(v.Elements)
	case *parser.MatchExpr:
		return len(v.Arms) > 0
	case *parser.ArrowFnExpr:
		return isHuggableArrowBody(v.Body)
	case *parser.CallExpr:
		// A multi-step chain body hangs cleanly after `=> $q` (the arrow hugs and
		// the chain breaks one `->` per line). This is the one place a multi-step
		// chain IS huggable; as a direct call argument it has no `=>` anchor.
		if steps := collectChainSpine(v); len(steps) >= 2 {
			return true
		}

		return callBodyWraps(v.Args)
	case *parser.NewExpr:
		return callBodyWraps(v.Args)
	}

	return false
}

// callBodyWraps reports whether a call/`new` argument list breaks cleanly: 2+
// arguments, a huggable last argument, or a single argument that is a fluent chain
// (including behind a cast like `(float) $q->…->sum(…)`). A single scalar does not
// qualify.
func callBodyWraps(args []parser.Expr) bool {
	if len(args) >= 2 {
		return true
	}

	if len(args) == 0 {
		return false
	}

	last := args[len(args)-1]
	if isHuggableArg(last) {
		return true
	}

	if c, ok := last.(*parser.CastExpr); ok {
		last = c.Expr
	}

	steps := collectChainSpine(last)

	return len(steps) >= 2
}

// delimitedListComment puts one item per line when they do not fit or breaks
// is set, with a trailing comma only then. comment goes after that comma,
// since a line comment on the last item takes the rest of its line. Id 0
// leaves the group unnamed.
func delimitedListComment(open string, items []*document.Doc, trailing trailingComma, id int32, comment *document.Doc, breaks bool) *document.Doc {
	separator := document.Concat(document.Text(","), document.Line())

	tail := document.Text("")
	if trailing {
		tail = document.IfBreak(document.Text(","), document.Text(""))
	}

	if comment != nil {
		tail = document.Concat(tail, comment)
	}

	if breaks {
		tail = document.Concat(tail, document.BreakParent())
	}

	return document.GroupID(id, document.Concat(
		document.Text(open),
		document.Indent(document.Concat(document.SoftLine(), document.Join(separator, items))),
		tail,
		document.SoftLine(),
		document.Text(")"),
	))
}

// trailingComma selects whether a broken delimited list ends with a comma.
type trailingComma bool

const (
	withTrailingComma    trailingComma = true
	withoutTrailingComma trailingComma = false
)

// phpConstructCalls are the callees PHP parses with a fixed grammar rather than
// as a function call, so their parentheses have no room for a trailing comma:
// `T_EMPTY '(' expr ')'` and `T_EVAL '(' expr ')'` on every version, and
// `exit_expr: '(' optional_expr ')'` before 8.4. laravel-fmt has no target
// version, so exit and die are listed too: Laravel still supports PHP 8.2.
var phpConstructCalls = [][]byte{
	[]byte("empty"),
	[]byte("eval"),
	[]byte("exit"),
	[]byte("die"),
}

// rejectsTrailingComma reports whether a call's parentheses belong to one of
// those constructs. A qualified name (`\empty(...)`) is an ordinary call to a
// global function of that name, so it keeps the comma.
func rejectsTrailingComma(callee parser.Expr) bool {
	ident, ok := callee.(*parser.IdentExpr)
	if !ok || ident.LeadingBackslash.Kind != parser.KindInvalid || len(ident.Parts) > 0 {
		return false
	}

	return slices.ContainsFunc(phpConstructCalls, func(name []byte) bool {
		return bytes.EqualFold(ident.Token.Text, name)
	})
}

func (b *builder) array(n *parser.ArrayExpr) *document.Doc {
	// Long `array(...)` is rewritten to short `[...]` unless the config disables
	// it (Pint array_syntax: short, default on); a semantics-preserving rewrite.
	open, closer := "[", "]"

	switch {
	case n.KeywordPrefix.Kind == parser.KindList:
		// list(...) shares this node with an array literal because it is the same
		// grammar, but it is a destructuring pattern and stays written as one:
		// rewriting it to [...] is list_syntax, a rule this formatter leaves alone.
		open, closer = "list(", ")"
	case n.KeywordPrefix.Kind == parser.KindArray && !b.cfg.PHP.ShortArray:
		open, closer = "array(", ")"
	}

	// A comment on the array's opening token (`/* c */ array(...)` or
	// `/* c */ [...]`) is mid-expression trivia the layout would otherwise drop.
	// Re-home it, unless this array opens its statement (then buildSequence already
	// emits that leading comment).
	firstTok := n.Open
	if has(n.KeywordPrefix) {
		firstTok = n.KeywordPrefix
	}

	var lead *document.Doc
	if off := firstTok.Span.Start.Offset; off != b.stmtStart && off != b.elemStart {
		lead = b.inlineLeadComment(off)
	}

	if len(n.Elements) == 0 {
		// An array with no elements can still hold a comment (a commented-out
		// entry); print it on its own line rather than dropping it.
		comments := b.allComments(n.Close.Span.Start.Offset)
		if len(comments) == 0 {
			return document.Text(open + closer)
		}

		parts := make([]*document.Doc, 0, len(comments)*2)
		for _, c := range comments {
			parts = append(parts, document.HardLine(), c)
		}

		return document.GroupBreak(document.Concat(
			document.Text(open),
			document.Indent(document.Concat(parts...)),
			document.HardLine(),
			document.Text(closer),
		))
	}

	// A hole (`[$a, , $c]`) carries meaning in its exact comma position, which a
	// reflowed list cannot round-trip; keep such arrays verbatim.
	for _, el := range n.Elements {
		if el.Value == nil && !el.Spread {
			return b.verbatim(n.Span())
		}
	}

	if inner := b.soleHuggedElement(n); inner != nil && open == "[" {
		d := document.Concat(document.Text(open), b.expr(inner), document.Text(closer))
		if lead != nil {
			return document.Concat(lead, d)
		}

		return d
	}

	// With `=>` alignment on, each keyed element gets a gap the printer widens
	// once the layout is settled. A flat array keeps the plain ` => `, so only a
	// broken one is ever aligned.
	var gaps []*document.Doc
	if b.cfg.PHP.AlignFatArrow && hasKeyedElement(n.Elements) {
		b.alignScope++

		keys := make([]*document.Doc, len(n.Elements))
		b.measureWidth(func() int {
			for i, el := range n.Elements {
				if el.Key != nil && !el.Spread {
					keys[i] = b.expr(el.Key)
				}
			}

			return 0
		})

		offsets := make([]int, len(n.Elements))
		for i, el := range n.Elements {
			offsets[i] = el.Span().Start.Offset
		}

		gaps = b.alignGaps(keys, b.alignBreaks(offsets), func(i int) (int, int, bool) {
			breaks := false
			first := 0
			width := b.measureWidth(func() int {
				value := b.expr(n.Elements[i].Value)
				breaks = document.CanBreak(value)
				first, _ = document.BrokenPrefixWidth(value, b.cfg.TabWidth)

				return b.oneLineWidth(value)
			})

			return width, first, breaks
		})

		for i, g := range gaps {
			if g != nil {
				gaps[i] = document.IfBreak(g, document.Text(" "))
			}
		}
	}

	b.depth++
	inner, hasComment := b.arrayElements(n, gaps)
	b.depth--

	body := document.Concat(
		document.Text(open),
		document.Indent(document.Concat(document.SoftLine(), inner)),
		document.SoftLine(),
		document.Text(closer),
	)

	// Force the array broken when it carries an interior comment (a line
	// comment cannot survive a flat line) or when the author put its first
	// element on its own line. Both markers are reproduced in the output, so
	// the choice is a stable fixed point.
	result := document.Group(body)
	if hasComment || b.authorBroke(n.Elements[0].Span().Start.Offset) {
		result = document.GroupBreak(body)
	}

	if lead != nil {
		return document.Concat(lead, result)
	}

	return result
}

// soleHuggedElement returns an array's one element when it hugs the brackets:
// an array (`[[…]]`) or a `new` whose only argument is an array
// (`[new X([…])]`), which both open and close on the outer brackets' lines.
func (b *builder) soleHuggedElement(n *parser.ArrayExpr) parser.Expr {
	if len(n.Elements) != 1 {
		return nil
	}

	el := n.Elements[0]
	if el.Key != nil || el.Spread || el.ByRef || b.authorBroke(el.Value.Span().Start.Offset) {
		return nil
	}

	inner, ok := el.Value.(*parser.ArrayExpr)
	if created, isNew := el.Value.(*parser.NewExpr); isNew && len(created.Args) == 1 {
		inner, ok = created.Args[0].(*parser.ArrayExpr)
	}

	if !ok || !arrayHasValue(inner.Elements) {
		return nil
	}

	if inner.KeywordPrefix.Kind == parser.KindList || inner.KeywordPrefix.Kind == parser.KindArray && !b.cfg.PHP.ShortArray {
		return nil
	}

	before := parser.Span{Start: n.Open.Span.Start, End: el.Value.Span().Start}
	after := parser.Span{Start: el.Value.Span().End, End: n.Close.Span.End}

	if b.spanHasComment(before) || b.spanHasComment(after) {
		return nil
	}

	return el.Value
}

func hasKeyedElement(elements []*parser.ArrayElement) bool {
	for _, el := range elements {
		if el.Spread {
			continue
		}

		if el.Key != nil {
			return true
		}
	}

	return false
}

// arrayHasValue reports whether elems holds a real entry rather than only
// destructuring holes, which give a wrapped layout nothing to break around.
func arrayHasValue(elems []*parser.ArrayElement) bool {
	for _, el := range elems {
		if el.Value != nil {
			return true
		}
	}

	return false
}

// inlineLeadComment returns the block comments in the leading trivia at offset as
// inline `/* c */ ` docs, for re-homing a comment attached to a token the layout
// rewrites away (the `array` keyword normalized to `[`).
func (b *builder) inlineLeadComment(offset int) *document.Doc {
	var parts []*document.Doc

	for _, tr := range b.leadingAt(offset) {
		if tr.Kind == parser.TriviaBlockComment && !b.emitted[tr.Span.Start.Offset] {
			b.claim(tr)
			parts = append(parts, document.Concat(b.commentVerbatim(tr), document.Text(" ")))
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return document.Concat(parts...)
}

// arrayElements lays out an array's elements with their comments and commas,
// reporting whether any comment was found so the caller can force the array
// broken. gaps, when non-nil, carries each element's aligned key-to-`=>` gap.
func (b *builder) arrayElements(n *parser.ArrayExpr, gaps []*document.Doc) (*document.Doc, bool) {
	parts := make([]*document.Doc, 0, len(n.Elements)*4)
	hasComment := false
	last := len(n.Elements) - 1

	for i, el := range n.Elements {
		offset := el.Span().Start.Offset

		if i == 0 {
			if b.blankBefore(offset) {
				parts = append(parts, document.HardLine())
			}

			// Nothing precedes the first element, so every comment before it prints
			// on its own line. allComments keeps the raw text (no leading space),
			// which is what a comment sharing the `[` line collapses to on a reprint.
			comments := b.allComments(offset)
			for _, c := range comments {
				parts = append(parts, c, document.HardLine())
				hasComment = true
			}

			if len(comments) > 0 && b.blankAfterComments(offset) {
				parts = append(parts, document.HardLine())
			}
		} else {
			lead, trail := b.splitComments(offset)

			if len(trail) > 0 {
				parts = append(parts, document.LineSuffix(document.Concat(trail...)))
				hasComment = true
			}

			parts = append(parts, document.Line())

			if b.blankBefore(offset) {
				parts = append(parts, document.HardLine())
			}

			for _, c := range lead {
				parts = append(parts, c, document.HardLine())
				hasComment = true
			}

			if len(lead) > 0 && b.blankAfterComments(offset) {
				parts = append(parts, document.HardLine())
			}
		}

		var gap *document.Doc
		if gaps != nil {
			gap = gaps[i]
		}

		prev := b.elemStart
		b.elemStart = offset
		parts = append(parts, b.arrayElement(el, gap))
		b.elemStart = prev

		if i < last {
			parts = append(parts, document.Text(","))
		} else {
			parts = append(parts, document.IfBreak(document.Text(","), document.Text("")))
		}
	}

	// Comments between the last element and the closing bracket: the last
	// element's trailing comment, then any standalone comments before the closer.
	tailLead, tailTrail := b.splitComments(n.Close.Span.Start.Offset)

	if len(tailTrail) > 0 {
		parts = append(parts, document.LineSuffix(document.Concat(tailTrail...)))
		hasComment = true
	}

	if b.blankBefore(n.Close.Span.Start.Offset) {
		parts = append(parts, document.HardLine())
	}

	for _, c := range tailLead {
		parts = append(parts, document.HardLine(), c)
		hasComment = true
	}

	return document.Concat(parts...), hasComment
}

// arrayElement lays out one element; gap, when non-nil, replaces the default
// single space before `=>`.
func (b *builder) arrayElement(el *parser.ArrayElement, gap *document.Doc) *document.Doc {
	if el.Spread {
		return document.Concat(document.Text("..."), b.elementValue(el))
	}

	// A skipped element in a destructuring pattern (`[$a, , $c]`) has no value.
	if el.Value == nil {
		return document.Text("")
	}

	value := b.elementValue(el)

	if el.ByRef {
		value = document.Concat(document.Text("&"), value)
	}

	if el.Key != nil {
		key := b.expr(el.Key)

		arrow := " => "
		if b.breaksAfterArrow(el.Value, key, value) {
			arrow = " =>"
			value = document.Group(document.Indent(document.Concat(document.Line(), value)))
		}

		sep := document.Text(arrow)
		if gap != nil {
			sep = document.Concat(gap, document.Text(arrow[len(" "):]))
		}

		return document.Concat(key, sep, value)
	}

	return value
}

// Concatenation has its own fill.
func (b *builder) breaksAfterArrow(e parser.Expr, key, value *document.Doc) bool {
	if b.cfg.PHP.ArrowValueOnKeyLine {
		return false
	}

	run, binary := e.(*parser.BinaryExpr)
	if binary && run.Op.Kind == parser.KindDot {
		return false
	}

	width := document.FlatWidth(value, b.cfg.TabWidth) + len(",")
	indent := b.depth * b.cfg.TabWidth
	head := indent + document.FlatWidth(key, b.cfg.TabWidth) + len(" => ")

	if head+width <= b.cfg.PrintWidth {
		return false
	}

	if binary && indent+b.cfg.TabWidth+width <= b.cfg.PrintWidth {
		return true
	}

	// An unbreakable value overflows beside its key, as Pint keeps it.
	if !document.CanBreak(value) {
		return false
	}

	opener, _ := document.BrokenPrefixWidth(value, b.cfg.TabWidth)

	// A first operand that is itself a binary opens whole: a break inside it
	// would split a tighter operator (`=== $id` left under `&&`).
	if binary {
		if first, _ := flattenBinary(run); isBinaryExpr(first) {
			firstWidth := b.measureWidth(func() int {
				return document.FlatWidth(b.expr(first), b.cfg.TabWidth)
			})
			opener = max(opener, firstWidth)
		}
	}

	return head+opener > b.cfg.PrintWidth
}

// elementValue builds an element's value with the element's lead-comment
// offset claimed, so a value that re-homes leading trivia (an array) does not
// duplicate comments the element list already prints.
func (b *builder) elementValue(el *parser.ArrayElement) *document.Doc {
	prev := b.elemStart
	b.elemStart = el.Span().Start.Offset
	b.lineKnownNext = true

	// An assignment measures from the column its target starts at, the list's
	// indent once the list breaks; any lead before the list is not on its line.
	savedLead := b.chainStmtLead
	if _, assigns := el.Value.(*parser.AssignExpr); assigns && el.Key == nil {
		b.chainStmtLead = 0
	}

	value := b.expr(el.Value)
	b.chainStmtLead = savedLead
	b.lineKnownNext = false
	b.elemStart = prev

	return value
}

func castsArray(cast *parser.CastExpr) bool {
	arr, ok := cast.Expr.(*parser.ArrayExpr)

	return ok && arrayHasValue(arr.Elements)
}

func isBinaryExpr(e parser.Expr) bool {
	_, ok := e.(*parser.BinaryExpr)

	return ok
}
