package php

// Member-chain layout: a postfix expression (calls, `->`/`?->`, `::`,
// subscripts) flattens into a head plus links. A fluent chain breaks one link
// per line when it overflows, unless the author already broke it and it fits.

import (
	"bytes"
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
	unhugged    *document.Doc // the link with that list un-hugged, for the head line its arrow fits alone on
}

func (b *builder) postfix(e parser.Expr, stmtLevel bool) *document.Doc {
	arrowBody := b.arrowBodyNext
	b.arrowBodyNext = false
	preserve := b.preserveChainNext
	b.preserveChainNext = false
	structure := b.structureLinksNext
	b.structureLinksNext = false

	// Only the chain's own link group takes the structural indent: what opens
	// inside its links (a closure, an array) indents from its line as ever.
	linkGroup := func(broken *document.Doc) *document.Doc {
		if structure {
			return document.FromStructure(document.Group(broken))
		}

		return document.Group(broken)
	}

	// A comment trailing a chain step (`->where(...) // note`) attaches to the
	// next step's arrow and has no slot in a reflowed chain; keep the author's
	// form so it is not lost. Comments buried in arguments (closures, arrays) are
	// handled by their own builders, so only the chain's own steps are checked.
	if b.chainHasStepComment(e) {
		return b.verbatim(e.Span())
	}

	// A statement's own call is the one place the hug's real line is known; when
	// it overflows into a call or an arrow function, which hug inside-out, or a
	// short-list closure that is its only argument, the call goes one argument
	// per line. Any other closure, array or match keeps its hug, the Laravel shape
	// even past the width (Pest's `it('…', function () use ($x) {`).
	// flattenChain builds the outermost call first.
	if call, ok := e.(*parser.CallExpr); ok && stmtLevel && hugsInsideOut(call.Args) &&
		b.staysOnStatementLine(e) && (b.hugLineOverflowsFromCloser(call) || b.stmtArrowFitsAlone(call)) && b.unhuggingHelps(call) {
		b.noHugNext = true
	}

	savedChain, savedArrowBody := b.stmtChain, b.arrowBodyChain
	b.stmtChain, b.arrowBodyChain = stmtLevel, arrowBody
	head, links := b.flattenChain(e)
	b.stmtChain, b.arrowBodyChain = savedChain, savedArrowBody

	// A ConditionalGroup in a link (a nested chain hugging a closure) hides its
	// forced break from the groups around it, which would then print flat while
	// their indents still move the link's broken lines.
	for i, l := range links {
		if !document.HasForcedBreak(l.doc) && document.HidesForcedBreak(l.doc) {
			links[i].doc = document.Concat(l.doc, document.BreakParent())
		}
	}

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

	general := func() *document.Doc {
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

				return document.ConditionalGroup(document.Opener(opener, glued), linkGroup(broken))
			}
		}

		call := trailingMemberCall(e)

		if methodCalls >= 2 && call != nil && !slices.ContainsFunc(links, brokenBefore) && b.argsPrintBroken(call) {
			glued := chainConcat(headDoc, links)
			if !stmtLevel {
				return glued
			}

			glued = chainConcat(headDoc, headLineLinks(links))

			document.HasForcedBreak(glued)

			if width, _ := document.OpeningLineWidth(glued, b.cfg.IndentWidth); b.depth*b.cfg.IndentWidth+b.chainStmtLead+width <= b.cfg.PrintWidth {
				return glued
			}

			// Laddered, the last call's opener overflows all the same; the first
			// pass, whose arguments were not yet broken, kept it on the head.
			if b.lastCallBreaksAnyway(e, headDoc, links) {
				return b.onlyLastCallBreaks(headDoc, headLineLinks(links))
			}

			return linkGroup(broken)
		}

		if stmtLevel && methodCalls >= 2 && !slices.ContainsFunc(links, brokenBefore) && b.middleCallBreaks(headDoc, links) {
			return chainConcat(headDoc, links)
		}

		if stmtLevel && methodCalls >= 2 && !slices.ContainsFunc(links, brokenBefore) && b.lastCallBreaksAnyway(e, headDoc, links) {
			return b.onlyLastCallBreaks(headDoc, headLineLinks(links))
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

		return linkGroup(broken)
	}

	if (headEnd >= 0 || isNewExpr(head)) && !stmtLevel && !chainEndsInBlockHug(e) {
		if doc, ok := b.closerTail(head, headDoc, links, headEnd, structure, general); ok {
			return doc
		}
	}

	return general()
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

