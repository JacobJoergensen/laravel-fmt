package xml

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/xml"
)

// xmlSpace is XML 1.0's S production. bytes.TrimSpace would also strip a
// no-break space or NEL, which is character data.
const xmlSpace = " \t\r\n"

func emitFile(w *formatter.Writer, file *parser.File, cfg *config.File) {
	first := true

	var prev parser.Node

	for _, node := range file.Nodes {
		if isWhitespaceText(node) {
			if blankLineInText(node) {
				w.RequestBlankLine()
			}

			continue
		}

		if !first {
			w.Newline()
		}

		if isIgnoreDirectiveComment(prev) {
			emitNodeVerbatim(w, file.Src, node)
		} else {
			emitNode(w, file.Src, node, cfg)
		}

		prev = node
		first = false
	}

	if !first {
		w.Newline()
	}
}

func emitNode(w *formatter.Writer, src []byte, node parser.Node, cfg *config.File) {
	switch n := node.(type) {
	case *parser.Element:
		emitElement(w, src, n, cfg)
	case *parser.Text:
		emitTopLevelText(w, src, n)
	case *parser.Comment:
		emitVerbatimRange(w, src, n.Open.Start.Offset, n.Close.End.Offset)
	case *parser.CDATA:
		emitVerbatimRange(w, src, n.Open.Start.Offset, n.Close.End.Offset)
	case *parser.PI:
		emitVerbatimRange(w, src, n.Open.Start.Offset, n.Close.End.Offset)
	case *parser.Doctype:
		emitVerbatimRange(w, src, n.Open.Start.Offset, n.Close.End.Offset)
	}
}

// Content is re-indented only when it is pure element content and nothing asks
// for it to be preserved; significant text, CDATA or `xml:space="preserve"` makes
// it character data, written byte for byte.
func emitElement(w *formatter.Writer, src []byte, el *parser.Element, cfg *config.File) {
	emitOpenTag(w, el, cfg)

	if el.SelfClose {
		return
	}

	switch {
	case len(el.Children) == 0:
	case isStructuralContent(el.Children) && !preservesSpace(el):
		emitStructuralChildren(w, src, el, cfg)
	default:
		// XML never collapses whitespace, so the characters in a text node
		// are the value: the span goes out byte for byte at no indent. Only
		// the line terminator follows the configured end of line, the one
		// rewrite an XML processor makes itself (XML 1.0 section 2.11).
		w.WithoutIndent(func() {
			start, end := el.OpenClose.End.Offset, el.CloseOpen.Start.Offset
			formatter.EmitVerbatimStrippingKeepTrailing(w, src[start:end], 0)
		})
	}

	emitCloseTag(w, el)
}

// preservesSpace reports whether the element carries `xml:space="preserve"`, the
// specification's own signal that the whitespace in its content is data. It
// freezes the whole subtree, since the attribute is inherited; a descendant's
// `xml:space="default"` cannot un-freeze it, which preserves more than required
// and never less.
func preservesSpace(el *parser.Element) bool {
	for _, attr := range el.Attributes {
		if !bytes.Equal(attr.Name, []byte("xml:space")) {
			continue
		}

		return bytes.Equal(attr.Value, []byte("preserve"))
	}

	return false
}

func emitOpenTag(w *formatter.Writer, el *parser.Element, cfg *config.File) {
	fit := w.TryFlat(func() {
		emitOpenTagFlat(w, el, cfg)
	})

	if !fit {
		emitOpenTagMultiline(w, el, cfg)
	}
}

func emitOpenTagFlat(w *formatter.Writer, el *parser.Element, cfg *config.File) {
	w.Write("<")
	w.WriteBytes(el.Name)

	for _, attr := range el.Attributes {
		w.Space()
		emitAttr(w, attr, cfg)
	}

	emitTagClose(w, el)
}

func emitOpenTagMultiline(w *formatter.Writer, el *parser.Element, cfg *config.File) {
	w.Write("<")
	w.WriteBytes(el.Name)
	w.Indent()

	for _, attr := range el.Attributes {
		w.Newline()
		emitAttr(w, attr, cfg)
	}

	w.Dedent()
	w.Newline()
	emitTagClose(w, el)
}

func emitTagClose(w *formatter.Writer, el *parser.Element) {
	if el.SelfClose {
		w.Write("/>")
	} else {
		w.Write(">")
	}
}

func emitAttr(w *formatter.Writer, a *parser.Attr, cfg *config.File) {
	w.WriteBytes(a.Name)
	w.Write("=")

	quote := formatter.AttrQuote(a.Quote, a.Value, cfg)
	w.Write(string(quote))
	w.WriteBytes(a.Value)
	w.Write(string(quote))
}

func emitStructuralChildren(w *formatter.Writer, src []byte, el *parser.Element, cfg *config.File) {
	w.Indent()

	var prev parser.Node

	for _, child := range el.Children {
		if t, ok := child.(*parser.Text); ok {
			if bytes.Count(t.Text, []byte("\n")) >= 2 {
				w.RequestBlankLine()
			}

			continue
		}

		w.Newline()

		if isIgnoreDirectiveComment(prev) {
			emitNodeVerbatim(w, src, child)
		} else {
			emitNode(w, src, child, cfg)
		}

		prev = child
	}

	w.Dedent()
	w.Newline()
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

func emitVerbatimRange(w *formatter.Writer, src []byte, startOffset, endOffset int) {
	formatter.EmitVerbatim(w, src, startOffset, endOffset)
}

// A bare text node at document top level is not valid XML content, so its
// surrounding whitespace is layout the formatter owns; emitting it verbatim
// would compound with the separator newline and grow a blank line on each pass.
func emitTopLevelText(w *formatter.Writer, src []byte, t *parser.Text) {
	start, end := t.Start.Offset, t.End.Offset
	raw := src[start:end]
	start += len(raw) - len(bytes.TrimLeft(raw, xmlSpace))
	end = start + len(bytes.Trim(raw, xmlSpace))
	emitVerbatimRange(w, src, start, end)
}

func isStructuralContent(children []parser.Node) bool {
	hasElement := false

	for _, c := range children {
		switch n := c.(type) {
		case *parser.Element:
			hasElement = true
		case *parser.CDATA:
			return false
		case *parser.Text:
			if len(bytes.Trim(n.Text, xmlSpace)) > 0 {
				return false
			}
		}
	}

	return hasElement
}

func isWhitespaceText(node parser.Node) bool {
	t, ok := node.(*parser.Text)
	if !ok {
		return false
	}

	return len(bytes.Trim(t.Text, xmlSpace)) == 0
}

func blankLineInText(node parser.Node) bool {
	t, ok := node.(*parser.Text)
	if !ok {
		return false
	}

	return bytes.Count(t.Text, []byte("\n")) >= 2
}
