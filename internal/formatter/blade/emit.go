package blade

import (
	"bytes"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/alpine"
	htmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/html"
	phpfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/php"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	htmlparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
	"github.com/JacobJoergensen/laravel-fmt/internal/textwidth"
)

// emitter holds src because PHP expression emission reads source spans on its
// fallback path.
type emitter struct {
	w       *formatter.Writer
	folding bool
	src     []byte
	cfg     *config.File
	// closeIndex maps a lowercased tag name to its </name> offsets, built lazily.
	closeIndex map[string][]int
	// pageWSClasses is the set of classes a <style> block gives a preserving
	// white-space, built lazily.
	pageWSClasses map[string]bool
	// livewireView is set for the template of a Livewire single-file component;
	// livewireBlocks counts the @script/@assets bodies being emitted, which
	// Livewire strips before it looks for a component <script> or <style>.
	livewireView   bool
	livewireBlocks int
	dirs           *parser.Directives
	roots          []parser.Node
	srcCopied      bool
	hangClose      *htmlparser.Element
	hangCloseAtTag *htmlparser.Element
}

// livewireTagColumn keeps a <script> or <style> where Livewire 4 reads it. In a
// single-file component's view, Livewire lifts such a tag out as the component's
// script or style only when it starts a line at column 0 (SingleFileParser's
// `(?:^|\n)<script`), so laying the tag out onto or off column 0 changes what the
// component does. zero asks for the open tag at column 0; prefix is written
// before it to keep a tag that stayed in the view off column 0. Anywhere else
// both are zero and the tag lays out as usual.
func (e *emitter) livewireTagColumn(el *htmlparser.Element) (zero bool, prefix []byte) {
	if !e.livewireView || e.livewireBlocks > 0 ||
		!bytes.EqualFold(el.Name, []byte("script")) && !bytes.EqualFold(el.Name, []byte("style")) {
		return false, nil
	}

	start := el.Open.Start.Offset
	lifted := start > 0 && e.src[start-1] == '\n'

	out := e.w.Bytes()
	atLineStart := len(out) == 0 || out[len(out)-1] == '\n'

	switch {
	case lifted:
		return atLineStart && e.w.IndentColumn() > 0, nil
	case atLineStart && e.w.IndentColumn() == 0:
		lineStart := bytes.LastIndexByte(e.src[:start], '\n') + 1

		if indent := e.src[lineStart:start]; len(bytes.Trim(indent, " \t")) == 0 && len(indent) > 0 {
			return false, indent
		}

		return false, []byte(" ")
	}

	return false, nil
}

// voidNamePaired reports whether a </name> close tag appears anywhere in src. A
// self-closed void element keeps its `/>` when its name is also used paired:
// emitted bare (`<link>`), it would re-pair with that close on re-parse and the
// output would not round-trip.
func (e *emitter) voidNamePaired(name []byte) bool {
	if e.closeIndex == nil {
		e.closeIndex = htmlparser.CloseTagIndex(e.src)
	}

	return len(e.closeIndex[string(bytes.ToLower(name))]) > 0
}

func (e *emitter) emitFile(file *parser.File) {
	e.dirs = file.Directives
	e.roots = file.Nodes

	if e.cfg.Tailwind {
		for _, n := range file.Nodes {
			e.presortClasses(n)
		}
	}

	if e.emitInterleaved(file.Nodes, false) {
		e.w.Newline()
	}
}

// presortClasses sorts every static single-line class list in e.src itself. A
// body that must stay byte-exact is copied from e.src, so this is how its
// classes reach the order the rest of the file gets. Only a sort that keeps
// the value's length is written, which leaves every node offset valid; the
// caller's buffer is copied before the first write.
func (e *emitter) presortClasses(n htmlparser.Node) {
	switch v := n.(type) {
	case *htmlparser.Element:
		for _, a := range v.Attributes {
			e.presortClassAttr(a)
		}

		if parser.IsRawBodyComponent(v.Name) {
			return
		}

		for _, c := range v.Children {
			e.presortClasses(c)
		}
	case *parser.DirectiveBlockNode:
		for _, c := range v.Body {
			e.presortClasses(c)
		}

		for _, b := range v.Branches {
			for _, c := range b.Body {
				e.presortClasses(c)
			}
		}
	}
}

func (e *emitter) presortClassAttr(a *htmlparser.Attr) {
	if a.Bound || a.Expr != nil || len(a.Parts) > 0 || !htmlfmt.IsClassAttr(a.Name) || hasValueLineBreak(a.Value, false) {
		return
	}

	start, end := a.ValueStart, a.ValueStart+len(a.Value)
	if end > len(e.src) || !bytes.Equal(e.src[start:end], a.Value) {
		return
	}

	sorted := sortClassSegment(e.cfg.TailwindTheme, a.Value, false, false)
	if len(sorted) != len(a.Value) || bytes.Equal(sorted, a.Value) {
		return
	}

	if !e.srcCopied {
		e.src = bytes.Clone(e.src)
		e.srcCopied = true
	}

	copy(e.src[start:end], sorted)
}

func (e *emitter) emitNode(node parser.Node) {
	switch n := node.(type) {
	case *parser.EchoNode:
		e.emitEcho(n)
	case *parser.RawEchoNode:
		e.emitRawEcho(n)
	case *parser.CommentNode:
		e.emitBladeComment(n)
	case *parser.DirectiveNode:
		e.emitDirective(n)
	case *parser.DirectiveBlockNode:
		e.emitDirectiveBlock(n)
	case *parser.PhpBlockNode:
		e.emitPhpBlock(n)
	case *parser.VerbatimNode:
		e.emitVerbatim(n)
	case *parser.EscapedAtNode:
		e.emitEscapedAt(n)
	case *parser.EscapedEchoNode:
		e.emitEscapedEcho(n)
	case *parser.RawPhpNode:
		e.emitRawPhp(n)
	case *htmlparser.Element:
		e.emitElement(n)
	case *htmlparser.Text:
		e.emitHtmlText(n)
	case *htmlparser.Comment:
		htmlfmt.EmitComment(e.w, e.src, n)
	case *htmlparser.Doctype:
		htmlfmt.EmitDoctype(e.w, n)
	}
}

// emitInlineEcho is the shared tail of the switches that emit mixed text and
// echo runs: children of an inline element, and the parts of an attribute value.
// Anything that is not an echo is the caller's to handle before it gets here.
func (e *emitter) emitInlineEcho(n parser.Node) {
	switch nn := n.(type) {
	case *parser.EchoNode:
		e.emitEcho(nn)
	case *parser.RawEchoNode:
		e.emitRawEcho(nn)
	case *parser.EscapedEchoNode:
		e.emitEscapedEcho(nn)
	case *parser.EscapedAtNode:
		e.emitEscapedAt(nn)
	case *parser.DirectiveNode:
		e.emitDirective(nn)
	}
}

// The author's delimiter is reproduced rather than normalized: `{{{ }}}` and `{{ }}`
// compile to the same echo, and converting between two spellings of the same thing is
// not this formatter's business. The width is read from the Open span.
func (e *emitter) emitEcho(n *parser.EchoNode) {
	braces := max(n.Open.End.Offset-n.Open.Start.Offset, 2)

	e.emitEchoArgs(n.Open, n.Close, n.Tokens, n.Args, strings.Repeat("{", braces), strings.Repeat("}", braces))
}

func (e *emitter) emitRawEcho(n *parser.RawEchoNode) {
	e.emitEchoArgs(n.Open, n.Close, n.Tokens, n.Args, "{!!", "!!}")
}

// emitEchoArgs writes an echo between its delimiters: its argument list flat, then
// any comment before the closer, which no argument's build reaches. A trailing
// comma is dropped, as a flat PHP call list drops it; the parser already refused
// one in a raw echo, where PHP does not accept it. An argument that keeps an
// author's break goes on lines of its own between the delimiters, as Pint lays
// it out; width alone never breaks an echo.
func (e *emitter) emitEchoArgs(open, closeTag parser.Span, tokens []php.Token, args []php.Expr, opener, closer string) {
	content := e.echoContent(open, closeTag)

	if phpfmt.SeparatorComment(tokens, args) {
		e.w.Write(opener + " ")

		lead := len(content) - len(bytes.TrimLeft(content, " \t\r\n"))
		trimmed := bytes.TrimRight(content, " \t\r\n")

		if e.emitArgsAsWritten(open.End.Offset+lead, open.End.Offset+len(trimmed), open.Start.Offset) {
			e.w.Write(closer)
		} else {
			e.w.Write(" " + closer)
		}

		return
	}

	// An array or match that is the whole echo hugs the delimiters, as its body
	// already reads as a block. A string literal's own newlines are content, not
	// a break, so an echo holding one stays inline.
	lines := (len(args) != 1 || !phpfmt.ExprHasBlockForm(args[0])) && !phpfmt.HasMultilineStringLiteral(tokens)

	bodyCols := e.w.IndentColumn()
	if lines {
		bodyCols += e.cfg.TabWidth
	}

	var buf [2][]byte

	built := buf[:0]
	broken := false

	for _, arg := range args {
		b := phpfmt.BuildEchoExpr(content, tokens, arg, e.cfg, bodyCols)
		built = append(built, b)
		broken = broken || (lines && bytes.IndexByte(b, '\n') >= 0)
	}

	broken = broken || lines && echoAuthorBroken(tokens) && !e.echoFitsFlat(built, opener, closer)

	lastEnd := args[len(args)-1].Span().End.Offset
	if comma := tokens[len(tokens)-2]; comma.Kind == php.KindComma {
		lastEnd = comma.Span.End.Offset
	}

	if !broken {
		e.w.Write(opener)
		e.w.Space()
		e.writeEchoArgs(built)
		e.emitTrailingArgComment(content, open.End.Offset, lastEnd, true, " ", closer)

		return
	}

	e.w.Write(opener)
	e.w.Indent()
	e.w.Newline()
	e.writeEchoArgs(built)

	if start, end, _ := phpfmt.TrailingArgComment(content, lastEnd); start != end {
		e.w.Write(" ")
		formatter.EmitVerbatimAtIndent(e.w, e.src, open.End.Offset+start, open.End.Offset+end)
	}

	e.w.Dedent()
	e.w.Newline()
	e.w.Write(closer)
}

func (e *emitter) echoFitsFlat(built [][]byte, opener, closer string) bool {
	width := e.w.CursorColumn() + len(opener) + len(" ") + len(" ") + len(closer)

	for i, b := range built {
		if i > 0 {
			width += len(", ")
		}

		width += len(b)
	}

	return width <= e.w.PrintWidth()
}

func echoAuthorBroken(tokens []php.Token) bool {
	return slices.ContainsFunc(tokens, func(t php.Token) bool {
		return slices.ContainsFunc(t.Leading, func(tr php.Trivia) bool {
			return tr.Kind == php.TriviaWhitespace && bytes.ContainsAny(tr.Text, "\n\r")
		})
	})
}

func (e *emitter) writeEchoArgs(built [][]byte) {
	for i, b := range built {
		if i > 0 {
			e.w.Write(", ")
		}

		e.w.WriteBytes(b)
	}
}

// echoContent is the exact fragment the echo was parsed from, which the
// tokens' offsets refer to.
func (e *emitter) echoContent(open, closeTag parser.Span) []byte {
	return e.src[open.End.Offset:closeTag.Start.Offset]
}

func (e *emitter) emitBladeComment(n *parser.CommentNode) {
	e.w.Write("{{--")
	e.w.WriteBytes(n.Text)
	e.w.Write("--}}")
}

// controlFlowDirectives lists the directives written `@if (…)` with a space
// before the paren, following the Laravel Blade docs. The docs write `@switch`,
// `@isset`, `@empty`, and function-like directives (`@include`, `@can`, …)
// tight, so those are deliberately absent.
var controlFlowDirectives = map[string]bool{
	"if":      true,
	"elseif":  true,
	"unless":  true,
	"for":     true,
	"foreach": true,
	"forelse": true,
	"while":   true,
}

func (e *emitter) emitDirective(n *parser.DirectiveNode) {
	e.w.Write("@")
	e.w.WriteBytes(n.Name)

	if n.RParen.End.Offset == 0 {
		return
	}

	if controlFlowDirectives[string(n.Name)] {
		e.w.Space()
	}

	e.w.Write("(")

	switch {
	case n.Opaque:
		// Not PHP, so no byte of it is this formatter's to change: not a trailing
		// space (`<?php ` needs its one), not an indent, and not a lone CR, which
		// ends a `#` comment before the `)` and whose LF twin can turn the argument
		// into PHP (`<<<A` then a break then `A`) that the next pass formats.
		formatter.EmitVerbatimExact(e.w, e.src[n.LParen.End.Offset:n.RParen.Start.Offset])
	case n.Loop != nil && !phpfmt.HasMultilineStringLiteral(n.LoopTokens):
		// The clause is PHP, laid out like a foreach's or a for's own. A string
		// literal spanning lines keeps the raw path, which re-homes nothing
		// inside it, for the reason the call-argument path gives below.
		if e.emitWrappedLoop(n) {
			break
		}

		clause := phpfmt.BuildLoopClause(n.LoopSrc, n.LoopTokens, n.Loop, e.cfg, e.w.IndentColumn(), e.w.CursorColumn())
		e.w.WriteBytes(clause)

		// A clause kept as written because of a comment can end in a line comment,
		// which takes the rest of the line; the `)` has to start the next one.
		if phpfmt.EndsInLineComment(clause) {
			e.w.Newline()
		}
	case n.Raw != nil:
		e.emitArgsAsWritten(n.LParen.End.Offset, n.RParen.Start.Offset, n.At.Offset)
	default:
		content := e.src[n.LParen.End.Offset:n.RParen.Start.Offset]
		indentCols := e.w.IndentColumn()

		// A string literal carrying a carriage return cannot survive a rebuild:
		// the host re-emits the built bytes line by line and would strip the CR
		// inside `"a\r\nb"` as a line ending, so the argument goes out as
		// written. A literal holding only a line feed rebuilds safely and still
		// wraps.
		if phpfmt.HasCRStringLiteral(n.Tokens) {
			e.w.WriteBytes(content)
			e.w.Write(")")

			return
		}

		if phpfmt.SeparatorComment(n.Tokens, n.Args) {
			e.emitArgsAsWritten(n.LParen.End.Offset, n.RParen.Start.Offset, n.At.Offset)
			e.w.Write(")")

			return
		}

		// `@class([...])` is a class list, so its string literals are Tailwind-sorted
		// like a `class="..."` attribute (the array's bare values and keyed keys hold
		// classes; keyed values are conditions and stay put).
		sortClasses := e.cfg.Tailwind && string(n.Name) == "class"

		args := make([][]byte, len(n.Args))

		// Each argument is laid out at the column it lands at, after the ones
		// before it: its width budget and the comments it aligns depend on it.
		col := e.w.CursorColumn()

		for i, arg := range n.Args {
			if sortClasses {
				phpfmt.SortClassExpr(arg, e.sortPinned)
			}

			// The directive's `)` shares the argument's last line, so it is
			// reserved there; the @php statement build reserves it itself.
			if string(n.Name) == "php" {
				args[i] = phpfmt.BuildStmtDirectiveArg(content, n.Tokens, arg, e.cfg, indentCols, col)
			} else {
				args[i] = phpfmt.BuildDirectiveArg(content, n.Tokens, arg, e.cfg, indentCols, col, e.sourceColumnAt(n.LParen.End.Offset), len(")"))
			}

			nl := bytes.LastIndexByte(args[i], '\n')
			if nl >= 0 {
				col = 0
			}

			col += formatter.VisualWidthBytes(args[i][nl+1:], e.cfg.TabWidth) + len(", ")
		}

		if e.emitWrappedCondition(n, content, args, indentCols) {
			return
		}

		for i, arg := range args {
			if i > 0 {
				e.w.Write(", ")
			}

			e.w.WriteBytes(arg)
		}

		// An empty list holds no argument to sit after, so the whole region
		// between the parens is the trivia to carry: `@if(/* note */)`.
		lastEnd := 0
		if len(n.Args) > 0 {
			lastEnd = n.Args[len(n.Args)-1].Span().End.Offset
		}

		// A trailing comma is kept: whether it compiles depends on the directive
		// (`@can(…,)` splices into a call where it is valid, `@include(…,)` into
		// one where it doubles up with the argument Blade appends). It rides the
		// last argument.
		if len(n.Args) > 0 && len(n.Tokens) >= 2 && n.Tokens[len(n.Tokens)-2].Kind == php.KindComma {
			e.w.Write(",")
			lastEnd = n.Tokens[len(n.Tokens)-2].Span.End.Offset
		}

		e.emitTrailingArgComment(content, n.LParen.End.Offset, lastEnd, len(n.Args) > 0, "", ")")

		return
	}

	e.w.Write(")")
}

// emitArgsAsWritten writes the source argument region [start, end) unchanged. A
// multi-line region re-homes its continuation lines under indentAt's line,
// unless a string literal spans a line (re-indenting it would corrupt the
// value). A region ending inside a line comment takes the rest of its line, so
// the closer has to move to the next one; it reports that break.
func (e *emitter) emitArgsAsWritten(start, end, indentAt int) bool {
	raw := e.src[start:end]

	if bytes.IndexByte(raw, '\n') >= 0 && phpfmt.RawDirectiveArgRehomable(raw) {
		formatter.EmitVerbatimRebased(e.w, e.src, start, end, indentAt)
	} else {
		e.w.WriteBytes(raw)
	}

	if phpfmt.EndsInLineComment(raw) {
		e.w.Newline()

		return true
	}

	return false
}

