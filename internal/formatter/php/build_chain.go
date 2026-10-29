package php

// Member-chain layout: a postfix expression (calls, `->`/`?->`, `::`,
// subscripts) flattens into a head plus links. A fluent chain breaks one link
// per line when it overflows, unless the author already broke it and it fits.

import (
	"slices"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

type chainLink struct {
	doc         *document.Doc
	arrow       bool          // a `->`/`?->` step
	call        bool          // an invocation (has an argument list), not a bare property/const/subscript
	noArgs      bool          // an invocation whose argument list is empty
	blockHug    bool          // its last argument is a braced/bracketed body that hugs the opener
	access      bool          // carries a chain operator (`->`, `?->`, `::`), so it counts as a link
	brokeBefore bool          // the author wrote a line break before this link's operator
	method      string        // the name a `->name(...)` call link invokes, else empty
	args        *document.Doc // that call's argument list, else nil
	argsHugged  bool          // that list hugs its last argument, so it breaks inside it, not on its own `)` line
	hugCloses   bool          // that hug is an arrow returning a plain call, so its broken closer line is `))`
}

func (b *builder) postfix(e parser.Expr, stmtLevel bool) *document.Doc {
	arrowBody := b.arrowBodyNext
	b.arrowBodyNext = false
	preserve := b.preserveChainNext
	b.preserveChainNext = false

	// A comment trailing a chain step (`->where(...) // note`) attaches to the
	// next step's arrow and has no slot in a reflowed chain; keep the author's
	// form so it is not lost. Comments buried in arguments (closures, arrays) are
	// handled by their own builders, so only the chain's own steps are checked.
	if b.chainHasStepComment(e) {
		return b.verbatim(e.Span())
	}

	// A statement's own call is the one place the hug's real line is known; when
	// it overflows into a call or an arrow function, which hug inside-out, or a
	// short-list closure, the call goes one argument per line. Any other closure,
	// array or match keeps its hug, the Laravel shape even past the width.
	// flattenChain builds the outermost call first.
	if call, ok := e.(*parser.CallExpr); ok && stmtLevel && hugsInsideOut(call) &&
		b.staysOnStatementLine(e) && (b.hugLineOverflows(call, true) || b.stmtArrowFitsAlone(call)) && b.unhuggingHelps(call) {
		b.noHugNext = true
	}

	savedChain := b.stmtChain
	b.stmtChain = stmtLevel
	head, links := b.flattenChain(e)
	b.stmtChain = savedChain

	methodCalls := 0
	for _, l := range links {
		if l.arrow && l.call {
			methodCalls++
		}
	}

	headDoc := b.parenBlockDoc(head)

	// A chain the author laid out across lines keeps that shape, so a run of sibling
	// chains reads as a table instead of collapsing on per-line fit. Two guards keep
	// this idempotent. The layout reproduces the author's own per-link breaks rather
	// than the width-driven head-hug below, so the emitted break pattern equals the
	// input's. And it only applies when every line of that layout fits, so a
	// width break the formatter emits itself reads back as the same layout. A
	// `->property` link counts as a step here, as the author broke before it.
	preserve = preserve || b.preserveAt != 0 && e.Span().Start.Offset == b.preserveAt

	if (stmtLevel || preserve) && chainAuthorBroken(links) && b.chainLayoutFits(headDoc, links) {
		return b.preservedChain(headDoc, links)
	}

	// Fewer than two `->method()` steps is not a fluent chain, so keep it inline
	// (a bare `->property` step is not a call and does not force a break),
	// EXCEPT a single method call on a call head with a non-huggable argument:
	// when it overflows, break the call onto its own line (`Route::get(...)`
	// then `->name(x)`) rather than breaking the method's argument.
	if methodCalls == 0 || (methodCalls == 1 && !b.qualifiesTrailingBreak(e, stmtLevel)) {
		return chainConcat(headDoc, links)
	}

	if isPestExpect(head, links) {
		firstAnd := 1 + slices.IndexFunc(links[1:], isPestAnd)
		// The ladder breaks before `->and` even after a property, where
		// tailRidesCloser looks past it, so that break is checked here.
		if !links[firstAnd].brokeBefore && b.tailRidesCloser(headDoc, links, firstAnd-1, false, stmtLevel) {
			return chainConcat(headDoc, links)
		}

		return b.pestChain(headDoc, links, stmtLevel)
	}

	headEnd := b.chainHeadEnd(head, links, methodCalls >= 2)

	// A variable or `$this` head attaches its first call, but only while that call
	// fits on the head line: attaching it anyway breaks the call's own arguments and
	// leaves every later link resuming after the closing paren. The group makes that
	// a width decision the printer takes at the real column, the only place the
	// statement's prefix is known. A chain with no later link cannot strand one, and
	// a call hugging a block argument keeps its opener, since carrying that body is
	// the point of the hug.
	hugged := headEnd >= 0 && headEnd < len(links)-1 &&
		links[headEnd].arrow && links[headEnd].call && !links[headEnd].blockHug

	// A head that prints multi-line anyway ends on its closer, and the links
	// after it ride that closer while they fit there: moving them down adds a
	// line and saves none.
	if headEnd >= 0 && b.tailRidesCloser(headDoc, links, headEnd, hugged, stmtLevel) {
		return chainConcat(headDoc, links)
	}

	// A tail ending in a block hug rides the closer below, breaking inside its
	// block: laddered, its arguments break and read back as authored.
	if headEnd >= 0 && !stmtLevel && !chainEndsInBlockHug(e) {
		if doc, ok := b.closerTailGroup(headDoc, links, headEnd); ok {
			return doc
		}
	}

	attached := headEnd
	if hugged {
		attached = headEnd - 1
	}

	// A step the layout gives no break of its own (the call after a property, a
	// bare `(...)` invocation) renders on the line before it, so it belongs to
	// the head and has to share the head's indent.
	//
	// When hugged the run belongs to the hug instead, which absorbs it below.
	if !hugged {
		attached = runEnd(links, attached)
	}

	headParts := make([]*document.Doc, 0, attached+2)
	headParts = append(headParts, headDoc)
	for i := 0; i <= attached; i++ {
		headParts = append(headParts, links[i].doc)
	}

	steps := make([]*document.Doc, 0, len(links))
	hugEnd := attached

	if hugged {
		// The hug takes its own trailing run for the same reason the head does:
		// those steps render on the hug's line, so they have to share its indent.
		hugEnd = runEnd(links, headEnd)

		hugParts := make([]*document.Doc, 0, hugEnd-headEnd+2)
		hugParts = append(hugParts, document.SoftLine())

		for i := headEnd; i <= hugEnd; i++ {
			hugParts = append(hugParts, links[i].doc)
		}

		steps = append(steps, document.Group(document.Concat(hugParts...)))
	}

	for i := max(hugEnd, attached) + 1; i < len(links); i++ {
		if stepTakesBreak(links, i) {
			steps = append(steps, document.SoftLine())
		}

		steps = append(steps, links[i].doc)
	}

	broken := document.Concat(document.Concat(headParts...), document.Indent(document.Concat(steps...)))

	// A statement's chain hugging a bracketed block keeps every step on the
	// line that opens it when that line fits (`X::factory()->for($a)->create([`,
	// `$this->post(route(…, [` … `])->assertOk()`). The printer cannot judge it:
	// it measures the array flat, contents and all.
	if stmtLevel && !b.loneArrayLinkFitsAlone(e) && b.compactBlockChainFits(e, links) {
		return chainConcat(headDoc, links)
	}

	// Elsewhere the column is the printer's, so it chooses: the chain stays on
	// the line that opens the array while that opening line fits. An arrow body
	// ladders a level in from the arrow's line, so, as at statement level, its
	// array link moves down when it fits flat there.
	if !stmtLevel && (!arrowBody || !b.arrowBodyLinkFitsAlone(e)) {
		if opener, ok := b.arrayHugOpener(e, headDoc, links); ok {
			glued := chainConcat(headDoc, links)

			return document.ConditionalGroup(document.Opener(opener, glued), document.Group(broken))
		}
	}

	call := trailingMemberCall(e)

	if methodCalls >= 2 && call != nil && !slices.ContainsFunc(links, brokenBefore) && b.argsPrintBroken(call) {
		glued := chainConcat(headDoc, links)
		if !stmtLevel {
			return glued
		}

		document.HasForcedBreak(glued)

		if width, _ := document.OpeningLineWidth(glued, b.cfg.IndentWidth); b.depth*b.cfg.IndentWidth+b.chainStmtLead+width <= b.cfg.PrintWidth {
			return glued
		}

		// Laddered, the last call's opener overflows all the same; the first
		// pass, whose arguments were not yet broken, kept it on the head.
		if b.lastCallBreaksAnyway(e, headDoc, links) {
			return b.onlyLastCallBreaks(headDoc, links)
		}

		return document.Group(broken)
	}

	if stmtLevel && methodCalls >= 2 && !slices.ContainsFunc(links, brokenBefore) && b.middleCallBreaks(headDoc, links) {
		return chainConcat(headDoc, links)
	}

	// Only the last call may break. The head and the links before it were
	// measured to fit, but left as groups the printer would break the first of
	// them against the flat width of everything after it.
	if stmtLevel && methodCalls >= 2 && !slices.ContainsFunc(links, brokenBefore) && b.lastCallBreaksAnyway(e, headDoc, links) {
		return b.onlyLastCallBreaks(headDoc, links)
	}

	// A chain ending in a hugged closure (`Route::prefix(...)->group(fn ...)`)
	// keeps every step on the opening line when that line fits, breaking only
	// the closure body; the broken layout is the fallback. A plain Group cannot
	// do this: the closure's forced break would always collapse it to broken. A
	// single call on a head that hugs a block argument gets the same choice, for
	// the same reason; beyond one call the hug belongs to an inner step rather
	// than to the head, and collapsing the whole chain onto it reads wrong.
	headHug := methodCalls == 1 && headHugsBlockArg(e) && !b.arrowHeadBreaksOnlyWhereWrapped(e)
	if chainEndsInBlockHug(e) || headHug {
		chosen := document.ConditionalGroup(chainConcat(headDoc, links), broken)
		if call != nil && b.argsPrintBroken(call) {
			return document.Concat(chosen, document.BreakParent())
		}

		return chosen
	}

	return document.Group(broken)
}

func (b *builder) headCallArgsBreak(headDoc *document.Doc, links []chainLink, i int, stmtLevel bool) bool {
	l := links[i]
	if l.args == nil {
		return false
	}

	if document.HasForcedBreak(l.args) {
		return true
	}

	// Only a list that can break absorbs the overflow; `()` cannot, and a hug
	// breaks inside its argument, whose closing line is known only for an
	// arrow returning a plain call.
	if !stmtLevel || !document.CanBreak(l.args) || l.argsHugged && !l.hugCloses {
		return false
	}

	base := b.depth * b.cfg.IndentWidth
	movedDown := base + b.cfg.IndentWidth + document.FlatWidth(l.doc, b.cfg.IndentWidth)

	onHead := base + b.chainStmtLead + document.FlatWidth(headDoc, b.cfg.IndentWidth)
	for _, prev := range links[:i+1] {
		onHead += document.FlatWidth(prev.doc, b.cfg.IndentWidth)
	}

	return onHead > b.cfg.PrintWidth && movedDown > b.cfg.PrintWidth
}

func (b *builder) tailRidesCloser(headDoc *document.Doc, links []chainLink, headEnd int, hugged, stmtLevel bool) bool {
	last := runEnd(links, headEnd)
	if last >= len(links)-1 || links[last+1].brokeBefore {
		return false
	}

	tailWidth := 0

	for _, l := range links[last+1:] {
		// Outside a statement the closer's column is only estimated, and a tail
		// that breaks on width would read back as authored.
		if document.HasForcedBreak(l.doc) || !stmtLevel && document.CanBreak(l.doc) {
			return false
		}

		tailWidth += document.FlatWidth(l.doc, b.cfg.IndentWidth)
	}

	closer, broke := b.closerLineWidth(headDoc, links[:last+1])
	if !broke {
		if !hugged || !b.headCallArgsBreak(headDoc, links, headEnd, stmtLevel) {
			return false
		}

		// The call breaks its own arguments, so its `)` opens the closer line;
		// a hug breaks inside its argument, whose own closer comes first.
		closer = len(")")
		if links[headEnd].argsHugged {
			closer += len(")")
		}

		for _, l := range links[headEnd+1 : last+1] {
			closer += document.FlatWidth(l.doc, b.cfg.IndentWidth)
		}
	}

	width := b.depth*b.cfg.IndentWidth + closer + tailWidth
	if stmtLevel {
		width += b.chainStmtTail
	}

	return width <= b.cfg.PrintWidth
}

// closerTailGroup lets the printer decide, at the column only it knows,
// whether the links after a head that breaks by force ride its closer or
// ladder below it. A tail that rides fits flat, so nothing in it breaks. As
// at statement level, only a break before the tail's first link is the
// author's choice to ladder.
func (b *builder) closerTailGroup(headDoc *document.Doc, links []chainLink, headEnd int) (*document.Doc, bool) {
	last := runEnd(links, headEnd)
	if last >= len(links)-1 || !links[last].blockHug || slices.ContainsFunc(links[:last+2], brokenBefore) {
		return nil, false
	}

	for _, l := range links[last+1:] {
		if document.HasForcedBreak(l.doc) {
			return nil, false
		}
	}

	if _, broke := b.closerLineWidth(headDoc, links[:last+1]); !broke {
		return nil, false
	}

	parts := make([]*document.Doc, 0, last+3)
	parts = append(parts, headDoc)

	for _, l := range links[:last+1] {
		parts = append(parts, l.doc)
	}

	tail := make([]*document.Doc, 0, 2*(len(links)-last))

	for i := last + 1; i < len(links); i++ {
		if stepTakesBreak(links, i) {
			tail = append(tail, document.SoftLine())
		}

		tail = append(tail, links[i].doc)
	}

	group := document.Group(document.Indent(document.Concat(tail...)))

	return document.Concat(append(parts, group)...), true
}

func (b *builder) closerLineWidth(headDoc *document.Doc, links []chainLink) (int, bool) {
	width := 0

	for _, l := range slices.Backward(links) {
		document.HasForcedBreak(l.doc)

		w, broke := document.ClosingLineWidth(l.doc, b.cfg.IndentWidth)
		width += w

		if broke {
			return width, true
		}
	}

	document.HasForcedBreak(headDoc)

	w, broke := document.ClosingLineWidth(headDoc, b.cfg.IndentWidth)

	return width + w, broke
}

func isPestExpect(head parser.Expr, links []chainLink) bool {
	if len(links) == 0 || !isExpectCall(head, links[0]) {
		return false
	}

	return slices.ContainsFunc(links[1:], isPestAnd)
}

func isExpectCall(head parser.Expr, first chainLink) bool {
	id, ok := head.(*parser.IdentExpr)

	return ok && string(id.Token.Text) == "expect" && isHelperCall(head, first)
}

func isPestAnd(l chainLink) bool {
	return l.call && l.method == "and"
}

// pestChain lays a Pest expectation out as Pest's docs do: each `->and(...)`
// starts a line and its matchers ride it. The width break produces the same
// layout, so an author's break before `->and` reads back unchanged. A line
// that overflows on its own breaks between its calls at the same indent.
func (b *builder) pestChain(headDoc *document.Doc, links []chainLink, stmtLevel bool) *document.Doc {
	var segments [][]int

	authored := false

	for i := range links {
		if i == 0 || isPestAnd(links[i]) {
			if i > 0 {
				authored = authored || links[i].brokeBefore
			}

			segments = append(segments, nil)
		}

		segments[len(segments)-1] = append(segments[len(segments)-1], i)
	}

	// A lone matcher that breaks anyway (a forced break, or too wide for its own
	// line) stays on its `->and(…)` line while that line fits up to the break:
	// moved down, the break would read back as authored. Only a statement knows
	// what follows the chain, so only a statement glues.
	matchers := func(idx []int, lead, tail int) (*document.Doc, bool) {
		calls := make([]*document.Doc, 0, len(idx)*2)
		glued := make([]*document.Doc, 0, len(idx))

		for _, i := range idx[1:] {
			if stepTakesBreak(links, i) {
				calls = append(calls, document.SoftLine())
			}

			calls = append(calls, links[i].doc)
			glued = append(glued, links[i].doc)
		}

		line := document.Concat(glued...)
		movedDown := (b.depth+1)*b.cfg.IndentWidth + document.FlatWidth(line, b.cfg.IndentWidth) + tail

		if stmtLevel && len(idx) == 2 && (document.HasForcedBreak(line) || movedDown > b.cfg.PrintWidth) {
			if width, _ := document.BrokenPrefixWidth(line, b.cfg.IndentWidth); lead+width <= b.cfg.PrintWidth {
				return line, true
			}
		}

		return document.Group(document.Concat(calls...)), false
	}

	rest := make([]*document.Doc, 0, len(segments)*2)
	for n, idx := range segments[1:] {
		tail := 0
		if n == len(segments)-2 {
			tail = b.chainStmtTail
		}

		lead := (b.depth+1)*b.cfg.IndentWidth + document.FlatWidth(links[idx[0]].doc, b.cfg.IndentWidth)
		segment, _ := matchers(idx, lead, tail)
		rest = append(rest, document.SoftLine(), links[idx[0]].doc, segment)
	}

	headRun := runEnd(links, 0)
	headParts := make([]*document.Doc, 0, headRun+2)
	headParts = append(headParts, headDoc)

	for _, l := range links[:headRun+1] {
		headParts = append(headParts, l.doc)
	}

	head := document.Concat(headParts...)
	first := segments[0][headRun:]
	lead := b.depth*b.cfg.IndentWidth + b.chainStmtLead + document.FlatWidth(head, b.cfg.IndentWidth)
	// A glued first matcher belongs to the head line, so its block closes at the
	// statement's indent and only the pairs after it are indented.
	firstDoc, glued := matchers(first, lead, 0)

	body := document.Concat(head, document.Indent(document.Concat(firstDoc, document.Concat(rest...))))
	if glued {
		body = document.Concat(head, firstDoc, document.Indent(document.Concat(rest...)))
	}

	if authored {
		return document.GroupBreak(body)
	}

	return document.Group(body)
}

// runEnd is the last link of the run that renders on the same line as link i:
// i plus every following link the layout gives no break of its own. A run has
// to stay under one indent, or its tail's own breaks land a level deeper than
// its head. i may be -1, meaning the head has taken no link yet.
func runEnd(links []chainLink, i int) int {
	for i+1 < len(links) && !stepTakesBreak(links, i+1) {
		i++
	}

	return i
}

// stepTakesBreak reports whether the broken layout starts a new line at link i.
// Only a chain operator does, and not one right after a property: a property
// modifies the call after it (`->not->toBeNull()`), so it takes the break that
// starts the pair and the call follows on the same line. Breaking between them
// strands a fragment that reads like a typo.
//
// A link this answers false for renders on the line before it, which is what
// decides whether it belongs to the head rather than to the indented run.
func stepTakesBreak(links []chainLink, i int) bool {
	return links[i].arrow && (i == 0 || !isPropertyArrow(links[i-1]))
}

func isPropertyArrow(l chainLink) bool {
	return l.arrow && !l.call
}

// chainEndsInBlockHug reports whether the outermost call's last argument hugs
// its opener (see argsEndInBlockHug). An arrow function does not: its
// expression body forces no break, so the ordinary group handles it.
func chainEndsInBlockHug(e parser.Expr) bool {
	call, ok := e.(*parser.CallExpr)
	if !ok {
		return false
	}

	return argsEndInBlockHug(call.Args)
}

// argsEndInBlockHug reports whether args end in a braced or bracketed body, so
// the call's opener carries it and keeps what precedes it on the same line.
func arrowReturnsPlainCall(args []parser.Expr) bool {
	if len(args) == 0 {
		return false
	}

	arrow, ok := args[len(args)-1].(*parser.ArrowFnExpr)
	if !ok {
		return false
	}

	call, ok := arrow.Body.(*parser.CallExpr)

	return ok && len(call.Args) > 0 && !argsEndInBlockHug(call.Args) && !hugsBracketedBlock(call)
}

func argsEndInBlockHug(args []parser.Expr) bool {
	if len(args) == 0 {
		return false
	}

	last := args[len(args)-1]
	if n, ok := last.(*parser.NamedArgExpr); ok {
		last = n.Value
	}

	switch v := last.(type) {
	case *parser.ClosureExpr, *parser.MatchExpr:
		return true
	case *parser.ArrayExpr:
		// `$model->relation()->create([...])` is the most common statement shape in a
		// Laravel app. Without this the array's break propagates to the chain group
		// and every link goes to its own line, so a 40-column head becomes a ladder.
		return arrayHasValue(v.Elements)
	}

	return false
}

// chainHeadEnd is the index of the last link that stays on the head line.
// Leading properties, subscripts and a factory call always attach. A variable
// or `$this` head keeps its first method call, and in a fluent chain so does
// a helper without arguments, which stands for a service (`response()->json(`),
// or Pest's `expect($x)`. A helper building from its arguments (`collect($rows)`)
// is a factory head, so every method breaks.
func (b *builder) chainHeadEnd(head parser.Expr, links []chainLink, fluent bool) int {
	firstCall := len(links)
	headHasCall := isNewExpr(head)

	for i, l := range links {
		if l.arrow && l.call {
			firstCall = i

			break
		}

		helper := fluent && i == 0 && isHelperCall(head, l) && (l.noArgs || isExpectCall(head, l))

		if l.call && !helper {
			headHasCall = true
		}
	}

	if headHasCall {
		return firstCall - 1
	}

	return firstCall
}

// parenBlockDoc breaks a parenthesised binary as a block, `(`, its operands
// one level in, then `)`, so no operator dangles before `)->` or `);`.
func (b *builder) parenBlockDoc(head parser.Expr) *document.Doc {
	p, ok := head.(*parser.ParenExpr)
	if !ok || b.spanHasComment(p.Span()) {
		return b.expr(head)
	}

	bin, ok := p.Expr.(*parser.BinaryExpr)
	if !ok {
		return b.expr(head)
	}

	// Inside the parentheses nothing is the statement's own value, so a chain
	// there compacts like any nested one.
	b.chainStmtLevel = false

	b.depth++
	inner := b.binaryIndented(bin, false, false)
	b.depth--

	return document.Group(document.Concat(
		document.Text("("),
		document.Indent(document.Concat(document.SoftLine(), inner)),
		document.SoftLine(),
		document.Text(")"),
	))
}

func isHelperCall(head parser.Expr, first chainLink) bool {
	_, named := head.(*parser.IdentExpr)

	return named && first.call && !first.access && !first.arrow
}

func isNewExpr(e parser.Expr) bool {
	_, ok := e.(*parser.NewExpr)

	return ok
}

// qualifiesTrailingBreak reports whether a single `head->m(args)` chain (head
// itself a call) should break the call onto its own line when it overflows,
// rather than break an argument. A huggable last argument keeps its hug while
// the line it opens fits; past that, the head's short arguments would break
// instead, so the call moves down.
func (b *builder) qualifiesTrailingBreak(e parser.Expr, stmtLevel bool) bool {
	whole := e
	e = trailingCall(e)

	// A call without arguments cannot break, so moving it down is the only way
	// to fit; only a statement knows the head's column well enough to judge it.
	call, ok := e.(*parser.CallExpr)
	if !ok || len(call.Args) == 0 && !stmtLevel {
		return false
	}

	head, headArgs, ok := chainHeadWithArgs(e)
	if !ok || b.keepsCallOnHead(call, stmtLevel) {
		return false
	}

	huggable := len(call.Args) > 0 && isHuggableArg(call.Args[len(call.Args)-1])
	if huggable && !b.hugLineOverflows(call, stmtLevel) && !b.arrayLinkFitsAlone(call, stmtLevel) {
		return false
	}

	if stmtLevel && b.closureUseListAbsorbs(head, call) {
		return false
	}

	// A head that renders multi-line anyway (an author-broken array or
	// argument list) ends on its own closer, and the call reads best glued to
	// it. Outside a statement only a hug's head is judged so.
	if huggable || stmtLevel {
		forced := false
		b.measureWidth(func() int {
			forced = document.HasForcedBreak(b.expr(head))

			return 0
		})

		if forced {
			return false
		}
	}

	// A head hugging a braced argument renders multi-line and ends on its own
	// closer, so the method reads better against that closer than over-indented
	// under a broken chain. An arrow function is excluded for the reason
	// chainEndsInBlockHug excludes it: an expression body forces no break, so the
	// head may be one long line, and headHugsBlockArg leaves the shape to the
	// printer instead of fixing it here.
	if n := len(headArgs); n > 0 && isBlockLikeArg(headArgs[n-1]) && !isArrowFnArg(headArgs[n-1]) {
		return false
	}

	// Moving the call down helps only if its opening line then fits on its own
	// line: a call that overflows there too (a long string argument, an arrow
	// body) stays on the head and breaks its arguments instead. A block it hugs
	// counts up to its opener.
	if mem, isMember := call.Callee.(*parser.MemberExpr); isMember {
		// A subscript after the call (`->m($x)[$k]`) rides its line too.
		tail := b.subscriptsWidth(whole, e)
		if stmtLevel {
			tail += b.chainStmtTail
		}

		if (b.depth+1)*b.cfg.IndentWidth+b.linkOpenerWidth(mem, call)+tail > b.cfg.PrintWidth {
			return false
		}

		if len(call.Args) > 0 && isArrayArg(call.Args[len(call.Args)-1]) && !b.movedLinkHugsOrFits(mem, call, tail) {
			return false
		}
	}

	// Otherwise break the chain only when the head is narrow enough to fit flat.
	// FlatWidth ignores author-expanded array breaks and the head is built with
	// its concatenation unbroken, so the decision does not flip between passes
	// when a head array or `.` run wraps on width. A statement's head is judged
	// on its real line: a head that breaks there anyway reads back as
	// author-broken, and the next pass glues the call to its closer.
	width := b.measureWidth(func() int {
		return document.FlatWidth(b.headWithoutConcatBreaks(head), b.cfg.IndentWidth)
	})

	limit := b.cfg.PrintWidth - 2*b.cfg.IndentWidth
	if stmtLevel {
		limit = b.cfg.PrintWidth - b.depth*b.cfg.IndentWidth - b.chainStmtLead
	}

	return width <= limit
}

// headWithoutConcatBreaks builds a chain's head call with every `.` run
// unbroken: a run the printer wraps on width reads back as author-broken, so
// a decision judged on it would flip on the next pass. Call it under
// measureWidth.
func (b *builder) headWithoutConcatBreaks(head parser.Expr) *document.Doc {
	saved := b.noConcatBreaks
	b.noConcatBreaks = true

	defer func() { b.noConcatBreaks = saved }()

	return b.expr(head)
}

// arrowHeadBreaksOnlyWhereWrapped reports whether a head call hugging an arrow
// function breaks, if at all, only at a `.` run or an array. The printer wraps
// those on width and the next pass reads them as authored, so they say nothing
// about where the next call belongs.
func (b *builder) arrowHeadBreaksOnlyWhereWrapped(e parser.Expr) bool {
	headCall := chainHeadCall(e)
	if headCall == nil || len(headCall.Args) == 0 || !isArrowFnArg(headCall.Args[len(headCall.Args)-1]) {
		return false
	}

	forced := false
	b.measureWidth(func() int {
		saved := b.noAuthorBreaks
		b.noAuthorBreaks = true

		defer func() { b.noAuthorBreaks = saved }()

		forced = document.HasForcedBreak(b.headWithoutConcatBreaks(headCall))

		return 0
	})

	return !forced
}

func (b *builder) subscriptsWidth(whole, inner parser.Expr) int {
	return b.measureWidth(func() int {
		width := 0

		for cur := whole; cur != inner; {
			s, ok := cur.(*parser.SubscriptExpr)
			if !ok {
				break
			}

			width += len("[]")
			if s.Index != nil {
				width += document.FlatWidth(b.expr(s.Index), b.cfg.IndentWidth)
			}

			cur = s.Array
		}

		return width
	})
}

// movedLinkHugsOrFits reports whether a call moved a level down either hugs its
// last array, as linkOpenerWidth assumed, or fits flat there. One whose
// arguments go one per line breaks no less for the move, and the next pass,
// reading those breaks, keeps the call on its head.
func (b *builder) movedLinkHugsOrFits(mem *parser.MemberExpr, call *parser.CallExpr, tail int) bool {
	savedNoHug, savedLinkArgs := b.noHugNext, b.linkArgsNext
	b.noHugNext, b.linkArgsNext = false, true

	defer func() { b.noHugNext, b.linkArgsNext = savedNoHug, savedLinkArgs }()

	hugged := false
	width := b.measureWidth(func() int {
		b.depth++
		defer func() { b.depth-- }()

		args, hug := b.argListHug(call.Args, withTrailingComma)
		hugged = hug

		return document.FlatWidth(document.Concat(b.tok(mem.Arrow), b.expr(mem.Property), args), b.cfg.IndentWidth)
	})

	return hugged || (b.depth+1)*b.cfg.IndentWidth+width+tail <= b.cfg.PrintWidth
}

// onlyLastCallBreaks keeps the head and every link before the last one flat, so
// the printer cannot break them against the width of what follows.
func (b *builder) onlyLastCallBreaks(headDoc *document.Doc, links []chainLink) *document.Doc {
	parts := make([]*document.Doc, 0, len(links)+1)
	parts = append(parts, b.flatDoc(headDoc))

	for _, l := range links[:len(links)-1] {
		parts = append(parts, b.flatDoc(l.doc))
	}

	return document.Concat(append(parts, links[len(links)-1].doc)...)
}

func (b *builder) lastCallBreaksAnyway(e parser.Expr, headDoc *document.Doc, links []chainLink) bool {
	call, ok := e.(*parser.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}

	mem, ok := call.Callee.(*parser.MemberExpr)
	if !ok {
		return false
	}

	if (b.depth+1)*b.cfg.IndentWidth+b.linkOpenerWidth(mem, call)+b.chainStmtTail <= b.cfg.PrintWidth {
		return false
	}

	head := make([]*document.Doc, 0, len(links))
	head = append(head, headDoc)

	for _, l := range links[:len(links)-1] {
		head = append(head, l.doc)
	}

	if slices.ContainsFunc(head, document.HasForcedBreak) {
		return false
	}

	glued := b.measureWidth(func() int {
		width := 0
		for _, d := range head {
			width += document.FlatWidth(d, b.cfg.IndentWidth)
		}

		opener := document.Concat(b.tok(mem.Arrow), b.expr(mem.Property))

		return width + document.FlatWidth(opener, b.cfg.IndentWidth) + len("(")
	})

	return b.depth*b.cfg.IndentWidth+b.chainStmtLead+glued <= b.cfg.PrintWidth
}

func (b *builder) linkOpenerWidth(mem *parser.MemberExpr, call *parser.CallExpr) int {
	if len(call.Args) == 0 {
		return b.measureWidth(func() int {
			return document.FlatWidth(document.Concat(b.tok(mem.Arrow), b.expr(mem.Property)), b.cfg.IndentWidth) + len("()")
		})
	}

	last := call.Args[len(call.Args)-1]
	if n, ok := last.(*parser.NamedArgExpr); ok {
		last = n.Value
	}

	if arr, isArray := last.(*parser.ArrayExpr); isArray && len(arr.Elements) > 0 {
		return b.measureWidth(func() int {
			width := document.FlatWidth(document.Concat(b.tok(mem.Arrow), b.expr(mem.Property)), b.cfg.IndentWidth) + len("(")

			for _, a := range call.Args[:len(call.Args)-1] {
				width += document.FlatWidth(b.expr(a), b.cfg.IndentWidth) + len(", ")
			}

			return width + b.hugOpenerWidth(call.Args[len(call.Args)-1])
		})
	}

	// argList consumes a pending noHugNext meant for the real build.
	savedNoHug := b.noHugNext
	b.noHugNext = false

	defer func() { b.noHugNext = savedNoHug }()

	return b.measureWidth(func() int {
		// A concatenation the printer wrapped reads back as an author break and
		// would end the measure there on the next pass.
		savedConcat := b.noConcatBreaks
		b.noConcatBreaks = true

		defer func() { b.noConcatBreaks = savedConcat }()

		link := document.Concat(b.tok(mem.Arrow), b.expr(mem.Property), b.argList(call.Args, withTrailingComma))
		width, _ := document.FlatPrefixWidth(link, b.cfg.IndentWidth)

		return width
	})
}

func (b *builder) keepsCallOnHead(call *parser.CallExpr, stmtLevel bool) bool {
	if !b.argsPrintBroken(call) {
		return false
	}

	mem, isMember := call.Callee.(*parser.MemberExpr)

	return stmtLevel || !isMember || !b.newlineBefore(mem.Arrow.Span.Start.Offset)
}

func trailingMemberCall(e parser.Expr) *parser.CallExpr {
	call, ok := trailingCall(e).(*parser.CallExpr)
	if !ok || len(call.Args) == 0 {
		return nil
	}

	if _, isMember := call.Callee.(*parser.MemberExpr); !isMember {
		return nil
	}

	return call
}

func brokenBefore(l chainLink) bool {
	return l.brokeBefore
}

func (b *builder) argsPrintBroken(call *parser.CallExpr) bool {
	// argListHug consumes a pending noHugNext meant for the real build.
	savedNoHug := b.noHugNext
	b.noHugNext = false

	defer func() { b.noHugNext = savedNoHug }()

	forced := false

	b.measureWidth(func() int {
		list, lastHugged := b.argListHug(call.Args, withTrailingComma)

		// A measured build leaves comments out, so a line comment, which forces
		// the list onto several lines, is read from the source.
		inside := parser.Span{Start: call.LParen.Span.End, End: call.RParen.Span.Start}
		forced = !lastHugged && (document.HasForcedBreak(list) || b.spanHasLineComment(inside))

		return 0
	})

	return forced
}

func (b *builder) loneArrayLinkFitsAlone(e parser.Expr) bool {
	call, ok := trailingCall(e).(*parser.CallExpr)

	return ok && len(call.Args) > 0 && !rootsAtFactory(call) && b.arrayLinkFitsAlone(call, true)
}

func (b *builder) arrowBodyLinkFitsAlone(e parser.Expr) bool {
	saved := b.chainStmtTail
	b.chainStmtTail = len("),")

	defer func() { b.chainStmtTail = saved }()

	return b.loneArrayLinkFitsAlone(e)
}

// rootsAtFactory reports whether a chain starts at `Model::factory()`, whose
// `->create([` the Laravel docs open on the chain's line however long the head.
func rootsAtFactory(call *parser.CallExpr) bool {
	for {
		mem, ok := call.Callee.(*parser.MemberExpr)
		if !ok {
			break
		}

		inner, ok := mem.Object.(*parser.CallExpr)
		if !ok {
			return false
		}

		call = inner
	}

	static, ok := call.Callee.(*parser.StaticMemberExpr)
	if !ok {
		return false
	}

	name, ok := static.Name.(*parser.IdentExpr)

	return ok && string(name.Token.Text) == "factory"
}

// An array the author broke keeps its hug, and so does a head without
// arguments: `User::factory()` reads as one unit with its call.
func (b *builder) arrayLinkFitsAlone(call *parser.CallExpr, stmtLevel bool) bool {
	mem, isMember := call.Callee.(*parser.MemberExpr)
	if !stmtLevel || !isMember {
		return false
	}

	if head := chainHeadCall(call); head == nil || len(head.Args) == 0 {
		return false
	}

	last := call.Args[len(call.Args)-1]
	if n, ok := last.(*parser.NamedArgExpr); ok {
		last = n.Value
	}

	if _, ok := last.(*parser.ArrayExpr); !ok && !callEndsInArray(last) {
		return false
	}

	// argList consumes a pending noHugNext meant for the real build.
	savedNoHug := b.noHugNext
	b.noHugNext = false

	defer func() { b.noHugNext = savedNoHug }()

	link := b.measureWidth(func() int {
		doc := document.Concat(b.tok(mem.Arrow), b.expr(mem.Property), b.argList(call.Args, withTrailingComma))
		if document.HasForcedBreak(doc) {
			return b.cfg.PrintWidth
		}

		return document.FlatWidth(doc, b.cfg.IndentWidth)
	})

	return (b.depth+1)*b.cfg.IndentWidth+link+b.chainStmtTail <= b.cfg.PrintWidth
}

// unhuggingHelps reports whether one argument per line beats the hug. It does
// whenever leading arguments ride the hug line; a sole argument gains only if
// it then fits on its own line, or it breaks there too, one level deeper.
func (b *builder) unhuggingHelps(call *parser.CallExpr) bool {
	last := call.Args[len(call.Args)-1]
	if shortListClosure(last) != nil {
		width := b.measureWidth(func() int { return b.hugOpenerWidth(last) })

		return (b.depth+1)*b.cfg.IndentWidth+width <= b.cfg.PrintWidth
	}

	if len(call.Args) > 1 {
		return true
	}

	width := b.measureWidth(func() int { return document.FlatWidth(b.expr(call.Args[0]), b.cfg.IndentWidth) })

	if arrow, ok := call.Args[0].(*parser.ArrowFnExpr); ok {
		if head := b.arrowBinaryHeadWidth(arrow); head > 0 {
			return (b.depth+1)*b.cfg.IndentWidth+head <= b.cfg.PrintWidth
		}
	}

	return (b.depth+1)*b.cfg.IndentWidth+width+len(",") <= b.cfg.PrintWidth
}

func hugsInsideOut(call *parser.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}

	last := call.Args[len(call.Args)-1]

	return isCallLikeArg(last) || isArrowFnArg(last) || shortListClosure(last) != nil
}

