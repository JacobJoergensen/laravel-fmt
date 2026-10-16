package html

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/alpine"
	cssfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/css"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

func emitFile(w *formatter.Writer, file *parser.File, cfg *config.File) {
	first := true
	idx := &docIndex{src: file.Src, closeTags: parser.CloseTagIndex(file.Src)}

	var prev parser.Node

	prevEnd := -1

	for _, node := range file.Nodes {
		if IsWhitespaceText(node) {
			if BlankLineInText(node) {
				w.RequestBlankLine()
			}

			prevEnd = -1

			continue
		}

		if !first && !gluedTo(prevEnd, node) {
			w.Newline()
		}

		before := len(w.Bytes())

		if isIgnoreDirectiveComment(prev) {
			emitNodeVerbatim(w, file.Src, node)
		} else {
			emitNode(w, file.Src, idx, node, cfg)
		}

		broke := bytes.ContainsAny(w.Bytes()[before:], "\n\r")
		prevEnd = nodeEndForGlue(node, broke)
		prev = node
		first = false
	}

	if !first {
		w.Newline()
	}
}

// nodeEndForGlue returns the offset a following node must start at to be glued
// to node, or -1 when none can be. Text ending in whitespace is never glued to,
// since that whitespace was the separator and the emitter trims it.
func nodeEndForGlue(node parser.Node, broke bool) int {
	if !Gluable(node) {
		return -1
	}

	t, ok := node.(*parser.Text)
	if ok && len(bytes.TrimRight(t.Text, CollapsibleSpace)) < len(t.Text) {
		return -1
	}

	// A node that broke can end on a line the Writer did not lay out, such as
	// a verbatim body leaving its close tag at column zero, and a node glued
	// onto that line gets indented on the next pass. Text and raw-text
	// elements end at the Writer's indent, so they are exempt.
	if el, isEl := node.(*parser.Element); broke && !ok && (!isEl || !parser.IsRawTextElement(el.Name)) {
		return -1
	}

	return node.Span().End.Offset
}

// gluedTo reports whether node starts exactly where the previous node ended
// and both render inline, so a newline between them would render as a space.
// Text opening with whitespace is not glued: that whitespace is trimmed, and
// the newline stands in for it.
func gluedTo(prevEnd int, node parser.Node) bool {
	if prevEnd < 0 || node.Span().Start.Offset != prevEnd || !Gluable(node) {
		return false
	}

	t, ok := node.(*parser.Text)

	return !ok || len(bytes.TrimLeft(t.Text, CollapsibleSpace)) == len(t.Text)
}

func emitNode(w *formatter.Writer, src []byte, idx *docIndex, node parser.Node, cfg *config.File) {
	switch n := node.(type) {
	case *parser.Element:
		emitElement(w, src, idx, n, cfg)
	case *parser.Text:
		emitText(w, n)
	case *parser.Comment:
		EmitComment(w, src, n)
	case *parser.Doctype:
		EmitDoctype(w, n)
	}
}

func emitElement(w *formatter.Writer, src []byte, idx *docIndex, el *parser.Element, cfg *config.File) {
	void := parser.IsVoidElement(el.Name)
	selfClose := el.SlashClosed || (el.SelfClose && void && voidNamePaired(idx, el.Name))

	openFlat := emitOpenTag(w, src, el, cfg, selfClose)

	if el.SelfClose {
		return
	}

	switch {
	case parser.IsRcdataElement(el.Name):
		EmitRcdataContent(w, el.Children)
	case parser.IsPreformattedElement(el.Name), idx.preservesWhitespace(el):
		EmitPreformattedContent(w, src, el, nil)
	case parser.IsRawTextElement(el.Name):
		EmitRawTextContent(w, el, cfg)
	case len(el.Children) == 0:
	case allTextChildren(el.Children):
		emitTextChildren(w, el, openFlat)
	case isInlineContent(el.Children) && !KeepsAuthoredBreak(el.Children):
		emitInlineContent(w, src, idx, el, cfg, openFlat)
	default:
		emitStructuralChildren(w, src, idx, el, cfg, isInlineContent(el.Children))
	}

	emitCloseTag(w, el)
}