// emitTrailingArgComment writes the comment between the last argument and the
// closer (a directive's `)`, an echo's `}}`), then the closer, which pad
// separates from what precedes it on its line. A line comment runs to the end
// of its line, so the closer moves to the next one.
func (e *emitter) emitTrailingArgComment(content []byte, base, lastEnd int, spaced bool, pad, closer string) {
	start, end, breaks := phpfmt.TrailingArgComment(content, lastEnd)

	switch {
	case start == end:
		e.w.Write(pad + closer)
	case !breaks:
		if spaced {
			e.w.Write(" ")
		}

		e.w.WriteBytes(content[start:end])
		e.w.Write(pad + closer)
	default:
		// Emitted from its real source offsets, never a detached slice: the
		// emitter strips each line's source indent by where the line starts,
		// and a slice starting mid-line hides that, so a second comment line
		// would gain a level of indent every pass.
		e.w.Indent()
		e.w.Newline()
		formatter.EmitVerbatimAtIndent(e.w, e.src, base+start, base+end)
		e.w.Dedent()
		e.w.Newline()
		e.w.Write(closer)
	}
}

// emitWrappedCondition breaks a control-flow directive's condition under its opener
// when the flat form overflows. Lone argument only; a list needs argument layout.
func (e *emitter) emitWrappedCondition(n *parser.DirectiveNode, content []byte, args [][]byte, indentCols int) bool {
	if len(args) != 1 || !controlFlowDirectives[string(n.Name)] {
		return false
	}

	// A multi-line string literal in the condition carries its own newlines; the
	// re-home below would indent the string's continuation lines, rewriting its
	// value. Leave such a condition on the opener line (it wraps on the string's own
	// break) rather than corrupt the string.
	if phpfmt.HasMultilineStringLiteral(n.Tokens) {
		return false
	}

	// A condition that opens its own bracket breaks inside it instead, as a PHP
	// header does: `@if (in_array($status, [`.
	if phpfmt.HeaderHugs(n.Args[0]) {
		return false
	}

	// Measured flat, never from the built form: the formatter breaks the argument
	// itself, so reading its current shape would flip the layout between passes.
	flat := phpfmt.DirectiveArgFlatWidth(content, n.Tokens, n.Args[0], e.cfg)
	if e.w.CursorColumn()+flat+len(")") <= e.cfg.PrintWidth {
		return false
	}

	// A condition that fits on its own line still takes this form when the hugged
	// one would break: hugged, it breaks at whatever operator reaches the edge.
	body := phpfmt.BuildDirectiveArgWrapped(content, n.Tokens, n.Args[0], e.cfg, indentCols+e.cfg.TabWidth)
	if bytes.IndexByte(body, '\n') < 0 && bytes.IndexByte(args[0], '\n') < 0 {
		return false
	}

	e.w.Indent()
	e.w.Newline()
	formatter.EmitVerbatim(e.w, body, 0, len(body))
	e.w.Dedent()
	e.w.Newline()
	e.w.Write(")")

	return true
}

// emitWrappedLoop moves an over-wide loop clause onto its own line under the
// opener, as an over-wide foreach header does in PHP, unless its subject opens
// its own bracket: `@foreach ([` breaks inside the array. The caller writes `)`.
func (e *emitter) emitWrappedLoop(n *parser.DirectiveNode) bool {
	if loop, ok := n.Loop.(*php.ForEachStmt); ok && phpfmt.HeaderHugs(loop.Expr) {
		return false
	}

	flat := phpfmt.LoopClauseFlatWidth(n.LoopSrc, n.LoopTokens, n.Loop, e.cfg)
	if e.w.CursorColumn()+flat+len(")") <= e.cfg.PrintWidth {
		return false
	}

	indentCols := e.w.IndentColumn() + e.cfg.TabWidth

	e.w.Indent()
	e.w.Newline()
	e.w.WriteBytes(phpfmt.BuildLoopClause(n.LoopSrc, n.LoopTokens, n.Loop, e.cfg, indentCols, indentCols))
	e.w.Dedent()
	e.w.Newline()

	return true
}

// indentedBranchDirectives lists openers whose branches sit one indent deeper
// than the opener. Only `@switch` qualifies: its `@case`/`@default` are
// subordinate, matching PHP switch layout; other openers' branches are peer
// clauses (`@else`/`@elseif`) at the opener's indent.
var indentedBranchDirectives = map[string]bool{
	"switch": true,
}

// isMailComponent reports whether a directive is `@component('mail::…')`, the
// directive spelling of <x-mail::…>: its body is Markdown, not markup.
func isMailComponent(d *parser.DirectiveNode) bool {
	if string(d.Name) != "component" || len(d.Args) == 0 {
		return false
	}

	s, ok := d.Args[0].(*php.StringExpr)
	if !ok || len(s.Token.Text) < 2 {
		return false
	}

	return bytes.HasPrefix(s.Token.Text[1:], []byte("mail::"))
}

// emitMailComponentBody writes a Markdown mail body exactly as it came, the way
// a <x-mail::…> body goes out: an indent there would turn a paragraph into a
// code block, and a line break joined into a space would merge a list.
func (e *emitter) emitMailComponentBody(n *parser.DirectiveBlockNode) {
	start, end := n.Opener.RParen.End.Offset, n.End.At.Offset
	raw := e.src[start:end]

	body := make([]htmlparser.Node, len(n.Body))
	for i, c := range n.Body {
		body[i] = c
	}

	if !htmlfmt.EmitVerbatimLines(e.w, raw, start, phpSpansFor(raw, body)) {
		e.w.Newline()
	}
}

// livewireStrippedBlocks are the blocks Livewire removes from a single-file
// component's view before it looks for the component's <script> and <style>.
var livewireStrippedBlocks = map[string]bool{"script": true, "assets": true}

// emitDirectiveBlock emits a block directive with its body indented below the
// opener; branches in indentedBranchDirectives indent one level deeper. n.End
// is always non-nil: the parser builds a DirectiveBlockNode only when it finds
// a matching closer, else it falls back to a flat DirectiveNode.
func (e *emitter) emitDirectiveBlock(n *parser.DirectiveBlockNode) {
	// A body glued to its markers renders against them: a line break after
	// `@if (…)` leaves its indent in the output, since PHP eats only the newline
	// after `?>`. Only the directives and echoes inside are rebuilt.
	if directiveMarkersGlued(n) {
		span := n.Span()
		if !e.emitGluedBodyNormalized([]htmlparser.Node{n}, span.Start.Offset, span.End.Offset, true, 0) {
			e.emitNodeVerbatim(n)
		}

		return
	}

	if e.emitBlockOnOneLine(n) {
		return
	}

	e.emitDirective(n.Opener)

	if isMailComponent(n.Opener) {
		e.emitMailComponentBody(n)
		e.emitDirective(n.End)

		return
	}

	stripped := livewireStrippedBlocks[string(n.Opener.Name)]
	if stripped {
		e.livewireBlocks++
	}

	e.emitBlockBody(n.Body)

	if stripped {
		e.livewireBlocks--
	}

	indentBranches := indentedBranchDirectives[string(n.Opener.Name)]

	if indentBranches {
		e.w.Indent()
	}

	for _, br := range n.Branches {
		e.w.Newline()
		e.emitDirective(br.Directive)
		e.emitBlockBody(br.Body)
	}

	if indentBranches {
		e.w.Dedent()
	}

	e.w.Newline()
	e.emitDirective(n.End)
}

func (e *emitter) emitBlockBody(body []parser.Node) {
	e.w.Indent()
	e.emitInterleaved(body, true)
	e.w.Dedent()
}

func (e *emitter) emitPhpBlock(n *parser.PhpBlockNode) {
	// The body is re-emitted line by line at the block's indent, which would
	// land inside a string literal spanning lines and rewrite its value, so
	// such a block stays as written, as a raw `<?php` island does.
	if phpfmt.HasMultilineStringLiteral(n.Tokens) {
		e.w.WriteBytes(e.src[n.Open.Start.Offset:n.Close.End.Offset])

		return
	}

	e.w.Write("@php")

	if len(n.Stmts) == 0 {
		e.w.Write(" @endphp")

		return
	}

	content := e.src[n.Open.End.Offset:n.Close.Start.Offset]

	// A block the author wrote on one line stays there while its statement
	// does and the line fits, as Pint keeps it.
	if !bytes.ContainsAny(content, "\r\n") {
		line := phpfmt.BuildStmts(content, n.Tokens, n.Stmts, e.cfg, e.w.IndentColumn())
		if bytes.IndexByte(line, '\n') < 0 && e.w.TryFlat(func() {
			e.w.Write(" ")
			e.w.WriteBytes(line)
			e.w.Write(" @endphp")
		}) {
			return
		}
	}

	e.w.Indent()
	body := phpfmt.BuildStmts(content, n.Tokens, n.Stmts, e.cfg, e.w.IndentColumn())
	e.w.Newline()
	formatter.EmitVerbatim(e.w, body, 0, len(body))
	e.w.Dedent()
	e.w.Newline()
	e.w.Write("@endphp")
}

func (e *emitter) emitVerbatim(n *parser.VerbatimNode) {
	e.w.Write("@verbatim")

	if len(n.Text) == 0 {
		e.w.Write("@endverbatim")

		return
	}

	e.w.WithoutIndent(func() {
		e.w.WriteBytes(n.Text)
	})

	e.w.Write("@endverbatim")
}

func (e *emitter) emitEscapedAt(_ *parser.EscapedAtNode) {
	e.w.Write("@@")
}

// emitEscapedEcho writes `@{{ text }}` byte for byte: Blade leaves the body
// unparsed for JS frameworks and renders it as the source minus the `@`. It
// compiles to no PHP, so a lone CR in it is an HTML line break.
func (e *emitter) emitEscapedEcho(n *parser.EscapedEchoNode) {
	e.w.WriteBytes(htmlfmt.NormalizeLineBreaks(e.src[n.At.Offset:n.Close.End.Offset]))
}

// emitRawPhp writes a raw `<?php … ?>` island. A body the parser could read as a
// complete statement list is formatted like an `@php` block; anything else (an
// alt-syntax or brace fragment, a `<?=` echo, a lone comment) goes out verbatim.
//
// Verbatim path: the first line takes the current indent and the PHP is emitted
// byte-for-byte, so interior newlines keep their source form, a stable fixed point.
// Trailing whitespace is trimmed: a terminated island ends at `?>` and has none,
// while an unterminated one runs to EOF and would otherwise absorb the file's final
// newline and drift against it each pass.
func (e *emitter) emitRawPhp(n *parser.RawPhpNode) {
	// A string literal spanning lines carries its own newlines, and re-homing the
	// body indents them, rewriting the value the author wrote (and compounding it
	// one level per pass). The directive path refuses such a body for the same
	// reason.
	if len(n.Stmts) > 0 && !phpfmt.HasMultilineStringLiteral(n.Tokens) {
		e.emitRawPhpFormatted(n)

		return
	}

	island := e.src[n.Start.Offset:n.End.Offset]
	e.w.WriteBytes(bytes.TrimRight(island, " \t\r\n\f\v"))
}

// emitRawPhpFormatted lays out an island whose body is a statement list.
//
// An island the author wrote on one line stays on one line, the way an array
// keeps the break its author chose; expanding every `<?php $x = 1; ?>` in a
// legacy template to three lines would be churn, not formatting. A body that
// does not fit on one line takes the `@php` shape, and so does an island that
// was already written across lines. That is also what makes the output a fixed
// point: the second pass reads back a multi-line source with a multi-line body
// and reaches the same branch.
//
// The layout of the PHP inside renders nothing either way, since only what the
// statements echo reaches the page, so neither shape changes the output.
func (e *emitter) emitRawPhpFormatted(n *parser.RawPhpNode) {
	body := e.src[n.Body.Start.Offset:n.Body.End.Offset]

	// The author's spelling of the tag is reproduced, not normalized: PHP accepts
	// `<?PHP` and rewriting it is a change the formatter was not asked to make.
	open := e.src[n.Start.Offset:n.Body.Start.Offset]

	if !bytes.ContainsAny(body, "\r\n") {
		// The one-line form spends columns on its own delimiters, which the body
		// has to fit inside, so they come out of its width budget.
		flat := e.rawPhpBody(n, e.w.IndentColumn()+len(open)+len(" ")+len(" ?>"))

		if !bytes.ContainsAny(flat, "\r\n") {
			e.w.WriteBytes(open)
			e.w.Write(" ")
			e.w.WriteBytes(flat)
			e.w.Write(" ?>")

			return
		}
	}

	e.w.WriteBytes(open)
	e.w.Indent()
	e.w.Newline()
	formatted := e.rawPhpBody(n, e.w.IndentColumn())
	formatter.EmitVerbatim(e.w, formatted, 0, len(formatted))
	e.w.Dedent()
	e.w.Newline()
	e.w.Write("?>")
}

func (e *emitter) rawPhpBody(n *parser.RawPhpNode, indentCols int) []byte {
	body := e.src[n.Body.Start.Offset:n.Body.End.Offset]
	built := phpfmt.BuildStmts(body, n.Tokens, n.Stmts, e.cfg, indentCols)

	return bytes.TrimRight(built, " \t\r\n\f\v")
}

// The slash on a void element carries no meaning in HTML5, so the spelling is
// the author's and is reproduced. It is forced only where it is load-bearing:
// a void name this document also uses paired needs the marker, or the empty
// use reads as the open half of the paired one on the next parse.
func (e *emitter) selfCloseMarker(el *htmlparser.Element) bool {
	return el.SlashClosed || el.SelfClose && htmlparser.IsVoidElement(el.Name) && e.voidNamePaired(el.Name)
}

// emitElement emits an HTML element inside a Blade tree. el.SelfClose is true
// for HTML5-void and explicit self-close, false when the parser captured a
// paired close (including a relaxed-void `<link>` used as a paired element).
func (e *emitter) emitElement(el *htmlparser.Element) {
	selfClose := e.selfCloseMarker(el)

	// An inline container glued to its body whose body cannot be copied (a
	// same-name element left open inside it) has no safe layout: a structural
	// one adds a rendered space at the glue. It goes out as written.
	if !el.SelfClose && !el.Loose && htmlfmt.IsInlineContainer(el.Name) && isWhitespaceSensitive(el) && !bodyCopyable(el) {
		end := max(el.CloseFinal.End.Offset, el.CloseOpen.Start.Offset)
		formatter.EmitVerbatimUnmoved(e.w, e.src, el.Open.Start.Offset, end)

		return
	}

	if !selfClose && e.foldsSingleAttribute(el) {
		return
	}

	// A sibling hanging this element's close tag takes the plain path, which
	// writes that hung close; the glued layout writes its own.
	if run, ok := e.gluedTagWrap(el, selfClose); ok && (e.hangClose != el || !e.fitsAsSpacedText(el, run)) {
		e.emitGluedTagWrapped(el, run)

		return
	}

	tail := e.gluedBodyTail(el)

	var openFlat bool

	hang := el.SelfClose && e.hangClose == el

	open := func() bool { return e.emitOpenTag(el, selfClose, hang, tail) }
	if e.hangsGluedBodyOpen(el) {
		open = func() bool {
			e.emitOpenTag(el, selfClose, true, nil)
			e.w.Indent()
			e.w.Newline()
			e.w.Write(">")
			e.w.Dedent()

			return false
		}
	}

	switch zero, prefix := e.livewireTagColumn(el); {
	case zero:
		e.w.WithoutIndent(func() { openFlat = open() })
	default:
		e.w.WriteBytes(prefix)
		openFlat = open()
	}

	if el.SelfClose {
		if hang {
			e.closeHungOpenTag(el, selfClose)
		}

		return
	}

	switch {
	case htmlparser.IsRcdataElement(el.Name):
		htmlfmt.EmitRcdataContent(e.w, el.Children)
	case htmlparser.IsPreformattedElement(el.Name):
		htmlfmt.EmitPreformattedContent(e.w, e.src, el, e.bodyPHPSpans(el))
	case e.isWhitespacePreservingElement(el) || htmlfmt.IsEmptinessSensitiveElement(el):
		htmlfmt.EmitPreformattedContent(e.w, e.src, el, e.bodyPHPSpans(el))
	case htmlparser.IsRawTextElement(el.Name) || parser.IsRawBodyComponent(el.Name):
		htmlfmt.EmitRawTextContent(e.w, el, e.cfg)
	case len(el.Children) == 0 || allWhitespaceChildren(el.Children):
		// Empty (or whitespace-only) body: the close tag emits immediately, so no
		// indent-only line is left between the open and close tags.
	case e.allInlineChildren(el.Children) && openFlat && !inlineChildrenAuthoredMultiline(el.Children):
		e.emitInlineOrBlockChildren(el)
	case parser.TrimsSlot(el.Name) && e.allInlineChildren(el.Children) && openFlat && edgeBroken(el.Children) &&
		!lastChildEndsAtClose(el):
		// A trimmed slot renders no edge whitespace, so a break the author put at
		// an edge is layout and the body goes block. A break only inside the body
		// is not: laid out as a block, it flips a glued sibling's hung closer
		// (seed 89140264c7932498).
		e.emitBlockInlineChildren(el)
	case isWhitespaceSensitive(el) && bodyCopyable(el) && e.gluedOnlyBetweenLines(el):
		if !e.emitGluedBodyOnOneLine(el) {
			e.emitStructuralChildren(el)
		}
	case isWhitespaceSensitive(el) && bodyCopyable(el):
		// A source-glued inline boundary blocks a structural reflow (it would inject a
		// rendered space at the glue). For a white-space:normal block element the body
		// is still safe to re-home to the tree's indent (leading-line whitespace
		// collapses); an inline element/component keeps verbatim because its edge
		// whitespace renders, as does a body with a descendant that renders leading
		// whitespace literally (<pre>/<textarea>/whitespace-pre*).
		//
		// Must precede the all-inline block case: an inline element whose open tag
		// wraps would otherwise take that path and gain a rendered space per side.
		keepsBranch := htmlfmt.IsInlineContainer(el.Name) || e.bodyPreservesInnerWhitespace(el.Children) ||
			lastChildEndsAtClose(el)

		if keepsBranch || !bodyIsRehomable(e.src, el.Children) {
			if !e.emitGluedBodyNormalized(el.Children, el.OpenClose.End.Offset, el.CloseOpen.Start.Offset, !keepsBranch, e.closeTagWidth(el)) {
				htmlfmt.EmitGluedBodyContent(e.w, e.src, el, e.bodyPHPSpans(el))
			}
		} else {
			e.emitRehomedGluedBody(el)
		}
	case e.allInlineChildren(el.Children):
		e.emitBlockInlineChildren(el)
	case e.isInlineContent(el.Children, htmlfmt.RunAuthoredOnOneLine(el.Children)) && !htmlfmt.KeepsAuthoredBreak(el.Children):
		e.emitInlineContent(el, openFlat)
	default:
		e.emitStructuralChildren(el)
	}

	if el.Loose {
		return
	}

	e.emitCloseTag(el)
}

