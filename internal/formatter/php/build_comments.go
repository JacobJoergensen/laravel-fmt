package php

// Comment and blank-line layout: a node's leading comments print on their own
// lines above it; a same-line trailing comment rides a LineSuffix. Comments are
// read from each token's leading trivia, keyed by start offset. Which statements
// and class members are separated by a blank line is decided here too.

import (
	"bytes"
	"math"
	"slices"
	"sort"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// leadingAt returns the leading trivia of the token starting at offset. It
// searches the tokens rather than indexing them in a map: only about a third
// carry trivia, and building the map was a tenth of a format's allocated bytes.
// Trivia goes to the first token after it, so where a zero-width token shares
// an offset with the next one, the first holds it.
func (b *builder) leadingAt(offset int) []parser.Trivia {
	i := sort.Search(len(b.tokens), func(i int) bool { return b.tokens[i].Span.Start.Offset >= offset })

	if i < len(b.tokens) && b.tokens[i].Span.Start.Offset == offset {
		return b.tokens[i].Leading
	}

	return nil
}

func isCommentTrivia(k parser.TriviaKind) bool {
	return k == parser.TriviaLineComment || k == parser.TriviaBlockComment || k == parser.TriviaDocComment
}

// commentOffsets returns every comment's source offset, ascending (sorted by
// construction, since tokens and their trivia are already in source order).
func commentOffsets(tokens []parser.Token) []int {
	var offsets []int

	for i := range tokens {
		for _, tr := range tokens[i].Leading {
			if isCommentTrivia(tr.Kind) {
				offsets = append(offsets, tr.Span.Start.Offset)
			}
		}
	}

	return offsets
}

// spanHasComment reports whether a comment sits strictly inside span (after its
// first token, before its last); a leading comment at the span start does not
// count (the enclosing sequence handles it).
func (b *builder) spanHasComment(span parser.Span) bool {
	lo, hi := span.Start.Offset, span.End.Offset

	i := sort.Search(len(b.comments), func(i int) bool { return b.comments[i] > lo })

	return i < len(b.comments) && b.comments[i] < hi
}

func (b *builder) commentsIn(span parser.Span) int {
	lo, hi := span.Start.Offset, span.End.Offset

	first := sort.Search(len(b.comments), func(i int) bool { return b.comments[i] > lo })
	end := sort.Search(len(b.comments), func(i int) bool { return b.comments[i] >= hi })

	return max(end-first, 0)
}

func commentTrivia(trivia []parser.Trivia) int {
	n := 0

	for _, tr := range trivia {
		if isCommentTrivia(tr.Kind) {
			n++
		}
	}

	return n
}

func (b *builder) spanHasLineComment(span parser.Span) bool {
	lo, hi := span.Start.Offset, span.End.Offset

	for i := sort.Search(len(b.comments), func(i int) bool { return b.comments[i] > lo }); i < len(b.comments) && b.comments[i] < hi; i++ {
		if text := b.src[b.comments[i]:]; bytes.HasPrefix(text, []byte("//")) || text[0] == '#' {
			return true
		}
	}

	return false
}

// spanDroppedComment reports whether a comment strictly inside span was never
// emitted while the span was built, so printing that build would delete it.
func (b *builder) spanDroppedComment(span parser.Span) bool {
	lo, hi := span.Start.Offset, span.End.Offset

	// A span widened to its leading comments (withLeadingComments) starts at
	// one, which must count too.
	i := sort.Search(len(b.comments), func(i int) bool { return b.comments[i] >= lo })

	for ; i < len(b.comments) && b.comments[i] < hi; i++ {
		if !b.emitted[b.comments[i]] && !b.placed[b.comments[i]] {
			return true
		}
	}

	return false
}

func (b *builder) place(offset int) {
	if b.placed == nil {
		b.placed = make(map[int]bool)
	}

	b.placed[offset] = true
}

func (b *builder) placeRange(lo, hi int) {
	i := sort.Search(len(b.comments), func(i int) bool { return b.comments[i] >= lo })

	for ; i < len(b.comments) && b.comments[i] < hi; i++ {
		b.place(b.comments[i])
	}
}

// keepComments returns doc, the layout built for span, unless building it left
// a comment behind; then the span goes out as written. A comment on a token
// the layout prints as fixed text (a list's `,`, a union's `|`) has no reader
// otherwise. Checked per statement and class member, so the fallback covers
// the smallest unit that lost one.
//
// node is the statement or member the span belongs to, or nil. A `?>` or the
// end of a fragment can stand in for its `;`, which the parser then only
// synthesizes, so the verbatim span gets one written after it.
func (b *builder) keepComments(span parser.Span, doc *document.Doc, node any) *document.Doc {
	if !b.spanDroppedComment(span) {
		return doc
	}

	// A declare with a body has no terminator at all, only a zero token.
	if semi, ok := terminator(node); ok && has(semi) && semi.Span.Start.Offset == semi.Span.End.Offset {
		return document.Concat(b.verbatim(span), document.Text(";"))
	}

	return b.verbatim(span)
}

func terminator(node any) (parser.Token, bool) {
	switch n := node.(type) {
	case *parser.ExpressionStmt:
		return n.Semicolon, true
	case *parser.EchoStmt:
		return n.Semicolon, true
	case *parser.ReturnStmt:
		return n.Semicolon, true
	case *parser.BreakStmt:
		return n.Semicolon, true
	case *parser.ContinueStmt:
		return n.Semicolon, true
	case *parser.GlobalStmt:
		return n.Semicolon, true
	case *parser.StaticVarStmt:
		return n.Semicolon, true
	case *parser.UnsetStmt:
		return n.Semicolon, true
	case *parser.ConstStmt:
		return n.Semicolon, true
	case *parser.UseStmt:
		return n.Semicolon, true
	case *parser.GotoStmt:
		return n.Semicolon, true
	case *parser.DoWhileStmt:
		return n.Semicolon, true
	case *parser.DeclareStmt:
		return n.Semicolon, true
	case *parser.NamespaceStmt:
		return n.Semicolon, true
	case *parser.ShortEchoStmt:
		return n.Semicolon, true
	case *parser.PropertyDecl:
		// A hooked property ends at its `}`; it has no `;` to restore.
		return n.Semicolon, len(n.Hooks) == 0
	case *parser.ClassConstDecl:
		return n.Semicolon, true
	case *parser.EnumCase:
		return n.Semicolon, true
	case *parser.TraitUseStmt:
		return n.Semicolon, true
	}

	return parser.Token{}, false
}

// tagOffsets returns the start offset of every template-boundary token (?>,
// <?php, <?=, inline HTML). A statement whose span encloses one is a template
// region.
func tagOffsets(tokens []parser.Token) []int {
	var offsets []int

	for i := range tokens {
		switch tokens[i].Kind {
		case parser.KindCloseTag, parser.KindOpenTag, parser.KindOpenTagEcho, parser.KindInlineHTML:
			offsets = append(offsets, tokens[i].Span.Start.Offset)
		}
	}

	return offsets
}

// spanHasInlineHTML reports whether a template boundary sits strictly inside span
// (after its first token, before its last). The file-opening `<?php` and a
// trailing `?>` fall outside every statement span, so they never match.
func (b *builder) spanHasInlineHTML(span parser.Span) bool {
	lo, hi := span.Start.Offset, span.End.Offset

	i := sort.Search(len(b.htmlTags), func(i int) bool { return b.htmlTags[i] > lo })

	return i < len(b.htmlTags) && b.htmlTags[i] < hi
}

// crLiteral is the source range of a string or heredoc/nowdoc token whose bytes
// carry a CR. Those CRs are the string's value; a CR anywhere else in the file is
// a line ending like any other, so the range is what must be left alone, not the
// slice that happens to contain it.
type crLiteral struct {
	start, end int
}

func crLiteralOffsets(tokens []parser.Token) []crLiteral {
	var literals []crLiteral

	for i := range tokens {
		switch tokens[i].Kind {
		case parser.KindString, parser.KindShellExec:
			if bytes.IndexByte(tokens[i].Text, '\r') >= 0 {
				span := tokens[i].Span
				literals = append(literals, crLiteral{span.Start.Offset, span.End.Offset})
			}
		}
	}

	return literals
}

func (b *builder) spanHasCRLiteral(span parser.Span) bool {
	return b.rangeHasCRLiteral(span.Start.Offset, span.End.Offset)
}

func (b *builder) rangeHasCRLiteral(lo, hi int) bool {
	i := sort.Search(len(b.crLiterals), func(i int) bool { return b.crLiterals[i].start >= lo })

	return i < len(b.crLiterals) && b.crLiterals[i].start < hi
}

func (b *builder) tokenHasComment(offset int) bool {
	for _, tr := range b.leadingAt(offset) {
		if isCommentTrivia(tr.Kind) {
			return true
		}
	}

	return false
}

// splitComments returns the comment docs at a token offset, partitioned into the
// trailing comment (before the first line break, belonging to the previous line)
// and the leading comments (on their own lines above the node).
func (b *builder) splitComments(offset int) (lead, trail []*document.Doc) {
	seenNewline := b.useRunTailMoved[offset]
	newlines := 0

	for _, tr := range b.leadingAt(offset) {
		switch tr.Kind {
		case parser.TriviaWhitespace:
			n := triviaLineBreaks(tr.Text)
			if n > 0 {
				seenNewline = true
			}

			newlines += n
		case parser.TriviaLineComment, parser.TriviaBlockComment, parser.TriviaDocComment:
			if b.emitted[tr.Span.Start.Offset] || b.movedTrail[tr.Span.Start.Offset] {
				continue
			}

			if seenNewline {
				lead = b.appendOwnLineComment(lead, tr, newlines)
			} else {
				trail = append(trail, b.trailingComment(tr))
			}

			newlines = 0
		}
	}

	return lead, trail
}

func (b *builder) appendOwnLineComment(lead []*document.Doc, tr parser.Trivia, newlines int) []*document.Doc {
	if len(lead) == 0 || newlines > 0 {
		return append(lead, b.leadCommentWithGap(tr, len(lead) > 0, newlines))
	}

	last := len(lead) - 1
	lead[last] = document.Concat(lead[last], b.trailingComment(tr))

	return lead
}

func (b *builder) trailingComment(tr parser.Trivia) *document.Doc {
	// Each authored group keeps its own scope: two groups the output happens to
	// put on adjacent lines would otherwise pad each other.
	group := b.alignedCommentGroup(tr.Span.Start.Offset)
	gap := document.AlignGap(b.commentScope(group), 0)

	return document.Concat(gap, b.commentVerbatim(tr))
}

func (b *builder) alignedCommentGroup(offset int) int {
	start := b.lineStart(offset)
	column := b.sourceColumn(start, offset)
	first := offset

	for {
		prevStart, prevEnd, found := b.prevLine(start)
		if !found {
			break
		}

		at, aligned := b.trailingCommentAt(prevStart, prevEnd, column)
		if !aligned {
			break
		}

		first, start = at, prevStart
	}

	return first
}

func (b *builder) trailingCommentAt(start, end, column int) (int, bool) {
	i := sort.SearchInts(b.comments, start)

	for ; i < len(b.comments) && b.comments[i] < end; i++ {
		at := b.comments[i]
		if b.sourceColumn(start, at) == column && len(bytes.TrimSpace(b.src[start:at])) > 0 {
			return at, true
		}
	}

	return 0, false
}

func (b *builder) prevLine(start int) (int, int, bool) {
	if start == 0 {
		return 0, 0, false
	}

	end := start - 1
	if end > 0 && b.src[end] == '\n' && b.src[end-1] == '\r' {
		end--
	}

	return b.lineStart(end), end, true
}

// A group is numbered by the order it is first seen, not by its offset, which
// could overflow an int32 scope.
func (b *builder) commentScope(first int) int32 {
	id, ok := b.commentGroups[first]
	if !ok {
		if b.commentGroups == nil {
			b.commentGroups = make(map[int]int32)
		}

		b.commentGroupIDs++
		id = b.commentGroupIDs
		b.commentGroups[first] = id
	}

	return document.CommentScope(id)
}

// A CR inside a token (a string, a heredoc) or a block comment prints as a
// byte, not a line break, so it does not start a line here either.
func (b *builder) lineStart(offset int) int {
	for offset > 0 {
		c := b.src[offset-1]
		if c == '\n' || c == '\r' && !b.insideToken(offset-1) && !b.insideBlockComment(offset-1) {
			break
		}

		offset--
	}

	return offset
}

func (b *builder) insideToken(offset int) bool {
	i := sort.Search(len(b.tokens), func(i int) bool { return b.tokens[i].Span.End.Offset > offset })

	return i < len(b.tokens) && b.tokens[i].Span.Start.Offset <= offset
}

func (b *builder) insideBlockComment(offset int) bool {
	i := sort.SearchInts(b.comments, offset)
	if i == 0 {
		return false
	}

	start := b.comments[i-1]
	if !bytes.HasPrefix(b.src[start:], []byte("/*")) {
		return false
	}

	return !bytes.Contains(b.src[start+2:offset], []byte("*/"))
}

// Measured as the printer measures, or the group the author aligned and the
// group the printer aligns differ between passes.
func (b *builder) sourceColumn(start, offset int) int {
	prefix := document.Text(string(b.src[start:offset]))

	column := document.FlatWidth(prefix, b.cfg.TabWidth)
	if start == 0 {
		column += b.firstLineColumn
	}

	return column
}

// triviaLineBreaks counts the line terminators in inter-token whitespace. A
// lone CR is one, since PHP ends a line at CR as well as LF.
func triviaLineBreaks(text []byte) int {
	n := 0

	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			n++
		case '\r':
			n++

			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
		}
	}

	return n
}