// voidNamePaired reports whether a </name> close tag appears anywhere in the
// document. A self-closed void element keeps its `/>` when its name is also used
// paired: emitted bare (`<link>`), it would re-pair with that close on reparse
// and the output would not round-trip.
func voidNamePaired(idx *docIndex, name []byte) bool {
	return len(idx.closeTags[string(bytes.ToLower(name))]) > 0
}

type docIndex struct {
	src       []byte
	closeTags map[string][]int
	wsClasses map[string]bool
}

func (d *docIndex) preservesWhitespace(el *parser.Element) bool {
	if d.wsClasses == nil {
		d.wsClasses = ScanPageStyleWhitespaceClasses(d.src)
	}

	return IsWhitespacePreservingElement(el, d.wsClasses) || IsEmptinessSensitiveElement(el)
}

func emitOpenTag(w *formatter.Writer, src []byte, el *parser.Element, cfg *config.File, selfClose bool) bool {
	attrs := el.Attributes

	fit := w.TryFlat(func() {
		emitOpenTagFlat(w, src, el, attrs, cfg, selfClose)
	})

	if fit {
		return true
	}

	emitOpenTagMultiline(w, src, el, attrs, cfg, selfClose)

	return false
}

func emitOpenTagFlat(w *formatter.Writer, src []byte, el *parser.Element, attrs []*parser.Attr, cfg *config.File, selfClose bool) {
	w.Write("<")
	w.WriteBytes(el.Name)

	for _, attr := range attrs {
		w.Space()
		emitAttr(w, src, attr, cfg)
	}

	if selfClose {
		w.Space()
		w.Write("/>")
	} else {
		CloseBracket(w)
	}
}

// CloseBracket writes the `>` that ends a tag or doctype, with a space before
// it after a `?`: a `?>` would close a PHP block opened by an earlier `<?`. So
// after a `/`, which can only end an unquoted value here: `/>` would read the
// tag as self-closing.
func CloseBracket(w *formatter.Writer) {
	if out := w.Bytes(); len(out) > 0 && (out[len(out)-1] == '?' || out[len(out)-1] == '/') {
		w.Space()
	}

	w.Write(">")
}

func emitOpenTagMultiline(w *formatter.Writer, src []byte, el *parser.Element, attrs []*parser.Attr, cfg *config.File, selfClose bool) {
	w.Write("<")
	w.WriteBytes(el.Name)
	w.Indent()

	for _, attr := range attrs {
		w.Newline()
		emitAttr(w, src, attr, cfg)
	}

	w.Dedent()
	w.Newline()

	if selfClose {
		w.Write("/>")
	} else {
		w.Write(">")
	}
}

func emitAttr(w *formatter.Writer, src []byte, a *parser.Attr, cfg *config.File) {
	if a.Bound {
		w.Write(":")
	}

	if a.Shorthand {
		w.Write("$")
	}

	w.WriteBytes(a.Name)

	if a.Shorthand {
		return
	}

	// Quote is unset for a flag attribute and for an unquoted value alike, so
	// the `=` tells them apart: `<meta charset=utf-8>` must keep its value.
	if a.Quote == 0 && (a.Eq.End.Offset == a.Eq.Start.Offset || len(a.Value) == 0) {
		return
	}

	w.Write("=")

	quote := a.Quote
	if quote == 0 {
		quote = formatter.UnquotedAttrDelimiter(a.Value, cfg)

		if quote == 0 {
			w.WriteBytes(a.Value)

			return
		}
	}

	if HasLineBreak(a.Value) {
		quote = formatter.AttrQuote(quote, a.Value, cfg)
		w.Write(string(quote))
		EmitMultilineAttrValue(w, src, a)
		w.Write(string(quote))

		return
	}

	value := a.Value
	if !a.Bound && cfg.Tailwind && IsClassAttr(a.Name) {
		value = []byte(tailwind.Sort(cfg.TailwindTheme, string(value)))
	}

	if !a.Bound && alpine.IsAlpineJSAttr(a.Name) {
		if out, ok := alpine.Normalize(value, cfg.PHP.NotOperatorSpace); ok {
			value = out
		}
	}

	formatter.WriteAttrValue(w, quote, value, cfg)
}

// IsClassAttr reports whether name is `class`, case-insensitively.
func IsClassAttr(name []byte) bool {
	return bytes.EqualFold(name, []byte("class"))
}