// closerTail lets the printer decide, at the column only it knows, whether
// the links after a head that breaks by force ride its closer. As at
// statement level, only a break before the tail's first link is the author's
// choice to ladder.
//
// After a hugged block the tail ladders below the closer when it cannot ride.
// After any other closer the ladder moves the head's own call down too, so
// the fallback is general, the layout the next pass builds from that ladder.
func (b *builder) closerTail(head parser.Expr, headDoc *document.Doc, links []chainLink, headEnd int, structure bool, general func() *document.Doc) (*document.Doc, bool) {
	last := runEnd(links, headEnd)
	if last >= len(links)-1 || slices.ContainsFunc(links[:last+2], brokenBefore) {
		return nil, false
	}

	for _, l := range links[last+1:] {
		if document.HasForcedBreak(l.doc) {
			return nil, false
		}
	}

	_, broke := b.closerLineWidth(headDoc, links[:last+1])
	if !broke {
		return nil, false
	}

	// A multi-line string spans lines without forcing a break, so its head
	// lays out as any other chain.
	run := chainConcat(headDoc, links[:last+1])
	if !document.HasForcedBreak(run) && !document.HidesForcedBreak(run) {
		return nil, false
	}

	// A sole call rides while the line it opens fits and breaks inside its
	// arguments, as it does once that break reads back as authored.
	only := links[len(links)-1]
	sole := last == len(links)-2 && only.call

	blockClosed := newEndsInBlockHug(head)
	if last >= 0 {
		blockClosed = links[last].blockHug
	}

	if !blockClosed {
		closed := document.Continued(run)
		riding := chainConcat(closed, links[last+1:])

		if sole {
			opener, _ := document.BrokenPrefixWidth(only.doc, b.cfg.IndentWidth)
			riding = document.Concat(closed, document.Opener(opener, only.doc))
		}

		choice := document.ConditionalGroup(riding, general())

		// The head breaks in either variant, which the choice hides from the
		// groups around it.
		return document.Concat(choice, document.BreakParent()), true
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
	if structure {
		group = document.FromStructure(group)
	}

	if sole {
		opener, _ := document.BrokenPrefixWidth(only.doc, b.cfg.IndentWidth)
		riding := document.Opener(opener, only.doc)
		parts = append(parts, document.ConditionalGroup(riding, group))

		return document.Concat(parts...), true
	}

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

// arrowReturnsPlainCall reports whether args end in an arrow returning a call
// whose broken closer line is `))`. A call on a fluent chain is not one: the
// chain ladders, and its last link opens that line.
func arrowReturnsPlainCall(args []parser.Expr) bool {
	if len(args) == 0 {
		return false
	}

	arrow, ok := args[len(args)-1].(*parser.ArrowFnExpr)
	if !ok {
		return false
	}

	call, ok := arrow.Body.(*parser.CallExpr)

	return ok && len(call.Args) > 0 && !argsEndInBlockHug(call.Args) && !hugsBracketedBlock(call) && !isFluentChain(call.Callee)
}

// argsEndInBlockHug reports whether args end in a braced or bracketed body, so
// the call's opener carries it and keeps what precedes it on the same line.
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

	huggable := len(call.Args) > 0 && b.huggableArg(call.Args[len(call.Args)-1])
	if huggable && !b.hugLineOverflows(call, stmtLevel) && !b.arrayLinkFitsAlone(call, stmtLevel) && !b.arrowLinkFitsAlone(call, stmtLevel) {
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

		if (b.depth+1)*b.cfg.IndentWidth+b.linkOpenerWidth(mem, call)+linkLineTail(call, tail) > b.cfg.PrintWidth {
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

// headLineLinks un-hugs the arrow of a statement's last call kept on the
// head's line, which flattenChain measured to fit alone there.
func headLineLinks(links []chainLink) []chainLink {
	last := links[len(links)-1]
	if last.unhugged == nil {
		return links
	}

	unhugged := slices.Clone(links)
	unhugged[len(links)-1].doc = last.unhugged

	return unhugged
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

	if (b.depth+1)*b.cfg.IndentWidth+b.linkOpenerWidth(mem, call)+linkLineTail(call, b.chainStmtTail) <= b.cfg.PrintWidth {
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

		link := document.Concat(b.tok(mem.Arrow), b.expr(mem.Property), b.argList(call.Args))
		width, _ := document.FlatPrefixWidth(link, b.cfg.IndentWidth)

		return width
	})
}

// linkLineTail is what follows a moved-down link on its opening line: tail,
// unless the link breaks inside its arguments first (a closure body).
func linkLineTail(call *parser.CallExpr, tail int) int {
	if len(call.Args) > 0 && argsEndInBlockHug(call.Args) && !isArrayArg(call.Args[len(call.Args)-1]) {
		return 0
	}

	return tail
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

	return b.linkFitsAlone(mem, call)
}

func (b *builder) arrowLinkFitsAlone(call *parser.CallExpr, stmtLevel bool) bool {
	mem, isMember := call.Callee.(*parser.MemberExpr)
	if !stmtLevel || !isMember || !isArrowFnArg(call.Args[len(call.Args)-1]) {
		return false
	}

	return b.linkFitsAlone(mem, call)
}

func (b *builder) linkFitsAlone(mem *parser.MemberExpr, call *parser.CallExpr) bool {
	// argList consumes a pending noHugNext meant for the real build.
	savedNoHug := b.noHugNext
	b.noHugNext = false

	defer func() { b.noHugNext = savedNoHug }()

	link := b.measureWidth(func() int {
		doc := document.Concat(b.tok(mem.Arrow), b.expr(mem.Property), b.argList(call.Args))
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
		return b.closureFitsAlone(last)
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

	if (b.depth+1)*b.cfg.IndentWidth+width+len(",") <= b.cfg.PrintWidth {
		return true
	}

	// A call that breaks moved down too helps only where even its narrowest
	// line overflows hugged: a level in, that line is narrower.
	if !isCallLikeArg(call.Args[0]) {
		return false
	}

	hugged, _ := b.hugLineWidth(call, true)
	opener := b.measureWidth(func() int { return b.hugOpenerWidth(call.Args[0]) })

	return hugged-opener+b.callOpenWidth(call.Args[0]) > b.cfg.PrintWidth
}

func (b *builder) callOpenWidth(arg parser.Expr) int {
	switch v := arg.(type) {
	case *parser.NamedArgExpr:
		return len(v.Name.Text) + len(": ") + b.callOpenWidth(v.Value)
	case *parser.CallExpr:
		if width, broke := b.brokenCalleeOpener(v); broke {
			return width
		}

		return b.measureWidth(func() int { return document.FlatWidth(b.expr(v.Callee), b.cfg.IndentWidth) }) + len("(")
	case *parser.NewExpr:
		return len("new ") + b.measureWidth(func() int { return document.FlatWidth(b.expr(v.Class), b.cfg.IndentWidth) }) + len("(")
	}

	return 0
}

func hugsInsideOut(args []parser.Expr) bool {
	if len(args) == 0 {
		return false
	}

	last := args[len(args)-1]

	return isCallLikeArg(last) || isArrowFnArg(last) || len(args) == 1 && shortListClosure(last) != nil
}

const maxShortListUses = 2

func shortListClosure(arg parser.Expr) *parser.ClosureExpr {
	if n, ok := arg.(*parser.NamedArgExpr); ok {
		arg = n.Value
	}

	c, ok := arg.(*parser.ClosureExpr)
	if !ok || len(c.Params) > 1 || len(c.Uses) > maxShortListUses || len(c.Params)+len(c.Uses) == 0 {
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
				w += document.FlatWidth(document.Concat(b.tok(links[i].Arrow), b.expr(links[i].Property), b.argList(cur.Args)), b.cfg.IndentWidth)
				cur, _ = links[i].Object.(*parser.CallExpr)
			}

			return w
		})

		if !breaks {
			return false
		}
	}

	width, hugs := b.hugLineWidth(call, false)
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

	width, hugs := b.hugLineWidth(call, false)

	return hugs && width > b.cfg.PrintWidth
}

func (b *builder) hugLineOverflowsFromCloser(call *parser.CallExpr) bool {
	width, hugs := b.hugLineWidth(call, true)

	return hugs && width > b.cfg.PrintWidth
}

// arrowFitsAlone reports whether a last arrow argument that the hugged line
// cannot hold whole fits flat on a line of its own. lead is the column the
// call's `(` opens at, indent the column its arguments take once broken, and
// tail what follows the `)`. hugLineWidth counts only a long body call's
// opener, so without this the hug splits a body the un-hugged arrow would keep
// on one line.
func (b *builder) arrowFitsAlone(args []parser.Expr, lead, indent, tail int) bool {
	leading, width, ok := b.lastArrowWidths(args)
	if !ok {
		return false
	}

	line := lead + len("(") + leading + width + len(")") + tail

	return line > b.cfg.PrintWidth && indent+width+len(",") <= b.cfg.PrintWidth
}

// lastArrowFitsAlone is arrowFitsAlone where the column is unknown, as in an
// arrow function's body: broken a level in, the arrow fits flat. The hugged
// line's width does not matter, since a list that fits whole prints flat
// hugged or not.
func (b *builder) lastArrowFitsAlone(args []parser.Expr) bool {
	_, width, ok := b.lastArrowWidths(args)

	return ok && (b.depth+1)*b.cfg.IndentWidth+width+len(",") <= b.cfg.PrintWidth
}

func (b *builder) bodyArrowFitsAlone(call *parser.CallExpr) bool {
	return b.staysOnStatementLine(call) && b.lastArrowFitsAlone(call.Args) && b.unhuggingHelps(call)
}

// lastArrowWidths measures a last arrow argument flat, and the leading
// arguments with their separators. ok is false when the arrow is not one that
// could move to a line of its own.
func (b *builder) lastArrowWidths(args []parser.Expr) (int, int, bool) {
	last := len(args) - 1
	if last < 0 {
		return 0, 0, false
	}

	arrow, ok := args[last].(*parser.ArrowFnExpr)
	if !ok {
		return 0, 0, false
	}

	// A body that is a block hugs it. A call opening one (`->create([`) hugs
	// only once the block breaks anyway, which the forced-break check below
	// sees; flat, the arrow fits alone like any other.
	switch arrow.Body.(type) {
	case *parser.ArrayExpr, *parser.MatchExpr:
		return 0, 0, false
	}

	leading, width := 0, 0
	ok = false

	// The arguments' own lists would consume flags pending for the call's.
	savedNoHug, savedLinkArgs, savedLead := b.noHugNext, b.linkArgsNext, b.hugLeadNext
	b.noHugNext, b.linkArgsNext, b.hugLeadNext = false, false, 0

	defer func() { b.noHugNext, b.linkArgsNext, b.hugLeadNext = savedNoHug, savedLinkArgs, savedLead }()

	b.measureWidth(func() int {
		for _, a := range args[:last] {
			d := b.expr(a)
			if document.CanBreak(d) {
				return 0
			}

			leading += document.FlatWidth(d, b.cfg.IndentWidth) + len(", ")
		}

		// An arrow holding a break of its own (an author-broken list, a block
		// it opens) prints broken wherever it goes; flat widths say nothing.
		doc := b.expr(args[last])
		if document.HasForcedBreak(doc) {
			return 0
		}

		width = document.FlatWidth(doc, b.cfg.IndentWidth)
		ok = true

		return 0
	})

	return leading, width, ok
}

// headLinkArrowFitsAlone reports whether a statement's first call, which
// stays on its head's line when neither that line nor the ladder holds it
// flat (headCallArgsBreak), un-hugs a last arrow that fits alone there. Its
// arguments are otherwise built for the ladder's column.
func (b *builder) headLinkArrowFitsAlone(link *parser.MemberExpr, arrow, property *document.Doc, args []parser.Expr) bool {
	if !reachesNoCall(link.Object) {
		return false
	}

	leading, width, ok := b.lastArrowWidths(args)
	if !ok {
		return false
	}

	tab := b.cfg.IndentWidth
	list := len("(") + leading + width + len(")")
	indent := (b.depth + 1) * tab

	onHead := b.headLineLead(link, arrow, property) + list
	movedDown := indent + document.FlatWidth(arrow, tab) + document.FlatWidth(property, tab) + list

	return onHead > b.cfg.PrintWidth && movedDown > b.cfg.PrintWidth && indent+width+len(",") <= b.cfg.PrintWidth
}

func (b *builder) headLineLead(link *parser.MemberExpr, arrow, property *document.Doc) int {
	tab := b.cfg.IndentWidth

	savedNoHug, savedLinkArgs, savedLead := b.noHugNext, b.linkArgsNext, b.hugLeadNext
	b.noHugNext, b.linkArgsNext, b.hugLeadNext = false, false, 0

	head := b.measureWidth(func() int { return document.FlatWidth(b.expr(link.Object), tab) })

	b.noHugNext, b.linkArgsNext, b.hugLeadNext = savedNoHug, savedLinkArgs, savedLead

	return b.depth*tab + b.chainStmtLead + head + document.FlatWidth(arrow, tab) + document.FlatWidth(property, tab)
}

func reachesNoCall(e parser.Expr) bool {
	for {
		switch n := e.(type) {
		case *parser.MemberExpr:
			e = n.Object
		case *parser.SubscriptExpr:
			e = n.Array
		case *parser.VariableExpr:
			return true
		default:
			return false
		}
	}
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
//
// fromCloser measures a callee kept broken (`collect(` … `)->map(`) from its
// closing line. Only the un-hug decision may ask for it: its output reads
// back as authored, so it holds, where a moved link would flip next pass.
func (b *builder) hugLineWidth(call *parser.CallExpr, fromCloser bool) (int, bool) {
	if len(call.Args) == 0 || !b.huggableArg(call.Args[len(call.Args)-1]) && !isArrowFnArg(call.Args[len(call.Args)-1]) {
		return 0, false
	}

	last := len(call.Args) - 1
	hugs := true
	lead := b.chainStmtLead

	// argList hugs only past leading arguments that cannot break, so a list
	// with a breakable one never opens that line.
	width := b.measureWidth(func() int {
		// A callee that renders multi-line (`it(…, function () {…})->with`) puts
		// the hug after its own closer, at a column no flat width tells.
		callee := b.expr(call.Callee)

		w := document.FlatWidth(callee, b.cfg.IndentWidth)
		if w >= document.ForcedBreakWidth {
			hugs = false
		} else if fromCloser && document.HasForcedBreak(callee) {
			if closing, broke := document.ClosingLineWidth(callee, b.cfg.IndentWidth); broke {
				w, lead = closing, 0
			}
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

	return b.depth*b.cfg.IndentWidth + lead + width, hugs
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
		// would read back as authored on the next pass. A last arrow that fits
		// alone a level in un-hugs too, as arrowFn builds it.
		if call, ok := v.Body.(*parser.CallExpr); ok && len(call.Args) > 0 &&
			(b.authorBroke(call.Args[0].Span().Start.Offset) || b.bodyArrowFitsAlone(call)) {
			return prefix + document.FlatWidth(b.expr(call.Callee), tab) + len("(")
		}

		return prefix + b.hugOpenerWidth(v.Body)
	case *parser.CallExpr:
		if width, broke := b.brokenCalleeOpener(v); broke {
			return width
		}

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

// brokenCalleeOpener is the width of the line a call's callee opens when an
// author break inside it (`Foo::make([` … `])->m(`) ends that line first.
func (b *builder) brokenCalleeOpener(call *parser.CallExpr) (int, bool) {
	if memberCalls(call) > 1 {
		return 0, false
	}

	span := call.Callee.Span()
	if !bytes.ContainsAny(b.src[span.Start.Offset:span.End.Offset], "\r\n") {
		return 0, false
	}

	broke := false
	width := b.measureWidth(func() int {
		w, ok := document.OpeningLineWidth(b.expr(call.Callee), b.cfg.IndentWidth)
		broke = ok

		return w
	})

	return width, broke
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

	if b.huggableArg(args[last]) && (len(args) < 2 || !allNamedArgs(args)) {
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

	if b.huggableArg(args[0]) && !isArrowFnArg(args[0]) && !isCallLikeArg(args[0]) && b.trailingArgsHug(args[0], args[1:]) &&
		!b.leadingArgStaysUnhugged(args) {
		return open + b.hugOpenerWidth(args[0])
	}

	return open
}

// headCloserWidth is the width of the closing line (`)`, `])`) a broken head
// call ends on, and whether a sole link rides it; more links ladder.
//
// Both passes have to measure alike. A head too wide for its line is predicted
// at the widest line it can close on, never narrower than the next pass reads
// back. A head that fits was broken by choice and measured as a ladder before,
// so its closer counts only up to one indent, and only in a statement's own
// chain, where the printer settles the hug.
func (b *builder) headCloserWidth(link *parser.MemberExpr) (int, bool) {
	head, ok := link.Object.(*parser.CallExpr)
	if !ok || memberCalls(head) != 0 || len(head.Args) == 0 {
		return 0, false
	}

	lead := b.depth * b.cfg.IndentWidth
	if b.stmtChain {
		lead += b.chainStmtLead
	}

	// Counted from the source, so it reads the same however the head is broken.
	span := head.Span()
	flat := b.flatSourceWidth(span.Start.Offset, span.End.Offset) + len(head.Args) - 1
	overflows := lead+flat > b.cfg.PrintWidth

	if !overflows && !bytes.ContainsAny(b.src[span.Start.Offset:span.End.Offset], "\r\n") {
		return 0, false
	}

	savedNoHug, savedLinkArgs, savedLead := b.noHugNext, b.linkArgsNext, b.hugLeadNext
	b.noHugNext, b.linkArgsNext, b.hugLeadNext = false, false, 0

	width, rides := 0, false

	b.measureWidth(func() int {
		doc := b.expr(head)

		if document.HasForcedBreak(doc) {
			closing, broke := document.ClosingLineWidth(doc, b.cfg.IndentWidth)
			width, rides = closing, broke && (overflows || b.stmtChain && closing <= b.cfg.IndentWidth)

			return 0
		}

		if !overflows {
			return 0
		}

		// A break the printer picks among variants cannot be predicted, so the
		// link measures as wide as it can: un-hugged, it reads back the same.
		width, rides = b.cfg.PrintWidth, true

		if closing, known := document.FirstBreakClosingWidth(doc, b.cfg.IndentWidth); known {
			width = closing
		}

		return 0
	})

	b.noHugNext, b.linkArgsNext, b.hugLeadNext = savedNoHug, savedLinkArgs, savedLead

	return width, rides
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

// A callee that prints broken (`(new X([` … `]))->through`) puts the `(`
// after its closing line, not after its flat width.
func (b *builder) calleeLead(callee parser.Expr, prefix int) int {
	span := callee.Span()
	lead := b.depth*b.cfg.IndentWidth + prefix + b.flatSourceWidth(span.Start.Offset, span.End.Offset)

	if !bytes.ContainsAny(b.src[span.Start.Offset:span.End.Offset], "\r\n") {
		return lead
	}

	savedNoHug, savedLinkArgs, savedLead := b.noHugNext, b.linkArgsNext, b.hugLeadNext
	b.noHugNext, b.linkArgsNext, b.hugLeadNext = false, false, 0

	b.measureWidth(func() int {
		doc := b.expr(callee)
		if !document.HasForcedBreak(doc) {
			return 0
		}

		if closing, broke := document.ClosingLineWidth(doc, b.cfg.IndentWidth); broke {
			lead = b.depth*b.cfg.IndentWidth + closing
		}

		return 0
	})

	b.noHugNext, b.linkArgsNext, b.hugLeadNext = savedNoHug, savedLinkArgs, savedLead

	return lead
}

// nestedLineSlack is how much of its line a nested call is assumed to share
// with what opens it (`if (`, `'key' => `, `$result = outer(`). A hug that
// fits with that much to spare is built once; a closer one is the printer's.
const nestedLineSlack = 40

// headLineArgs builds the arguments of a call that stays on its head's line.
// A statement's own call knows that line's lead. A nested one knows only its
// callee, so it hugs as if the line began there and leaves the hug to the
// printer where the line could still overflow: the fallback list reads back
// as authored, so the choice holds.
func (b *builder) headLineArgs(call *parser.CallExpr, build func() (*document.Doc, bool)) (*document.Doc, bool) {
	if b.stmtChain {
		b.hugLeadNext = b.calleeLead(call.Callee, b.chainStmtLead)

		return build()
	}

	// An arrow function's body is measured by the hug around the arrow, which
	// has to see it as one build, so it gets the lead and no second variant.
	if b.arrowBodyChain {
		b.hugLeadNext = b.calleeLead(call.Callee, b.arrowBodyLead)

		return build()
	}

	return b.nestedArgs(call.Args, b.calleeLead(call.Callee, 0), build)
}

func (b *builder) nestedArgs(args []parser.Expr, lead int, build func() (*document.Doc, bool)) (*document.Doc, bool) {
	b.hugLeadNext = lead
	mark := b.markComments()

	hugged, ok := build()
	if !ok || !hugsInsideOut(args) && !isArrayArg(args[len(args)-1]) {
		return hugged, ok
	}

	last := args[len(args)-1]

	opener, _ := document.BrokenPrefixWidth(hugged, b.cfg.IndentWidth)
	if lead+nestedLineSlack+opener <= b.cfg.PrintWidth {
		return hugged, ok
	}

	// A closure too wide for a line of its own gains nothing un-hugged.
	if shortListClosure(last) != nil && !b.closureFitsAlone(last) {
		return hugged, ok
	}

	b.releaseComments(mark)
	b.noHugNext = true
	unhugged, _ := build()

	return document.ConditionalGroup(document.Opener(opener, hugged), unhugged), ok
}

func (b *builder) closureFitsAlone(closure parser.Expr) bool {
	width := b.measureWidth(func() int { return b.hugOpenerWidth(closure) })

	return (b.depth+1)*b.cfg.IndentWidth+width <= b.cfg.PrintWidth
}

// hugWhileItsLineFits leaves a sole link's hug on a head call to the printer:
// the link lands on the head's line, its closer or a level down, which only
// the printer knows. The fallback list reads back as authored, so the choice
// holds. A closure, array or match keeps its hug past the width, and a link
// that fits wherever the head ends needs no second build.
func (b *builder) hugWhileItsLineFits(call *parser.CallExpr, method string, hugged *document.Doc, mark commentMark, linkArgs bool) *document.Doc {
	link, ok := call.Callee.(*parser.MemberExpr)
	if !ok || !hugsInsideOut(call.Args) {
		return hugged
	}

	head, ok := link.Object.(*parser.CallExpr)
	if !ok || memberCalls(head) != 0 {
		return hugged
	}

	opener, _ := document.BrokenPrefixWidth(hugged, b.cfg.IndentWidth)
	callee := call.Callee.Span()
	byOffset := func(t parser.Token, offset int) int { return t.Span.Start.Offset - offset }
	first, _ := slices.BinarySearchFunc(b.tokens, callee.Start.Offset, byOffset)
	end, _ := slices.BinarySearchFunc(b.tokens, callee.End.Offset, byOffset)

	// An upper bound whatever spaces the source has: each token prints with at
	// most one space after it.
	widest := b.depth*b.cfg.IndentWidth + b.flatSourceWidth(callee.Start.Offset, callee.End.Offset) + end - first
	if b.stmtChain {
		widest += b.chainStmtLead
	}

	if widest+opener <= b.cfg.PrintWidth {
		return hugged
	}

	choice, _ := b.hugWhileItFits(call, method, hugged, mark, linkArgs)

	return choice
}

// hugWhileItFits offers a link's hug while its opener fits where the printer
// puts it, and its list broken after `(` otherwise. The next pass reads that
// break as authored, so the choice holds.
func (b *builder) hugWhileItFits(call *parser.CallExpr, method string, hugged *document.Doc, mark commentMark, linkArgs bool) (*document.Doc, *document.Doc) {
	opener, _ := document.BrokenPrefixWidth(hugged, b.cfg.IndentWidth)

	b.releaseComments(mark)

	b.noHugNext, b.linkArgsNext = true, linkArgs
	unhugged, _ := b.linkArgList(method, call.Args)

	return document.ConditionalGroup(document.Opener(opener, hugged), unhugged), unhugged
}

func linkWith(arrow, property, args *document.Doc) *document.Doc {
	if args == nil {
		return nil
	}

	return document.Concat(arrow, property, args)
}

// flattenChain peels postfix operations off e, returning the head expression and
// the links in head-to-outermost order.
func (b *builder) flattenChain(e parser.Expr) (parser.Expr, []chainLink) {
	var links []chainLink

	// A link's arguments sit a level in from its head only on a ladder.
	ladders := !callStaysOnHead(e)
	soleLink := memberCalls(e) == 1

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
				level := b.depth + 1
				lead := level * b.cfg.IndentWidth

				rides := false

				if soleLink {
					var closer int

					if closer, rides = b.headCloserWidth(c); rides {
						level = b.depth
						lead = level*b.cfg.IndentWidth + closer
					}
				}

				tail := 0
				if b.stmtChain && len(links) == 0 {
					tail = b.chainStmtTail
				}

				if ladders && b.arrowFitsAlone(n.Args, lead+len("->")+len(method), (level+1)*b.cfg.IndentWidth, tail) {
					b.noHugNext = true
				}

				if ladders && b.stmtChain && len(links) > 0 && b.headLinkArrowFitsAlone(c, arrow, property, n.Args) {
					b.noHugNext = true
				}

				linkArgs := ladders && !rides

				var (
					args       *document.Doc
					argsHugged bool
					unhugged   *document.Doc
				)

				if ladders {
					b.hugLeadNext = lead + len("->") + document.FlatWidth(property, b.cfg.IndentWidth)
					b.linkArgsNext = linkArgs
					mark := b.markComments()
					args, argsHugged = b.linkArgList(method, n.Args)

					switch {
					case soleLink && argsHugged:
						args = b.hugWhileItsLineFits(n, method, args, mark, linkArgs)
					case b.stmtChain && len(links) == 0 && argsHugged && (isArrowFnArg(n.Args[len(n.Args)-1]) || len(n.Args) == 1 && shortListClosure(n.Args[0]) != nil):
						// A statement's last call may stay on the head's line
						// (onlyLastCallBreaks), a level further out than the
						// ladder. There its arrow un-hugs where it fits alone,
						// and a hug whose line overflows gives way.
						lead := b.headLineLead(c, arrow, property)
						opener, _ := document.BrokenPrefixWidth(args, b.cfg.IndentWidth)
						fitsAlone := isArrowFnArg(n.Args[len(n.Args)-1]) && b.arrowFitsAlone(n.Args, lead, (b.depth+1)*b.cfg.IndentWidth, b.chainStmtTail)

						if fitsAlone || lead+opener > b.cfg.PrintWidth {
							args, unhugged = b.hugWhileItFits(n, method, args, mark, linkArgs)
						}

						if !fitsAlone {
							unhugged = nil
						}
					}
				} else {
					args, argsHugged = b.headLineArgs(n, func() (*document.Doc, bool) {
						return b.linkArgList(method, n.Args)
					})
				}

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
					unhugged:    linkWith(arrow, property, unhugged),
				})
				e = c.Object
			case *parser.StaticMemberExpr:
				scopeRes := b.tok(c.ScopeRes)
				name := b.expr(c.Name)

				// Built after the name: a call inside a dynamic name would take the lead.
				var args *document.Doc

				if _, named := c.Class.(*parser.IdentExpr); named {
					args, _ = b.headLineArgs(n, func() (*document.Doc, bool) {
						return b.argListHug(n.Args, withTrailingComma)
					})
				} else {
					args = b.argList(n.Args)
				}

				links = append(links, chainLink{
					doc:         document.Concat(scopeRes, name, args),
					call:        true,
					blockHug:    argsEndInBlockHug(n.Args),
					access:      true,
					brokeBefore: b.newlineBefore(c.ScopeRes.Span.Start.Offset),
				})
				e = c.Class
			default:
				trailing := withTrailingComma
				if rejectsTrailingComma(n.Callee) {
					trailing = withoutTrailingComma
				}

				args, _ := b.headLineArgs(n, func() (*document.Doc, bool) { return b.argListHug(n.Args, trailing) })

				links = append(links, chainLink{
					doc:      args,
					call:     true,
					blockHug: argsEndInBlockHug(n.Args),
					noArgs:   len(n.Args) == 0,
				})
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

func newEndsInBlockHug(head parser.Expr) bool {
	n, ok := head.(*parser.NewExpr)

	return ok && argsEndInBlockHug(n.Args)
}