// leadCommentWithGap builds an own-line comment, preserving one authored blank
// line before it when it follows another own-line comment (hasPrev) separated by
// a blank (newlines counts the line breaks since that comment). The blank keeps an
// unrelated commented-out entry from gluing onto the next section's comment; the
// gap before the very first comment is the caller's blankBefore to place.
func (b *builder) leadCommentWithGap(tr parser.Trivia, hasPrev bool, newlines int) *document.Doc {
	c := b.leadComment(tr)

	if hasPrev && newlines >= 2 {
		return document.Concat(document.HardLine(), c)
	}

	return c
}

// leadComment builds the doc for an own-line comment. A docblock carrying a
// breakable type shape is rebuilt so the printer can reflow it; every other
// comment is emitted verbatim.
func (b *builder) leadComment(tr parser.Trivia) *document.Doc {
	b.place(tr.Span.Start.Offset)

	if tr.Kind == parser.TriviaDocComment {
		if d, ok := docCommentDoc(stripLineCR(tr.Text)); ok {
			return d
		}
	}

	return b.commentVerbatim(tr)
}

// commentVerbatim emits a comment's raw text, re-homed so a multi-line block
// comment follows the printer's placement instead of its source columns.
func (b *builder) commentVerbatim(tr parser.Trivia) *document.Doc {
	b.place(tr.Span.Start.Offset)

	text := string(bytes.TrimRight(tr.Text, " \t\r"))

	// A comment's continuation lines are positioned against its own opener,
	// which moves to the current indent, so they move with it. Docblocks
	// already re-home, through docCommentDoc.
	//
	// rehomeVerbatim strips the smallest continuation indent, so the ` * ` run
	// stays aligned; b.rehome cannot be used here because it keeps source columns
	// outside a BuildStmts body, which is right for a frozen statement and wrong
	// for a comment.
	offset := tr.Span.Start.Offset
	base := sourceIndent(b.src, offset)

	// A comment that trails something on its line is printed at a column this base
	// cannot predict: `<?=` gains a space, and an island's line may carry HTML before
	// it. In a mixed file the island's layout also moves a comment between its own
	// line and a shared one, so the two measurements would disagree from one pass to
	// the next. Strip every shared column instead and let the continuation lines land
	// at the indent they are printed at.
	if b.mixed || sourceColumn(b.src, offset) > base {
		base = math.MaxInt
	}

	return rehomeVerbatim(text, base)
}