// shortListClosure returns a closure whose signature lists are short: one
// parameter at most, which never breaks, and two `use` variables at most, a list
// that broken across lines reads worse than the closure on a line of its own.
func shortListClosure(arg parser.Expr) *parser.ClosureExpr {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	c, ok := arg.(*parser.ClosureExpr)
	if !ok || len(c.Params) > 1 || len(c.Uses) > 2 || len(c.Params)+len(c.Uses) == 0 {
		return nil
	}

	return c
}

func (b *builder) staysOnStatementLine(e parser.Expr) bool {
	calls := memberCalls(e)

	return calls == 0 || calls == 1 && !b.qualifiesTrailingBreak(e, true)
}

// A single call on a head that is not itself a call never leaves the head
// line: postfix keeps it there before it measures anything.
func callStaysOnHead(e parser.Expr) bool {
	return memberCalls(e) == 1 && chainHeadCall(trailingCall(e)) == nil
}

func memberCalls(e parser.Expr) int {
	calls := 0

	for cur := e; ; {
		switch n := cur.(type) {
		case *parser.CallExpr:
			if m, ok := n.Callee.(*parser.MemberExpr); ok {
				calls++
				cur = m.Object

				continue
			}

			cur = n.Callee

			continue
		case *parser.MemberExpr:
			cur = n.Object

			continue
		case *parser.StaticMemberExpr:
			cur = n.Class

			continue
		case *parser.SubscriptExpr:
			cur = n.Array

			continue
		}

		break
	}

	return calls
}

