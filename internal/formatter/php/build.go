package php

// Doc-based PHP layout: the builder turns the CST into a document.Doc of
// layout intent, and document.Print resolves it against the print width in one
// pass. A node the builder does not model falls back to its verbatim source.

import (
	"bytes"
	"sort"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

type builder struct {
	src           []byte
	cfg           *config.File
	tokens        []parser.Token // in source order, so leadingAt can binary-search them
	comments      []int          // sorted offsets of every comment, for interior-comment checks
	htmlTags      []int          // sorted offsets of `?>`/`<?php`/`<?=`/inline-HTML tokens, marking template regions
	crLiterals    []crLiteral    // source ranges of string/heredoc tokens holding a CR, whose bytes must not be EOL-normalized
	headerSpacing bool           // emit PER-CS file-header blank lines (pure-PHP files only)
	rehomeSlices  bool           // BuildStmts only: verbatim slices drop source indent so the host's re-home controls placement
	embed         bool           // embedded fragment (Blade/Livewire): the host re-emits line by line and normalizes EOLs, so heredoc CRs are stripped before reindent
	// noAuthorBreaks drops the "author broke this array" marker for a directive
	// wrapper, which picks its layout from the argument: reading back the break
	// it wrote itself would flip that layout on every pass.
	noAuthorBreaks bool
	// firstLineColumn is the file column src's first line starts at, for a
	// fragment cut from mid-line (a directive's arguments), so a comment there
	// measures where it really sits.
	firstLineColumn int
	// noConcatBreaks lets a concatenation fill as if unbroken, for a layout that
	// must read the same whether a fill break came from the author or a pass.
	noConcatBreaks bool
	depth          int          // indent levels entered so far, for width predictions that need the true print column
	useRunInterior map[int]bool // start offsets of non-first imports in a sorted run, whose original blank-before no longer applies
	stmtStart      int          // start offset of the statement being built (its leading comments are the sequence's, not a node's)
	elemStart      int          // start offset of the array element being built (its leading comments are the element list's, not the value's)
	armStart       int          // start offset of the match arm being built (its leading comments are the arm list's, not the condition's)
	measuring      bool         // inside a throwaway width probe, where no comment may be claimed
	mixed          bool         // mixed PHP/HTML template, where an island's layout moves a comment between its own line and a shared one
	// emitted records the source offset of every comment already placed, so a
	// comment an expression claims inline is not also emitted by the sequence above
	// it. Expressions are built before buildSequence reads its comments, so without
	// this the two would both write it.
	emitted map[int]bool
	claims  []int // the offsets in emitted, in claim order, so a discarded doc can release its own
	// placed records every comment some doc was built for, verbatim spans
	// included. Only keepComments reads it; unlike emitted it never makes a
	// layout skip a comment, so a discarded measurement cannot cost one.
	placed map[int]bool
	// useTrail maps a sorted import to the same-line comment written after it,
	// which trivia stores on the token that follows it in the source; movedTrail
	// holds those comments' offsets so the reader of that token skips them.
	useTrail   map[parser.Stmt][]parser.Trivia
	movedTrail map[int]bool
	// useRunHead maps the import sorted to the front of a run to the one that
	// opened it in the source, whose leading trivia (the comments and blank line
	// before the run) stays at the head; useRunHeadMoved marks that original.
	useRunHead      map[int]int
	useRunHeadMoved map[int]bool
	// useRunTailMoved marks the token after an import that sorting moved off
	// the end of its run; the comments before that token go on their own line.
	useRunTailMoved   map[int]bool
	groupID           int32         // last allocated document group id; 0 is "unnamed", so ids start at 1
	alignScope        int32         // last allocated `=>` alignment scope; one per aligned array or match
	commentGroups     map[int]int32 // trailing-comment group, by its first comment offset, to its CommentScope id
	commentGroupIDs   int32         // last allocated commentGroups id
	chainStmtLevel    bool          // the current expression is a statement's own value (expression statement, return, assignment RHS), the only place a preserved chain's forced break feeds no enclosing width decision
	chainStmtLead     int           // columns that statement puts before the value on its first line (its target, `return `), which a chain must fit alongside
	chainStmtTail     int           // columns that statement puts after the value on its last line (the terminator)
	noHugNext         bool          // the next argList skips its last-argument hug; read and cleared on entry
	linkArgsNext      bool          // the next argList belongs to a chain link, a level in from its head; read and cleared on entry
	hugLeadNext       int           // the column the next argList's `(` follows, 0 when unknown; read and cleared on entry
	stmtChain         bool          // flattenChain walks a statement's own chain, whose lead is known
	arrowBodyNext     bool          // the next postfix is an arrow function's body, its links laddering a level in; read and cleared on entry
	preserveChainNext bool          // the next postfix is an operand of a statement's binary, which keeps an author ladder as the statement's own chain does; read and cleared on entry
	preserveAt        int           // source offset of a nested chain that keeps its author ladder (a Pest `->and(` argument), 0 for none
	lineKnownNext     bool          // the next value starts a line whose column the depth gives (an array entry's value); read and cleared in exprInner
	inKey             bool          // inside a subscript key, which lays out flat unless the author broke it
}

func (b *builder) nextGroupID() int32 {
	b.groupID++

	return b.groupID
}

// stmtExpr builds a statement's own value with chainStmtLevel set, so a fluent
// chain that is the whole statement (or an assignment's right side) may keep
// the author's line breaks; exprInner clears the flag, so a chain nested
// anywhere else does not. lead and tail are the widths the statement itself
// puts before and after the value.
func (b *builder) stmtExpr(e parser.Expr, lead, tail int) *document.Doc {
	savedLevel, savedLead, savedTail := b.chainStmtLevel, b.chainStmtLead, b.chainStmtTail
	b.chainStmtLevel, b.chainStmtLead, b.chainStmtTail = true, lead, tail
	d := b.expr(e)
	b.chainStmtLevel, b.chainStmtLead, b.chainStmtTail = savedLevel, savedLead, savedTail

	return d
}

// BuildFile lays out a parsed PHP file via the Doc model and returns the bytes.
func BuildFile(file *parser.File, cfg *config.File) []byte {
	b := &builder{src: file.Src, cfg: cfg, tokens: file.Tokens, comments: commentOffsets(file.Tokens), htmlTags: tagOffsets(file.Tokens), crLiterals: crLiteralOffsets(file.Tokens)}

	// A trailing whitespace-only inline-HTML run does not survive a reparse (PHP
	// eats the newline after `?>`), so it must not reach the mixed/pure choice
	// below or the builder's own output flips paths on the next pass. Format drops
	// it earlier, before the trailing-tag decision that needs the tag to be last;
	// this covers livewire, which reaches the builder directly.
	file.Stmts = stripTrailingBlankHTML(file.Stmts)

	// Nothing but whitespace: formatted like an empty file in every language.
	if len(file.Stmts) == 0 {
		return nil
	}

	if isMixedMode(file.Stmts) {
		return b.buildMixed(file)
	}

	b.headerSpacing = startsWithOpenTag(file.Stmts) && !containsInlineHTML(file.Stmts)

	src := b.preprocessUseRuns(file.Stmts)
	parts := make([]*document.Doc, 0, 4)

	start := 0

	if len(src) > 0 {
		if tag, ok := src[0].(*parser.TagStmt); ok && tag.Token.Kind == parser.KindOpenTag {
			start = 1
		}
	}

	body := src[start:]
	spans := make([]parser.Span, len(body))
	docs := make([]*document.Doc, len(body))

	for i, s := range body {
		spans[i] = s.Span()
		docs[i] = b.stmt(s)
	}

	sequence := b.buildSequence(spans, docs, b.stmtForceBlank(body), false)

	// Trivia attaches to the token after it, so a comment with no statement
	// after it belongs to EOF and is emitted here.
	tail := b.fileTailComments(eofToken(file.Tokens), len(body) > 0)

	if start == 1 {
		parts = append(parts, document.Text("<?php"))

		if len(body) > 0 || tail != nil {
			parts = append(parts, document.HardLine(), document.HardLine())
		}
	}

	parts = append(parts, sequence)

	if tail != nil {
		parts = append(parts, tail)
	}

	parts = append(parts, document.HardLine())

	out := document.Print(document.Concat(parts...), document.Options{
		PrintWidth: cfg.PrintWidth,
		TabWidth:   cfg.TabWidth,
		UseTabs:    cfg.UseTabs(),
		EOL:        cfg.EOL(),
	})

	return out
}

func isMixedMode(stmts []parser.Stmt) bool {
	for _, s := range stmts {
		switch s.(type) {
		case *parser.InlineHTMLStmt, *parser.ShortEchoStmt:
			return true
		}
	}

	return false
}

// buildMixed lays out a mixed PHP/HTML file: verbatim inline-HTML runs
// interleaved with PHP islands laid out by the doc statement builders. The
// separator between two statements reproduces the author's source spacing, so a
// break appears only where the source had a newline. An inline `<?= $x ?>` in an
// attribute value or an inline `<?php echo $x; ?>` therefore stays on its HTML
// line; breaking it would inject whitespace into the rendered output.
func (b *builder) buildMixed(file *parser.File) []byte {
	b.headerSpacing = false
	b.mixed = true

	stmts := b.preprocessUseRuns(file.Stmts)
	parts := make([]*document.Doc, 0, len(stmts)*2+1)

	prevEnd := -1
	i := 0

	// The pieces of a split `use A, B;` share one offset; its comments go out
	// with the first.
	commented := make(map[int]bool)

	for i < len(stmts) {
		// A comment that shared the previous line has to be queued before the
		// separator writes that line's break, or it rides the wrong one.
		var trail, lead []*document.Doc

		if offset := stmts[i].Span().Start.Offset; !commented[offset] {
			commented[offset] = true
			trail, lead = b.mixedComments(offset)
		}

		parts = append(parts, trail...)

		broke := false

		if prevEnd >= 0 {
			sep := b.mixedSeparator(prevEnd, stmts[i].Span().Start.Offset, stmts[i-1], stmts[i])

			// A line comment runs to the end of its line, so the statement
			// after it cannot share that line: `<?php #c ?>` would put the
			// `?>` inside the comment.
			if len(trail) > 0 && b.hasBreakingTrail(stmts[i].Span().Start.Offset) {
				sep = b.mixedBreak(stmts[i-1], stmts[i])
			}

			if sep != nil {
				parts = append(parts, sep)

				broke = true
			}
		}

		// An own-line comment needs the line to itself. The gap the separator measures
		// can be empty even here, because a `?>` is the previous statement's implicit
		// terminator and its span runs to it, so the comment sits inside that span.
		if len(lead) > 0 && !broke && len(parts) > 0 {
			parts = append(parts, document.HardLine())
		}

		parts = append(parts, lead...)

		// The comments above belong to the statement, so its own expression
		// must not claim them again; stmtStart marks that, as in buildSequence.
		b.stmtStart = stmts[i].Span().Start.Offset

		if consumed, d := b.tryBuildShortEcho(stmts[i:]); consumed > 0 {
			parts = append(parts, d)
			prevEnd = stmts[i+consumed-1].Span().End.Offset
			i += consumed

			continue
		}

		parts = append(parts, b.mixedStmt(stmts[i]))
		prevEnd = stmts[i].Span().End.Offset
		i++
	}

	if tail := b.fileTailComments(eofToken(file.Tokens), true); tail != nil {
		parts = append(parts, tail)
	}

	// Inline width: a PHP island shares its line with the HTML it abuts, which is
	// not part of the island's width budget. Author-driven breaks (block bodies,
	// multi-line arrays, the source-reproducing separators above) still emit, so
	// only speculative width wrapping is suppressed. That keeps the island's own
	// layout independent of the surrounding markup, which the printer cannot see,
	// and removes the flat/broken flip that shared line would otherwise cause.
	out := document.Print(document.Concat(parts...), document.Options{
		PrintWidth: inlineWidth,
		TabWidth:   b.cfg.TabWidth,
		UseTabs:    b.cfg.UseTabs(),
		EOL:        b.cfg.EOL(),
	})

	return ensureTrailingNewline(out, b.cfg.EOL())
}

// mixedSeparator reproduces the source spacing between two mixed-file statements.
// No newline in the source gap keeps them on one line (nil for an empty gap where
// verbatim inline HTML already carries the spacing, a single space inside a PHP
// island); a newline breaks, with a blank line when the author left one. The blank
// test runs over leading trivia via blankBefore, not the raw gap, so a comment in
// the gap is not double-counted (its own emitted newline would otherwise reappear
// as a blank on the next pass).
func (b *builder) mixedSeparator(prevEnd, curStart int, prev, next parser.Stmt) *document.Doc {
	// preprocessUseRuns may sort or expand a `use` run, leaving the statements out
	// of source order; the gap slice is then meaningless. Those are always PHP
	// imports, so a break (one per line) is the right fallback.
	if prevEnd < 0 || prevEnd > curStart || curStart > len(b.src) {
		return b.mixedBreak(prev, next)
	}

	// A lone CR ends a line for the lexer, so it ends one here too.
	if !bytes.ContainsAny(b.src[prevEnd:curStart], "\n\r") {
		if prevEnd == curStart {
			return nil
		}

		return document.Text(" ")
	}

	return b.mixedBreak(prev, next)
}

func (b *builder) mixedBreak(prev, next parser.Stmt) *document.Doc {
	if b.blankBefore(next.Span().Start.Offset) || b.forceBlankBetween(prev, next) {
		return document.Concat(document.HardLine(), document.HardLine())
	}

	return document.HardLine()
}

// mixedStmt builds one statement of a mixed file. An inline-HTML run is emitted
// verbatim (its bytes are literal template output PHP must not alter); every
// other statement uses the shared doc builder.
func (b *builder) mixedStmt(s parser.Stmt) *document.Doc {
	if _, ok := s.(*parser.InlineHTMLStmt); ok {
		return b.inlineHTML(s.Span())
	}

	return b.stmt(s)
}

// ensureTrailingNewline normalizes a mixed-file result to exactly one trailing
// newline: verbatim inline HTML may end with any number of newlines or none, so
// the doc printer's own break count cannot be relied on here.
func ensureTrailingNewline(out []byte, eol string) []byte {
	return append(bytes.TrimRight(out, "\r\n"), eol...)
}

// tryBuildShortEcho builds a `<?= ... ?>` island from a leading ShortEchoStmt and
// any expression statements that follow it (`<?= $a; $b ?>`), plus a closing tag
// when present. It returns the number of statements consumed and the island doc,
// or (0, nil) when stmts does not open with a short echo.
func (b *builder) tryBuildShortEcho(stmts []parser.Stmt) (int, *document.Doc) {
	head, ok := stmts[0].(*parser.ShortEchoStmt)
	if !ok {
		return 0, nil
	}

	var trailing []parser.Expr

	idx := 1

	for idx < len(stmts) {
		exprStmt, isExpr := stmts[idx].(*parser.ExpressionStmt)
		if !isExpr {
			break
		}

		trailing = append(trailing, exprStmt.Expr)
		idx++
	}

	hasClose := false

	if idx < len(stmts) {
		closeTag, isTag := stmts[idx].(*parser.TagStmt)
		hasClose = isTag && closeTag.Token.Kind == parser.KindCloseTag
	}

	parts := make([]*document.Doc, 0, len(head.Exprs)+len(trailing)*2+2)
	parts = append(parts, document.Text("<?= "))

	for i, expr := range head.Exprs {
		if i > 0 {
			parts = append(parts, document.Text(", "))
		}

		parts = append(parts, b.expr(expr))
	}

	for _, expr := range trailing {
		parts = append(parts, document.Text("; "), b.expr(expr))
	}

	// Built from its statements directly, so the island's comments are kept
	// by keepComments over the whole span below.
	consumed := 1 + len(trailing)

	if hasClose {
		parts = append(parts, document.Text(" ?>"))
		consumed++
	} else {
		parts = append(parts, document.Text(";"))
	}

	span := parser.Span{Start: head.Span().Start, End: stmts[consumed-1].Span().End}

	return consumed, b.keepComments(span, document.Concat(parts...), nil)
}

func (b *builder) verbatim(span parser.Span) *document.Doc {
	start, end := span.Start.Offset, span.End.Offset
	if start < 0 || end > len(b.src) || start >= end {
		return document.Text("")
	}

	b.placeRange(start, end)

	return b.rehome(string(b.src[start:end]), start)
}

// inlineHTML emits literal template output, normalizing only its line endings to
// the file EOL so a CRLF template is not left mixed with the LF-printed islands
// around it. HTML newline normalization is render-preserving.
func (b *builder) inlineHTML(span parser.Span) *document.Doc {
	start, end := span.Start.Offset, span.End.Offset
	if start < 0 || end > len(b.src) || start >= end {
		return document.Text("")
	}

	return document.Text(normalizeEOL(string(b.src[start:end]), b.cfg.EOL()))
}

// normalizeEOL rewrites every CRLF and lone CR in s to eol, returning s
// unchanged (no allocation) on the common no-CR, LF path.
func normalizeEOL(s, eol string) string {
	if eol == "\n" && !strings.Contains(s, "\r") {
		return s
	}

	lf := strings.ReplaceAll(s, "\r\n", "\n")
	lf = strings.ReplaceAll(lf, "\r", "\n")

	if eol == "\n" {
		return lf
	}

	return strings.ReplaceAll(lf, "\n", eol)
}

// normalizeLayoutEOL rewrites the line terminators of a raw source slice to the
// configured EOL, leaving the bytes of any CR-bearing literal it covers exactly
// as they are: those CRs are the literal's value, while a CR between tokens is
// a line break like any other.
func (b *builder) normalizeLayoutEOL(raw string, start int) string {
	eol := b.cfg.EOL()
	end := start + len(raw)

	var out strings.Builder

	pos := start

	for _, lit := range b.crLiterals {
		lo, hi := max(lit.start, start), min(lit.end, end)
		if lo >= hi || hi <= pos {
			continue
		}

		out.WriteString(normalizeEOL(raw[pos-start:lo-start], eol))
		out.WriteString(raw[lo-start : hi-start])
		pos = hi
	}

	if pos == start {
		return normalizeEOL(raw, eol)
	}

	out.WriteString(normalizeEOL(raw[pos-start:], eol))

	return out.String()
}

// rehome routes a raw source slice to the right verbatim form. Only a
// BuildStmts body re-homes: its host re-indents every printed line, so
// source columns must be dropped or they compound per pass. BuildFile and
// BuildExpr print slices at their source columns, which is the stable form.
func (b *builder) rehome(raw string, start int) *document.Doc {
	// The slice's line terminators are layout, not value, so they follow the
	// configured EOL like every line the printer writes.
	raw = b.normalizeLayoutEOL(raw, start)

	if !b.rehomeSlices {
		return document.Text(raw)
	}

	return rehomeVerbatim(raw, sourceIndent(b.src, start))
}

func sourceColumn(src []byte, offset int) int {
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	return offset - lineStart
}

// sourceIndent counts the whitespace bytes opening the line that holds offset:
// the column a verbatim slice's continuation lines are relative to.
func sourceIndent(src []byte, offset int) int {
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	n := 0
	for i := lineStart; i < offset && (src[i] == ' ' || src[i] == '\t'); i++ {
		n++
	}

	return n
}

// rehomeVerbatim converts a raw source slice into a doc the printer can place:
// single-line slices stay plain text; multi-line slices become a
// document.Verbatim whose continuation lines drop their shared leading indent
// so only construct-relative indentation remains. The strip is uniform (the
// smallest non-blank continuation indent, capped at base) so hand alignment,
// outdents, and heredoc bodies shift as one block and cannot change value.
func rehomeVerbatim(raw string, base int) *document.Doc {
	if !strings.Contains(raw, "\n") {
		return document.Text(raw)
	}

	lines := strings.Split(raw, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, "\r")
	}

	strip := base

	for _, ln := range lines[1:] {
		ws := leadingIndentWidth(ln)
		if ws == len(ln) {
			continue
		}

		strip = min(strip, ws)
	}

	rest := make([]string, len(lines)-1)

	for i, ln := range lines[1:] {
		if leadingIndentWidth(ln) == len(ln) {
			continue
		}

		rest[i] = ln[strip:]
	}

	return document.Verbatim(lines[0], rest)
}