// stripLineCR drops the carriage returns closing each line, the same cleanup
// verbatim emission applies, so the docblock-shape check above judges the text
// the next pass will actually see.
func stripLineCR(text []byte) []byte {
	if !bytes.ContainsRune(text, '\r') {
		return text
	}

	lines := bytes.Split(text, []byte("\n"))
	for i, ln := range lines {
		lines[i] = bytes.TrimRight(ln, "\r")
	}

	return bytes.Join(lines, []byte("\n"))
}

// emptyListComments returns `(…)` carrying the comments an empty parameter
// list holds, which attach to its closing paren, or nil when it holds none.
func (b *builder) emptyListComments(rparen parser.Token) *document.Doc {
	if rparen.Kind == parser.KindInvalid {
		return nil
	}

	inner := b.allComments(rparen.Span.Start.Offset)
	if len(inner) == 0 {
		return nil
	}

	// A line comment runs to the end of its line, so the `)` cannot follow it;
	// a multi-line block comment takes its own lines for the same reason.
	if b.hasBreakingComment(rparen.Span.Start.Offset) {
		body := make([]*document.Doc, 0, len(inner)*2)
		for _, c := range inner {
			body = append(body, document.HardLine(), c)
		}

		return document.Concat(
			document.Text("("),
			document.Indent(document.Concat(body...)),
			document.HardLine(),
			document.Text(")"),
		)
	}

	parts := make([]*document.Doc, 0, len(inner)*2+2)
	parts = append(parts, document.Text("("))

	for i, c := range inner {
		if i > 0 {
			parts = append(parts, document.Text(" "))
		}

		parts = append(parts, c)
	}

	return document.Concat(append(parts, document.Text(")"))...)
}