func trailingCall(e parser.Expr) parser.Expr {
	for {
		s, ok := e.(*parser.SubscriptExpr)
		if !ok {
			return e
		}

		e = s.Array
	}
}

// compactBlockChainFits reports whether a statement's chain can stay on the
// line that opens a bracketed block it hugs: the block opened by the last call,
// or by the call before it, whose closer line then carries that last call. A
// closure keeps its own ConditionalGroup path, and a longer tail after the
// block is a fluent chain that breaks.
func (b *builder) compactBlockChainFits(e parser.Expr, chain []chainLink) bool {
	outer, ok := e.(*parser.CallExpr)
	if !ok {
		return false
	}

	call := outer
	tail := 0

	if !hugsBracketedBlock(call) {
		var links []*parser.MemberExpr

		for cur := outer; ; {
			mem, isMember := cur.Callee.(*parser.MemberExpr)
			if !isMember {
				return false
			}

			inner, isCall := mem.Object.(*parser.CallExpr)
			if !isCall {
				return false
			}

			links = append(links, mem)

			if hugsBracketedBlock(inner) || hugsClosure(inner) {
				call = inner

				break
			}

			cur = inner
		}

		// One call after the block is the closer's own (`])->refresh()`); a longer
		// tail is a fluent chain, compact only when the block opens it.
		if len(links) > 1 && !isChainHeadCall(call) {
			return false
		}

		// The tail rides the block's closer only if the block breaks: a closure
		// body always does, an array when author broken or too wide to close on
		// its opening line. A block that stays flat would leave the tail to
		// overflow that line instead. The block call's link was built with its
		// comments, which a measured rebuild drops, so only that link shows a line
		// comment forcing the break.
		breaks := hugsClosure(call) || document.HasForcedBreak(chain[len(chain)-1-len(links)].doc)
		tail = b.measureWidth(func() int {
			if !breaks {
				d := b.expr(call)
				breaks = document.HasForcedBreak(d) ||
					b.depth*b.cfg.IndentWidth+b.chainStmtLead+document.FlatWidth(d, b.cfg.IndentWidth) > b.cfg.PrintWidth
			}

			w := 0

			for cur, i := outer, 0; i < len(links); i++ {
				w += document.FlatWidth(document.Concat(b.tok(links[i].Arrow), b.expr(links[i].Property), b.argList(cur.Args, withTrailingComma)), b.cfg.IndentWidth)
				cur, _ = links[i].Object.(*parser.CallExpr)
			}

			return w
		})

		if !breaks {
			return false
		}
	}

	width, hugs := b.hugLineWidth(call)
	if !hugs || width > b.cfg.PrintWidth {
		return false
	}

	closer := b.depth*b.cfg.IndentWidth + hugCloserWidth(call.Args[len(call.Args)-1]) + len(")") + tail + b.chainStmtTail

	return closer <= b.cfg.PrintWidth
}