func leadingIndentWidth(line string) int {
	n := 0
	for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}

	return n
}

func (b *builder) tok(t parser.Token) *document.Doc {
	return document.Text(string(t.Text))
}

func (b *builder) stmt(s parser.Stmt) *document.Doc {
	doc := b.keepComments(s.Span(), b.layoutStmt(s), s)

	if trail := b.useTrail[s]; len(trail) > 0 {
		suffix := make([]*document.Doc, 0, len(trail))
		for _, tr := range trail {
			suffix = append(suffix, document.Text(" "), b.commentVerbatim(tr))
		}

		doc = document.Concat(doc, document.LineSuffix(document.Concat(suffix...)))
	}

	return doc
}

// closeTagTerminates reports whether a `?>` rather than a `;` ends the
// statement spanning span. A `#` or `//` comment before the tag ends there, so
// it still counts.
func (b *builder) closeTagTerminates(span parser.Span) bool {
	content := bytes.TrimRight(b.src[span.Start.Offset:span.End.Offset], " \t\r\n")
	if len(content) == 0 || bytes.ContainsAny(content[len(content)-1:], ";}:") {
		return false
	}

	next := sort.Search(len(b.tokens), func(i int) bool { return b.tokens[i].Span.Start.Offset >= span.End.Offset })

	return next < len(b.tokens) && b.tokens[next].Kind == parser.KindCloseTag
}

