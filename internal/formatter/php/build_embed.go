package php

// Doc-based entry points for PHP embedded in another language (Blade echoes and
// directives, bound attributes, `@php` bodies). Unlike BuildFile these receive a
// fragment, not a whole file: src and the tokens the host's parser lexed it into,
// which carry the leading trivia and comment offsets the doc builder needs.

import (
	"bytes"
	"slices"
	"sort"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// inlineWidth is an effectively unbounded print width, for output laid out on
// one line (an embedded expression, a mixed file's PHP island), so no group
// breaks on width.
const inlineWidth = 1 << 30

// BuildExpr lays out a single PHP expression parsed from src, whose tokens are
// the ones the parse lexed, on one line, for bound attributes. Width never
// breaks it, but a break the author kept (a multi-line array) does, so its
// lines are indented from indentCols, the host's indent.
func BuildExpr(src []byte, tokens []parser.Token, expr parser.Expr, cfg *config.File, indentCols int) []byte {
	return buildInline(src, tokens, expr, cfg, indentCols, false)
}

// BuildEchoExpr is BuildExpr for a Blade echo, whose expression is the
// statement Blade compiles it to, so a chain the author broke keeps its lines.
func BuildEchoExpr(src []byte, tokens []parser.Token, expr parser.Expr, cfg *config.File, indentCols int) []byte {
	return buildInline(src, tokens, expr, cfg, indentCols, true)
}

func buildInline(src []byte, tokens []parser.Token, expr parser.Expr, cfg *config.File, indentCols int, stmt bool) []byte {
	if expr == nil {
		return nil
	}

	build := func(b *builder) *document.Doc {
		if stmt {
			return b.stmtExpr(expr, 0, 0)
		}

		return b.expr(expr)
	}

	// The indent only places the lines a kept break makes, and most expressions
	// have none, so they skip the indent's cost.
	out := buildWidth(src, tokens, cfg, 0, inlineWidth, 0, expr.Span(), build)
	if indentCols == 0 || bytes.IndexByte(out, '\n') < 0 {
		return out
	}

	return buildWidth(src, tokens, cfg, indentCols, inlineWidth, indentCols, expr.Span(), build)
}

// BuildDirectiveArg lays a directive argument out against the print width at the
// host's indent. The width applies whether or not the author broke the argument,
// so both routes to a multi-line form pick the same layout (idempotent). srcCol
// is the file column src starts at. closer is the width the host writes after
// the argument, reserved on its last line: counted into the start column
// instead, it would shift the first line's columns and the comments aligned on
// them.
func BuildDirectiveArg(src []byte, tokens []parser.Token, expr parser.Expr, cfg *config.File, indentCols, startCol, srcCol, closer int) []byte {
	if expr == nil || closer == 0 {
		return buildExprWidth(src, tokens, expr, cfg, indentCols, cfg.PrintWidth, startCol, srcCol)
	}

	// A private-use rune stands in for the closer: one column wide, and never
	// in source, so a layout that dropped it (kept verbatim) cannot pass for
	// one that ends in a real `)`.
	tail := strings.Repeat("", closer)

	out := buildWidth(src, tokens, cfg, indentCols, cfg.PrintWidth, startCol, expr.Span(), func(b *builder) *document.Doc {
		b.firstLineColumn = srcCol

		return document.Concat(b.expr(expr), document.Text(tail))
	})

	// A comment kept after the argument prints past the reservation.
	if !bytes.HasSuffix(out, []byte(tail)) {
		return buildExprWidth(src, tokens, expr, cfg, indentCols, cfg.PrintWidth, startCol+closer, srcCol)
	}

	return bytes.TrimSuffix(out, []byte(tail))
}

// BuildStmtDirectiveArg lays out an inline @php(…) argument as a statement's
// value, so it breaks as in an @php block.
func BuildStmtDirectiveArg(src []byte, tokens []parser.Token, expr parser.Expr, cfg *config.File, indentCols, startCol int) []byte {
	if expr == nil {
		return nil
	}

	lead := startCol - indentCols

	return buildWidth(src, tokens, cfg, indentCols, cfg.PrintWidth, startCol, expr.Span(), func(b *builder) *document.Doc {
		return b.stmtExpr(expr, lead, len(")"))
	})
}

// TrailingArgComment returns the comment text between a directive's last
// argument and its closing paren, and whether a line comment among them takes
// the rest of its line, which forces the `)` onto the next one.
//
// Everything past the last argument is trivia, since the parser consumed the
// expression. The range is returned rather than the bytes so a host can
// re-home a multi-line region against its own indent.
func TrailingArgComment(src []byte, tokens []parser.Token, lastEnd int) (start, end int, breaks bool) {
	if lastEnd < 0 || lastEnd >= len(src) {
		return 0, 0, false
	}

	region := src[lastEnd:]

	lead := len(region) - len(bytes.TrimLeft(region, " \t\r\n\f\v"))

	trimmed := bytes.TrimSpace(region)
	if len(trimmed) == 0 {
		return 0, 0, false
	}

	start = lastEnd + lead
	end = start + len(trimmed)

	return start, end, EndsInLineComment(tokens, end)
}

// SeparatorComment reports whether a comment sits on a comma after one of args
// (`$a /* note */, $b`). A host builds each argument on its own, so such a
// comment is in no argument's span and would be dropped; the host keeps the
// list as written instead.
func SeparatorComment(tokens []parser.Token, args []parser.Expr) bool {
	for _, arg := range args {
		end := arg.Span().End.Offset
		i := sort.Search(len(tokens), func(i int) bool { return tokens[i].Span.Start.Offset >= end })

		if i == len(tokens) || tokens[i].Kind != parser.KindComma {
			continue
		}

		for _, tr := range tokens[i].Leading {
			if isCommentTrivia(tr.Kind) {
				return true
			}
		}
	}

	return false
}

// EndsInLineComment reports whether a line comment is still open at offset end
// of the source tokens were lexed from.
func EndsInLineComment(tokens []parser.Token, end int) bool {
	if len(tokens) == 0 {
		return false
	}

	return lineCommentOpen(tokens[len(tokens)-1].Leading, end)
}

// LoopClauseEndsInLineComment reports whether the clause BuildLoopClause lays
// out for stmt ends in a line comment, which it keeps last, so the `)` has to
// start the next line.
func LoopClauseEndsInLineComment(src []byte, stmt parser.Stmt) bool {
	build, lparen, rparen := loopClause(stmt)
	if build == nil {
		return false
	}

	span := loopClauseSpan(src, lparen, rparen)

	return lineCommentOpen(rparen.Leading, span.End.Offset)
}

func lineCommentOpen(trivia []parser.Trivia, end int) bool {
	inLine := false

	for _, t := range trivia {
		if t.Span.Start.Offset >= end {
			break
		}

		switch t.Kind {
		case parser.TriviaLineComment:
			inLine = true
		case parser.TriviaBlockComment, parser.TriviaDocComment:
			inLine = false
		case parser.TriviaWhitespace:
			if bytes.ContainsAny(t.Text, "\n\r") {
				inLine = false
			}
		}
	}

	return inLine
}

// HasMultilineStringLiteral reports whether tokens hold a string, heredoc, or
// nowdoc literal whose value carries a line terminator. Re-homing such source under a
// new indent would rewrite the bytes after each one, which are the string's own
// value, so a host must leave an argument that carries one verbatim. The scan runs
// over the lexer's tokens rather than the typed AST so a terminator hidden inside any
// expression shape (an assignment, an array, a match arm) still trips it.
//
// A lone CR counts, since the re-home rewrites it into a newline plus indent
// just as it does an LF.
func HasMultilineStringLiteral(tokens []parser.Token) bool {
	return slices.ContainsFunc(tokens, func(t parser.Token) bool {
		return t.Kind == parser.KindString && bytes.ContainsAny(t.Text, "\n\r")
	})
}

// HasCRStringLiteral reports whether tokens hold a string literal carrying a
// raw carriage return. The host re-emits built bytes line by line and strips
// CRs as line endings, which would delete that CR from the value; a line feed
// alone is safe.
func HasCRStringLiteral(tokens []parser.Token) bool {
	return slices.ContainsFunc(tokens, func(t parser.Token) bool {
		return t.Kind == parser.KindString && bytes.IndexByte(t.Text, '\r') >= 0
	})
}

// ExprHasBlockForm reports whether expr's multi-line form is an array or match body.
func ExprHasBlockForm(expr parser.Expr) bool {
	switch v := expr.(type) {
	case *parser.ArrayExpr:
		return arrayHasValue(v.Elements)
	case *parser.MatchExpr:
		return true
	}

	return false
}

// DirectiveArgFlatWidth is expr's one-line width, ignoring author breaks, so the
// layout a caller picks from it does not flip once the formatter breaks the arg.
func DirectiveArgFlatWidth(src []byte, tokens []parser.Token, expr parser.Expr, cfg *config.File) int {
	if expr == nil {
		return 0
	}

	b := &builder{src: src, cfg: cfg, tokens: tokens, comments: commentOffsets(tokens), crLiterals: crLiteralOffsets(tokens), embed: true, noAuthorBreaks: true}

	return document.FlatWidth(b.expr(expr), cfg.TabWidth)
}

// BuildDirectiveArgWrapped lays an over-wide argument out against the width left at
// indentCols and returns it at column zero, for the host to re-home under the opener.
func BuildDirectiveArgWrapped(src []byte, tokens []parser.Token, expr parser.Expr, cfg *config.File, indentCols int) []byte {
	if expr == nil {
		return nil
	}

	// rehomeSlices: the host re-homes this output under its own indent
	// (EmitVerbatim in blade), so a verbatim slice must drop its source columns
	// or they compound one indent per pass. BuildDirectiveArg is the opposite
	// case: its host writes the bytes as they are, so its slices keep them.
	b := &builder{
		src: src, cfg: columnZeroConfig(cfg, indentCols), tokens: tokens, comments: commentOffsets(tokens),
		crLiterals:   crLiteralOffsets(tokens),
		rehomeSlices: true, embed: true, noAuthorBreaks: true,
	}
	b.depth = indentCols / max(cfg.TabWidth, 1)

	// A condition holding comments goes out as written, in one piece: a piece
	// kept verbatim inside a rebuilt one would be re-homed from its own first
	// line, which the next pass finds at a different column.
	span := b.withLeadingComments(expr.Span())
	if b.spanHasComment(span) || span.Start.Offset < expr.Span().Start.Offset {
		return stripLineCR(document.Print(b.verbatim(span), document.Options{
			PrintWidth: max(cfg.PrintWidth-indentCols, 1),
			TabWidth:   cfg.TabWidth,
			UseTabs:    cfg.UseTabs(),
			EOL:        docEOL(cfg),
		}))
	}

	// The host indents the condition under `@if (` the way a PHP header does, so
	// it is built like one, without the binary's own continuation indent.
	doc := b.condition(expr)

	out := document.Print(doc, document.Options{
		PrintWidth: max(cfg.PrintWidth-indentCols, 1),
		TabWidth:   cfg.TabWidth,
		UseTabs:    cfg.UseTabs(),
		EOL:        docEOL(cfg),
	})

	return stripLineCR(out)
}

// SortClassExpr reorders the Tailwind classes inside the string literals of
// a class expression, in place on the parsed node the caller then hands to
// BuildDirectiveArg/BuildExpr. It is the shared engine behind `@class([...])`
// and a bound `:class="..."`. sortClasses is the class sorter (injected so
// this package stays independent of the Tailwind registry). Only positions
// whose value is itself a class list are descended: a bare array element and
// a keyed element's key (the keyed value is its boolean condition, matching
// Laravel `@class`), a ternary's branches, the operands of concatenation
// and short-circuit/null-coalesce operators (`.`, `&&`, `||`, `and`, `or`,
// `??`), and a parenthesized inner expression. A comparison, a function
// argument, and a variable are left alone, so a condition like
// `$type === 'admin'` never has its literal reordered; an interpolated or
// escaped string is skipped too.
func SortClassExpr(expr parser.Expr, sortClasses func(classes string, pinFirst, pinLast bool) string) {
	sortClassExpr(expr, sortClasses, false, false)
}

func sortClassExpr(expr parser.Expr, sortClasses func(string, bool, bool) string, pinFirst, pinLast bool) {
	switch e := expr.(type) {
	case *parser.StringExpr:
		sortClassStringToken(e, sortClasses, pinFirst, pinLast)
	case *parser.ArrayExpr:
		for _, el := range e.Elements {
			switch {
			case el.Key != nil:
				sortClassExpr(el.Key, sortClasses, false, false)
			case el.Value != nil && !el.Spread:
				sortClassExpr(el.Value, sortClasses, false, false)
			}
		}
	case *parser.TernaryExpr:
		sortClassExpr(e.Then, sortClasses, pinFirst, pinLast)
		sortClassExpr(e.Else, sortClasses, pinFirst, pinLast)
	case *parser.ParenExpr:
		sortClassExpr(e.Expr, sortClasses, pinFirst, pinLast)
	case *parser.BinaryExpr:
		if !classOperator(e.Op.Kind) {
			return
		}

		if e.Op.Kind == parser.KindDot {
			// Concatenation joins the operands, but a space on either side of the join
			// ends the fragment: `'base ' . $x` yields a whole class, `'base bg-' . $x`
			// does not. Inherited flags still apply outward, so the middle operand of
			// `a . b . c` can pin both edges.
			glued := !edgeSpace(e.Left, true) && !edgeSpace(e.Right, false)

			sortClassExpr(e.Left, sortClasses, pinFirst, glued)
			sortClassExpr(e.Right, sortClasses, glued, pinLast)

			return
		}

		sortClassExpr(e.Left, sortClasses, pinFirst, pinLast)
		sortClassExpr(e.Right, sortClasses, pinFirst, pinLast)
	}
}

// edgeSpace reports whether expr is known to end (trailing) or begin (leading)
// with whitespace. An unreadable edge answers false, treating the value as glued:
// that only forgoes a reorder, where the opposite would change the rendered class.
func edgeSpace(expr parser.Expr, trailing bool) bool {
	const ws = " \t\r\n\f\v"

	switch e := expr.(type) {
	case *parser.StringExpr:
		body, _, ok := plainStringBody(e.Token.Text)
		if !ok || body == "" {
			return false
		}

		if trailing {
			return strings.ContainsRune(ws, rune(body[len(body)-1]))
		}

		return strings.ContainsRune(ws, rune(body[0]))
	case *parser.ParenExpr:
		return edgeSpace(e.Expr, trailing)
	case *parser.TernaryExpr:
		return edgeSpace(e.Then, trailing) && edgeSpace(e.Else, trailing)
	case *parser.BinaryExpr:
		if !classOperator(e.Op.Kind) {
			return false
		}

		if e.Op.Kind == parser.KindDot {
			if trailing {
				return edgeSpace(e.Right, true)
			}

			return edgeSpace(e.Left, false)
		}

		return edgeSpace(e.Left, trailing) && edgeSpace(e.Right, trailing)
	default:
		return false
	}
}

func classOperator(k parser.Kind) bool {
	switch k {
	case parser.KindDot, parser.KindAmpAmp, parser.KindPipePipe,
		parser.KindAnd, parser.KindOr, parser.KindNullCoalesce:
		return true
	default:
		return false
	}
}

func sortClassStringToken(s *parser.StringExpr, sortClasses func(string, bool, bool) string, pinFirst, pinLast bool) {
	body, quote, ok := plainStringBody(s.Token.Text)
	if !ok {
		return
	}

	// Sort only the inner run, keeping leading and trailing whitespace verbatim: in
	// a concatenation (`'base ' . $x`) an edge space joins the fragments, so trimming
	// it would fuse classes across the join.
	const ws = " \t\r\n\f\v"

	inner := strings.Trim(body, ws)
	if inner == "" {
		return
	}

	lead := body[:len(body)-len(strings.TrimLeft(body, ws))]
	trail := body[len(strings.TrimRight(body, ws)):]

	pinFirst = pinFirst && lead == ""
	pinLast = pinLast && trail == ""

	newBody := lead + sortClasses(inner, pinFirst, pinLast) + trail
	if newBody == body {
		return
	}

	rebuilt := make([]byte, 0, len(newBody)+2)
	rebuilt = append(rebuilt, quote)
	rebuilt = append(rebuilt, newBody...)
	rebuilt = append(rebuilt, quote)
	s.Token.Text = rebuilt
}

// plainStringBody returns the inner text of a single- or double-quoted string
// literal that is safe to reorder as a class list, or ok=false.
func plainStringBody(text []byte) (body string, quote byte, ok bool) {
	if len(text) < 2 {
		return "", 0, false
	}

	quote = text[0]
	if (quote != '\'' && quote != '"') || text[len(text)-1] != quote {
		return "", 0, false
	}

	inner := text[1 : len(text)-1]

	// Acceptance must be quote-style invariant: the builder normalizes the quote
	// after this runs, so a value accepted under one quote and rejected (or printed
	// differently) under the other would sort on the second pass but not the first.
	// Reject every character that a single- and double-quoted literal render
	// differently or that the sorter would rewrite: a backslash escape, either quote
	// character (`"it's"` gains a `\'` when single-quoted), `$`/`{`/`}` interpolation
	// or braces, and any whitespace but a plain space (a tab or newline the sorter
	// collapses). None is a real class character, so the literal is left untouched.
	if bytes.ContainsAny(inner, "\\\"'${}\t\n\r\f\v") {
		return "", 0, false
	}

	return string(inner), quote, true
}

// RawDirectiveArgRehomable reports whether a statement-shaped directive argument
// (a `@foreach`/`@for`/`@forelse` argument the call-argument parser can't consume,
// so it is captured raw and emitted verbatim) is safe to re-home line by line under
// its directive. It is unsafe when a string literal (including heredoc/nowdoc) spans
// a newline: the interior whitespace is content, so re-indenting the line would
// change the value. A fragment that did not lex cleanly stays verbatim too, since a
// mis-lex could hide such a string.
func RawDirectiveArgRehomable(raw []byte, tokens []parser.Token, lexed bool) bool {
	// A lone CR is a line terminator to PHP but not to the re-home, which drops
	// it and can leave a `)` inside a line comment; a CRLF is rewritten whole
	// and is safe.
	if formatter.HasLoneCR(raw) {
		return false
	}

	return lexed && !HasMultilineStringLiteral(tokens)
}

func buildExprWidth(src []byte, tokens []parser.Token, expr parser.Expr, cfg *config.File, indentCols, width, startCol, srcCol int) []byte {
	if expr == nil {
		return nil
	}

	return buildWidth(src, tokens, cfg, indentCols, width, startCol, expr.Span(), func(b *builder) *document.Doc {
		b.firstLineColumn = srcCol

		return b.expr(expr)
	})
}

// BuildLoopClause lays out what a loop's parentheses hold, for a Blade @foreach,
// @forelse or @for whose argument was parsed inside the statement Blade compiles
// it to: stmt is that *parser.ForEachStmt or *parser.ForStmt, parsed from src.
// It is laid out against the print width at the host's indent, like any other
// directive argument, and returns nil for another statement.
func BuildLoopClause(src []byte, tokens []parser.Token, stmt parser.Stmt, cfg *config.File, indentCols, startCol int) []byte {
	build, lparen, rparen := loopClause(stmt)
	if build == nil {
		return nil
	}

	span := loopClauseSpan(src, lparen, rparen)

	return buildWidth(src, tokens, cfg, indentCols, cfg.PrintWidth, startCol, span, build)
}

func loopClauseSpan(src []byte, lparen, rparen parser.Token) parser.Span {
	lo, hi := lparen.Span.End.Offset, rparen.Span.Start.Offset
	for lo < hi && parser.IsWhitespace(src[lo]) {
		lo++
	}

	for hi > lo && parser.IsWhitespace(src[hi-1]) {
		hi--
	}

	return parser.Span{Start: parser.Position{Offset: lo}, End: parser.Position{Offset: hi}}
}

// LoopClauseFlatWidth is a loop clause's one-line width, ignoring author breaks,
// measured like DirectiveArgFlatWidth for the same reason.
func LoopClauseFlatWidth(src []byte, tokens []parser.Token, stmt parser.Stmt, cfg *config.File) int {
	build, _, _ := loopClause(stmt)
	if build == nil {
		return 0
	}

	b := &builder{src: src, cfg: cfg, tokens: tokens, comments: commentOffsets(tokens), crLiterals: crLiteralOffsets(tokens), embed: true, noAuthorBreaks: true}

	return document.FlatWidth(build(b), cfg.TabWidth)
}

// loopClause returns the builder for what a foreach's or a for's parentheses
// hold, and those parentheses; the builder is nil for another statement.
func loopClause(stmt parser.Stmt) (func(b *builder) *document.Doc, parser.Token, parser.Token) {
	switch n := stmt.(type) {
	case *parser.ForEachStmt:
		return func(b *builder) *document.Doc { return b.foreachClause(n) }, n.LParen, n.RParen
	case *parser.ForStmt:
		return func(b *builder) *document.Doc { return b.forClause(n) }, n.LParen, n.RParen
	}

	return nil, parser.Token{}, parser.Token{}
}

// buildWidth prints the doc build returns for span, one embedded fragment,
// against width at the host's indent, falling back to span as written when
// the doc would drop one of its comments.
func buildWidth(src []byte, tokens []parser.Token, cfg *config.File, indentCols, width, startCol int, span parser.Span, build func(b *builder) *document.Doc) []byte {
	b := &builder{src: src, cfg: cfg, tokens: tokens, comments: commentOffsets(tokens), crLiterals: crLiteralOffsets(tokens), embed: true}

	// The column the host will re-home this fragment at, so a `=>` alignment's block
	// test measures the real line rather than one starting at zero.
	b.depth = indentCols / max(cfg.TabWidth, 1)

	doc := b.keepComments(b.withLeadingComments(span), build(b), nil)
	for range indentCols / max(cfg.TabWidth, 1) {
		doc = document.Indent(doc)
	}

	out := document.Print(doc, document.Options{
		PrintWidth:  width,
		StartColumn: startCol,
		TabWidth:    cfg.TabWidth,
		UseTabs:     cfg.UseTabs(),
		EOL:         docEOL(cfg),
	})

	// A string literal spanning lines can carry source CRLF line endings; the
	// host re-emits embedded output line by line and normalizes EOLs, so a
	// carriage return must not survive here or the two disagree pass to pass.
	return stripLineCR(out)
}

// BuildStmts lays out a statement list parsed from src (a Blade `@php` body) and
// returns it at column zero, with no leading or trailing line break. indentCols
// is the visual column the host will re-home the block at (the `@php` body
// indent); the print width is reduced by it so width decisions match the block's
// true position, and the host adds the physical indent back.
//
// CONTRACT: src holds no string literal spanning lines (HasMultilineStringLiteral).
// The host re-homes every line, and inside such a literal that rewrites the
// value, so a caller keeps that body as written instead.
func BuildStmts(src []byte, tokens []parser.Token, stmts []parser.Stmt, cfg *config.File, indentCols int) []byte {
	b := &builder{src: src, cfg: columnZeroConfig(cfg, indentCols), tokens: tokens, comments: commentOffsets(tokens), crLiterals: crLiteralOffsets(tokens), rehomeSlices: true, embed: true}

	b.headerSpacing = startsWithOpenTag(stmts) && !containsInlineHTML(stmts)
	b.depth = indentCols / max(cfg.TabWidth, 1)

	stmts = b.preprocessUseRuns(stmts)

	spans := make([]parser.Span, len(stmts))
	docs := make([]*document.Doc, len(stmts))

	for i, s := range stmts {
		spans[i] = s.Span()
		docs[i] = b.stmt(s)
	}

	body := b.buildSequence(spans, docs, b.stmtForceBlank(stmts), false)

	// Collected after the statements: sorting a use run may already have moved a
	// comment of the EOF trivia.
	tail := b.fileTailComments(eofToken(tokens), len(stmts) > 0)

	switch {
	case tail != nil:
		body = document.Concat(body, tail)
	case len(stmts) == 0:
		return nil
	}

	width := max(cfg.PrintWidth-indentCols, 1)

	return document.Print(body, document.Options{
		PrintWidth: width,
		TabWidth:   cfg.TabWidth,
		UseTabs:    cfg.UseTabs(),
		EOL:        docEOL(cfg),
	})
}

// columnZeroConfig is the config a builder sees when its output prints at
// column zero against the width left at indentCols. The builder measures in
// whole indent levels, so the columns past the last level come off its width,
// or its predictions run that many columns wider than the printer.
func columnZeroConfig(cfg *config.File, indentCols int) *config.File {
	rest := indentCols % max(cfg.TabWidth, 1)
	if rest == 0 {
		return cfg
	}

	shifted := *cfg
	shifted.PrintWidth -= rest

	return &shifted
}

// withLeadingComments widens an embedded fragment's span back over the comments
// before its first token: they sit in that token's leading trivia, outside the
// expression's own span, so a verbatim fallback would otherwise drop them.
func (b *builder) withLeadingComments(span parser.Span) parser.Span {
	for _, tr := range b.leadingAt(span.Start.Offset) {
		if isCommentTrivia(tr.Kind) {
			span.Start = tr.Span.Start

			break
		}
	}

	return span
}