func (b *builder) arrayHugOpener(e parser.Expr, headDoc *document.Doc, links []chainLink) (int, bool) {
	call := trailingMemberCall(e)
	if call == nil || trailingCall(e) != e {
		return 0, false
	}

	last := call.Args[len(call.Args)-1]
	if n, ok := last.(*parser.NamedArgExpr); ok {
		last = n.Value
	}

	if arr, ok := last.(*parser.ArrayExpr); !ok || !arrayHasValue(arr.Elements) {
		return 0, false
	}

	// argListHug consumes a pending noHugNext meant for the real build.
	savedNoHug := b.noHugNext
	b.noHugNext = false

	defer func() { b.noHugNext = savedNoHug }()

	hugged := false

	b.measureWidth(func() int {
		_, hugged = b.argListHug(call.Args, withTrailingComma)

		return 0
	})

	if !hugged {
		return 0, false
	}

	width := document.FlatWidth(headDoc, b.cfg.IndentWidth)
	for _, l := range links[:len(links)-1] {
		width += document.FlatWidth(l.doc, b.cfg.IndentWidth)
	}

	mem, _ := call.Callee.(*parser.MemberExpr)
	width += b.linkOpenerWidth(mem, call)

	if width >= document.ForcedBreakWidth {
		return 0, false
	}

	return width, true
}