func emitCloseTag(w *formatter.Writer, el *parser.Element) {
	name := el.CloseName
	if len(name) == 0 {
		name = el.Name
	}

	w.Write("</")
	w.WriteBytes(name)
	w.Write(">")
}

// rawBody returns a raw-text or RCDATA body with its source offset and PHP
// spans.
func rawBody(children []parser.Node) ([]byte, int, []parser.Span) {
	if len(children) == 1 {
		if t, ok := children[0].(*parser.Text); ok {
			return t.Text, t.Start.Offset, t.PHP
		}
	}

	return textChildren(children), 0, nil
}

func rawPHPLiterals(children []parser.Node) []parser.Span {
	if len(children) == 1 {
		if t, ok := children[0].(*parser.Text); ok {
			return t.PHPLiterals
		}
	}

	return nil
}

func textChildren(children []parser.Node) []byte {
	var raw []byte

	for _, c := range children {
		if t, ok := c.(*parser.Text); ok {
			raw = append(raw, t.Text...)
		}
	}

	return raw
}

// An inline element with a glued edge stays hugged: a line break there would
// render as a space.
func emitTextChildren(w *formatter.Writer, el *parser.Element, openFlat bool) {
	raw := textChildren(el.Children)

	if len(bytes.Trim(raw, CollapsibleSpace)) == 0 {
		return
	}

	if parser.IsInlineLevelElement(el.Name) && !edgesSpaced(raw) {
		emitInlineTextChildren(w, el)

		return
	}

	fits := openFlat && !HasLineBreak(raw) && w.TryFlat(func() {
		w.WithTrailing(CloseTagWidth(el), func() {
			emitInlineTextChildren(w, el)
		})
	})

	if !fits {
		emitBlockTextChildren(w, el)
	}
}

func edgesSpaced(raw []byte) bool {
	return len(raw) > 0 && IsCollapsibleSpace(raw[0]) && IsCollapsibleSpace(raw[len(raw)-1])
}

func emitInlineTextChildren(w *formatter.Writer, el *parser.Element) {
	raw := textChildren(el.Children)
	text := bytes.Trim(raw, CollapsibleSpace)

	// An inline-level element's edge whitespace renders, so it collapses to one
	// space instead of being trimmed. A block element's edge whitespace is
	// only layout.
	if parser.IsInlineLevelElement(el.Name) && len(text) > 0 {
		if len(bytes.TrimLeft(raw, CollapsibleSpace)) < len(raw) {
			w.Write(" ")
		}

		WriteReindentedText(w, text)

		if len(bytes.TrimRight(raw, CollapsibleSpace)) < len(raw) {
			w.Write(" ")
		}

		return
	}

	WriteReindentedText(w, text)
}

func emitBlockTextChildren(w *formatter.Writer, el *parser.Element) {
	text := bytes.Trim(textChildren(el.Children), CollapsibleSpace)

	w.Indent()
	w.Newline()
	WriteReindentedText(w, text)
	w.Dedent()
	w.Newline()
}

// In an inline run the whitespace between children renders, so a child the
// author kept on its neighbor's line stays there: a newline in place of that
// space would change the rendered HTML.
func emitStructuralChildren(w *formatter.Writer, src []byte, idx *docIndex, el *parser.Element, cfg *config.File, inline bool) {
	w.Indent()

	var prev parser.Node

	prevEnd := -1

	// A blank line separates two children, so it is only asked for once the child
	// on the far side arrives; a blank the body ended on is trailing whitespace.
	wantBlank := false

	brokeSincePrev := false

	for _, child := range el.Children {
		t, isText := child.(*parser.Text)

		if isText && len(bytes.Trim(t.Text, CollapsibleSpace)) == 0 {
			if CountLineBreaks(t.Text) >= 2 {
				wantBlank = true
			}

			brokeSincePrev = brokeSincePrev || CountLineBreaks(t.Text) > 0
			prevEnd = -1

			continue
		}

		if isText {
			lead := t.Text[:len(t.Text)-len(bytes.TrimLeft(t.Text, CollapsibleSpace))]
			brokeSincePrev = brokeSincePrev || CountLineBreaks(lead) > 0
		}

		if prev != nil && TextEdgeBlank(child, true) {
			wantBlank = true
		}

		if wantBlank {
			w.RequestBlankLine()

			wantBlank = false
		}

		switch {
		case gluedTo(prevEnd, child):
		case inline && prev != nil && !brokeSincePrev:
			w.Space()
		default:
			w.Newline()
		}

		switch {
		case isText:
			WriteReindentedText(w, bytes.Trim(t.Text, CollapsibleSpace))
		case isIgnoreDirectiveComment(prev):
			emitNodeVerbatim(w, src, child)
		default:
			emitNode(w, src, idx, child, cfg)
		}

		if TextEdgeBlank(child, false) {
			wantBlank = true
		}

		brokeSincePrev = false
		if isText {
			trail := t.Text[len(bytes.TrimRight(t.Text, CollapsibleSpace)):]
			brokeSincePrev = CountLineBreaks(trail) > 0
		}

		// Unlike at the document root, glue after a child holds even when the
		// child broke, so `</b>tail` keeps its hug.
		prevEnd = nodeEndForGlue(child, false)
		prev = child
	}

	w.Dedent()
	w.Newline()
}