func (b *builder) layoutStmt(s parser.Stmt) *document.Doc {
	// Restored on return, so a nested statement does not leave its own start
	// behind and the enclosing expression still treats its leading comments
	// as the statement's.
	saved := b.stmtStart
	defer func() { b.stmtStart = saved }()

	b.stmtStart = s.Span().Start.Offset

	if b.hasIgnoreDirective(s.Span().Start.Offset) {
		// A `?>` ends the statement as a `;` would; the printer drops a final
		// `?>`, so the frozen copy writes the `;` the reprinted path writes.
		if b.closeTagTerminates(s.Span()) {
			return document.Concat(b.frozen(s.Span()), document.Text(";"))
		}

		return b.frozen(s.Span())
	}

	// A control structure whose body is interrupted by `?> html <?php` is a template
	// region: reprinting would reflow the literal HTML and change the output, so emit
	// it verbatim. EOL is normalized to the file's, except when the region holds a CR
	// inside a string literal, whose value must survive byte for byte.
	if !isLeafHTMLStmt(s) && b.spanHasInlineHTML(s.Span()) {
		if b.spanHasCRLiteral(s.Span()) {
			return b.verbatim(s.Span())
		}

		return b.inlineHTML(s.Span())
	}

	switch n := s.(type) {
	case *parser.ExpressionStmt:
		return document.Concat(b.stmtExpr(n.Expr, 0, len(";")), document.Text(";"))
	case *parser.ReturnStmt:
		if n.Expr == nil {
			return document.Text("return;")
		}

		return document.Concat(document.Text("return "), b.stmtExpr(n.Expr, len("return "), len(";")), document.Text(";"))
	case *parser.EchoStmt:
		return document.Concat(document.Text("echo "), b.exprList(n.Exprs), document.Text(";"))
	case *parser.BlockStmt:
		return b.controlBlock(n)
	case *parser.IfStmt:
		return b.ifStmt(n)
	case *parser.ForEachStmt:
		return b.foreachStmt(n)
	case *parser.WhileStmt:
		return b.whileStmt(n)
	case *parser.TryStmt:
		return b.tryStmt(n)
	case *parser.SwitchStmt:
		return b.switchStmt(n)
	case *parser.DoWhileStmt:
		return b.doWhileStmt(n)
	case *parser.ForStmt:
		return b.forStmt(n)
	case *parser.DeclareStmt:
		return b.declareStmt(n)
	case *parser.FunctionDecl:
		return b.functionDecl(n)
	case *parser.ClassStmt:
		return b.classStmt(n)
	case *parser.InterfaceStmt:
		return b.interfaceStmt(n)
	case *parser.TraitStmt:
		return b.traitStmt(n)
	case *parser.EnumStmt:
		return b.enumStmt(n)
	case *parser.NamespaceStmt:
		return b.namespaceStmt(n)
	case *parser.UseStmt:
		return b.useStmt(n)
	case *parser.ConstStmt:
		return b.constStmt("const", n.Items)
	case *parser.BreakStmt:
		return b.jump("break", n.Level)
	case *parser.ContinueStmt:
		return b.jump("continue", n.Level)
	case *parser.GlobalStmt:
		return document.Concat(document.Text("global "), b.tokenList(n.Vars), document.Text(";"))
	case *parser.UnsetStmt:
		return document.Concat(document.Text("unset"), b.argList(n.Exprs, withTrailingComma), document.Text(";"))
	case *parser.StaticVarStmt:
		return b.staticVarStmt(n)
	case *parser.EmptyStmt:
		return document.Text(";")
	case *parser.GotoStmt:
		return document.Concat(document.Text("goto "), b.tok(n.Label), document.Text(";"))
	case *parser.LabelStmt:
		return document.Concat(b.tok(n.Name), document.Text(":"))
	case *parser.TagStmt:
		if n.Token.Kind == parser.KindCloseTag {
			return document.Text("?>")
		}

		return b.verbatim(s.Span())
	case *parser.HaltCompilerStmt:
		// The call only. Its payload is an archive, and Format appends it after
		// the printer has run: routed through here it would be re-indented, have
		// its line endings rewritten and pick up the file's final newline.
		return b.tok(n.Token)
	case *parser.InlineHTMLStmt:
		return b.inlineHTML(s.Span())
	}

	return b.verbatim(s.Span())
}