// trailingParenComment returns the comments wedged before a closing paren, led
// by their gap from the content, and whether the group must break: a line
// comment, or a block comment spanning lines, takes the rest of its line. A
// lone comment on the content's line aligns with the comments around it.
func (b *builder) trailingParenComment(rparen parser.Token) (*document.Doc, bool) {
	if rparen.Kind == parser.KindInvalid {
		return nil, false
	}

	offset := rparen.Span.Start.Offset

	// Read before allComments, which is what consumes them.
	breaks := b.hasBreakingComment(offset)
	gap := b.closerCommentGap(offset)

	inner := b.allComments(offset)
	if len(inner) == 0 {
		return nil, false
	}

	if len(inner) > 1 {
		gap = document.Text(" ")
	}

	// Two comments cannot share a line when the first takes the rest of it.
	separator := document.Text(" ")
	if breaks {
		separator = document.HardLine()
	}

	parts := make([]*document.Doc, 0, len(inner)*2+1)
	parts = append(parts, gap)

	for i, c := range inner {
		if i > 0 {
			parts = append(parts, separator)
		}

		parts = append(parts, c)
	}

	return document.Concat(parts...), breaks
}

func (b *builder) closerCommentGap(offset int) *document.Doc {
	for _, tr := range b.leadingAt(offset) {
		switch {
		case tr.Kind == parser.TriviaWhitespace && triviaLineBreaks(tr.Text) > 0:
			return document.Text(" ")
		case isCommentTrivia(tr.Kind):
			group := b.alignedCommentGroup(tr.Span.Start.Offset)

			return document.AlignGap(b.commentScope(group), 0)
		}
	}

	return document.Text(" ")
}

func (b *builder) withTrailingParenComment(content *document.Doc, rparen parser.Token) (*document.Doc, bool) {
	comment, breaks := b.trailingParenComment(rparen)
	if comment == nil {
		return content, false
	}

	return document.Concat(content, comment), breaks
}

// parenGroup wraps content in `(…)`, carrying any comment wedged before the
// closing paren. A breaking comment puts the `)` on its own line.
func (b *builder) parenGroup(content *document.Doc, rparen parser.Token) *document.Doc {
	content, breaks := b.withTrailingParenComment(content, rparen)

	if !breaks {
		return document.Concat(document.Text("("), content, document.Text(")"))
	}

	return document.Concat(
		document.Text("("),
		document.Indent(document.Concat(document.HardLine(), content)),
		document.HardLine(),
		document.Text(")"),
	)
}

// hasBreakingComment reports whether an unclaimed comment at offset takes the rest
// of its line: a line comment always does, and a block comment that spans lines
// leaves the cursor on its last one.
func (b *builder) hasBreakingComment(offset int) bool {
	seen := false
	newlines := 0

	for _, tr := range b.leadingAt(offset) {
		if tr.Kind == parser.TriviaWhitespace {
			newlines += triviaLineBreaks(tr.Text)

			continue
		}

		if !isCommentTrivia(tr.Kind) || b.emitted[tr.Span.Start.Offset] {
			continue
		}

		// allComments keeps a blank line between two comments, which only
		// line breaks around them can hold.
		if tr.Kind == parser.TriviaLineComment || bytes.ContainsAny(tr.Text, "\n\r") || seen && newlines >= 2 {
			return true
		}

		seen = true
		newlines = 0
	}

	return false
}