type inlineRun struct {
	atoms []htmlfmt.InlineAtom
	lead  bool
	trail bool
}

// gluedTagWrap reports whether el must take the hanging-bracket layout, and the
// run to emit if so.
//
// A boundary the author wrote glued cannot carry a line break: the newline would
// render as a space nobody wrote. When such a run does not fit, the break has to
// go where whitespace is insignificant, and the only such place is inside the
// tags. Breaking at the glue would change the page, and copying the body
// verbatim would stop formatting it.
func (e *emitter) gluedTagWrap(el *htmlparser.Element, selfClose bool) (inlineRun, bool) {
	// Deliberately not isWhitespaceSensitive: that reads source offsets to find a
	// glue, so the newlines this layout introduces change its answer and the next
	// pass picks a different branch. Everything below is a function of the content
	// instead, which is the same whatever shape the source is in.
	if el.SelfClose || el.Loose || selfClose {
		return inlineRun{}, false
	}

	// The hook runs before the body switch, so it has to decline everything the
	// cases above the glued one claim: their whitespace is content, not layout.
	if bodyIsVerbatim(el) || e.isWhitespacePreservingElement(el) || htmlfmt.IsEmptinessSensitiveElement(el) {
		return inlineRun{}, false
	}

	if !e.isInlineContent(el.Children, false) || e.bodyPreservesInnerWhitespace(el.Children) {
		return inlineRun{}, false
	}

	atoms, lead, trail := htmlfmt.InlineAtoms(el.Children)
	if len(atoms) == 0 {
		return inlineRun{}, false
	}

	run := inlineRun{atoms: atoms, lead: lead, trail: trail}

	// Prettier keeps the attributes on the tag line while the body up to its
	// first break opportunity fits beside them, and hangs `>`; a body with no
	// space is one unbroken stretch, so the attributes break (gluedBodyTail).
	// Without attributes there is nothing to break, so `>` hangs.
	if !lead && !trail && len(el.Attributes) > 0 && edgesRender(el) && !slices.ContainsFunc(atoms, notTextOrEcho) {
		first := slices.IndexFunc(atoms, func(a htmlfmt.InlineAtom) bool { return a.SpaceBefore })
		if first < 0 {
			return inlineRun{}, false
		}

		return run, e.w.Measure(func() {
			e.emitOpenTagFlat(el, el.Attributes, false, false)
			e.emitInlineFlat(atoms[:first], false, false)
		})
	}

	// Prettier hangs the bracket when the first line overflows; Pint keeps a
	// glued text body on one line and breaks a two-attribute tag instead
	// (gluedBodyTail).
	if !htmlfmt.InlineRunHasGluedInterior(atoms) {
		// A body that prints a line break counts as one that has it: an echo
		// whose `#` comment ends at a lone CR has none in the source, but its
		// output does, and the next pass reads it from there.
		owned := trail && (e.bodyHasLineBreak(el) || !e.w.MeasureOneLine(func() { e.emitInlineRun(el, run) })) ||
			!trail && len(el.Attributes) < 2 && slices.ContainsFunc(atoms, isElementAtom)
		// The close tag is written plain: a fill around el may have marked it to
		// hang its bracket, and that layout state must not decide this.
		if lead || !owned || !edgesRender(el) || e.w.Measure(func() {
			e.emitOpenTag(el, false, false, nil)
			e.emitInlineRun(el, run)
			e.w.Write("</")
			e.writeCloseTagName(el)
			e.w.Write(">")
		}) {
			return inlineRun{}, false
		}

		return run, true
	}

	// No width test: once a boundary is glued this layout owns the element
	// whether the run fits or not, and decides that below on one budget.
	// Handing a short run back would let a path that reserves the close tag
	// twice break it at the glue.
	return run, true
}

// emitGluedTagWrapped emits the whole element with its open bracket hung, so
// every line break falls inside tag punctuation:
//
//	<a class="x"
//	    >text</a>
//
// The close tag stays joined, as Pint joins Prettier's dangling `>`.
func (e *emitter) emitGluedTagWrapped(el *htmlparser.Element, run inlineRun) {
	// Fit first: a run that fits whole stays on one line.
	if e.w.TryFlat(func() {
		e.emitOpenTag(el, false, false, nil)
		e.emitInlineRun(el, run)
		e.emitCloseTag(el)
	}) {
		return
	}

	// Only an element whose own edges render needs its brackets hung; a block
	// element collapses the whitespace at its edges. Edges that already hold
	// whitespace take the same layout, as Prettier does, and a line break the
	// author wrote beside an element stays, as Pint's Prettier keeps it.
	if !edgesRender(el) || run.lead && run.trail {
		e.emitOpenTag(el, false, false, nil)
		e.w.Indent()
		e.w.Newline()
		e.fillInlineAtoms(run.atoms, 0, false)
		e.w.Dedent()
		e.w.Newline()
		e.emitCloseTag(el)

		return
	}

	if run.lead {
		run.lead = false

		e.emitOpenTag(el, false, false, nil)
		e.w.Indent()
		e.w.Newline()
		e.emitInlineFilled(el, run, htmlfmt.CloseTagWidth(el), false)
		e.emitCloseTag(el)
		e.w.Dedent()

		return
	}

	// After a bare `<tag` line (no attribute on it), the `>` line goes back to
	// the tag's column only when it holds the whole body and the close tag, as
	// Pint's DedentHuggedTerminator does; a body that goes on stays indented.
	bare := !e.emitOpenTag(el, false, true, nil) || len(el.Attributes) == 0

	trail := run.trail
	run.trail = false

	closer := htmlfmt.CloseTagWidth(el)
	if trail {
		closer = 0
	}

	closeTag := func() {
		e.w.Write("</")
		e.writeCloseTagName(el)
		e.w.Write(">")
	}

	body := func() {
		e.w.Write(">")
		e.emitInlineFilled(el, run, closer, !trail)

		if !trail {
			closeTag()
		}
	}

	e.w.Indent()
	e.w.Newline()

	dedented := bare && !trail && e.w.MeasureOneLine(body)
	if dedented {
		e.w.Dedent()
	}

	body()

	if !dedented {
		e.w.Dedent()
	}

	if trail {
		e.w.Newline()
		closeTag()
	}
}

// bodyIsVerbatim reports whether the element's body is content the emitter
// copies rather than lays out: a `<pre>`, a raw-text or rcdata element, or a
// component whose body the author owns.
func bodyIsVerbatim(el *htmlparser.Element) bool {
	return htmlparser.IsRcdataElement(el.Name) ||
		htmlparser.IsPreformattedElement(el.Name) ||
		htmlparser.IsRawTextElement(el.Name) ||
		parser.IsRawBodyComponent(el.Name)
}

// Pint's CollapseSingleAttribute, applied to the real layout: rendered in a
// probe and folded when it hangs the attribute or `>` with a one-line body.
// Folding the output, not a width guess, keeps the next pass's decision the
// same.
func (e *emitter) foldsSingleAttribute(el *htmlparser.Element) bool {
	if e.folding || e.w.InFlatProbe() || el.SelfClose || el.Loose || len(el.Attributes) != 1 || attrsMustBreak(el.Attributes) ||
		e.w.CursorColumn() > e.w.IndentColumn() {
		return false
	}

	if len(el.Children) > 0 && !allWhitespaceChildren(el.Children) && !e.foldableBody(el) {
		return false
	}

	var out, openTag, closeTag []byte

	e.folding = true
	e.w.MeasureOneLine(func() {
		start := len(e.w.Bytes())
		e.emitElement(el)
		out = bytes.Clone(e.w.Bytes()[start:])
	})
	if bytes.IndexByte(out, '\n') >= 0 {
		e.w.MeasureOneLine(func() {
			start := len(e.w.Bytes())
			e.emitOpenTagFlat(el, el.Attributes, false, true)
			openTag = bytes.Clone(bytes.TrimSpace(e.w.Bytes()[start:]))

			start = len(e.w.Bytes())
			e.w.Write("</")
			e.writeCloseTagName(el)
			e.w.Write(">")
			closeTag = bytes.Clone(e.w.Bytes()[start:])
		})
	}
	e.folding = false

	line, ok := collapseSingleAttribute(out, openTag, closeTag)
	if !ok {
		return false
	}

	// The probe wrote the line's indent itself; the writer adds its own.
	e.w.WriteBytes(bytes.TrimLeft(line, " \t"))

	return true
}

// Whether the body prints on one line is left to the probe.
func (e *emitter) foldableBody(el *htmlparser.Element) bool {
	if !edgesRender(el) || bodyIsVerbatim(el) || e.isWhitespacePreservingElement(el) ||
		htmlfmt.IsEmptinessSensitiveElement(el) || !e.isInlineContent(el.Children, false) ||
		e.bodyPreservesInnerWhitespace(el.Children) {
		return false
	}

	atoms, lead, trail := htmlfmt.InlineAtoms(el.Children)

	return len(atoms) > 0 && !lead && !trail
}

// collapseSingleAttribute folds `<tag attr` + `>body</tag>`, or `<tag` +
// `attr` + `>body</tag>`, onto one line, as Pint's CollapseSingleAttribute
// does; output already on one line is returned as it is. openTag is the open
// tag as written without its `>`: a line is that tag only when it equals it,
// since an attribute name may hold a quote that hides a `>` from a scan.
func collapseSingleAttribute(out, openTag, closeTag []byte) ([]byte, bool) {
	// A lone CR is a line break in HTML content; the unfolded layout owns it.
	if bytes.IndexByte(out, '\r') >= 0 {
		return nil, false
	}

	lines := bytes.Split(out, []byte("\n"))

	if len(lines) == 1 {
		return out, true
	}

	head := bytes.TrimSpace(lines[0])

	last := bytes.TrimSpace(lines[len(lines)-1])
	if len(last) == 0 || last[0] != '>' || !bytes.HasSuffix(last, closeTag) {
		return nil, false
	}

	switch {
	case len(lines) == 2 && bytes.Equal(head, openTag):
		return slices.Concat(lines[0], last), true
	case len(lines) == 3 && bytes.Equal(slices.Concat(head, []byte(" "), bytes.TrimSpace(lines[1])), openTag):
		return slices.Concat(lines[0], []byte(" "), bytes.TrimSpace(lines[1]), last), true
	}

	return nil, false
}

// edgesRender reports whether whitespace at the element's own edges reaches the
// page, which is what decides whether the break has to move inside the tags.
func edgesRender(el *htmlparser.Element) bool {
	return htmlparser.IsInlineElement(el.Name) || htmlfmt.IsInlineContainer(el.Name)
}

// emitInlineRun writes the run flat. A block container trims its edge whitespace,
// which does not render; an inline one keeps it, because it does.
func (e *emitter) emitInlineRun(el *htmlparser.Element, run inlineRun) {
	if htmlfmt.IsInlineContainer(el.Name) {
		e.emitInlineFlat(run.atoms, run.lead, run.trail)

		return
	}

	e.emitInlineFlat(run.atoms, false, false)
}

// emitInlineFilled breaks only where the author left whitespace: a break at a
// glued boundary would render as a space. closer is the width after the last
// atom; with hangLast, a last atom that cannot share its line with it hangs
// its closer at the tag's column.
func (e *emitter) emitInlineFilled(el *htmlparser.Element, run inlineRun, closer int, hangLast bool) {
	edges := htmlfmt.IsInlineContainer(el.Name)

	if edges && run.lead {
		e.w.Space()
	}

	e.fillInlineAtoms(run.atoms, closer, hangLast)

	if edges && run.trail {
		e.w.Space()
	}
}

func (e *emitter) fillInlineAtoms(atoms []htmlfmt.InlineAtom, closer int, hangLast bool) {
	for i, a := range atoms {
		switch {
		case i > 0 && a.NewlineBefore:
			e.w.Newline()

			if a.BlankBefore {
				e.w.RequestBlankLine()
			}
		case i > 0 && a.SpaceBefore:
			e.w.Space()
		}

		e.emitInlineAtomHanging(atoms, i, closer, hangLast)
	}
}

// emitInlineAtomHanging writes atoms[i]. When the next atom is glued to it and
// the pair overflows, a break there would render as a space, so an element
// atom hangs its closing bracket instead, as Prettier breaks inside the tag.
func (e *emitter) emitInlineAtomHanging(atoms []htmlfmt.InlineAtom, i, closer int, hangLast bool) {
	el, ok := atoms[i].Node.(*htmlparser.Element)
	if !ok || !e.hangHelps(el, atoms, i, closer, hangLast) {
		e.emitInlineAtom(atoms[i])

		return
	}

	prevHang, prevAtTag := e.hangClose, e.hangCloseAtTag
	e.hangClose = el

	if i+1 == len(atoms) {
		e.hangCloseAtTag = el
	}

	e.emitInlineAtom(atoms[i])
	e.hangClose, e.hangCloseAtTag = prevHang, prevAtTag
}

// An element the glued-body layout takes closes on its own terms. One that
// overflows by itself keeps its line long either way, so it hangs only to keep
// a glued element after it from breaking inside its own tag.
func (e *emitter) hangHelps(el *htmlparser.Element, atoms []htmlfmt.InlineAtom, i, closer int, hangLast bool) bool {
	last := i+1 == len(atoms)

	if last && (!hangLast || closer == 0) || !last && atoms[i+1].SpaceBefore {
		return false
	}

	if run, wraps := e.gluedTagWrap(el, false); wraps && !e.fitsAsSpacedText(el, run) {
		return false
	}

	alone := func() { e.emitInlineAtom(atoms[i]) }
	if el.SelfClose {
		alone = func() { e.emitOpenTagFlat(el, el.Attributes, true, true) }
	}

	// An atom the author broke across lines ends on its own last line, which a
	// flat measure cannot judge; the hang would flatten the body it measured.
	if !e.w.MeasureOneLine(alone) {
		return false
	}

	// The parent's close tag takes a last child's `/>` whether or not its
	// attributes break, as Prettier borrows it.
	if !e.w.Measure(alone) {
		return last && el.SelfClose || !last && isElementAtom(atoms[i+1])
	}

	// Under a hung `>` Prettier always puts a self-closing last child's `/>`
	// on its own line with the parent's close tag, fitting or not.
	if last {
		return el.SelfClose || !e.w.Measure(func() {
			e.w.WithTrailing(closer, func() { e.emitInlineAtom(atoms[i]) })
		})
	}

	trailing := 0
	if i+2 == len(atoms) {
		trailing = closer
	}

	return !e.w.Measure(func() {
		e.w.WithTrailing(trailing, func() {
			e.emitInlineAtom(atoms[i])
			e.emitInlineAtom(atoms[i+1])
		})
	})
}

// emitOpenTag writes the tag flat when it fits printWidth, else one attribute
// per line. A non-nil tail is what the flat tag would share its line with, so
// it counts toward the fit. Attributes stay in source order: reordering would
// risk the runtime behavior of framework directives (Alpine/Livewire/Vue) and
// discard the author's grouping. The return reports whether the flat layout
// won, so the caller knows if inline children are safe.
func (e *emitter) emitOpenTag(el *htmlparser.Element, selfClose, hangBracket bool, tail func()) bool {
	attrs := el.Attributes

	fit := false

	// Pint's CollapseSingleAttribute joins a lone attribute back onto a tag
	// that starts its line, whatever the width; the value must print on one
	// line, as Pint counts no attribute in a value open across lines.
	if len(attrs) == 1 && !hangBracket && !attrsMustBreak(attrs) &&
		e.w.CursorColumn() <= e.w.IndentColumn() && e.foldsLoneAttribute(el, selfClose) {
		return true
	}

	// A block element's body takes its own lines once the tag breaks, its edge
	// whitespace not rendering; only an inline element can keep a glued body.
	// An empty body puts `></tag>` there, which Pint does not fold either.
	hasBody := len(el.Children) > 0 && !allWhitespaceChildren(el.Children)
	bodyOnOwnLines := selfClose || hasBody && (!edgesRender(el) || startsWithWhitespace(el.Children[0]))

	// The directive writes its built argument as text, which a flat probe does
	// not see break; one that wraps before a glued body stays on its own line.
	if len(attrs) == 1 && attrs[0].Directive != nil && !bodyOnOwnLines &&
		!e.w.MeasureOneLine(func() { e.emitAttr(attrs[0], parser.IsComponentTag(el.Name)) }) {
		e.emitOpenTagMultiline(el, attrs, selfClose, hangBracket)

		return false
	}

	// Pint also folds a lone Blade construct attribute that wraps
	// (`<span @class([` … `])>`): it opens on the tag's line and its body
	// takes the tag's indent. Only where the `>` ends its line: a body glued
	// to it leaves Pint nothing to fold.
	if len(attrs) == 1 && !hangBracket && !attrsMustBreak(attrs) && attrs[0].Directive != nil && bodyOnOwnLines &&
		e.w.CursorColumn() <= e.w.IndentColumn() {
		if opens, closes, branch := e.attrDirectiveRole(attrs[0]); !opens && !closes && !branch {
			start := len(e.w.Bytes())

			e.writeOpenTagName(el)
			e.w.Space()
			e.emitAttr(attrs[0], parser.IsComponentTag(el.Name))

			if selfClose {
				e.w.Space()
				e.w.Write("/>")
			} else {
				htmlfmt.CloseBracket(e.w)
			}

			return !bytes.ContainsAny(e.w.Bytes()[start:], "\r\n")
		}
	}

	// A `/>` hung for a glued sibling belongs to the attributes' group, as
	// Prettier prints it: it leaves the tag's line only when every attribute
	// does. Before the parent's close tag the attributes stay on the line.
	hungSelfClose := hangBracket && el.SelfClose && len(attrs) > 0 && e.hangCloseAtTag != el

	if !hungSelfClose && !attrsMustBreak(attrs) && (tail == nil || e.w.Measure(func() {
		e.emitOpenTagFlat(el, attrs, selfClose, hangBracket)
		tail()
	})) {
		fit = e.w.TryFlat(func() {
			e.emitOpenTagFlat(el, attrs, selfClose, hangBracket)
		})
	}

	if fit {
		return true
	}

	e.emitOpenTagMultiline(el, attrs, selfClose, hangBracket)

	return false
}