func has(t parser.Token) bool { return t.Kind != parser.KindInvalid }

func (b *builder) name(n *parser.Name) *document.Doc {
	parts := make([]*document.Doc, 0, len(n.Parts)*2+1)

	if has(n.LeadingBackslash) {
		parts = append(parts, document.Text("\\"))
	}

	for i, p := range n.Parts {
		// The namespace separator is always `\`; emit it literally rather than
		// from a Separators token, so a synthesized name (an expanded grouped use,
		// which carries no separator tokens) lays out without panicking.
		if i > 0 {
			parts = append(parts, document.Text("\\"))
		}

		parts = append(parts, b.tok(p))
	}

	return document.Concat(parts...)
}

func (b *builder) tokenList(toks []parser.Token) *document.Doc {
	docs := make([]*document.Doc, len(toks))
	for i := range toks {
		docs[i] = b.tok(toks[i])
	}

	return document.Join(document.Text(", "), docs)
}

func startsWithOpenTag(stmts []parser.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}

	tag, ok := stmts[0].(*parser.TagStmt)

	return ok && tag.Token.Kind == parser.KindOpenTag
}

func containsInlineHTML(stmts []parser.Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*parser.InlineHTMLStmt); ok {
			return true
		}
	}

	return false
}

// isLeafHTMLStmt reports whether s is a leaf template statement (inline HTML, a
// ?>/<?php tag, or a <?= ?> echo) with its own emitter, so the verbatim
// template-region guard in layoutStmt must skip it.
func isLeafHTMLStmt(s parser.Stmt) bool {
	switch s.(type) {
	case *parser.InlineHTMLStmt, *parser.TagStmt, *parser.ShortEchoStmt:
		return true
	default:
		return false
	}
}

// stripTrailingBlankHTML drops a final inline-HTML run that is only whitespace.
// PHP eats the one newline after a closing `?>`, so `?>\n\n` parses to a lone `\n`
// inline-HTML run and `?>\n` parses to none; without this the file would enter the
// mixed path on one pass and the pure path on the next, never reaching a fixed
// point. Trailing blank lines at EOF are insignificant, so dropping the run is safe
// and matches RemoveTrailingCloseTag already cutting the tag before them.
func stripTrailingBlankHTML(stmts []parser.Stmt) []parser.Stmt {
	if n := len(stmts); n > 0 {
		html, ok := stmts[n-1].(*parser.InlineHTMLStmt)

		if ok && len(bytes.TrimSpace(html.Token.Text)) == 0 {
			return stmts[:n-1]
		}
	}

	return stmts
}