// hasBreakingTrail is hasBreakingComment for the trail mixedComments printed,
// which for the head of a sorted import run includes the run opener's.
func (b *builder) hasBreakingTrail(offset int) bool {
	if before, moved := b.runHeadTrivia(offset); moved && b.hasBreakingComment(before) {
		return true
	}

	return b.hasBreakingComment(offset)
}

// danglingComments returns the braced body for a construct whose only content
// is comments (the leading trivia of its closing brace), or nil when the body
// is truly empty.
func (b *builder) danglingComments(rbrace parser.Token) *document.Doc {
	comments := b.allComments(rbrace.Span.Start.Offset)
	if len(comments) == 0 {
		return nil
	}

	inner := make([]*document.Doc, 0, len(comments)*2)
	for _, c := range comments {
		inner = append(inner, document.HardLine(), c)
	}

	return document.Concat(
		document.Text("{"),
		document.Indent(document.Concat(inner...)),
		document.HardLine(),
		document.Text("}"),
	)
}

// bodyTailComments returns the comments between a body's last element and its
// closing brace, or nil when there are none. They attach to the `}`, so no
// element reproduces them. The placement follows buildSequence: a comment that
// shared the last element's line rides a LineSuffix, and own-line comments
// keep their line and any single authored blank above them.
func (b *builder) bodyTailComments(rbrace parser.Token) *document.Doc {
	offset := rbrace.Span.Start.Offset

	if b.hasIgnoreDirective(offset) {
		return nil
	}

	lead, trail := b.splitComments(offset)
	if len(lead) == 0 && len(trail) == 0 {
		return nil
	}

	parts := make([]*document.Doc, 0, len(lead)*2+2)

	if len(trail) > 0 {
		parts = append(parts, document.LineSuffix(document.Concat(trail...)))
	}

	for i, c := range lead {
		parts = append(parts, document.HardLine())

		if i == 0 && b.blankBefore(offset) {
			parts = append(parts, document.HardLine())
		}

		parts = append(parts, c)
	}

	return document.Concat(parts...)
}

// allComments returns every comment at offset as an own-line doc, in source
// order, for positions where a comment has no element to attach to.
func (b *builder) allComments(offset int) []*document.Doc {
	var out []*document.Doc

	newlines := 0

	for _, tr := range b.leadingAt(offset) {
		switch {
		case tr.Kind == parser.TriviaWhitespace:
			newlines += triviaLineBreaks(tr.Text)
		case isCommentTrivia(tr.Kind):
			if b.emitted[tr.Span.Start.Offset] || b.movedTrail[tr.Span.Start.Offset] {
				continue
			}

			out = b.appendOwnLineComment(out, tr, newlines)
			newlines = 0
		}
	}

	return out
}

// hasIgnoreDirective reports whether the node starting at offset carries a
// laravel-fmt-ignore directive as the last non-whitespace entry in its leading
// trivia, so a comment between the directive and the node breaks the match.
func (b *builder) hasIgnoreDirective(offset int) bool {
	lead := b.leadingAt(offset)

	for _, tr := range slices.Backward(lead) {
		if tr.Kind == parser.TriviaWhitespace {
			continue
		}

		if isCommentTrivia(tr.Kind) {
			return formatter.IsIgnore(tr.Text)
		}

		return false
	}

	return false
}

// frozen emits a directive-frozen node verbatim: from its first leading comment
// (so the laravel-fmt-ignore marker itself is reproduced, keeping a second pass
// idempotent) through the node's source, with trailing whitespace trimmed so the
// block does not grow a blank line across passes. buildSequence skips its own
// comment handling for frozen nodes, so the leading comments are owned here.
func (b *builder) frozen(span parser.Span) *document.Doc {
	start, end := span.Start.Offset, span.End.Offset
	if first, _ := b.frozenCommentStart(start); first >= 0 {
		start = first
	}

	if start < 0 || end > len(b.src) || start >= end {
		return document.Text("")
	}

	content := bytes.TrimRight(b.src[start:end], " \t\r\n")

	// A frozen node reproduces its source, comments included, so it owns every
	// comment in that range; another reader printing one again would duplicate
	// it.
	b.claimRange(start, start+len(content))

	return b.rehome(string(content), start)
}

// frozenCommentStart is the offset where a frozen node's verbatim text begins,
// or -1 without a comment. A directive on its own line leaves the comments
// before that line to trail the previous node (trails reports it); a directive
// on the previous node's line moves down with the frozen node.
func (b *builder) frozenCommentStart(offset int) (start int, trails bool) {
	first, ownLine := -1, -1
	seenNewline := false

	for _, tr := range b.leadingAt(offset) {
		switch {
		case tr.Kind == parser.TriviaWhitespace && triviaLineBreaks(tr.Text) > 0:
			seenNewline = true
		case isCommentTrivia(tr.Kind) && first < 0:
			first = tr.Span.Start.Offset
		}

		if isCommentTrivia(tr.Kind) && seenNewline && ownLine < 0 {
			ownLine = tr.Span.Start.Offset
		}
	}

	if ownLine >= 0 && b.trailsNode(first) {
		return ownLine, ownLine != first
	}

	return first, false
}