// gluedBodyTail returns the body and close tag an element's open tag shares its
// line with, or nil. Only an element whose edges render qualifies, with a body
// whose glue renders (isInlineLevel, as the body routing reads it): that body
// cannot move without a rendered space, so the break goes between
// attributes, as Pint puts it. A lone attribute stays on a line the tag starts,
// as Pint's CollapseSingleAttribute folds only such a line.
func (e *emitter) gluedBodyTail(el *htmlparser.Element) func() {
	if el.SelfClose || el.Loose || len(el.Attributes) == 0 {
		return nil
	}

	if len(el.Attributes) == 1 && e.w.CursorColumn() <= e.w.IndentColumn() {
		return nil
	}

	if len(el.Children) == 0 || allWhitespaceChildren(el.Children) {
		return func() {
			e.w.Write("</")
			e.writeCloseTagName(el)
			e.w.Write(">")
		}
	}

	if !edgesRender(el) ||
		bodyIsVerbatim(el) || !bodyCopyable(el) || !isInlineLevel(el.Children[0]) || startsWithWhitespace(el.Children[0]) {
		return nil
	}

	// A body with a block child leaves the open tag's line anyway, so breaking
	// the attributes to make room for it gains nothing.
	if e.bodyHasLineBreak(el) || !e.isInlineContent(el.Children, false) {
		return nil
	}

	tail := func() {
		e.w.WithTrailing(htmlfmt.CloseTagWidth(el), func() {
			for _, c := range el.Children {
				if t, ok := c.(*htmlparser.Text); ok {
					e.w.WriteBytes(t.Text)

					continue
				}

				e.emitNode(c)
			}
		})
	}

	// The source test reads a lone CR inside PHP as a byte, yet a `#` comment
	// ends there and the echo prints over lines; the next pass sees that break.
	if !e.w.MeasureOneLine(tail) {
		return nil
	}

	return tail
}

// foldsLoneAttribute writes the tag on one line if its lone attribute prints on
// one line laid out where Prettier puts it, on its own line, before Pint joins
// it to the tag. Laid out at the tag line's column instead, a value that keeps
// an author break only past the width would break here, print on one line on
// its own line, and fold on the next pass.
func (e *emitter) foldsLoneAttribute(el *htmlparser.Element, selfClose bool) bool {
	e.w.Indent()
	ownLine := e.w.IndentColumn()
	e.w.Dedent()

	tagLine := e.w.IndentColumn() + e.openTagNameWidth(el) + len(" ")

	savedWidth := e.w.PrintWidth()
	e.w.SetPrintWidth(savedWidth + tagLine - ownLine)
	defer e.w.SetPrintWidth(savedWidth)

	return e.w.TryOneLine(func() {
		e.emitOpenTagFlat(el, el.Attributes, selfClose, false)
	})
}

// attrsMustBreak reports whether the tag has to take the one-attribute-per-line
// layout to keep its meaning. A bare directive attribute followed by an attribute
// name opening with `(` is the case: Blade binds `[ \t]*(` after a directive name as
// its arguments, so a space between them makes the next attribute those arguments,
// while a line break leaves both as the author wrote them.
func attrsMustBreak(attrs []*htmlparser.Attr) bool {
	for i, a := range attrs[:max(len(attrs)-1, 0)] {
		dir, ok := a.Directive.(*parser.DirectiveNode)
		if !ok || dir.LParen.End.Offset > dir.LParen.Start.Offset {
			continue
		}

		if next := attrs[i+1]; len(next.Name) > 0 && next.Name[0] == '(' {
			return true
		}
	}

	return false
}

func (e *emitter) writeOpenTagName(el *htmlparser.Element) {
	e.w.Write("<")
	e.w.WriteBytes(el.Name)

	if n, ok := el.NameEcho.(parser.Node); ok {
		e.emitNode(n)
	}
}

func (e *emitter) openTagNameWidth(el *htmlparser.Element) int {
	return len("<") + len(el.Name) + e.gluedEchoWidth(el.NameEcho)
}

func (e *emitter) gluedEchoWidth(echo htmlparser.Node) int {
	n, ok := echo.(parser.Node)
	if !ok {
		return 0
	}

	start := e.w.CursorColumn()
	width := 0

	e.w.MeasureOneLine(func() {
		e.emitNode(n)
		width = e.w.CursorColumn() - start
	})

	return width
}

func (e *emitter) emitOpenTagFlat(el *htmlparser.Element, attrs []*htmlparser.Attr, selfClose, hangBracket bool) {
	e.writeOpenTagName(el)

	for _, attr := range attrs {
		e.w.Space()
		e.emitAttr(attr, parser.IsComponentTag(el.Name))
	}

	if hangBracket {
		return
	}

	if selfClose {
		e.w.Space()
		e.w.Write("/>")
	} else {
		htmlfmt.CloseBracket(e.w)
	}
}

func (e *emitter) emitOpenTagMultiline(el *htmlparser.Element, attrs []*htmlparser.Attr, selfClose, hangBracket bool) {
	e.writeOpenTagName(el)
	e.w.Indent()

	// A block directive in attribute position (`@if ($cond) style="…" @endif`)
	// is a flat run of attributes, not a parsed block, so the body is indented
	// manually between opener and closer. bodyIndent tracks the depth so a
	// missing closer (unvalidated here) still unwinds.
	bodyIndent := 0
	component := parser.IsComponentTag(el.Name)

	for i := 0; i < len(attrs); i++ {
		attr := attrs[i]
		opens, closes, branch := e.attrDirectiveRole(attr)

		if (closes || branch) && bodyIndent > 0 {
			e.w.Dedent()
			bodyIndent--
		}

		e.w.Newline()

		if end, ok := e.authoredOneLineAttrRun(attrs, i); ok && e.w.TryFlat(func() { e.emitAttrRun(attrs[i:end+1], component) }) {
			i = end

			continue
		}

		e.emitAttr(attr, component)

		if opens || branch {
			e.w.Indent()
			bodyIndent++
		}
	}

	for ; bodyIndent > 0; bodyIndent-- {
		e.w.Dedent()
	}

	e.w.Dedent()

	if hangBracket {
		return
	}

	e.w.Newline()

	if selfClose {
		e.w.Write("/>")
	} else {
		e.w.Write(">")
	}
}

// authoredOneLineAttrRun finds the closer of a block directive opening at
// attrs[i] (`@if ($x) wire:sort="…" @endif`) that the author wrote on one
// line, with no nested block inside it.
func (e *emitter) authoredOneLineAttrRun(attrs []*htmlparser.Attr, i int) (int, bool) {
	if opens, _, branch := e.attrDirectiveRole(attrs[i]); !opens || branch {
		return 0, false
	}

	for j := i + 1; j < len(attrs); j++ {
		opens, closes, branch := e.attrDirectiveRole(attrs[j])

		switch {
		case opens && !branch:
			return 0, false
		case closes:
			run := e.src[attrs[i].Start.Offset:attrs[j].End.Offset]

			return j, !bytes.ContainsAny(run, "\n\r")
		}
	}

	return 0, false
}

func (e *emitter) emitAttrRun(run []*htmlparser.Attr, component bool) {
	for i, attr := range run {
		if i > 0 {
			e.w.Space()
		}

		e.emitAttr(attr, component)
	}
}

// directiveNamed reports whether the attribute name is `@`-prefixed, so dropping a
// trailing `=` would leave a bare directive behind.
func directiveNamed(name []byte) bool {
	rest, ok := bytes.CutPrefix(name, []byte("@"))

	return ok && len(rest) > 0
}

// echoNamed reports whether the attribute name opens with a Blade echo, which is
// what an attribute-bag looks like until an `=` says otherwise.
func echoNamed(name []byte) bool {
	return bytes.HasPrefix(name, []byte("{{")) || bytes.HasPrefix(name, []byte("{!!"))
}

func (e *emitter) attrDirectiveRole(a *htmlparser.Attr) (opens, closes, branch bool) {
	dir, ok := a.Directive.(*parser.DirectiveNode)
	if !ok {
		return false, false, false
	}

	name := string(dir.Name)

	return e.dirs.IsOpener(name),
		e.dirs.IsCloser(name),
		e.dirs.IsBranch(name)
}

func (e *emitter) emitAttr(a *htmlparser.Attr, component bool) {
	if len(a.RawPhp) > 0 {
		// The island is written as one piece to keep the layout the tag gave
		// it, but its terminators are still the file's, so they follow the
		// configured end of line; written as they came, a `\r\n` would survive
		// into an LF file until the next pass normalized it.
		e.w.WriteBytes(formatter.NormalizeLineTerminators(a.RawPhp, e.w.EOL()))

		return
	}

	if n, ok := a.Directive.(parser.Node); ok {
		e.emitNode(n)

		return
	}

	if a.Bound {
		e.w.Write(":")
	}

	if a.Shorthand {
		e.w.Write("$")
	}

	e.w.WriteBytes(a.Name)

	if a.Shorthand {
		return
	}

	// Quote is unset for a flag attribute and an unquoted value alike, so the
	// `=` tells them apart; reading Quote alone would turn
	// `<a href=index.html>` into `<a href>`.
	if a.Quote == 0 && (a.Eq.End.Offset == a.Eq.Start.Offset || len(a.Value) == 0) {
		// An `@`-named attribute keeps an `=` the author wrote, even with no
		// value: without it the name is a bare directive again and what follows
		// binds to it. A name opening with an echo keeps it too, since the `=`
		// is what makes `{{ $name }}="v"` a composed name rather than the
		// attribute-bag idiom. The empty value is written as `=""`, because
		// HTML skips the whitespace after a bare `=` and would take the next
		// attribute as its value.
		authored := a.Eq.End.Offset > a.Eq.Start.Offset

		if authored && (directiveNamed(a.Name) || echoNamed(a.Name)) {
			e.w.Write(`=""`)
		}

		return
	}

	e.w.Write("=")

	// An unquoted value adopts a delimiter, so every pass a quoted value gets
	// runs on it in the same pass, or `x-data={open:false}` would take two
	// passes to settle. A value that cannot adopt one goes out byte-exact, so
	// no pass below can add a space and move the end of the value.
	quote := a.Quote
	if quote == 0 {
		quote = e.unquotedDelimiter(a)

		if quote == 0 {
			e.w.WriteBytes(a.Value)

			return
		}
	}

	sortClasses := !a.Bound && e.cfg.Tailwind && htmlfmt.IsClassAttr(a.Name)

	// A formatted expression value is redelimited to a quote it does not contain,
	// since HTML has no escape for the delimiter inside the value: emitting `''`
	// inside `'...'` would fuse into `''''` and re-parse as an empty value.
	if a.Expr != nil {
		// A bound `:class` value is a class expression, so Tailwind-sort the classes
		// in its string literals like a `class="..."` attribute (the whole value
		// resolves to a class list, so every class-position literal is fair game;
		// conditions and function arguments are skipped inside SortClassExpr).
		// Only a value that parsed as PHP reaches here.
		if e.cfg.Tailwind && a.Bound && htmlfmt.IsClassAttr(a.Name) {
			phpfmt.SortClassExpr(a.Expr, e.sortPinned)
		}

		// An array or match value prints at the attribute's indent so its structural
		// line breaks re-home under the attribute (like the @class([...]) directive
		// path). A string literal's own newlines pass through raw, which a byte-level
		// re-home could not tell apart from layout whitespace.
		//
		// Every other value stays on one line, as a Blade echo does: the attribute's
		// column is fixed by the markup, so at depth breaking only ladders a ternary
		// or concat run inside a quoted value.
		indentCols := e.w.IndentColumn()

		formatted := phpfmt.BuildExpr(a.Value, a.ExprTokens, a.Expr, e.cfg, indentCols)
		if phpfmt.ExprHasBlockForm(a.Expr) {
			// The value sits between the quotes, so it starts one column past the
			// cursor and gives another column back to the closing quote.
			formatted = phpfmt.BuildDirectiveArg(a.Value, a.ExprTokens, a.Expr, e.cfg, indentCols, e.w.CursorColumn()+2, e.sourceColumnAt(a.ValueStart), 0)
		}
		quote = attrDelimiterFor(quote, formatted)

		e.w.Write(string(quote))
		e.w.WriteBytes(formatted)
		e.w.Write(string(quote))

		return
	}

	// Blade-echo parts may embed quotes the conflict check cannot see, so the
	// authored delimiter is kept.
	if len(a.Parts) > 0 {
		e.w.Write(string(quote))
		e.emitAttrParts(a, sortClasses, component)
		e.w.Write(string(quote))

		return
	}

	// A multiline value (an Alpine `x-data` object, say) is re-indented so its
	// continuation lines follow the tag rather than keeping their source columns.
	// Tailwind sort and Alpine normalize are single-line only, so they are skipped.
	// A component value compiles into PHP, so it keeps a lone CR as the byte of the
	// string it is.
	if hasValueLineBreak(a.Value, component) {
		if !a.KeepQuote {
			quote = formatter.AttrQuote(quote, a.Value, e.cfg)
		}

		e.w.Write(string(quote))

		switch {
		case !htmlfmt.ValueReflows(a):
			e.w.WithoutIndent(func() { e.writeAttrTextAsWritten(a.Value, component) })
		case component:
			formatter.EmitVerbatimRebasedKeepTrailing(e.w, e.src, a.ValueStart, a.ValueStart+len(a.Value), a.Start.Offset)
		default:
			htmlfmt.EmitMultilineAttrValue(e.w, e.src, a)
		}

		e.w.Write(string(quote))

		return
	}

	value := a.Value
	if sortClasses {
		value = sortClassSegment(e.cfg.TailwindTheme, value, false, false)
	}

	if !a.Bound && alpine.IsAlpineJSAttr(a.Name) {
		if out, ok := alpine.Normalize(value); ok {
			value = out
		}
	}

	if a.KeepQuote {
		e.w.Write(string(quote))
		e.w.WriteBytes(value)
		e.w.Write(string(quote))

		return
	}

	formatter.WriteAttrValue(e.w, quote, value, e.cfg)
}

// unquotedDelimiter returns the quote an unquoted attribute value adopts, or 0
// when it has to go out exactly as authored.
//
// A value the emitter rebuilds (one holding a `{{ }}` echo, or a bound value
// routed through the PHP grammar) can come back carrying a quote the source did
// not have, since PHP string literals are re-quoted to the configured style, and
// that quote would collide with a delimiter chosen from the source. So one is
// adopted there only when the source has no quote at all. Every other value
// passes through untouched apart from Alpine and Tailwind, neither of which
// introduces a quote, so it can also take the alternative delimiter.
func (e *emitter) unquotedDelimiter(a *htmlparser.Attr) byte {
	if a.KeepUnquoted {
		return 0
	}

	if len(a.Parts) > 0 || a.Expr != nil {
		if bytes.ContainsAny(a.Value, `"'`) {
			return 0
		}

		return formatter.MarkupQuoteTarget(e.cfg)
	}

	return formatter.UnquotedAttrDelimiter(a.Value, e.cfg)
}

// attrDelimiterFor returns the quote to delimit an attribute whose formatted value
// is val. It keeps the authored quote unless val contains it (an unescapable
// conflict in HTML) and the alternative quote is free, in which case it switches to
// the alternative. A value carrying both quotes cannot be delimited safely and
// keeps the authored quote unchanged.
func attrDelimiterFor(quote byte, val []byte) byte {
	if bytes.IndexByte(val, quote) < 0 {
		return quote
	}

	alt := byte('"')
	if quote == '"' {
		alt = '\''
	}

	if bytes.IndexByte(val, alt) < 0 {
		return alt
	}

	return quote
}