func hugsClosure(call *parser.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}

	last := call.Args[len(call.Args)-1]
	if n, ok := last.(*parser.NamedArgExpr); ok {
		last = n.Value
	}

	_, ok := last.(*parser.ClosureExpr)

	return ok
}

func isChainHeadCall(call *parser.CallExpr) bool {
	switch c := call.Callee.(type) {
	case *parser.IdentExpr:
		return true
	case *parser.StaticMemberExpr:
		_, onCall := c.Class.(*parser.CallExpr)

		return !onCall
	case *parser.MemberExpr:
		for obj := c.Object; ; {
			switch o := obj.(type) {
			case *parser.CallExpr:
				return false
			case *parser.MemberExpr:
				obj = o.Object
			case *parser.SubscriptExpr:
				obj = o.Array
			default:
				return true
			}
		}
	}

	return false
}

func hugCloserWidth(arg parser.Expr) int {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	switch v := arg.(type) {
	case *parser.CallExpr:
		if len(v.Args) > 0 {
			return hugCloserWidth(v.Args[len(v.Args)-1]) + len(")")
		}
	case *parser.NewExpr:
		if len(v.Args) > 0 {
			return hugCloserWidth(v.Args[len(v.Args)-1]) + len(")")
		}
	}

	return len("]")
}