func isInlineContent(children []parser.Node) bool {
	hasContent := false

	for _, c := range children {
		switch n := c.(type) {
		case *parser.Text, *parser.Comment:
			hasContent = true
		case *parser.Element:
			if !parser.IsInlineLevelElement(n.Name) {
				return false
			}

			hasContent = true
		default:
			return false
		}
	}

	return hasContent
}

func emitInlineContent(w *formatter.Writer, src []byte, idx *docIndex, el *parser.Element, cfg *config.File, openFlat bool) {
	atoms, lead, trail := InlineAtoms(el.Children)
	if len(atoms) == 0 {
		return
	}

	keepEdges := IsInlineContainer(el.Name)

	tight := parser.IsInlineElement(el.Name) && !InlineRunFullyLoose(atoms, lead, trail)
	if tight || keepEdges && (!lead || !trail) {
		emitInlineFlat(w, src, idx, atoms, lead, trail, cfg)

		return
	}

	savedWidth := w.PrintWidth()
	w.SetPrintWidth(savedWidth - CloseTagWidth(el))
	folds := openFlat && w.TryFlat(func() {
		emitInlineFlat(w, src, idx, atoms, keepEdges && lead, keepEdges && trail, cfg)
	})
	w.SetPrintWidth(savedWidth)

	if folds {
		return
	}

	if InlineRunHasGluedInterior(atoms) {
		w.Indent()
		w.Newline()
		emitInlineFlat(w, src, idx, atoms, false, false, cfg)
		w.Dedent()
		w.Newline()

		return
	}

	emitStructuralChildren(w, src, idx, el, cfg, true)
}

func emitInlineAtom(w *formatter.Writer, src []byte, idx *docIndex, a InlineAtom, cfg *config.File) {
	if a.Text != nil {
		w.WriteBytes(a.Text)

		return
	}

	emitNode(w, src, idx, a.Node, cfg)
}

func emitInlineFlat(w *formatter.Writer, src []byte, idx *docIndex, atoms []InlineAtom, leadSpace, trailSpace bool, cfg *config.File) {
	if leadSpace {
		w.Space()
	}

	for i, a := range atoms {
		switch {
		case i > 0 && a.NewlineBefore:
			w.Newline()

			if a.BlankBefore {
				w.RequestBlankLine()
			}
		case i > 0 && a.SpaceBefore:
			w.Space()
		}

		emitInlineAtom(w, src, idx, a, cfg)
	}

	if trailSpace {
		w.Space()
	}
}

// EmitRawTextContent writes the body of a <script> or <style> element.
func EmitRawTextContent(w *formatter.Writer, el *parser.Element, cfg *config.File) {
	name := el.Name
	raw, base, php := rawBody(el.Children)

	if bytes.EqualFold(name, []byte("style")) && emitStyleCSS(w, raw, cfg) {
		return
	}

	if isShiftableScript(el) && emitShiftedScript(w, raw, base, php, rawPHPLiterals(el.Children)) {
		return
	}

	lines := trimRawTextLines(raw, base, php)
	if len(lines) == 0 {
		return
	}

	// Leading whitespace on a CSS line is insignificant, so a <style> body the
	// CSS formatter declined is re-homed under the tag. A <script> body stays
	// verbatim.
	rehomable := bytes.EqualFold(name, []byte("style")) && styleBodyRehomable(lines)

	if rehomable {
		common := commonRawIndent(lines)

		w.Indent()

		for _, line := range lines {
			w.Newline()

			if body := bytes.TrimRight(line[dropIndent(line, common):], " \t\r"); len(body) > 0 {
				w.WriteBytes(body)
			}
		}

		w.Dedent()
		w.Newline()

		return
	}

	writeVerbatimLines(w, lines)
}