// emitAttrParts writes the interleaved text-and-echo parts of an attribute
// value. When sortClasses is true, each pure-text segment is sorted on its own:
// sorting across an echo boundary is not attempted because the echo's runtime
// value is unknown, and moving classes past it would change the rendered string.
//
// A multiline value re-homes each text run under the (re-indented) tag, the same
// treatment the echo-free multiline path gives so a continuation line follows the
// attribute instead of stranding at its source column. Sort is skipped there, as
// it is for the echo-free case: it is single-line only. naturalIndent is the
// attribute's own source indent, the amount stripped from each continuation line.
func (e *emitter) emitAttrParts(a *htmlparser.Attr, sortClasses, php bool) {
	// Re-home only when a text run spans lines: that newline is the attribute's
	// layout. A newline inside an echo is content the echo emitter re-lays onto one
	// line, so keying rehome off the whole value flips the decision between passes
	// (pass one keeps the run verbatim, pass two sorts it) once the echo collapses.
	rehome := false
	for _, p := range a.Parts {
		if t, ok := p.(*htmlparser.Text); ok && hasValueLineBreak(t.Text, php) {
			rehome = true

			break
		}
	}

	if rehome && !htmlfmt.ValueReflows(a) {
		e.w.WithoutIndent(func() { e.emitAttrPartsAsWritten(a.Parts, php) })

		return
	}

	naturalIndent := 0
	if rehome {
		naturalIndent = attrPartsStrip(a.Parts, formatter.NaturalIndentBefore(e.src, a.Start.Offset), php)
	}

	// The closing line keeps its offset, as in EmitMultilineAttrValue.
	closerOffset := 0
	if last, ok := a.Parts[len(a.Parts)-1].(*htmlparser.Text); rehome && ok {
		if nl := bytes.LastIndexByte(last.Text, '\n'); nl >= 0 && len(bytes.TrimLeft(last.Text[nl+1:], " \t")) == 0 {
			closerOffset = min(len(last.Text)-nl-1-naturalIndent, 0)
		}
	}

	for i, p := range a.Parts {
		switch pp := p.(type) {
		case *htmlparser.Text:
			if rehome && closerOffset < 0 && i == len(a.Parts)-1 {
				e.writeRebasedAttrText(pp.Text[:bytes.LastIndexByte(pp.Text, '\n')+1], naturalIndent, php)

				column := max(e.w.IndentColumn()+closerOffset, 0)
				e.w.WithoutIndent(func() { e.w.Write(strings.Repeat(" ", column)) })

				continue
			}

			if rehome {
				e.writeRebasedAttrText(pp.Text, naturalIndent, php)

				continue
			}

			text := pp.Text

			if sortClasses {
				// A neighboring part is an echo, so a text run that touches it holds a
				// fragment of the class the echo completes (`bg-{{ $color }}-500`).
				text = sortClassSegment(e.cfg.TailwindTheme, text, i > 0, i < len(a.Parts)-1)
			}

			e.w.WriteBytes(text)
		default:
			e.emitInlineEcho(pp)
		}
	}
}

// attrPartsStrip returns the indent a multi-line value's continuation lines lose:
// the attribute's own, or less when a line sits left of it (a closer at the
// tag's column), so the lines keep their shape as the echo-free path keeps it.
// A line an echo starts counts with the whitespace before the echo.
func attrPartsStrip(parts []htmlparser.Node, natural int, php bool) int {
	strip := natural

	for i, p := range parts {
		t, ok := p.(*htmlparser.Text)
		if !ok {
			continue
		}

		text := t.Text
		for {
			nl, width := valueLineBreak(text, php)
			if nl < 0 {
				break
			}

			text = text[nl+width:]

			ws := 0
			for ws < len(text) && (text[ws] == ' ' || text[ws] == '\t') {
				ws++
			}

			next, _ := valueLineBreak(text, php)
			lineHasContent := ws < len(text) && next != ws || next < 0 && ws == len(text) && i < len(parts)-1
			if lineHasContent {
				strip = min(strip, ws)
			}
		}
	}

	return strip
}

// writeRebasedAttrText emits one text run of a multi-line attribute value,
// re-homing each line the run starts onto the Writer's current indent: up to
// naturalIndent leading whitespace is stripped from a continuation line so deeper
// relative indentation is kept, matching the echo-free multiline path. Whitespace
// before a real line end is dropped (an empty line writes nothing, so no
// indent-only trailing whitespace survives), but the run's final tail is preserved
// verbatim: it abuts an echo or the closing quote, where a space is part of the
// value, not layout, and trimming it would corrupt the value (`'x {{ $n }}'`).
func (e *emitter) emitAttrPartsAsWritten(parts []htmlparser.Node, php bool) {
	for _, p := range parts {
		if t, ok := p.(*htmlparser.Text); ok {
			e.writeAttrTextAsWritten(t.Text, php)
		} else {
			e.emitInlineEcho(p)
		}
	}
}

// The caller clears the writer's indent first.
func (e *emitter) writeAttrTextAsWritten(text []byte, php bool) {
	for {
		nl, width := valueLineBreak(text, php)
		if nl < 0 {
			e.w.WriteBytes(text)

			return
		}

		// A CR before the break reads back as part of it, as the verbatim path
		// drops it.
		e.w.WriteBytes(bytes.TrimRight(text[:nl], "\r"))
		e.w.Newline()
		text = text[nl+width:]
	}
}

func (e *emitter) writeRebasedAttrText(text []byte, naturalIndent int, php bool) {
	for {
		nl, width := valueLineBreak(text, php)
		if nl < 0 {
			if len(text) > 0 {
				e.w.WriteBytes(text)
			}

			return
		}

		if line := bytes.TrimRight(text[:nl], " \t"); len(line) > 0 {
			e.w.WriteBytes(line)
		}

		e.w.Newline()

		rest := text[nl+width:]

		strip := 0
		for strip < naturalIndent && strip < len(rest) && (rest[strip] == ' ' || rest[strip] == '\t') {
			strip++
		}

		text = rest[strip:]
	}
}

func (e *emitter) sortPinned(classes string, pinFirst, pinLast bool) string {
	return tailwind.SortPinned(e.cfg.TailwindTheme, classes, pinFirst, pinLast)
}

// sortClassSegment runs tailwind.Sort on a text segment's inner content,
// preserving leading and trailing whitespace verbatim. The boundary space
// matters because the segment abuts an echo: dropping it would collapse
// `px-4 bg-white {{ $x }}` to `px-4 bg-white{{ $x }}`.
func sortClassSegment(theme *tailwind.Theme, text []byte, pinFirst, pinLast bool) []byte {
	// A real class never contains a brace (arbitrary values use `[...]`). A segment
	// that does could, once reordered, place a token ending in `{` against a
	// following `{{ ... }}` echo (or `}` before `}}`), fusing into `{{{`/`}}}` that
	// reparses differently. Leave such a segment untouched.
	if bytes.ContainsAny(text, "{}") {
		return text
	}

	inner := bytes.Trim(text, tailwind.ClassSeparators)

	if len(inner) == 0 {
		return text
	}

	before, after, ok := bytes.Cut(text, inner)

	if !ok {
		return text
	}

	leading := before
	trailing := after

	// Edge whitespace ends the fragment, so the token beside it is a whole class.
	pinFirst = pinFirst && len(leading) == 0
	pinLast = pinLast && len(trailing) == 0

	sorted := sortAroundDirectives(theme, inner, pinFirst, pinLast)

	out := make([]byte, 0, len(leading)+len(sorted)+len(trailing))
	out = append(out, leading...)
	out = append(out, sorted...)
	out = append(out, trailing...)

	return out
}

// sortAroundDirectives sorts each run of plain classes independently, keeping
// every Blade directive (`@if (...)`, `@else`, `@endif`) fixed in place: a
// directive bounds a conditional region, so moving a class across it would
// change which condition guards the class. Runs inside a branch still sort
// among themselves.
func sortAroundDirectives(theme *tailwind.Theme, inner []byte, pinFirst, pinLast bool) string {
	if bytes.IndexByte(inner, '@') < 0 {
		return tailwind.SortPinned(theme, string(inner), pinFirst, pinLast)
	}

	var parts []string
	var run []byte

	sawDirective := false

	flush := func() {
		if len(run) > 0 {
			parts = append(parts, tailwind.Sort(theme, string(run)))
			run = run[:0]
		}
	}

	i := 0
	for i < len(inner) {
		for i < len(inner) && tailwind.IsClassSeparator(inner[i]) {
			i++
		}

		if i >= len(inner) {
			break
		}

		if end, ok := directiveEnd(inner, i); ok {
			flush()
			parts = append(parts, string(inner[i:end]))
			sawDirective = true
			i = end

			continue
		}

		j := i
		for j < len(inner) && !tailwind.IsClassSeparator(inner[j]) {
			j++
		}

		if len(run) > 0 {
			run = append(run, ' ')
		}

		run = append(run, inner[i:j]...)
		i = j
	}

	flush()

	// Every `@` belonged to a class token (a container query), so the segment is
	// an ordinary class list: sort the original text, honoring the pinned edges
	// the echo-adjacent caller asked for.
	if !sawDirective {
		return tailwind.SortPinned(theme, string(inner), pinFirst, pinLast)
	}

	// A directive can open or close the segment, so a pinned edge token is not
	// simply the first or last word: leave a segment that has both alone.
	if pinFirst || pinLast {
		return string(inner)
	}

	return strings.Join(parts, " ")
}

// directiveEnd reports whether a Blade directive starts at token position i
// and returns the offset just past it, including a balanced argument list
// (`@if ($tab !== 'trash')`). Unbalanced arguments disqualify the token.
func directiveEnd(b []byte, i int) (int, bool) {
	if b[i] != '@' {
		return 0, false
	}

	j := i + 1
	if j >= len(b) || !isASCIILetter(b[j]) {
		return 0, false
	}

	for j < len(b) && (isASCIILetter(b[j]) || b[j] >= '0' && b[j] <= '9' || b[j] == '_') {
		j++
	}

	// The name has to end the token. Blade compiles `@name`, `@name(...)`, and
	// `@name (...)`, so a token that runs on into `:`, `-`, or `/` is one class
	// whose leading `@` is Tailwind's container query (`@md:flex`,
	// `@container/main`), not a directive with a suffix.
	if j < len(b) && !tailwind.IsClassSeparator(b[j]) && b[j] != '(' {
		return 0, false
	}

	// A bare `@name` the Tailwind registry knows is a utility, not a directive:
	// `@container` compiles to nothing, so pinning it would only stop its
	// neighbors sorting across it. An unknown name stays pinned, because Blade
	// compiles every registered directive (`@empty` becomes real PHP) and the
	// registered set is open-ended.
	if tailwind.IsKnownUtility(string(b[i:j])) {
		return 0, false
	}

	k := j
	for k < len(b) && tailwind.IsClassSeparator(b[k]) {
		k++
	}

	// A balanced `@name(...)` consumes its argument list. An unbalanced `(` is not an
	// argument list, so the directive is the bare `@name`; treating it as "not a
	// directive" (balancedParenEnd's false) would flip the token's boundary once
	// sorting slid a stray `(` next to the name, breaking idempotence.
	if k < len(b) && b[k] == '(' {
		if end, ok := balancedParenEnd(b, k); ok {
			return end, true
		}
	}

	return j, true
}

// balancedParenEnd returns the offset just past the `(...)` group opening at
// open, skipping parens inside quoted strings so `@if ($a === ')')` closes at
// the real end.
func balancedParenEnd(b []byte, open int) (int, bool) {
	depth := 0

	var quote byte

	for i := open; i < len(b); i++ {
		c := b[i]

		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}

			continue
		}

		switch c {
		case '\'', '"':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}

	return 0, false
}

func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func (e *emitter) emitCloseTag(el *htmlparser.Element) {
	e.w.Write("</")
	e.writeCloseTagName(el)

	if e.hangClose != el {
		e.w.Write(">")

		return
	}

	e.writeHungCloser(el, ">")
}

func (e *emitter) closeHungOpenTag(el *htmlparser.Element, selfClose bool) {
	if selfClose {
		e.writeHungCloser(el, "/>")
	} else {
		e.writeHungCloser(el, ">")
	}
}

func (e *emitter) writeHungCloser(el *htmlparser.Element, closer string) {
	atTag := e.hangCloseAtTag == el
	if atTag {
		e.w.Dedent()
	}

	e.w.Newline()
	e.w.Write(closer)

	if atTag {
		e.w.Indent()
	}
}

func (e *emitter) closeTagWidth(el *htmlparser.Element) int {
	return len("</") + len(e.closeTagName(el)) + e.gluedEchoWidth(el.CloseNameEcho) + len(">")
}

func (e *emitter) writeCloseTagName(el *htmlparser.Element) {
	name := e.closeTagName(el)
	e.w.WriteBytes(name)

	if n, ok := el.CloseNameEcho.(parser.Node); ok {
		e.emitNode(n)
	}
}

// closeTagName is the author's spelling of the close tag, or the open tag's name
// when the parser had to close the element for them.
func (e *emitter) closeTagName(el *htmlparser.Element) []byte {
	if len(el.CloseName) == 0 {
		return el.Name
	}

	return el.CloseName
}

func edgeBroken(children []htmlparser.Node) bool {
	if len(children) == 0 {
		return false
	}

	if first, ok := children[0].(*htmlparser.Text); ok {
		lead := first.Text[:len(first.Text)-len(bytes.TrimLeft(first.Text, htmlfmt.CollapsibleSpace))]
		if htmlfmt.HasLineBreak(lead) {
			return true
		}
	}

	last, ok := children[len(children)-1].(*htmlparser.Text)

	return ok && htmlfmt.HasLineBreak(last.Text[len(bytes.TrimRight(last.Text, htmlfmt.CollapsibleSpace)):])
}

// inlineChildrenAuthoredMultiline reports whether the children carry a newline; such
// content reindents via emitBlockInlineChildren, since emitInlineChildren writes text
// verbatim (leaving a child at its source column).
func inlineChildrenAuthoredMultiline(children []htmlparser.Node) bool {
	for _, c := range children {
		if t, ok := c.(*htmlparser.Text); ok && htmlfmt.HasLineBreak(t.Text) {
			return true
		}
	}

	return false
}

// emitInlineOrBlockChildren keeps an element's inline children on the open-tag
// line when the whole line (open tag, children, close tag) fits the print
// width. When it overflows, the children move to an indented block line if the
// element is block-level and white-space:normal, so its edge whitespace
// collapses. An inline container keeps its edges, since a block break would
// inject a rendered space there, unless both already hold one the break only
// replaces, or the element is a component whose slot is trimmed, where no edge
// whitespace renders. The move is made even when the children overflow there
// too, as Prettier and Pint make it: the line still gets shorter.
func (e *emitter) emitInlineOrBlockChildren(el *htmlparser.Element) {
	if e.emitInlineChildrenIfFit(el) {
		return
	}

	if !htmlfmt.IsInlineContainer(el.Name) || edgesAlreadySpaced(el.Children) || parser.TrimsSlot(el.Name) {
		e.emitBlockInlineChildren(el)

		return
	}

	e.emitInlineChildren(el)
}

// emitInlineChildrenIfFit emits the element's inline children on the current line
// and reports whether they, plus the close tag that follows, fit the print width;
// when they do not it rolls back, leaving nothing emitted. The close tag is written
// by emitCloseTag, not here, but shares the line, so its width is reserved.
func (e *emitter) emitInlineChildrenIfFit(el *htmlparser.Element) bool {
	return e.w.TryFlat(func() {
		e.w.WithTrailing(htmlfmt.CloseTagWidth(el), func() {
			e.emitInlineChildren(el)
		})
	})
}

func (e *emitter) emitInlineChildren(el *htmlparser.Element) {
	e.emitInlineChildNodes(el.Children)
}

func (e *emitter) emitInlineChildNodes(children []htmlparser.Node) {
	for _, c := range children {
		switch cc := c.(type) {
		case *htmlparser.Text:
			e.w.WriteBytes(cc.Text)
		case *htmlparser.Element, *htmlparser.Comment:
			e.emitNode(cc)
		default:
			e.emitInlineEcho(cc)
		}
	}
}

func (e *emitter) emitBlockInlineChildren(el *htmlparser.Element) {
	e.w.Indent()
	e.w.Newline()

	for i, line := range authoredLines(el.Children) {
		if i > 0 {
			e.w.Newline()
		}

		e.emitAuthoredLine(line)
	}

	e.w.Dedent()
	e.w.Newline()
}

// authoredLines splits a body into the lines its author wrote. Only the run's
// outer edges are trimmed, not each text node: whitespace between adjacent
// text nodes (or around an echo) is content, so per-node trimming would drop
// it and `@ A0!` would collapse to a directive on the next pass.
func authoredLines(children []htmlparser.Node) [][]htmlparser.Node {
	last := len(children) - 1

	lines := [][]htmlparser.Node{nil}

	for i, c := range children {
		text, ok := c.(*htmlparser.Text)
		if !ok {
			lines[len(lines)-1] = append(lines[len(lines)-1], c)

			continue
		}

		rest := text.Text

		if i == 0 {
			rest = bytes.TrimLeft(rest, htmlfmt.CollapsibleSpace)
		}

		if i == last {
			rest = bytes.TrimRight(rest, htmlfmt.CollapsibleSpace)
		}

		for {
			nl, width := htmlfmt.LineBreak(rest)
			if nl < 0 {
				lines[len(lines)-1] = append(lines[len(lines)-1], &htmlparser.Text{Text: rest})

				break
			}

			lines[len(lines)-1] = append(lines[len(lines)-1], &htmlparser.Text{Text: bytes.TrimRight(rest[:nl], " \t")})
			lines = append(lines, nil)
			rest = bytes.TrimLeft(rest[nl+width:], " \t")
		}
	}

	return lines
}

// authoredLinesFit reports whether every line fits flat at the body's indent,
// one level in from the open tag the cursor is on.
func (e *emitter) authoredLinesFit(lines [][]htmlparser.Node) bool {
	e.w.Indent()
	budget := e.w.PrintWidth() - e.w.IndentColumn()
	e.w.Dedent()

	savedWidth := e.w.PrintWidth()
	e.w.SetPrintWidth(e.w.CursorColumn() + budget)
	defer e.w.SetPrintWidth(savedWidth)

	for _, line := range lines {
		if !e.w.Measure(func() { e.emitInlineChildNodes(line) }) {
			return false
		}
	}

	return true
}

func (e *emitter) emitAuthoredLine(line []htmlparser.Node) {
	write := func() { e.emitInlineChildNodes(line) }

	// A blank line the author left gets no indent written onto it.
	if !slices.ContainsFunc(line, func(n htmlparser.Node) bool {
		t, ok := n.(*htmlparser.Text)

		return !ok || len(t.Text) > 0
	}) {
		return
	}

	if e.w.TryFlat(write) {
		return
	}

	if !e.w.MeasureOneLine(write) {
		write()

		return
	}

	atoms, _, _ := htmlfmt.InlineAtoms(line)
	e.fillInlineAtoms(atoms, 0, false)
}