func hugsBracketedBlock(call *parser.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}

	last := call.Args[len(call.Args)-1]
	if n, ok := last.(*parser.NamedArgExpr); ok {
		last = n.Value
	}

	switch v := last.(type) {
	case *parser.ClosureExpr:
		return false
	case *parser.ArrowFnExpr:
		arr, ok := v.Body.(*parser.ArrayExpr)

		return ok && arrayHasValue(arr.Elements)
	case *parser.CallExpr:
		return hugsBracketedBlock(v)
	}

	return opensBlock(last)
}

// Only a statement's own chain knows the line's lead; anywhere else the
// column is unknown and the hug stays.
func (b *builder) hugLineOverflows(call *parser.CallExpr, stmtLevel bool) bool {
	if !stmtLevel {
		return false
	}

	width, hugs := b.hugLineWidth(call)

	return hugs && width > b.cfg.PrintWidth
}

// arrowFitsAlone reports whether a last arrow argument that the hugged line
// cannot hold whole fits flat on a line of its own. lead is the column the
// call's `(` opens at, indent the column its arguments take once broken, and
// tail what follows the `)`. hugLineWidth counts only a long body call's
// opener, so without this the hug splits a body the un-hugged arrow would keep
// on one line.
func (b *builder) arrowFitsAlone(args []parser.Expr, lead, indent, tail int) bool {
	last := len(args) - 1
	if last < 0 {
		return false
	}

	arrow, ok := args[last].(*parser.ArrowFnExpr)
	if !ok {
		return false
	}

	// A body that is a block hugs it. A call opening one (`->create([`) hugs
	// only once the block breaks anyway, which the forced-break check below
	// sees; flat, the arrow fits alone like any other.
	switch arrow.Body.(type) {
	case *parser.ArrayExpr, *parser.MatchExpr:
		return false
	}

	fits := false

	b.measureWidth(func() int {
		line := lead + len("(")

		for _, a := range args[:last] {
			d := b.expr(a)
			if document.CanBreak(d) {
				return 0
			}

			line += document.FlatWidth(d, b.cfg.IndentWidth) + len(", ")
		}

		// An arrow holding a break of its own (an author-broken list, a block
		// it opens) prints broken wherever it goes; flat widths say nothing.
		doc := b.expr(args[last])
		if document.HasForcedBreak(doc) {
			return 0
		}

		width := document.FlatWidth(doc, b.cfg.IndentWidth)
		fits = line+width+len(")")+tail > b.cfg.PrintWidth && indent+width+len(",") <= b.cfg.PrintWidth

		return 0
	})

	return fits
}

func (b *builder) stmtArrowFitsAlone(call *parser.CallExpr) bool {
	callee := b.measureWidth(func() int { return document.FlatWidth(b.expr(call.Callee), b.cfg.IndentWidth) })
	if callee >= document.ForcedBreakWidth {
		return false
	}

	lead := b.depth*b.cfg.IndentWidth + b.chainStmtLead + callee

	return b.arrowFitsAlone(call.Args, lead, (b.depth+1)*b.cfg.IndentWidth, b.chainStmtTail)
}

// hugLineWidth is the width of the line call's hugged last argument opens, from
// the statement's lead through the argument's opener, and whether the list
// hugs at all: a huggable last argument behind leading ones that cannot break,
// on a callee that stays on one line.
func (b *builder) hugLineWidth(call *parser.CallExpr) (int, bool) {
	if len(call.Args) == 0 || !isHuggableArg(call.Args[len(call.Args)-1]) && !isArrowFnArg(call.Args[len(call.Args)-1]) {
		return 0, false
	}

	last := len(call.Args) - 1
	hugs := true

	// argList hugs only past leading arguments that cannot break, so a list
	// with a breakable one never opens that line.
	width := b.measureWidth(func() int {
		// A callee that renders multi-line (`it(…, function () {…})->with`) puts
		// the hug after its own closer, at a column no flat width tells.
		w := document.FlatWidth(b.expr(call.Callee), b.cfg.IndentWidth)
		if w >= document.ForcedBreakWidth {
			hugs = false
		}

		w += len("(")

		for _, a := range call.Args[:last] {
			d := b.expr(a)
			hugs = hugs && !document.CanBreak(d)
			w += document.FlatWidth(d, b.cfg.IndentWidth) + len(", ")
		}

		// A binary arrow body opens with its first operand, as argList hugs it.
		if arrow, ok := call.Args[last].(*parser.ArrowFnExpr); ok {
			if head := b.arrowBinaryHeadWidth(arrow); head > 0 {
				return w + head
			}
		}

		return w + b.hugOpenerWidth(call.Args[last])
	})

	return b.depth*b.cfg.IndentWidth + b.chainStmtLead + width, hugs
}

func (b *builder) hugOpenerWidth(arg parser.Expr) int {
	tab := b.cfg.IndentWidth

	switch v := arg.(type) {
	case *parser.NamedArgExpr:
		return len(v.Name.Text) + len(": ") + b.hugOpenerWidth(v.Value)
	case *parser.ArrayExpr:
		return len(v.KeywordPrefix.Text) + len(v.Open.Text)
	case *parser.CastExpr:
		if castsArray(v) {
			return document.FlatWidth(b.expr(v), tab) - document.FlatWidth(b.expr(v.Expr), tab) + b.hugOpenerWidth(v.Expr)
		}
	case *parser.ArrowFnExpr:
		prefix := document.FlatWidth(b.expr(v), tab) - document.FlatWidth(b.expr(v.Body), tab)

		// argList keeps an author-broken list broken and never hugs into it. Only
		// the body's own list counts: a nested one the printer broke on width
		// would read back as authored on the next pass.
		if call, ok := v.Body.(*parser.CallExpr); ok && len(call.Args) > 0 && b.authorBroke(call.Args[0].Span().Start.Offset) {
			return prefix + document.FlatWidth(b.expr(call.Callee), tab) + len("(")
		}

		return prefix + b.hugOpenerWidth(v.Body)
	case *parser.CallExpr:
		if len(v.Args) > 0 {
			return b.callOpenerWidth(document.FlatWidth(b.expr(v.Callee), tab), v.Args)
		}
	case *parser.NewExpr:
		if len(v.Args) > 0 {
			return b.callOpenerWidth(len("new ")+document.FlatWidth(b.expr(v.Class), tab), v.Args)
		}
	}

	width, _ := document.FlatPrefixWidth(b.expr(arg), tab)

	return width
}