// EmitVerbatimLines writes raw at its source columns without its blank edge
// lines and reports whether any line was written. base is raw's source
// offset, and php the spans in which a lone CR is a byte, not a line break.
func EmitVerbatimLines(w *formatter.Writer, raw []byte, base int, php []parser.Span) bool {
	lines := trimRawTextLines(raw, base, php)
	if len(lines) == 0 {
		return false
	}

	writeVerbatimLines(w, lines)

	return true
}

// writeVerbatimLines keeps each line's trailing whitespace, which can be part
// of a multi-line string or a Markdown hard break. Only trailing CRs go, since
// Newline writes the line terminator.
func writeVerbatimLines(w *formatter.Writer, lines [][]byte) {
	w.WithoutIndent(func() {
		for _, line := range lines {
			w.Newline()
			w.WriteBytes(bytes.TrimRight(line, "\r"))
		}
	})

	w.Newline()
}

// styleBodyRehomable reports whether re-indenting lines leaves every value
// intact. It fails when a quoted string is still open at a line break, even
// after a `\`: the next line's leading whitespace is part of the string.
func styleBodyRehomable(lines [][]byte) bool {
	var quote byte

	for n, line := range lines {
		if n > 0 && quote != 0 {
			return false
		}

		for i := 0; i < len(line); i++ {
			c := line[i]

			switch {
			case quote != 0 && c == '\\':
				i++
			case quote != 0 && c == quote:
				quote = 0
			case quote == 0 && (c == '"' || c == '\''):
				quote = c
			}
		}
	}

	return true
}

func commonRawIndent(lines [][]byte) int {
	common := -1

	for _, line := range lines {
		if isDroppableRawLine(line) {
			continue
		}

		n := 0
		for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
			n++
		}

		if common < 0 || n < common {
			common = n
		}
	}

	if common < 0 {
		return 0
	}

	return common
}

func dropIndent(line []byte, limit int) int {
	n := 0
	for n < limit && n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}

	return n
}

func emitStyleCSS(w *formatter.Writer, raw []byte, cfg *config.File) bool {
	if len(bytes.TrimSpace(raw)) == 0 {
		return false
	}

	// A Blade echo (`{{ }}`, `{!! !!}`) is not valid CSS; the `{` would be parsed
	// as a block, so the whole body is left verbatim rather than mangled.
	if bytes.Contains(raw, []byte("{{")) || bytes.Contains(raw, []byte("{!!")) {
		return false
	}

	return cssfmt.EmitStyleBody(w, raw, cfg)
}

// EmitRcdataContent writes a <textarea> or <title> body untrimmed, since its
// content is a literal value.
func EmitRcdataContent(w *formatter.Writer, children []parser.Node) {
	raw, base, php := rawBody(children)

	emitVerbatimBytes(w, raw, base, php, false)
}

// EmitPreformattedContent writes the source between el's tags verbatim, for an
// element whose whitespace matters.
func EmitPreformattedContent(w *formatter.Writer, src []byte, el *parser.Element, php []parser.Span) {
	emitBodySpan(w, src, el, php, false)
}

// EmitGluedBodyContent is EmitPreformattedContent for an element whose
// whitespace collapses, kept verbatim only because a glued inline boundary
// blocks a reflow. A body ending on a line break leaves its close tag at the
// Writer's indent, not column zero, so the next pass does not re-home it.
func EmitGluedBodyContent(w *formatter.Writer, src []byte, el *parser.Element, php []parser.Span) {
	emitBodySpan(w, src, el, php, true)
}

func emitBodySpan(w *formatter.Writer, src []byte, el *parser.Element, php []parser.Span, indentClose bool) {
	start := el.OpenClose.End.Offset
	end := el.CloseOpen.Start.Offset

	if end < start || end > len(src) {
		return
	}

	emitVerbatimBytes(w, src[start:end], start, php, indentClose)
}