func (e *emitter) emitStructuralChildren(el *htmlparser.Element) {
	nodes := make([]parser.Node, len(el.Children))
	for i, c := range el.Children {
		nodes[i] = c
	}

	if e.isDocumentHTML(el) {
		e.emitInterleaved(nodes, true)
		e.w.Newline()

		return
	}

	e.w.Indent()
	e.emitInterleaved(nodes, true)
	e.w.Dedent()
	e.w.Newline()
}

// isDocumentHTML reports whether el is a root <html> whose only elements are
// <head> and <body>, which then stay at its column: prettier-plugin-blade, and
// so Pint, lays a page out that way.
func (e *emitter) isDocumentHTML(el *htmlparser.Element) bool {
	if !bytes.EqualFold(el.Name, []byte("html")) || !e.isRoot(el) {
		return false
	}

	sections := 0

	for _, c := range el.Children {
		child, ok := c.(*htmlparser.Element)
		if !ok {
			continue
		}

		if !bytes.EqualFold(child.Name, []byte("head")) && !bytes.EqualFold(child.Name, []byte("body")) {
			return false
		}

		sections++
	}

	return sections > 0
}

func (e *emitter) isRoot(el *htmlparser.Element) bool {
	for _, n := range e.roots {
		if root, ok := n.(*htmlparser.Element); ok && root == el {
			return true
		}
	}

	return false
}

// emitRehomedGluedBody emits a whitespace-sensitive block element's body from its
// source span, re-homed under the (re-indented) open tag. The body carries a glued
// inline boundary so it cannot reflow structurally, but the element is
// white-space:normal, so EmitVerbatim keeps every inner byte exact (glued
// boundaries and relative indentation) while re-basing the block's leading indent.
func (e *emitter) emitRehomedGluedBody(el *htmlparser.Element) {
	start := el.OpenClose.End.Offset
	end := el.CloseOpen.Start.Offset

	// Drop the source newlines/indent around the content; the layout below owns the
	// break after the open tag and before the close tag.
	for start < end && htmlfmt.IsCollapsibleSpace(e.src[start]) {
		start++
	}

	for end > start && htmlfmt.IsCollapsibleSpace(e.src[end-1]) {
		end--
	}

	// The trim must not cut into a child: an echo owns the lone CR after its
	// closer, and trimming that CR as whitespace would copy the echo raw on
	// this pass and rebuild it on the next.
	for _, c := range el.Children {
		if _, text := c.(*htmlparser.Text); text {
			continue
		}

		if span := c.Span(); span.Start.Offset < end && span.End.Offset > end {
			end = span.End.Offset
		}
	}

	if start >= end {
		return
	}

	e.w.Indent()
	e.w.Newline()

	// The trim above took the author's blank line with the leading whitespace.
	// Structural children keep that blank, so it is restored here, or the same
	// body would render differently depending on which layout it lands in.
	if htmlfmt.CountLineBreaks(e.src[el.OpenClose.End.Offset:start]) >= 2 {
		e.w.RequestBlankLine()
	}

	// The body is HTML around PHP, so a lone CR in the markup is read as the LF
	// it is; copied on LF alone it would stay a byte where its LF twin is a
	// line break.
	if !e.emitGluedBodyNormalized(el.Children, start, end, true, 0) {
		body := e.src[start:end]
		php := phpSpansFor(body, el.Children)
		lines := htmlfmt.NormalizeLineBreaksIn(body, start, php)
		nested := linesInsideChildElements(body, start, php, el.Children)

		formatter.EmitVerbatimRehomed(e.w, lines, formatter.NaturalIndentBefore(e.src, start), func(n int) bool {
			return nested[n]
		})
	}

	e.w.Dedent()
	e.w.Newline()
}

// linesInsideChildElements reports, per line of body (source offset base), whether
// the line starts inside one of children's elements, where that element's own
// layout placed it.
func linesInsideChildElements(body []byte, base int, php []htmlparser.Span, children []htmlparser.Node) []bool {
	inside := []bool{false}

	for pos := 0; ; {
		at, width := htmlfmt.LineBreakIn(body[pos:], base+pos, php)
		if at < 0 {
			return inside
		}

		pos += at + width
		offset := base + pos

		inside = append(inside, slices.ContainsFunc(children, func(c htmlparser.Node) bool {
			el, ok := c.(*htmlparser.Element)

			return ok && el.Span().Start.Offset < offset && offset < el.Span().End.Offset
		}))
	}
}

// emitGluedBodyNormalized rebuilds each echo and directive of a single-line
// glued body and copies the text between them byte for byte, since that
// whitespace renders. Only PHP is reformatted; a wrapped directive breaks
// inside its PHP, which does not render. oneLine is for a caller whose next
// pass would re-home a multi-line body: there a rebuild that breaks the line
// is dropped and the body is copied as written, or the layout would move.
//
// A descendant open tag the author broke is rebuilt flat too: a break inside a
// tag does not render. The body must then fit the width, with trailing columns
// for what shares its last line, or the author's breaks stand; a tag broken
// for its length keeps its break.
func (e *emitter) emitGluedBodyNormalized(children []htmlparser.Node, start, end int, oneLine bool, trailing int) bool {
	if start >= end {
		return false
	}

	var nodes []htmlparser.Node
	for _, c := range children {
		e.collectRebuildable(c, start, end, &nodes)
	}

	if len(nodes) == 0 || e.lineBreakOutsideTags(children, start, end, nodes) {
		return false
	}

	// A tag whose break keeps `(` from reading as a bare directive's arguments
	// cannot go flat.
	if slices.ContainsFunc(nodes, func(n htmlparser.Node) bool {
		el, ok := n.(*htmlparser.Element)

		return ok && attrsMustBreak(el.Attributes)
	}) {
		return false
	}

	tagRebuilt := slices.ContainsFunc(nodes, isElementNode)

	emit := func() {
		cursor := start

		for _, node := range nodes {
			span := node.Span()
			nodeStart, nodeEnd := span.Start.Offset, span.End.Offset

			if el, ok := node.(*htmlparser.Element); ok {
				nodeStart, nodeEnd = el.Open.Start.Offset, el.OpenClose.End.Offset
			}

			if nodeStart > cursor {
				e.w.WriteBytes(e.src[cursor:nodeStart])
			}

			switch n := node.(type) {
			case *htmlparser.Element:
				e.emitOpenTagFlat(n, n.Attributes, e.selfCloseMarker(n), false)
			case *parser.EchoNode:
				e.emitEcho(n)
			case *parser.RawEchoNode:
				e.emitRawEcho(n)
			case *parser.DirectiveNode:
				e.emitDirective(n)
			}

			cursor = nodeEnd
		}

		if cursor < end {
			e.w.WriteBytes(e.src[cursor:end])
		}
	}

	if tagRebuilt {
		return e.w.TryFlat(func() { e.w.WithTrailing(trailing, emit) })
	}

	if oneLine {
		return e.w.TryOneLine(emit)
	}

	emit()

	return true
}

// An escaped `@{{ }}` or `@@` is literal output, not PHP, so it stays in the
// copied text.
func (e *emitter) collectRebuildable(n htmlparser.Node, start, end int, out *[]htmlparser.Node) {
	span := n.Span()
	if span.Start.Offset < start || span.End.Offset > end {
		return
	}

	switch v := n.(type) {
	case *parser.EchoNode, *parser.RawEchoNode, *parser.DirectiveNode:
		*out = append(*out, n)
	case *htmlparser.Element:
		if bytes.ContainsAny(e.src[v.Open.Start.Offset:v.OpenClose.End.Offset], "\n\r") {
			*out = append(*out, v)
		}

		for _, c := range v.Children {
			e.collectRebuildable(c, start, end, out)
		}
	case *parser.DirectiveBlockNode:
		e.collectRebuildable(v.Opener, start, end, out)

		for _, c := range v.Body {
			e.collectRebuildable(c, start, end, out)
		}

		for _, b := range v.Branches {
			e.collectRebuildable(b.Directive, start, end, out)

			for _, c := range b.Body {
				e.collectRebuildable(c, start, end, out)
			}
		}

		if v.End != nil {
			e.collectRebuildable(v.End, start, end, out)
		}
	}
}

// bodyIsRehomable reports whether a glued block body can be re-homed by the
// byte-level EmitVerbatim: it must span multiple source lines (so re-indenting has
// an effect) while every newline in the body is a structural gap between nodes,
// never inside a string, attribute value, or expression, where stripping a line's
// leading whitespace would corrupt content. Such a body keeps its verbatim source
// columns instead.
func bodyIsRehomable(src []byte, children []htmlparser.Node) bool {
	structural := false

	for _, c := range children {
		if !nodeRehomable(src, c) {
			return false
		}

		// Counted the way the verbatim path writes it: a lone CR in the markup
		// as the LF it is, one inside PHP as a byte. Read any other way, a CR
		// body would come back as an LF one for the next pass to re-home.
		span := c.Span()
		raw := src[span.Start.Offset:span.End.Offset]

		if htmlfmt.HasLineBreakIn(raw, span.Start.Offset, phpSpansFor(raw, []htmlparser.Node{c})) {
			structural = true
		}
	}

	return structural
}

// nodeRehomable reports whether one body node is safe for the byte-level
// EmitVerbatim re-home. A single-line node always is: its only leading whitespace
// is the structural line indent. A multi-line node is safe only when its newlines
// are provably structural gaps between children -- a nested element (with
// single-line attribute values) or a block directive (with single-line arguments)
// whose own children are recursively re-homable. It is default-deny: a multi-line
// echo, raw `<?php ?>` island, `@php` block, or directive with a wrapped argument
// may carry a newline inside a string or expression, so it is kept verbatim.
func nodeRehomable(src []byte, n htmlparser.Node) bool {
	span := n.Span()
	raw := src[span.Start.Offset:span.End.Offset]

	if !htmlfmt.HasLineBreakIn(raw, span.Start.Offset, phpSpansFor(raw, []htmlparser.Node{n})) {
		return true
	}

	switch node := n.(type) {
	case *htmlparser.Text:
		return true
	case *htmlparser.Element:
		component := parser.IsComponentTag(node.Name)

		for _, a := range node.Attributes {
			// A single attribute that spans lines is a multi-line value (a string,
			// a bound expression, or a `<?php ?>` island); a wrapped attribute list
			// keeps each attribute single-line, so this allows that and rejects only
			// a value whose interior whitespace would be corrupted by the re-home.
			if attrSpansLines(src, a, component) {
				return false
			}
		}

		for _, c := range node.Children {
			if !nodeRehomable(src, c) {
				return false
			}
		}

		return true
	case *parser.DirectiveBlockNode:
		if directiveArgSpansLines(src, node.Opener) {
			return false
		}

		for _, c := range node.Body {
			if !nodeRehomable(src, c) {
				return false
			}
		}

		for _, br := range node.Branches {
			if directiveArgSpansLines(src, br.Directive) {
				return false
			}

			for _, c := range br.Body {
				if !nodeRehomable(src, c) {
					return false
				}
			}
		}

		return true
	default:
		return false
	}
}

// directiveArgSpansLines reports whether a directive's parenthesized argument list
// wraps across source lines (a multi-line condition or array), where a re-home
// could reach inside a string literal in the expression.
func directiveArgSpansLines(src []byte, d *parser.DirectiveNode) bool {
	if d == nil || d.RParen.End.Offset == 0 {
		return false
	}

	return bytes.IndexByte(src[d.LParen.Start.Offset:d.RParen.End.Offset], '\n') >= 0
}

// bodyPreservesInnerWhitespace reports whether any descendant renders its own
// leading whitespace literally (a <pre>/<textarea>, or a whitespace-pre* element),
// so re-homing the block would shift that descendant's significant indentation.
// Such a body keeps its verbatim source columns.
func (e *emitter) bodyPreservesInnerWhitespace(children []htmlparser.Node) bool {
	for _, c := range children {
		el, ok := c.(*htmlparser.Element)
		if !ok {
			continue
		}

		if htmlparser.IsPreformattedElement(el.Name) || htmlparser.IsRcdataElement(el.Name) ||
			e.isWhitespacePreservingElement(el) {
			return true
		}

		if e.bodyPreservesInnerWhitespace(el.Children) {
			return true
		}
	}

	return false
}

// emitInterleaved emits a node list mixing block content with inline runs:
// block nodes sit one per line, and an inline run flows on, breaking only at an
// authored blank line. A whitespace-only text node with two or more newlines is
// an authored blank line. Every item but a glued one starts a new line, the
// first only when leadFirst is set. A laravel-fmt-ignore comment makes the
// following item emit verbatim. It reports whether anything was emitted.
func (e *emitter) emitInterleaved(nodes []parser.Node, leadFirst bool) bool {
	var run []parser.Node

	var prev parser.Node

	emitted := false

	// Where the previously emitted item ended, or -1 when whitespace followed
	// it: a newline written at a boundary the author wrote glued renders as a
	// space nobody wrote, so `a<b>x</b>c` would read "a x c".
	prevEnd := -1

	// A blank line separates two items, so it is only asked for once the item
	// on the far side arrives; asked for early, the blank a body ends on would
	// become a blank line above the close tag that the next pass trims away.
	wantBlank := false

	// Unlike an authored blank, this yields to a glued boundary, where a line
	// break would render.
	forceBlank := false

	// A text run before the directive keeps its spacing: in a body Laravel
	// renders as Markdown, a blank line there is a paragraph break.
	prevText := false

	lead := func(next parser.Node) {
		force := forceBlank
		forceBlank = false

		if next == nil {
			if emitted || leadFirst {
				e.w.Newline()
			}

			return
		}

		// Before the first item it separates nothing.
		if wantBlank && emitted {
			e.w.RequestBlankLine()
		}

		wantBlank = false

		start := next.Span().Start.Offset

		// A text node opening with whitespace is not glued: emitInlineNodes trims the
		// run's leading edge, so the separator the author wrote is already gone and the
		// newline is what stands in for it.
		glued := prevEnd >= 0 && start == prevEnd &&
			!startsWithWhitespace(next) && gluableNode(next)

		if parser.GluedLiteralAt(e.src, start) || glued {
			return
		}

		if force && emitted {
			e.w.RequestBlankLine()
		}

		if emitted || leadFirst {
			e.w.Newline()
		}
	}

	emitItem := func(node parser.Node) {
		lead(node)

		broke := e.brokeLines(func() {
			if e.isIgnoreDirectiveComment(prev) {
				e.emitNodeVerbatim(node)
			} else {
				e.emitNode(node)
			}
		})

		prevEnd = -1
		if gluableNode(node) && (!broke || rawTextBody(node)) {
			prevEnd = node.Span().End.Offset
		}

		prev = node
		prevText = false
		emitted = true
	}

	flushRun := func() {
		if !runHasContent(run) {
			// The run held only whitespace, so whatever follows is separated from the
			// last emitted item and may take its own line.
			if len(run) > 0 {
				prevEnd = -1
			}

			run = nil

			return
		}

		// An element alone in its run is an item; filling it would only measure
		// it twice.
		if el := loneElement(run); el != nil {
			emitItem(el)

			run = nil

			return
		}

		// A blank line at the run's edge separates it from the item on the other side,
		// which is a Markdown paragraph break in a mailable body. The run's own trim
		// drops that whitespace, so the separator has to be asked for here.
		if emitted && htmlfmt.TextEdgeBlank(run[0], true) {
			wantBlank = true
		}

		lead(run[0])

		broke := e.brokeLines(func() {
			if e.isIgnoreDirectiveComment(prev) {
				e.emitInlineRunVerbatim(run)
			} else {
				e.emitInlineNodes(run)
			}
		})

		if htmlfmt.TextEdgeBlank(run[len(run)-1], false) {
			wantBlank = true
		}

		prevEnd = runEndForGlue(run, broke)
		prev = run[len(run)-1]
		prevText = true
		emitted = true
		run = nil
	}

	// An alt-syntax `<?php if:?>` run is flat siblings, not a block; bodyIndent indents
	// its body and unwinds an unpaired island, like a block directive in attr position.
	bodyIndent := 0

	for i, node := range nodes {
		switch {
		case isBlankSeparator(node):
			flushRun()

			wantBlank = true
			prevEnd = -1
		case isInlineNode(node) || runElement(node) || gluedElement(nodes, i):
			if len(run) > 0 && authoredBreakAtElement(run[len(run)-1], node) {
				flushRun()
			}

			run = append(run, node)
		default:
			flushRun()

			opens, closes, branch := rawPhpBlockRole(node)

			if (closes || branch) && bodyIndent > 0 {
				e.w.Dedent()
				bodyIndent--
			}

			_, afterComment := prev.(*parser.CommentNode)
			forceBlank = !prevText && !afterComment && e.blankBeforeDirective(nodes[i:])

			emitItem(node)

			if opens || branch {
				e.w.Indent()
				bodyIndent++
			}
		}
	}

	flushRun()

	for ; bodyIndent > 0; bodyIndent-- {
		e.w.Dedent()
	}

	return emitted
}

// rawTextBody reports whether n is an element whose body is always written on
// lines of its own with its closing tag at the writer's indent, so a line break
// inside it never strands what follows on a line nobody laid out.
func rawTextBody(n parser.Node) bool {
	switch nn := n.(type) {
	case *htmlparser.Element:
		return htmlparser.IsRawTextElement(nn.Name) || parser.IsRawBodyComponent(nn.Name)
	case *parser.DirectiveBlockNode, *parser.PhpBlockNode, *parser.VerbatimNode:
		// Its closing directive is written on a line of its own at the writer's
		// indent, so what glued to it in the source can stay on that line.
		return true
	}

	return false
}

func rawPhpBlockRole(n parser.Node) (opens, closes, branch bool) {
	raw, ok := n.(*parser.RawPhpNode)
	if !ok {
		return false, false, false
	}

	return raw.Opens, raw.Closes, raw.Branch
}