// callOpenerWidth follows argList's choice for a call's arguments: the line
// runs on into a hugged last or first argument's opener, and otherwise the
// list can break right after `(`. A call within half the print width counts
// whole: breaking one that short is the shatter the hug is meant to avoid,
// the same half-width budget argList gives a call-like hug.
func (b *builder) callOpenerWidth(callee int, args []parser.Expr) int {
	open := callee + len("(")
	last := len(args) - 1

	flat := open + len(")")
	forced := false

	for _, a := range args {
		d := b.expr(a)
		forced = forced || document.HasForcedBreak(d)
		flat += document.FlatWidth(d, b.cfg.IndentWidth) + len(", ")
	}

	// A call that breaks anyway (an author-broken array) is not short.
	if !forced && flat-len(", ") <= b.cfg.PrintWidth/2 {
		return flat - len(", ")
	}

	if isHuggableArg(args[last]) && (len(args) < 2 || !allNamedArgs(args)) {
		width := open
		simple := true

		for _, a := range args[:last] {
			d := b.expr(a)
			simple = simple && !document.CanBreak(d)
			width += document.FlatWidth(d, b.cfg.IndentWidth) + len(", ")
		}

		hugged := width + b.hugOpenerWidth(args[last])
		if simple && (!readsInsideOut(args[last]) || hugged-open <= b.cfg.PrintWidth/2) {
			return hugged
		}
	}

	if isHuggableArg(args[0]) && !isArrowFnArg(args[0]) && !isCallLikeArg(args[0]) && b.trailingArgsHug(args[0], args[1:]) &&
		!b.leadingArgStaysUnhugged(args) {
		return open + b.hugOpenerWidth(args[0])
	}

	return open
}

func chainHeadCall(e parser.Expr) *parser.CallExpr {
	call, ok := e.(*parser.CallExpr)
	if !ok {
		return nil
	}

	mem, ok := call.Callee.(*parser.MemberExpr)
	if !ok {
		return nil
	}

	head, ok := mem.Object.(*parser.CallExpr)
	if !ok {
		return nil
	}

	return head
}

func (b *builder) closureUseListAbsorbs(head parser.Expr, call *parser.CallExpr) bool {
	if !hugsClosure(call) || len(call.Args) != 1 {
		return false
	}

	c, ok := call.Args[0].(*parser.ClosureExpr)
	if !ok || len(c.Uses) == 0 || shortListClosure(c) != nil {
		return false
	}

	mem, ok := call.Callee.(*parser.MemberExpr)
	if !ok {
		return false
	}

	width := b.measureWidth(func() int {
		line := document.FlatWidth(document.Concat(b.expr(head), b.tok(mem.Arrow), b.expr(mem.Property)), b.cfg.IndentWidth)

		return line + len("(") + b.closureUseOpenerWidth(c, b.hugOpenerWidth(c))
	})

	return b.depth*b.cfg.IndentWidth+b.chainStmtLead+width <= b.cfg.PrintWidth
}

func chainHeadWithArgs(e parser.Expr) (parser.Expr, []parser.Expr, bool) {
	if head := chainHeadCall(e); head != nil {
		return head, head.Args, true
	}

	call, ok := e.(*parser.CallExpr)
	if !ok {
		return nil, nil, false
	}

	mem, ok := call.Callee.(*parser.MemberExpr)
	if !ok {
		return nil, nil, false
	}

	head, ok := mem.Object.(*parser.NewExpr)
	if !ok || !has(head.LParen) {
		return nil, nil, false
	}

	return head, head.Args, true
}

// headHugsBlockArg reports whether the head call's last argument is a block,
// so its closer can carry the method.
func headHugsBlockArg(e parser.Expr) bool {
	head := chainHeadCall(e)
	if head == nil || len(head.Args) == 0 {
		return false
	}

	return isBlockLikeArg(head.Args[len(head.Args)-1])
}

func isBlockLikeArg(arg parser.Expr) bool {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	switch arg.(type) {
	case *parser.ClosureExpr, *parser.ArrowFnExpr, *parser.MatchExpr:
		return true
	}

	return false
}

// chainHasStepComment reports whether a chain carries a comment on its own
// structural tokens (`->`/`::`/`[`, arg separators, closing paren). It never
// descends into an argument's subtree, so a comment inside a closure or array
// argument does not trigger a chain-wide verbatim fallback.
func (b *builder) chainHasStepComment(e parser.Expr) bool {
	for {
		switch n := e.(type) {
		case *parser.CallExpr:
			if b.callHasArgComment(n) {
				return true
			}

			switch c := n.Callee.(type) {
			case *parser.MemberExpr:
				if b.tokenHasComment(c.Arrow.Span.Start.Offset) {
					return true
				}

				e = c.Object
			case *parser.StaticMemberExpr:
				if b.tokenHasComment(c.ScopeRes.Span.Start.Offset) {
					return true
				}

				e = c.Class
			default:
				e = n.Callee
			}
		case *parser.MemberExpr:
			if b.tokenHasComment(n.Arrow.Span.Start.Offset) {
				return true
			}

			e = n.Object
		case *parser.StaticMemberExpr:
			if b.tokenHasComment(n.ScopeRes.Span.Start.Offset) {
				return true
			}

			e = n.Class
		case *parser.SubscriptExpr:
			if b.tokenHasComment(n.LBracket.Span.Start.Offset) {
				return true
			}

			e = n.Array
		default:
			return false
		}
	}
}

// A comment before the `)` of a non-empty list is placed by the list itself.
func (b *builder) callHasArgComment(n *parser.CallExpr) bool {
	for _, a := range n.Args {
		if a != nil && b.tokenHasComment(a.Span().Start.Offset) {
			return true
		}
	}

	return len(n.Args) == 0 && b.tokenHasComment(n.RParen.Span.Start.Offset)
}

// In a statement, the sole chain a Pest `->and(` checks keeps its author ladder,
// so the segment breaks anyway and its matcher rides the closer, as Pest's
// docs lay it out.
func (b *builder) linkArgList(method string, args []parser.Expr) (*document.Doc, bool) {
	if !b.stmtChain || method != "and" || len(args) != 1 {
		return b.argListHug(args, withTrailingComma)
	}

	switch args[0].(type) {
	case *parser.CallExpr, *parser.MemberExpr:
	default:
		return b.argListHug(args, withTrailingComma)
	}

	savedAt, savedLead, savedTail := b.preserveAt, b.chainStmtLead, b.chainStmtTail
	b.preserveAt, b.chainStmtLead, b.chainStmtTail = args[0].Span().Start.Offset, len("->and("), len(")")

	defer func() { b.preserveAt, b.chainStmtLead, b.chainStmtTail = savedAt, savedLead, savedTail }()

	return b.argListHug(args, withTrailingComma)
}

// flattenChain peels postfix operations off e, returning the head expression and
// the links in head-to-outermost order.
func (b *builder) flattenChain(e parser.Expr) (parser.Expr, []chainLink) {
	var links []chainLink

	// A link's arguments sit a level in from its head only on a ladder.
	ladders := !callStaysOnHead(e)

	for {
		switch n := e.(type) {
		case *parser.CallExpr:
			switch c := n.Callee.(type) {
			case *parser.MemberExpr:
				method := ""
				if id, ok := c.Property.(*parser.IdentExpr); ok {
					method = string(id.Token.Text)
				}

				arrow := b.tok(c.Arrow)
				property := b.expr(c.Property)
				// A laddered link sits one level in at the least, so a hug that
				// overflows there overflows wherever the link prints.
				if ladders && b.arrowFitsAlone(n.Args, (b.depth+1)*b.cfg.IndentWidth+len("->")+len(method), (b.depth+2)*b.cfg.IndentWidth, 0) {
					b.noHugNext = true
				}

				if ladders {
					b.hugLeadNext = (b.depth+1)*b.cfg.IndentWidth + len("->") + document.FlatWidth(property, b.cfg.IndentWidth)
				}

				b.linkArgsNext = ladders
				args, argsHugged := b.linkArgList(method, n.Args)

				links = append(links, chainLink{
					doc:         document.Concat(arrow, property, args),
					arrow:       true,
					call:        true,
					blockHug:    argsEndInBlockHug(n.Args),
					access:      true,
					brokeBefore: b.newlineBefore(c.Arrow.Span.Start.Offset),
					method:      method,
					args:        args,
					argsHugged:  argsHugged,
					hugCloses:   argsHugged && arrowReturnsPlainCall(n.Args),
				})
				e = c.Object
			case *parser.StaticMemberExpr:
				links = append(links, chainLink{
					doc:         document.Concat(b.tok(c.ScopeRes), b.expr(c.Name), b.argList(n.Args, withTrailingComma)),
					call:        true,
					access:      true,
					brokeBefore: b.newlineBefore(c.ScopeRes.Span.Start.Offset),
				})
				e = c.Class
			default:
				trailing := withTrailingComma
				if rejectsTrailingComma(n.Callee) {
					trailing = withoutTrailingComma
				}

				if b.stmtChain {
					b.hugLeadNext = b.depth*b.cfg.IndentWidth + b.chainStmtLead + b.flatSourceWidth(n.Callee.Span().Start.Offset, n.Callee.Span().End.Offset)
				}

				links = append(links, chainLink{doc: b.argList(n.Args, trailing), call: true, noArgs: len(n.Args) == 0})
				e = n.Callee
			}
		case *parser.MemberExpr:
			links = append(links, chainLink{
				doc:         document.Concat(b.tok(n.Arrow), b.expr(n.Property)),
				arrow:       true,
				access:      true,
				brokeBefore: b.newlineBefore(n.Arrow.Span.Start.Offset),
			})
			e = n.Object
		case *parser.StaticMemberExpr:
			links = append(links, chainLink{
				doc:         document.Concat(b.tok(n.ScopeRes), b.expr(n.Name)),
				access:      true,
				brokeBefore: b.newlineBefore(n.ScopeRes.Span.Start.Offset),
			})
			e = n.Class
		case *parser.SubscriptExpr:
			idx := document.Text("")
			if n.Index != nil {
				saved := b.inKey
				b.inKey = true
				idx = b.expr(n.Index)
				b.inKey = saved
			}

			// A key is read whole; a break inside it never shortens the line usefully.
			if !document.HasForcedBreak(idx) {
				idx = b.flatDoc(idx)
			}

			links = append(links, chainLink{doc: document.Concat(document.Text("["), idx, document.Text("]"))})
			e = n.Array
		default:
			slices.Reverse(links)

			return e, links
		}
	}
}