// splitHTMLLines splits raw on CRLF, a lone CR and LF alike, except that a
// lone CR inside php stays a byte; base is raw's source offset. The HTML spec's
// input stream preprocessing turns every CR into LF, so a lone CR renders as a
// line break everywhere, <pre> and <textarea> included.
func splitHTMLLines(raw []byte, base int, php []parser.Span) [][]byte {
	lines := make([][]byte, 0, bytes.Count(raw, []byte("\n"))+1)
	start := 0

	for {
		at, width := LineBreakIn(raw[start:], base+start, php)
		if at < 0 {
			return append(lines, raw[start:])
		}

		line := raw[start : start+at]

		// A run of lone CRs in PHP at a line end would pair with the line break
		// into a CRLF on the next pass, so the whole run is dropped.
		n := len(line)
		for n > 0 && line[n-1] == '\r' && insideAny(base+start+n-1, php) {
			n--
		}

		line = line[:n]

		lines = append(lines, line)
		start += at + width
	}
}

func emitVerbatimBytes(w *formatter.Writer, raw []byte, base int, php []parser.Span, indentClose bool) {
	if len(raw) == 0 {
		return
	}

	lines := splitHTMLLines(raw, base, php)
	last := len(lines) - 1

	w.WithoutIndent(func() {
		for i, line := range lines {
			if i > 0 {
				w.Newline()
			}

			// Skipping the empty last line leaves the pending indent armed, so
			// the close tag lands at the Writer's indent, not column zero.
			if indentClose && i == last && len(line) == 0 {
				break
			}

			w.WriteBytes(line)
		}
	})
}

func trimRawTextLines(raw []byte, base int, php []parser.Span) [][]byte {
	if len(raw) == 0 {
		return nil
	}

	lines := splitHTMLLines(raw, base, php)

	for len(lines) > 0 && isDroppableRawLine(lines[0]) {
		lines = lines[1:]
	}

	for len(lines) > 0 && isDroppableRawLine(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}

	if len(lines) == 0 {
		return nil
	}

	return lines
}

// isDroppableRawLine reports whether line holds only spaces, tabs and CRs,
// the set the per-line TrimRight uses. unicode.IsSpace would also drop a
// vertical tab or no-break space, which can make the CSS parser decline, and
// the next pass would then reformat a body this pass left verbatim.
func isDroppableRawLine(line []byte) bool {
	return len(bytes.Trim(line, " \t\r")) == 0
}

func emitText(w *formatter.Writer, t *parser.Text) {
	trimmed := bytes.Trim(t.Text, CollapsibleSpace)
	if len(trimmed) == 0 {
		return
	}

	WriteReindentedText(w, trimmed)
}

// EmitComment writes the comment as one verbatim span, since WriteBytes
// cannot take the line breaks of a multi-line comment.
func EmitComment(w *formatter.Writer, src []byte, c *parser.Comment) {
	start, end := c.Open.Start.Offset, c.Close.End.Offset

	if formatter.ColumnUnmoved(w, src, start) {
		formatter.EmitVerbatimUnmoved(w, src, start, end)

		return
	}

	formatter.EmitVerbatim(w, src, start, end)
}

// EmitDoctype writes the doctype with its text trimmed.
func EmitDoctype(w *formatter.Writer, d *parser.Doctype) {
	w.Write("<!DOCTYPE")

	text := bytes.TrimSpace(d.Text)
	if len(text) > 0 {
		w.Space()
		w.WriteBytes(text)
	}

	CloseBracket(w)
}

// IsWhitespaceText reports whether node is a text node holding only spaces,
// tabs and line breaks.
func IsWhitespaceText(node any) bool {
	t, ok := node.(*parser.Text)
	if !ok {
		return false
	}

	return len(bytes.Trim(t.Text, CollapsibleSpace)) == 0
}

// BlankLineInText reports whether node is a whitespace-only text node with an
// authored blank line.
func BlankLineInText(node any) bool {
	t, ok := node.(*parser.Text)
	if !ok {
		return false
	}

	if len(bytes.Trim(t.Text, CollapsibleSpace)) != 0 {
		return false
	}

	return CountLineBreaks(t.Text) >= 2
}

func allTextChildren(children []parser.Node) bool {
	if len(children) == 0 {
		return false
	}

	for _, c := range children {
		if _, ok := c.(*parser.Text); !ok {
			return false
		}
	}

	return true
}