func isInlineNode(n parser.Node) bool {
	switch n.(type) {
	case *htmlparser.Text, *parser.EchoNode, *parser.RawEchoNode,
		*parser.EscapedEchoNode, *parser.EscapedAtNode:
		return true
	default:
		return false
	}
}

// runElement reports whether an element flows in a text run rather than taking
// a line of its own: an inline element, an `<input>`, or a self-closing
// component leaf (`<x-datetime />`). A `<br>` ends its line, and a component
// with a body may be a block wrapper, so neither joins.
func runElement(n parser.Node) bool {
	el, ok := n.(*htmlparser.Element)
	if !ok || bytes.EqualFold(el.Name, []byte("br")) {
		return false
	}

	if htmlparser.IsInlineElement(el.Name) {
		return true
	}

	return el.SelfClose && (htmlparser.IsInlineBlockElement(el.Name) || displayUnknown(el))
}

func displayUnknown(el *htmlparser.Element) bool {
	return parser.IsComponentTag(el.Name) || len(el.Name) == 0
}

// That boundary renders, so a glued element flows in the run and breaks
// inside its tags, never beside them.
func gluedElement(nodes []parser.Node, i int) bool {
	el, ok := nodes[i].(*htmlparser.Element)
	if !ok || !displayUnknown(el) {
		return false
	}

	if i > 0 && isRunNeighbor(nodes[i-1]) && nodes[i-1].Span().End.Offset == el.Open.Start.Offset &&
		!endsWithWhitespace(nodes[i-1]) {
		return true
	}

	return i+1 < len(nodes) && isRunNeighbor(nodes[i+1]) && nodes[i+1].Span().Start.Offset == el.Span().End.Offset &&
		!startsWithWhitespace(nodes[i+1])
}

func isRunNeighbor(n parser.Node) bool {
	if el, ok := n.(*htmlparser.Element); ok {
		return runElement(el) || displayUnknown(el)
	}

	return isInlineNode(n) && nodeHasContent(n)
}

func runHasElement(run []parser.Node) bool {
	return slices.ContainsFunc(run, func(n parser.Node) bool {
		_, ok := n.(*htmlparser.Element)

		return ok
	})
}

func loneElement(run []parser.Node) *htmlparser.Element {
	var lone *htmlparser.Element

	for _, n := range run {
		if !nodeHasContent(n) {
			continue
		}

		el, ok := n.(*htmlparser.Element)
		if !ok || lone != nil {
			return nil
		}

		lone = el
	}

	return lone
}

// authoredBreakAtElement reports whether the author broke the line between
// prev and next where one of them is an element: that break stays, as Pint's
// Prettier keeps it, while text and echoes around it refill.
func authoredBreakAtElement(prev, next parser.Node) bool {
	if !runElement(prev) && !runElement(next) {
		return false
	}

	if t, ok := prev.(*htmlparser.Text); ok {
		return htmlfmt.HasLineBreak(t.Text[len(bytes.TrimRight(t.Text, htmlfmt.CollapsibleSpace)):])
	}

	if t, ok := next.(*htmlparser.Text); ok {
		return htmlfmt.HasLineBreak(t.Text[:len(t.Text)-len(bytes.TrimLeft(t.Text, htmlfmt.CollapsibleSpace))])
	}

	return false
}

// Breaks at the body's edges do not count.
func breakBesideElement(children []htmlparser.Node) bool {
	var prev htmlparser.Node

	broken := false

	for _, c := range children {
		t, isText := c.(*htmlparser.Text)
		if !isText {
			if prev != nil && broken && (runElement(prev) || runElement(c)) {
				return true
			}

			prev, broken = c, false

			continue
		}

		content := bytes.Trim(t.Text, htmlfmt.CollapsibleSpace)
		if len(content) == 0 {
			broken = broken || htmlfmt.HasLineBreak(t.Text)

			continue
		}

		lead := t.Text[:len(t.Text)-len(bytes.TrimLeft(t.Text, htmlfmt.CollapsibleSpace))]
		if prev != nil && runElement(prev) && (broken || htmlfmt.HasLineBreak(lead)) {
			return true
		}

		trail := t.Text[len(bytes.TrimRight(t.Text, htmlfmt.CollapsibleSpace)):]
		prev, broken = c, htmlfmt.HasLineBreak(trail)
	}

	return false
}

// A comment written directly over a block belongs to it, so the blank goes
// above the comment.
func (e *emitter) blankBeforeDirective(nodes []parser.Node) bool {
	names := e.cfg.Blade.BlankLineBeforeDirectives
	if len(names) == 0 {
		return false
	}

	for _, node := range nodes {
		switch n := node.(type) {
		case *parser.CommentNode:
		case *htmlparser.Text:
			if isBlankSeparator(n) || nodeHasContent(n) {
				return false
			}
		case *parser.DirectiveBlockNode:
			return slices.Contains(names, string(n.Opener.Name))
		case *parser.PhpBlockNode:
			return slices.Contains(names, config.BladeDirectivePHP)
		default:
			return false
		}
	}

	return false
}

func isBlankSeparator(n parser.Node) bool {
	t, ok := n.(*htmlparser.Text)
	if !ok {
		return false
	}

	// Trim only Blade's whitespace alphabet, not unicode.IsSpace: bytes like
	// 0x0B that collapseInlineWhitespace preserves must not count as blank here,
	// or a separator would drop them and break idempotence.
	if len(bytes.Trim(t.Text, " \t\n\r")) != 0 {
		return false
	}

	return htmlfmt.CountLineBreaks(t.Text) >= 2
}

func nodeHasContent(n parser.Node) bool {
	switch nn := n.(type) {
	case *htmlparser.Text:
		return len(bytes.Trim(nn.Text, htmlfmt.CollapsibleSpace)) > 0
	case *parser.EchoNode, *parser.RawEchoNode,
		*parser.EscapedEchoNode, *parser.EscapedAtNode, *htmlparser.Element:
		return true
	default:
		return false
	}
}

func runHasContent(run []parser.Node) bool {
	return slices.ContainsFunc(run, nodeHasContent)
}

// runEndForGlue returns the offset a following node must start at to count as glued
// to the run, or -1 when it cannot be glued. emitInlineNodes trims the run's outer
// edges, so a run ending in whitespace has already dropped the separator and what
// follows is free to take its own line.
func runEndForGlue(run []parser.Node, broke bool) int {
	last := run[len(run)-1]

	if endsWithWhitespace(last) {
		return -1
	}

	// A run that broke can still be glued to when text ended it: emitInlineNodes
	// writes every text line at the writer's own indent, so the cursor sits where
	// this loop put it. Any other item may have gone out verbatim.
	if broke {
		switch last.(type) {
		case *htmlparser.Text, *parser.EchoNode, *parser.RawEchoNode:
		default:
			return -1
		}
	}

	return last.Span().End.Offset
}

// bodyCopyable reports whether an element's body can be copied from the source
// between its tags. It needs a body, and when the author omitted the element's
// end tag the formatter writes it. An end tag closes the innermost open element
// of its name, so after a body ending inside a descendant of the same name whose
// end tag was omitted too, the written tag attaches to that descendant on the
// next parse: `<p>@A!<li><p>` would gain a `</p>` every pass.
func bodyCopyable(el *htmlparser.Element) bool {
	if el.CloseOpen.Start.Offset <= el.OpenClose.End.Offset {
		return false
	}

	if el.CloseFinal.End.Offset > 0 {
		return true
	}

	for open := lastOpenChild(el); open != nil; open = lastOpenChild(open) {
		if bytes.EqualFold(open.Name, el.Name) {
			return false
		}
	}

	return true
}

func lastOpenChild(el *htmlparser.Element) *htmlparser.Element {
	if len(el.Children) == 0 {
		return nil
	}

	last, ok := el.Children[len(el.Children)-1].(*htmlparser.Element)
	if !ok || last.SelfClose || last.CloseFinal.End.Offset > 0 {
		return nil
	}

	return last
}

// brokeLines runs fn and reports whether it wrote a line break. A break can leave
// the cursor on a line whose indent the caller did not choose, since a
// whitespace-sensitive body goes out verbatim and lands its closing tag at column
// zero. Gluing the next item onto such a line makes the following pass read the pair
// as one line and indent it, so the output would not be a fixpoint.
func (e *emitter) brokeLines(fn func()) bool {
	before := len(e.w.Bytes())

	fn()

	return bytes.ContainsAny(e.w.Bytes()[before:], "\n\r")
}

// gluableNode is htmlfmt.Gluable extended with the Blade nodes that render
// inline. A raw `<?php … ?>` island renders nothing and is a structural boundary
// this formatter gives its own line, so it never glues.
func gluableNode(n parser.Node) bool {
	switch n.(type) {
	case *parser.EchoNode, *parser.RawEchoNode,
		*parser.EscapedEchoNode, *parser.EscapedAtNode, *parser.CommentNode:
		// A `{{-- … --}}` comment compiles to nothing, so a break around it is a
		// rendered space with no markup between the two sides to absorb it:
		// `pre{{-- c --}}post` renders as `prepost` and must keep doing so.
		return true
	case *parser.DirectiveNode, *parser.DirectiveBlockNode, *parser.PhpBlockNode, *parser.VerbatimNode:
		// A directive glued to its neighbor stays glued. Before one, nothing eats a
		// break: `{{ $x }}@csrf` split rendered a newline, since Blade doubles the
		// one after an echo. After one, PHP eats only the newline straight after
		// `?>`, not the indent that follows it, and a directive Blade does not know
		// is not compiled at all: `@A{{ $x }}` stays text and split renders `@A`,
		// a newline, `X`.
		return true
	}

	return htmlfmt.Gluable(n)
}

func (e *emitter) emitInlineRunVerbatim(run []parser.Node) {
	var start, end int

	have := false

	for _, n := range run {
		if !nodeHasContent(n) {
			continue
		}

		if !have {
			start = n.Span().Start.Offset
			have = true
		}

		end = n.Span().End.Offset
	}

	if !have {
		return
	}

	// A frozen inline run still lets the formatter own the whitespace that
	// separates it from its siblings; emitting that outer whitespace verbatim
	// would compound with the separator newline and grow a blank line on each
	// pass. Internal whitespace between content nodes is preserved.
	raw := e.src[start:end]
	start += len(raw) - len(bytes.TrimLeft(raw, htmlfmt.CollapsibleSpace))
	end = start + len(bytes.Trim(raw, htmlfmt.CollapsibleSpace))

	formatter.EmitVerbatim(e.w, e.src, start, end)
}

func (e *emitter) emitHtmlText(t *htmlparser.Text) {
	trimmed := bytes.Trim(t.Text, htmlfmt.CollapsibleSpace)

	if len(trimmed) == 0 {
		return
	}

	e.w.WriteBytes(trimmed)
}

func (e *emitter) allInlineChildren(children []htmlparser.Node) bool {
	if len(children) == 0 {
		return false
	}

	for i, c := range children {
		switch cc := c.(type) {
		case *htmlparser.Text, *parser.EchoNode, *parser.RawEchoNode,
			*parser.EscapedEchoNode, *parser.EscapedAtNode:
			continue
		case *parser.DirectiveNode:
			if !e.inlineDirective(cc, children, i) {
				return false
			}

			continue
		default:
			return false
		}
	}

	return true
}

func (e *emitter) isWhitespacePreservingElement(el *htmlparser.Element) bool {
	if htmlfmt.IsWhitespacePreservingElement(el, e.pageStyleWhitespaceClasses()) {
		return true
	}

	for _, a := range el.Attributes {
		d, ok := a.Directive.(*parser.DirectiveNode)
		if !ok || d.RParen.End.Offset == 0 {
			continue
		}

		name := string(d.Name)
		args := e.src[d.LParen.End.Offset:d.RParen.Start.Offset]

		if (name == "class" || name == "style") && htmlfmt.MentionsPreservingWhitespace(args) {
			return true
		}
	}

	return false
}

func (e *emitter) pageStyleWhitespaceClasses() map[string]bool {
	if e.pageWSClasses == nil {
		e.pageWSClasses = htmlfmt.ScanPageStyleWhitespaceClasses(e.src)
	}

	return e.pageWSClasses
}

// isWhitespaceSensitive reports whether re-layout could change rendered inline
// whitespace (an inserted newline collapses to a space), so the body must emit
// verbatim.
func isWhitespaceSensitive(el *htmlparser.Element) bool {
	children := make([]parser.Node, len(el.Children))
	for i, c := range el.Children {
		children[i] = c
	}

	if flowGlued(children) {
		return true
	}

	if lastChildEndsAtClose(el) {
		return true
	}

	if htmlfmt.IsInlineContainer(el.Name) {
		return bodyGluedToOpenMarker(children) || bodyGluedToCloseMarker(children)
	}

	return false
}

// lastChildEndsAtClose reports whether el's last child is an inline element whose
// omitted end tag leaves el's own close tag as its close marker, with its body
// glued to it: a break before that tag would add a space inside the child.
func lastChildEndsAtClose(el *htmlparser.Element) bool {
	if len(el.Children) == 0 {
		return false
	}

	last, ok := el.Children[len(el.Children)-1].(*htmlparser.Element)
	if !ok || last.SelfClose || last.CloseFinal.End.Offset > 0 || !htmlfmt.IsInlineContainer(last.Name) {
		return false
	}

	body := make([]parser.Node, len(last.Children))
	for i, c := range last.Children {
		body[i] = c
	}

	return bodyGluedToCloseMarker(body)
}

// flowGlued reports a source-glued boundary between two inline-level siblings (one
// structural) or a directive glued to its markers. Nested elements re-check
// themselves, so they are not descended into.
func flowGlued(nodes []parser.Node) bool {
	var prev parser.Node

	for _, n := range nodes {
		if isWhitespaceOnlyText(n) {
			prev = nil

			continue
		}

		if prev != nil && (isStructuralNode(prev) || isStructuralNode(n)) &&
			isInlineLevel(prev) && isInlineLevel(n) &&
			!endsWithWhitespace(prev) && !startsWithWhitespace(n) {
			return true
		}

		if db, ok := n.(*parser.DirectiveBlockNode); ok && directiveBodyGlued(db) {
			return true
		}

		prev = n
	}

	return false
}

func directiveBodyGlued(db *parser.DirectiveBlockNode) bool {
	if bodyGluedToOpenMarker(db.Body) || bodyGluedToCloseMarker(db.Body) || flowGlued(db.Body) {
		return true
	}

	for _, br := range db.Branches {
		if bodyGluedToOpenMarker(br.Body) || bodyGluedToCloseMarker(br.Body) || flowGlued(br.Body) {
			return true
		}
	}

	return false
}

// emitBlockOnOneLine writes a block the author wrote on one line, an inline
// body between its markers, back on one line while it fits, as Pint keeps
// `@if ($a) text @endif`.
func (e *emitter) emitBlockOnOneLine(n *parser.DirectiveBlockNode) bool {
	span := n.Span()
	if len(n.Branches) > 0 || isMailComponent(n.Opener) || livewireStrippedBlocks[string(n.Opener.Name)] ||
		bytes.ContainsAny(e.src[span.Start.Offset:span.End.Offset], "\r\n") {
		return false
	}

	if !slices.ContainsFunc(n.Body, func(c parser.Node) bool { return !isInlineNode(c) && !runElement(c) }) {
		body := make([]htmlparser.Node, len(n.Body))
		for i, c := range n.Body {
			body[i] = c
		}

		atoms, lead, trail := htmlfmt.InlineAtoms(body)
		if len(atoms) == 0 {
			return false
		}

		return e.w.TryFlat(func() {
			e.emitDirective(n.Opener)

			if lead {
				e.w.Space()
			}

			e.emitInlineFlat(atoms, false, false)

			if trail {
				e.w.Space()
			}

			e.emitDirective(n.End)
		})
	}

	return false
}

func directiveMarkersGlued(db *parser.DirectiveBlockNode) bool {
	if bodyGluedToOpenMarker(db.Body) || bodyGluedToCloseMarker(db.Body) {
		return true
	}

	return slices.ContainsFunc(db.Branches, func(br *parser.DirectiveBranch) bool {
		return bodyGluedToOpenMarker(br.Body) || bodyGluedToCloseMarker(br.Body)
	})
}

func bodyGluedToOpenMarker(body []parser.Node) bool {
	return len(body) > 0 && isInlineLevel(body[0]) && !startsWithWhitespace(body[0])
}

func bodyGluedToCloseMarker(body []parser.Node) bool {
	last := len(body) - 1

	return last >= 0 && isInlineLevel(body[last]) && !endsWithWhitespace(body[last])
}

func isInlineLevel(n parser.Node) bool {
	switch nn := n.(type) {
	case *htmlparser.Text:
		return len(bytes.Trim(nn.Text, htmlfmt.CollapsibleSpace)) > 0
	case *parser.EchoNode, *parser.RawEchoNode, *parser.EscapedEchoNode, *parser.EscapedAtNode,
		*parser.DirectiveNode, *parser.DirectiveBlockNode, *parser.RawPhpNode:
		return true
	case *htmlparser.Element:
		return htmlfmt.IsInlineContainer(nn.Name)
	}

	return false
}

func isStructuralNode(n parser.Node) bool {
	switch n.(type) {
	case *parser.DirectiveNode, *parser.DirectiveBlockNode, *parser.RawPhpNode, *htmlparser.Element:
		return true
	}

	return false
}

// allWhitespaceChildren reports whether every child is text that
// emitBlockInlineChildren would trim to nothing. It trims by the same
// CollapsibleSpace, so the routing decision and the trim agree.
func allWhitespaceChildren(children []htmlparser.Node) bool {
	for _, c := range children {
		t, ok := c.(*htmlparser.Text)
		if !ok || len(bytes.Trim(t.Text, htmlfmt.CollapsibleSpace)) != 0 {
			return false
		}
	}

	return len(children) > 0
}