// newlineBefore reports whether the source breaks the line just before offset,
// skipping only spaces and tabs. Chains carrying a step comment are already
// emitted verbatim, so the only thing between a link and its operator here is
// layout whitespace.
func (b *builder) newlineBefore(offset int) bool {
	i := offset - 1
	for i >= 0 && (b.src[i] == ' ' || b.src[i] == '\t') {
		i--
	}

	return i >= 0 && (b.src[i] == '\n' || b.src[i] == '\r')
}

// chainAuthorBroken reports whether the author deliberately broke the chain
// across lines, so its shape is kept rather than reflowed. One break before a
// single `->` is not enough, since the formatter emits exactly that when a
// chain overflows: it takes two breaks, or one in a chain of more than three
// links. The key reads the current source, so a kept break is a fixed point.
func chainAuthorBroken(links []chainLink) bool {
	accesses, broken := 0, 0

	for _, l := range links {
		if !l.access {
			continue
		}

		accesses++

		if l.brokeBefore {
			broken++
		}
	}

	// A single link is not a table, and its lone break is usually the formatter's
	// own: the head's argument list broke and pushed the one call onto a new line.
	if accesses < 2 {
		return false
	}

	if broken >= 2 {
		return true
	}

	return broken == 1 && accesses > 3
}

// chainLayoutFits reports whether every line of the author's layout fits: the
// head with the links glued to it, after what its statement puts before the
// chain, and each broken link with the links glued to it, one indent in; the
// last line also carries what the statement puts after the chain.
// A line counts up to its first forced break, so a closure body or a kept
// array opening on it is not a reason to reflow. A chain the formatter broke
// on width itself only has lines that fit, so reading it back as the author's
// reproduces the same text.
//
// The measure is the structural depth, not the runtime column: an operand's
// column shifts as an enclosing group breaks, and gating on it would flip
// preservation between passes.
func (b *builder) chainLayoutFits(headDoc *document.Doc, links []chainLink) bool {
	base := b.depth * b.cfg.IndentWidth
	limit := b.cfg.PrintWidth - base - b.chainStmtLead
	line := []*document.Doc{headDoc}

	var opener *chainLink

	for i, l := range links {
		if !l.access || !l.brokeBefore {
			line = append(line, l.doc)

			continue
		}

		if !b.lineFits(line, limit) && !b.breaksInsideLink(opener, line, limit) {
			return false
		}

		limit = b.cfg.PrintWidth - base - b.cfg.IndentWidth
		line = []*document.Doc{l.doc}
		opener = &links[i]
	}

	return b.lineFits(line, limit-b.chainStmtTail) || b.breaksInsideLink(opener, line, limit-b.chainStmtTail)
}

func (b *builder) breaksInsideLink(l *chainLink, line []*document.Doc, limit int) bool {
	if l == nil || len(line) != 1 || l.args == nil || !document.CanBreak(l.args) {
		return false
	}

	opener := document.FlatWidth(l.doc, b.cfg.IndentWidth) - document.FlatWidth(l.args, b.cfg.IndentWidth) + len("([")

	return opener <= limit
}

func (b *builder) lineFits(parts []*document.Doc, limit int) bool {
	line := document.Concat(parts...)

	// An author-broken argument list ends the line like an author-broken array
	// once its break is propagated.
	document.HasForcedBreak(line)

	width, _ := document.OpeningLineWidth(line, b.cfg.IndentWidth)

	return width <= limit
}

// preservedChain reproduces the author's per-link line breaks: a link the author
// broke before starts a new indented line and the rest glue to the current line,
// so the head keeps whatever calls the author left attached. HardLine forces each
// preserved break and propagates to any enclosing group. The output's break
// pattern equals the input's, which is what makes preservation idempotent.
func (b *builder) preservedChain(headDoc *document.Doc, links []chainLink) *document.Doc {
	head := []*document.Doc{headDoc}
	steps := make([]*document.Doc, 0, len(links)*2)

	for _, l := range links {
		switch {
		case l.access && l.brokeBefore:
			steps = append(steps, document.HardLine(), l.doc)
		case len(steps) > 0:
			steps = append(steps, l.doc)
		default:
			head = append(head, l.doc)
		}
	}

	return document.Concat(document.Concat(head...), document.Indent(document.Concat(steps...)))
}

func chainConcat(headDoc *document.Doc, links []chainLink) *document.Doc {
	parts := make([]*document.Doc, 0, len(links)+1)
	parts = append(parts, headDoc)

	for _, l := range links {
		parts = append(parts, l.doc)
	}

	return document.Concat(parts...)
}

// chainStep is one `->`/`?->` step of a fluent chain: the property and the
// optional call on it.
type chainStep struct {
	member *parser.MemberExpr
	call   *parser.CallExpr
}

// collectChainSpine walks the left spine of a member-access chain, returning its
// steps in source order (the caller does not need the head). SubscriptExpr
// terminates the spine (so bracket access stays tight against its chain step).
func collectChainSpine(expr parser.Expr) []chainStep {
	var (
		steps   []chainStep
		pending *parser.CallExpr
		cur     = expr
	)

	for {
		switch x := cur.(type) {
		case *parser.CallExpr:
			// Stop at a call whose callee is not a member access (`f()()`,
			// `Foo::bar()`): descending would drop this call's own arguments.
			if _, ok := x.Callee.(*parser.MemberExpr); !ok {
				slices.Reverse(steps)

				return steps
			}

			pending = x
			cur = x.Callee
		case *parser.MemberExpr:
			steps = append(steps, chainStep{member: x, call: pending})
			pending = nil
			cur = x.Object
		default:
			slices.Reverse(steps)

			return steps
		}
	}
}

// middleCallBreaks reports whether a statement chain's one forced break sits in
// the arguments of a call between its first and its last, with a single call
// after it: the chain then reads as the head through that call, broken, and
// the last call riding its closer, while both of those lines fit.
func (b *builder) middleCallBreaks(headDoc *document.Doc, links []chainLink) bool {
	if document.HasForcedBreak(headDoc) {
		return false
	}

	k := -1

	for i, l := range links {
		if !document.HasForcedBreak(l.doc) {
			continue
		}

		if k >= 0 {
			return false
		}

		k = i
	}

	if k < 0 || k != len(links)-2 || !links[k].call || !links[k+1].call || !slices.ContainsFunc(links[:k], isCallLink) {
		return false
	}

	base := b.depth*b.cfg.IndentWidth + b.chainStmtLead

	opening, _ := document.OpeningLineWidth(chainConcat(headDoc, links[:k+1]), b.cfg.IndentWidth)
	if base+opening > b.cfg.PrintWidth {
		return false
	}

	closer, _ := b.closerLineWidth(headDoc, links[:k+1])
	tail := document.FlatWidth(links[k+1].doc, b.cfg.IndentWidth)

	return b.depth*b.cfg.IndentWidth+closer+tail+b.chainStmtTail <= b.cfg.PrintWidth
}

func isCallLink(l chainLink) bool {
	return l.call
}