func (b *builder) trailsNode(offset int) bool {
	before := bytes.TrimRight(b.src[b.lineStart(offset):offset], " \t")
	tag := []byte("<?php")

	if len(before) >= len(tag) && bytes.EqualFold(before[len(before)-len(tag):], tag) {
		return false
	}

	return len(before) > 0
}

func (b *builder) claimRange(lo, hi int) {
	i := sort.Search(len(b.comments), func(i int) bool { return b.comments[i] >= lo })

	for ; i < len(b.comments) && b.comments[i] < hi; i++ {
		if b.emitted == nil {
			b.emitted = make(map[int]bool)
		}

		b.emitted[b.comments[i]] = true
	}
}

func (b *builder) hasLeadingNewline(offset int) bool {
	for _, tr := range b.leadingAt(offset) {
		if tr.Kind == parser.TriviaWhitespace && triviaLineBreaks(tr.Text) > 0 {
			return true
		}
	}

	return false
}

// authorBroke reports whether the author put the list item at offset on its
// own line. The broken layout reproduces that newline, so keeping a list
// broken on it is a stable fixed point.
func (b *builder) authorBroke(offset int) bool {
	return !b.noAuthorBreaks && b.hasLeadingNewline(offset)
}

// blankBefore reports whether the author left a blank line before the node at
// offset, measured by counting whitespace line breaks in its leading trivia
// rather than absolute line numbers (which shift when a multi-line comment moves
// to its own line). A trailing comment, which belongs to the previous line,
// resets the count so the gap is measured from after it.
func (b *builder) blankBefore(offset int) bool {
	newlines := 0
	seenNewline := false

	for _, tr := range b.leadingAt(offset) {
		switch tr.Kind {
		case parser.TriviaWhitespace:
			newlines += triviaLineBreaks(tr.Text)
			if newlines > 0 {
				seenNewline = true
			}
		default:
			if seenNewline {
				return newlines >= 2
			}

			newlines = 0
		}
	}

	return newlines >= 2
}

// alignBreaks reports, per entry offset, whether an authored blank line or an
// own-line comment sits before the entry, either of which ends an alignment
// run in the output.
func (b *builder) alignBreaks(offsets []int) []bool {
	breaks := make([]bool, len(offsets))

	for i, offset := range offsets {
		if i == 0 {
			continue
		}

		breaks[i] = b.blankBefore(offset) || b.ownLineCommentBefore(offset)
	}

	return breaks
}

func (b *builder) ownLineCommentBefore(offset int) bool {
	seenNewline := false

	for _, tr := range b.leadingAt(offset) {
		switch {
		case tr.Kind == parser.TriviaWhitespace:
			seenNewline = seenNewline || triviaLineBreaks(tr.Text) > 0
		case isCommentTrivia(tr.Kind) && seenNewline:
			return true
		}
	}

	return false
}

func (b *builder) blankAfterComments(offset int) bool {
	newlines := 0
	seenComment := false

	for _, tr := range b.leadingAt(offset) {
		if tr.Kind == parser.TriviaWhitespace {
			newlines += triviaLineBreaks(tr.Text)

			continue
		}

		seenComment = true
		newlines = 0
	}

	return seenComment && newlines >= 2
}

// buildSequence joins item docs with hard line breaks, preserving one blank line
// where the author left one and attaching each item's leading and trailing
// comments. forceBlank, when non-nil, marks items that must be preceded by a
// blank line regardless of author spacing (the canonical import/symbol/member
// separation); index 0 is ignored. leadingBreak controls whether the first item
// is preceded by a hard line (true inside a brace body, false right after the
// file header).
func (b *builder) buildSequence(spans []parser.Span, docs []*document.Doc, forceBlank []bool, leadingBreak bool) *document.Doc {
	parts := make([]*document.Doc, 0, len(docs)*4)

	// The pieces of a split `use A, B;` share the original's span, so its
	// comments go out once, with the first piece printed.
	var emitted map[int]bool

	for i := range docs {
		offset := spans[i].Start.Offset
		repeat := emitted[offset]

		if !repeat && i+1 < len(spans) && slices.ContainsFunc(spans[i+1:], func(s parser.Span) bool { return s.Start.Offset == offset }) {
			if emitted == nil {
				emitted = make(map[int]bool)
			}

			emitted[offset] = true
		}

		before, moved := b.runHeadTrivia(offset)

		// A directive-frozen node reproduces its own leading comments (the
		// laravel-fmt-ignore marker) verbatim, so the sequence must not also emit
		// them; only the blank-line separator between nodes is handled here.
		var lead, trail []*document.Doc
		switch {
		case repeat, b.useRunHeadMoved[offset]:
		case b.hasIgnoreDirective(offset):
			if _, trails := b.frozenCommentStart(offset); trails {
				_, trail = b.splitComments(before)
			}
		case i == 0 && !leadingBreak:
			// The first node has no preceding line for a trailing comment to ride,
			// so every comment is a lead on its own line (clean text, so a comment
			// at the fragment start is neither dropped nor space-prefixed).
			lead = b.allComments(before)
			if moved {
				lead = append(lead, b.allComments(offset)...)
			}
		default:
			lead, trail = b.splitComments(before)
			if moved {
				ownLead, ownTrail := b.splitComments(offset)
				lead, trail = append(lead, ownLead...), append(trail, ownTrail...)
			}
		}

		switch {
		case i > 0:
			if len(trail) > 0 {
				parts = append(parts, document.LineSuffix(document.Concat(trail...)))
			}

			parts = append(parts, document.HardLine())

			forced := i < len(forceBlank) && forceBlank[i]

			// A reordered import's author blank-before no longer applies (it
			// pointed at a different statement); the kind-group separators from
			// forceBlank still do.
			authored := !b.useRunInterior[offset] && b.blankBefore(before)

			if forced || authored {
				parts = append(parts, document.HardLine())
			}
		case leadingBreak:
			if len(trail) > 0 {
				parts = append(parts, document.LineSuffix(document.Concat(trail...)))
			}

			parts = append(parts, document.HardLine())
		}

		for _, c := range lead {
			parts = append(parts, c, document.HardLine())
		}

		if len(lead) > 0 && b.blankAfterComments(before) {
			parts = append(parts, document.HardLine())
		}

		parts = append(parts, docs[i])
	}

	return document.Concat(parts...)
}