func isWhitespaceOnlyText(n parser.Node) bool {
	t, ok := n.(*htmlparser.Text)

	return ok && len(bytes.Trim(t.Text, " \t\n\r")) == 0
}

func edgesAlreadySpaced(children []htmlparser.Node) bool {
	return len(children) > 0 && startsWithWhitespace(children[0]) && endsWithWhitespace(children[len(children)-1])
}

func startsWithWhitespace(n parser.Node) bool {
	t, ok := n.(*htmlparser.Text)

	return ok && len(t.Text) > 0 && htmlfmt.IsCollapsibleSpace(t.Text[0])
}

func endsWithWhitespace(n parser.Node) bool {
	t, ok := n.(*htmlparser.Text)

	return ok && len(t.Text) > 0 && htmlfmt.IsCollapsibleSpace(t.Text[len(t.Text)-1])
}

func (e *emitter) emitInlineNodes(nodes []parser.Node) {
	write := func() { e.writeInlineNodes(nodes) }

	// An element can copy a multi-line attribute value as written, which a flat
	// try does not count as a break; how much followed it on the line would
	// then decide the layout, and the next pass splits the run there.
	if runHasElement(nodes) {
		flat := false
		if e.w.TryOneLine(func() { flat = e.w.TryFlat(write) }) && flat {
			return
		}
	} else if e.w.TryFlat(write) {
		return
	}

	for i, para := range inlineParagraphs(nodes) {
		if i > 0 {
			e.w.Newline()
			e.w.Newline()
		}

		atoms, _, _ := htmlfmt.InlineAtoms(para)
		e.fillInlineAtoms(atoms, 0, false)
	}
}

// A blank line at a text's own edge is the boundary with its neighbor and
// stays a plain space.
func inlineParagraphs(nodes []parser.Node) [][]htmlparser.Node {
	paras := [][]htmlparser.Node{nil}

	for _, n := range nodes {
		text, ok := n.(*htmlparser.Text)
		if !ok {
			paras[len(paras)-1] = append(paras[len(paras)-1], n)

			continue
		}

		pieces := splitBlankLines(text.Text)

		for i, piece := range pieces {
			if i > 0 && len(pieces[i-1]) > 0 && len(piece) > 0 {
				paras = append(paras, nil)
			} else if i > 0 {
				piece = append([]byte(" "), piece...)
			}

			paras[len(paras)-1] = append(paras[len(paras)-1], &htmlparser.Text{Text: piece})
		}
	}

	return paras
}

func (e *emitter) writeInlineNodes(nodes []parser.Node) {
	last := len(nodes) - 1

	for i, n := range nodes {
		switch nn := n.(type) {
		case *htmlparser.Text:
			e.writeInlineText(nn.Text, i == 0, i == last)
		case *htmlparser.Element:
			e.emitNode(nn)
		default:
			e.emitInlineEcho(nn)
		}
	}
}

// writeInlineText writes one text node of an inline run, collapsing whitespace the
// way white-space:normal renders it but keeping an authored blank line as a blank
// line.
//
// The blank line is not layout here. Laravel renders a mailable's body as
// Markdown (`@component('mail::message')`, `Str::markdown()`), where a blank
// line separates paragraphs, so collapsing it would merge a heading and two
// paragraphs into one line. An HTML view renders the same either way.
func (e *emitter) writeInlineText(text []byte, first, last bool) {
	paras := splitBlankLines(text)

	segments := make([][]byte, 0, len(paras))

	for _, para := range paras {
		if s := collapseInlineWhitespace(para); len(s) > 0 {
			segments = append(segments, s)
		}
	}

	if len(segments) == 0 {
		return
	}

	// A blank run at the text's own edge is the boundary with the neighboring
	// node and renders as a single space; the blank line itself is asked for at
	// that boundary. Dropping the run would glue the two sides, so `@`, a
	// blank line, then `A(` would read as the directive `@A(`. Only a blank
	// between two segments of this text is a paragraph break this emitter
	// reproduces.
	if len(paras[0]) == 0 {
		segments[0] = append([]byte(" "), segments[0]...)
	}

	lastIdx := len(segments) - 1

	if len(paras[len(paras)-1]) == 0 {
		segments[lastIdx] = append(segments[lastIdx], ' ')
	}

	for i, s := range segments {
		if i > 0 {
			e.w.Newline()
			e.w.Newline()
		}

		if first && i == 0 {
			s = bytes.TrimLeft(s, " \n")
		}

		if last && i == lastIdx {
			s = bytes.TrimRight(s, " \n")
		}

		htmlfmt.WriteReindentedText(e.w, s)
	}
}

// splitBlankLines splits text at every run of whitespace holding two or more
// line breaks, the boundary a blank line marks. A text with no blank line comes
// back as one segment.
func splitBlankLines(text []byte) [][]byte {
	var out [][]byte

	start := 0

	for i := 0; i < len(text); {
		if !htmlfmt.IsCollapsibleSpace(text[i]) {
			i++

			continue
		}

		runEnd := i

		for runEnd < len(text) && htmlfmt.IsCollapsibleSpace(text[runEnd]) {
			runEnd++
		}

		if htmlfmt.CountLineBreaks(text[i:runEnd]) < 2 {
			i = runEnd

			continue
		}

		out = append(out, text[start:i])
		start = runEnd
		i = runEnd
	}

	if out == nil {
		return [][]byte{text}
	}

	return append(out, text[start:])
}

// collapseInlineWhitespace replaces every run of spaces, tabs, LFs and CRs in b
// with a single space, or a single LF when the run holds a line break: the
// author's break stays a break. Leading and trailing runs are kept on purpose:
// emitInlineNodes trims only the whole run's outer edges, so these preserve the
// spacing between adjacent inline nodes.
func collapseInlineWhitespace(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inSpace, broke := false, false

	flush := func() {
		if broke {
			out = append(out, '\n')
		} else {
			out = append(out, ' ')
		}

		inSpace, broke = false, false
	}

	for _, c := range b {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			inSpace = true
			broke = broke || c == '\n' || c == '\r'

			continue
		}

		if inSpace {
			flush()
		}

		out = append(out, c)
	}

	if inSpace {
		flush()
	}

	return out
}

// isInlineContent reports whether children are a single inline run: text/comment, the
// Blade echo family, and inline elements. An element child counts only when it is a
// known inline HTML element or a self-closing leaf (`<x-datetime/>`); a component with
// its own children (`<flux:field>…`) is block and keeps the body structural, since the
// component name alone can't tell an inline badge from a block layout wrapper.
//
// inlineBlock admits form controls and other inline-block elements too. Only the
// body routing may pass it, and only for a run the author wrote on one line: a
// layout that does not fit changes that signal, which the routing absorbs,
// since every branch that declines to fold ends in the same structural layout.
// gluedTagWrap must stay a function of the content alone.
func (e *emitter) isInlineContent(children []htmlparser.Node, inlineBlock bool) bool {
	hasContent := false

	for i, c := range children {
		switch n := c.(type) {
		case *htmlparser.Text, *htmlparser.Comment:
			hasContent = true
		case *parser.EchoNode, *parser.RawEchoNode, *parser.EscapedEchoNode, *parser.EscapedAtNode:
			hasContent = true
		case *parser.DirectiveNode:
			if !e.inlineDirective(n, children, i) {
				return false
			}

			hasContent = true
		case *htmlparser.Element:
			// A component whose own body is inline content (`<flux:badge>x</…>`)
			// reads as inline too where the author kept the run on one line, as
			// Pint's Prettier keeps it; a layout wrapper holds block children.
			admitted := inlineBlock && (htmlparser.IsInlineBlockElement(n.Name) ||
				displayUnknown(n) && !allWhitespaceChildren(n.Children) && e.isInlineContent(n.Children, false))
			if !htmlparser.IsInlineElement(n.Name) && !n.SelfClose && !admitted {
				return false
			}

			hasContent = true
		default:
			return false
		}
	}

	return hasContent
}

func (e *emitter) emitInlineContent(el *htmlparser.Element, openFlat bool) {
	atoms, lead, trail := htmlfmt.InlineAtoms(el.Children)
	if len(atoms) == 0 {
		return
	}

	inline := htmlparser.IsInlineElement(el.Name)

	// Edges only: forcing a form control or component flat costs layout on a guess.
	keepEdges := htmlfmt.IsInlineContainer(el.Name)

	// A true inline element keeps its own edge whitespace. It must stay flat unless
	// the run is loose at every boundary: only a fully-loose run can reflow onto its
	// own lines without changing rendered whitespace, since each newline replaces a
	// space the source already had. A tight boundary forces flat regardless of width.
	if inline && !htmlfmt.InlineRunFullyLoose(atoms, lead, trail) {
		e.emitInlineFlat(atoms, lead, trail)

		return
	}

	// Fold onto one line only when the whole run fits AND the open tag stayed flat (a
	// wrapped multiline open tag keeps the body block). The fold budget is shrunk by
	// the close tag, which lands on the same line after the run but is emitted later by
	// emitCloseTag; without reserving it the fold can overflow. An inline element keeps
	// its edge whitespace in the fold; a block container trims it. Otherwise a block
	// body keeps its authored lines when every one fits, and else explodes
	// structurally, one element per line, rather than word-wrapping the run into a
	// blob with a tag split mid-line.
	held := !inline && lead && trail && breakBesideElement(el.Children)

	savedWidth := e.w.PrintWidth()
	e.w.SetPrintWidth(savedWidth - htmlfmt.CloseTagWidth(el))
	folds := openFlat && !held && e.w.TryFlat(func() {
		if keepEdges {
			e.emitInlineFlat(atoms, lead, trail)
		} else {
			e.emitInlineFlat(atoms, false, false)
		}
	})
	e.w.SetPrintWidth(savedWidth)

	if folds {
		return
	}

	if !inline && e.authoredLinesFit(authoredLines(el.Children)) {
		e.emitBlockInlineChildren(el)

		return
	}

	e.emitStructuralChildren(el)
}

func (e *emitter) emitInlineFlat(atoms []htmlfmt.InlineAtom, leadSpace, trailSpace bool) {
	if leadSpace {
		e.w.Space()
	}

	for i, a := range atoms {
		switch {
		case i > 0 && a.NewlineBefore:
			e.w.Newline()

			if a.BlankBefore {
				e.w.RequestBlankLine()
			}
		case i > 0 && a.SpaceBefore:
			e.w.Space()
		}

		e.emitInlineAtom(a)
	}

	if trailSpace {
		e.w.Space()
	}
}

func (e *emitter) emitInlineAtom(a htmlfmt.InlineAtom) {
	if a.Text != nil {
		e.w.WriteBytes(a.Text)

		return
	}

	e.emitNode(a.Node)
}

// A block's opener, closer or branch stays out, since the block's other end
// lies outside this element. So does an argument-less directive before text
// opening with `(`: a reflow joining them would make the text its argument.
func (e *emitter) inlineDirective(d *parser.DirectiveNode, children []htmlparser.Node, i int) bool {
	name := string(d.Name)

	if e.dirs.IsOpener(name) || e.dirs.IsCloser(name) || e.dirs.IsBranch(name) {
		return false
	}

	if d.RParen.End.Offset != 0 || i+1 == len(children) {
		return true
	}

	next, ok := children[i+1].(*htmlparser.Text)

	return !ok || !bytes.HasPrefix(bytes.TrimLeft(next.Text, htmlfmt.CollapsibleSpace), []byte("("))
}

// The hung layout writes a break before the close tag, so the answer holds on
// the next pass.
func (e *emitter) emitGluedBodyOnOneLine(el *htmlparser.Element) bool {
	if e.bodyHasLineBreak(el) {
		return false
	}

	start, end := el.OpenClose.End.Offset, el.CloseOpen.Start.Offset
	trailing := e.closeTagWidth(el)

	if e.emitGluedBodyNormalized(el.Children, start, end, true, trailing) {
		return true
	}

	return e.w.TryFlat(func() {
		e.w.WithTrailing(trailing, func() { htmlfmt.EmitGluedBodyContent(e.w, e.src, el, e.bodyPHPSpans(el)) })
	})
}

func (e *emitter) gluedOnlyBetweenLines(el *htmlparser.Element) bool {
	return !htmlfmt.IsInlineContainer(el.Name) && !lastChildEndsAtClose(el) && !e.bodyPreservesInnerWhitespace(el.Children)
}

func (e *emitter) bodyHasLineBreak(el *htmlparser.Element) bool {
	start, end := el.OpenClose.End.Offset, el.CloseOpen.Start.Offset
	if !htmlfmt.HasLineBreakIn(e.src[start:end], start, phpSpansFor(e.src[start:end], el.Children)) {
		return false
	}

	var nodes []htmlparser.Node
	for _, c := range el.Children {
		e.collectRebuildable(c, start, end, &nodes)
	}

	return e.lineBreakOutsideTags(el.Children, start, end, nodes)
}

// lineBreakOutsideTags reports a line break in src[start:end] outside the
// broken open tags and the echoes among nodes: a break inside a tag does not
// render, and is gone once the tag is rebuilt flat; an echo is rebuilt on one
// line too, and one whose PHP still prints a break is caught where it prints.
func (e *emitter) lineBreakOutsideTags(children []htmlparser.Node, start, end int, nodes []htmlparser.Node) bool {
	php := phpSpansFor(e.src[start:end], children)
	cursor := start

	// Collected on the first break found: most bodies have none.
	var code []htmlparser.Span

	collected := false
	directives := func() []htmlparser.Span {
		if !collected {
			code, collected = directiveSpans(children), true
		}

		return code
	}

	for _, node := range nodes {
		var from, to int

		switch n := node.(type) {
		case *htmlparser.Element:
			from, to = n.Open.Start.Offset, n.OpenClose.End.Offset
		case *parser.EchoNode, *parser.RawEchoNode:
			from, to = n.Span().Start.Offset, n.Span().End.Offset
		default:
			continue
		}

		if lineBreakOutsidePHP(e.src[cursor:from], cursor, php, directives) {
			return true
		}

		cursor = to
	}

	return lineBreakOutsidePHP(e.src[cursor:end], cursor, php, directives)
}

func lineBreakOutsidePHP(b []byte, base int, php []htmlparser.Span, directives func() []htmlparser.Span) bool {
	for {
		at, width := htmlfmt.LineBreakIn(b, base, php)
		if at < 0 {
			return false
		}

		inside := slices.ContainsFunc(directives(), func(s htmlparser.Span) bool {
			return base+at >= s.Start.Offset && base+at < s.End.Offset
		})
		if !inside {
			return true
		}

		b = b[at+width:]
		base += at + width
	}
}

func isElementNode(n htmlparser.Node) bool {
	_, ok := n.(*htmlparser.Element)

	return ok
}

func (e *emitter) sourceColumnAt(offset int) int {
	line := e.src[bytes.LastIndexByte(e.src[:offset], '\n')+1 : offset]

	column := 0
	for _, r := range string(line) {
		if r == '\t' {
			column += e.cfg.TabWidth

			continue
		}

		column += textwidth.Rune(r)
	}

	return column
}

// gluedTagWrap claims a spaced text body for its first break even when the
// element fits whole; such an element prints flat, so a glued sibling may hang
// its close tag.
func (e *emitter) fitsAsSpacedText(el *htmlparser.Element, run inlineRun) bool {
	spaced := len(el.Attributes) > 0 && !run.lead && !run.trail && edgesRender(el) &&
		!slices.ContainsFunc(run.atoms, notTextOrEcho) &&
		slices.ContainsFunc(run.atoms, func(a htmlfmt.InlineAtom) bool { return a.SpaceBefore })

	return spaced && e.w.Measure(func() {
		e.emitOpenTagFlat(el, el.Attributes, false, false)
		e.emitInlineRun(el, run)
		e.w.Write("</")
		e.writeCloseTagName(el)
		e.w.Write(">")
	})
}

func notTextOrEcho(a htmlfmt.InlineAtom) bool {
	switch a.Node.(type) {
	case nil, *htmlparser.Text, *parser.EchoNode, *parser.RawEchoNode:
		return false
	}

	return true
}

func isElementAtom(a htmlfmt.InlineAtom) bool {
	_, ok := a.Node.(*htmlparser.Element)

	return ok
}

func (e *emitter) authorHungOpenBracket(el *htmlparser.Element) bool {
	start := el.Open.Start.Offset + len("<") + len(el.Name)
	if n := len(el.Attributes); n > 0 {
		start = el.Attributes[n-1].End.Offset
	}

	if start > el.OpenClose.Start.Offset || attrsMustBreak(el.Attributes) {
		return false
	}

	return htmlfmt.HasLineBreak(e.src[start:el.OpenClose.Start.Offset]) &&
		!htmlfmt.HasLineBreak(e.src[el.Open.Start.Offset:start])
}

// The hang moves only tag punctuation, so it cannot change what the body
// renders.
func (e *emitter) hangsGluedBodyOpen(el *htmlparser.Element) bool {
	if el.SelfClose || el.Loose || len(el.Children) == 0 || allWhitespaceChildren(el.Children) || !edgesRender(el) || bodyIsVerbatim(el) {
		return false
	}

	if !isWhitespaceSensitive(el) || !bodyCopyable(el) || startsWithWhitespace(el.Children[0]) || !e.bodyHasLineBreak(el) {
		return false
	}

	// A first node that breaks where it prints, after the open tag (a directive
	// whose arguments wrap), already ends that line early, and its breaks would
	// read back as the body's own on the next pass.
	if !e.w.MeasureOneLine(func() {
		e.emitOpenTag(el, false, false, nil)
		e.emitNode(el.Children[0])
	}) {
		return false
	}

	// The body goes out as written, its later lines hung as `><child` against
	// a `>` the author hung too: flattening only the open tag would leave the
	// first child alone on the tag's line.
	if e.authorHungOpenBracket(el) {
		return true
	}

	return !e.w.Measure(func() {
		e.emitOpenTag(el, false, false, nil)
		e.emitNode(el.Children[0])
	})
}
