package blade

import (
	"bytes"
	"slices"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	htmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/html"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	htmlparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

func hasMarkup(nodes []parser.Node) bool {
	for _, n := range nodes {
		switch node := n.(type) {
		case *htmlparser.Element, *htmlparser.Comment, *htmlparser.Doctype:
			return true
		case *parser.DirectiveBlockNode:
			if hasMarkup(node.Body) || slices.ContainsFunc(node.Branches, func(b *parser.DirectiveBranch) bool {
				return hasMarkup(b.Body)
			}) {
				return true
			}
		}
	}

	return false
}

// emitTextTemplate writes a template without markup as it came, rebuilding
// only its echoes, directives and @php blocks in place. Their insides compile
// to PHP and never render, while an indent or a blank line outside them is
// output: in Markdown an indent turns a line into a code block. Blade drops
// the spaces between a directive's name and `(`, and keeps the line break
// after an echo, so neither rebuild moves a rendered byte.
func (e *emitter) emitTextTemplate(nodes []parser.Node) {
	src := e.src
	if len(bytes.TrimRight(src, " \t\r\n")) == 0 {
		return
	}

	var rebuilt []parser.Node

	e.collectTextRebuildable(nodes, &rebuilt)

	var php []htmlparser.Span

	if formatter.HasLoneCR(src) {
		all := make([]htmlparser.Node, len(nodes))
		for i, n := range nodes {
			all[i] = n
		}

		php = phpSpans(all)
	}

	cursor := 0

	for _, n := range rebuilt {
		span := n.Span()
		htmlfmt.EmitVerbatimBytes(e.w, src[cursor:span.Start.Offset], cursor, php, false)

		// A rebuild that breaks lines continues at the indent of the line it
		// starts on, which the next pass reads as the source, so an indented
		// @php keeps its body under it.
		indent := lineIndent(e.w.Bytes())

		e.w.WithoutIndent(func() {
			e.w.WithAlign(indent, func() { e.emitRebuilt(n) })
		})

		cursor = span.End.Offset
	}

	htmlfmt.EmitVerbatimBytes(e.w, bytes.TrimRight(src[cursor:], " \t\r\n"), cursor, php, false)
	e.w.Newline()
}

func lineIndent(out []byte) int {
	line := out[bytes.LastIndexByte(out, '\n')+1:]

	return len(line) - len(bytes.TrimLeft(line, " \t"))
}

// collectTextRebuildable lists the nodes emitTextTemplate rebuilds. After a
// laravel-fmt-ignore comment the next block, or the text run up to a blank
// line, stays as written, and a mail component's body is copied whole, both as
// on the markup path.
func (e *emitter) collectTextRebuildable(nodes []parser.Node, out *[]parser.Node) {
	ignoring, inRun := false, false

	for _, n := range nodes {
		switch {
		case isBlankSeparator(n):
			if inRun {
				ignoring, inRun = false, false
			}
		case isWhitespaceOnlyText(n):
		case e.isIgnoreDirectiveComment(n):
			ignoring, inRun = true, false
		case isInlineNode(n):
			if ignoring {
				inRun = true

				continue
			}

			e.collectTextNode(n, out)
		default:
			skip := ignoring && !inRun
			ignoring, inRun = false, false

			if !skip {
				e.collectTextNode(n, out)
			}
		}
	}
}

func (e *emitter) collectTextNode(n parser.Node, out *[]parser.Node) {
	switch v := n.(type) {
	case *parser.EchoNode, *parser.RawEchoNode, *parser.DirectiveNode, *parser.PhpBlockNode:
		*out = append(*out, n)
	case *parser.DirectiveBlockNode:
		*out = append(*out, v.Opener)

		if !isMailComponent(v.Opener) {
			e.collectTextRebuildable(v.Body, out)

			for _, b := range v.Branches {
				*out = append(*out, b.Directive)
				e.collectTextRebuildable(b.Body, out)
			}
		}

		*out = append(*out, v.End)
	}
}