// stmtForceBlank marks, for each statement, whether a blank line must precede it
// regardless of author spacing: the canonical PER-CS/Pint separation around the
// file header, import groups, and top-level symbols, plus the configured
// blank_line_before_statement kinds. Entry i governs the gap between stmts[i-1]
// and stmts[i]; entry 0 is always false.
func (b *builder) stmtForceBlank(stmts []parser.Stmt) []bool {
	force := make([]bool, len(stmts))

	for i := 1; i < len(stmts); i++ {
		force[i] = b.forceBlankBetween(stmts[i-1], stmts[i])
	}

	return force
}

// forceBlankBetween reports whether a blank line must separate prev and next
// regardless of author spacing: a configured blank_line_before_statement kind, a
// change of import kind, or (in a pure-PHP file) the PER-CS header/symbol
// spacing.
func (b *builder) forceBlankBetween(prev, next parser.Stmt) bool {
	kind := blankLineStmtKind(next)
	byConfig := kind != "" && slices.Contains(b.cfg.PHP.BlankLineBeforeStatements, kind)

	return byConfig || differentUseKinds(prev, next) ||
		(b.headerSpacing && requestsHeaderBlank(prev, next))
}

// memberForceBlank marks, for each class member, whether a blank line must
// precede it: after every method and between members of different kinds, the
// PER-CS/Laravel separation. A run of the same non-method kind stays grouped.
func (b *builder) memberForceBlank(members []parser.ClassMember) []bool {
	force := make([]bool, len(members))

	for i := 1; i < len(members); i++ {
		prevKind := classMemberKind(members[i-1])
		force[i] = prevKind == memberKindMethod || prevKind != classMemberKind(members[i])
	}

	return force
}

// eofToken returns the file's EOF token, whose leading trivia holds any comment
// written after the last statement.
func eofToken(tokens []parser.Token) parser.Token {
	for _, token := range slices.Backward(tokens) {
		if token.Kind == parser.KindEOF {
			return token
		}
	}

	return parser.Token{}
}

// fileTailComments returns the comments written after the last statement,
// which attach to EOF. When the body emitted nothing, the file header has
// already placed the blank line above them.
func (b *builder) fileTailComments(eof parser.Token, bodyEmitted bool) *document.Doc {
	offset := eof.Span.Start.Offset

	// With no body there is no preceding line for a trailing comment to ride,
	// so every comment is an own-line one.
	if !bodyEmitted {
		lead := b.allComments(offset)
		if len(lead) == 0 {
			return nil
		}

		parts := make([]*document.Doc, 0, len(lead)*2)

		for i, c := range lead {
			if i > 0 {
				parts = append(parts, document.HardLine())
			}

			parts = append(parts, c)
		}

		return document.Concat(parts...)
	}

	lead, trail := b.splitComments(offset)
	if len(lead) == 0 && len(trail) == 0 {
		return nil
	}

	parts := make([]*document.Doc, 0, len(lead)*2+3)

	if len(trail) > 0 {
		parts = append(parts, document.LineSuffix(document.Concat(trail...)))
	}

	if len(lead) > 0 && b.blankBefore(offset) {
		parts = append(parts, document.HardLine())
	}

	for i, c := range lead {
		if i > 0 || bodyEmitted {
			parts = append(parts, document.HardLine())
		}

		parts = append(parts, c)
	}

	return document.Concat(parts...)
}

// mixedComments returns the docs for the comments preceding a mixed-file statement:
// one that shared the previous line rides a LineSuffix and stays there, and each
// own-line comment takes its own line above the statement. The separator before the
// statement has already reproduced the source gap, so only the comments themselves
// add breaks here.
func (b *builder) mixedComments(offset int) (trailDocs, leadDocs []*document.Doc) {
	// A directive-frozen node reproduces its own leading comments, the marker
	// included, so they must not be printed here as well; only those trailing
	// the previous statement are left to this caller (frozenCommentStart).
	if b.useRunHeadMoved[offset] {
		return nil, nil
	}

	if b.hasIgnoreDirective(offset) {
		if _, trails := b.frozenCommentStart(offset); trails {
			_, trailDocs = b.splitComments(offset)
		}

		return trailDocs, nil
	}

	lead, trail := b.splitComments(offset)

	if before, moved := b.runHeadTrivia(offset); moved {
		headLead, headTrail := b.splitComments(before)
		lead, trail = append(headLead, lead...), append(headTrail, trail...)
	}

	// The trail goes out where it was written, not on a LineSuffix. These docs are
	// already queued ahead of the separator, so they close the previous statement's
	// line either way, while a LineSuffix defers them to the end of the printed line:
	// past the `?>` that shared it, where the comment's text renders on the page, and
	// past the statement the comment was written in front of.
	trailDocs = append(trailDocs, trail...)

	for _, c := range lead {
		leadDocs = append(leadDocs, c, document.HardLine())
	}

	return trailDocs, leadDocs
}

// measureWidth runs a throwaway width probe. Its doc never reaches the output,
// so no comment may be claimed inside it, or the real emitter would see that
// comment as placed and drop it. Not every emitter checks b.measuring, so the
// claims the probe made are given back as well.
func (b *builder) measureWidth(fn func() int) int {
	saved := b.measuring
	mark := b.claimMark()
	b.measuring = true
	width := fn()
	b.measuring = saved
	b.releaseClaims(mark)

	return width
}

func blankLineStmtKind(stmt parser.Stmt) string {
	switch s := stmt.(type) {
	case *parser.ReturnStmt:
		return config.StmtReturn
	case *parser.BreakStmt:
		return config.StmtBreak
	case *parser.ContinueStmt:
		return config.StmtContinue
	case *parser.DeclareStmt:
		return config.StmtDeclare
	case *parser.DoWhileStmt:
		return config.StmtDo
	case *parser.ForStmt:
		return config.StmtFor
	case *parser.ForEachStmt:
		return config.StmtForeach
	case *parser.GotoStmt:
		return config.StmtGoto
	case *parser.IfStmt:
		return config.StmtIf
	case *parser.SwitchStmt:
		return config.StmtSwitch
	case *parser.TryStmt:
		return config.StmtTry
	case *parser.WhileStmt:
		return config.StmtWhile
	case *parser.ExpressionStmt:
		return blankLineExprKind(s.Expr)
	}

	return ""
}

func blankLineExprKind(expr parser.Expr) string {
	switch e := expr.(type) {
	case *parser.ThrowExpr:
		return config.StmtThrow
	case *parser.YieldExpr:
		return config.StmtYield
	case *parser.YieldFromExpr:
		return config.StmtYieldFrom
	case *parser.IncludeExpr:
		switch e.Keyword.Kind {
		case parser.KindInclude:
			return config.StmtInclude
		case parser.KindIncludeOnce:
			return config.StmtIncludeOnce
		case parser.KindRequire:
			return config.StmtRequire
		case parser.KindRequireOnce:
			return config.StmtRequireOnce
		}
	}

	return ""
}

func requestsHeaderBlank(prev, next parser.Stmt) bool {
	switch p := prev.(type) {
	case *parser.TagStmt:
		return p.Token.Kind == parser.KindOpenTag
	case *parser.DeclareStmt:
		return true
	case *parser.NamespaceStmt:
		return p.Body == nil
	case *parser.UseStmt:
		_, nextUse := next.(*parser.UseStmt)

		return !nextUse
	}

	return isTopLevelSymbol(prev)
}

func isTopLevelSymbol(stmt parser.Stmt) bool {
	switch stmt.(type) {
	case *parser.FunctionDecl, *parser.ClassStmt, *parser.InterfaceStmt, *parser.TraitStmt, *parser.EnumStmt:
		return true
	}

	return false
}

// differentUseKinds reports whether prev and next are use statements of
// different import kinds (class vs `function` vs `const`). The blank between
// kinds is the visible counterpart of sortUseStmts' grouping, following
// php-cs-fixer's blank_line_between_import_groups.
func differentUseKinds(prev, next parser.Stmt) bool {
	pu, ok := prev.(*parser.UseStmt)
	if !ok {
		return false
	}

	nu, ok := next.(*parser.UseStmt)
	if !ok {
		return false
	}

	return pu.UseKind.Kind != nu.UseKind.Kind
}

// memberKind enumerates the class-member categories the blank-line rule
// distinguishes. The zero value memberKindOther covers any future member type,
// so it is never grouped with a known kind.
type memberKind int

const (
	memberKindOther memberKind = iota
	memberKindConst
	memberKindProperty
	memberKindMethod
	memberKindTraitUse
	memberKindEnumCase
)

func classMemberKind(m parser.ClassMember) memberKind {
	switch m.(type) {
	case *parser.ClassConstDecl:
		return memberKindConst
	case *parser.PropertyDecl:
		return memberKindProperty
	case *parser.MethodDecl:
		return memberKindMethod
	case *parser.TraitUseStmt:
		return memberKindTraitUse
	case *parser.EnumCase:
		return memberKindEnumCase
	}

	return memberKindOther
}
