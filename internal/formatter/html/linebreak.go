package html

import (
	"bytes"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/alpine"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

// HTML's input stream preprocessing turns a CRLF pair or a lone CR into one
// LF, so every layout decision over HTML content reads all three as a line
// break. One that reads only LF changes its answer on the next pass, after
// this one writes the CR back as LF.

// HasLineBreak reports whether b holds an HTML line break.
func HasLineBreak(b []byte) bool {
	return bytes.IndexByte(b, '\n') >= 0 || bytes.IndexByte(b, '\r') >= 0
}

// LineBreak returns the offset and byte width of the first HTML line break in
// b, or -1 and 0 when there is none. A CRLF is one break two bytes wide.
func LineBreak(b []byte) (int, int) {
	end := bytes.IndexByte(b, '\n')
	if end < 0 {
		end = len(b)
	}

	if cr := bytes.IndexByte(b[:end], '\r'); cr >= 0 {
		if cr+1 < len(b) && b[cr+1] == '\n' {
			return cr, 2
		}

		return cr, 1
	}

	if end == len(b) {
		return -1, 0
	}

	return end, 1
}

// CountLineBreaks returns the number of HTML line breaks in b.
func CountLineBreaks(b []byte) int {
	count := 0

	for {
		at, width := LineBreak(b)
		if at < 0 {
			return count
		}

		count++
		b = b[at+width:]
	}
}

// In a Blade body a lone CR inside compiled PHP is data: `md5("a<CR>b")` and
// `md5("a<LF>b")` differ. The functions below take those PHP spans as source
// offsets, with base the source offset of b[0], and read a lone CR inside one
// as a byte.

// LineBreakIn is LineBreak over text holding PHP spans.
func LineBreakIn(b []byte, base int, php []parser.Span) (int, int) {
	from := 0

	for {
		at, width := LineBreak(b[from:])
		if at < 0 {
			return -1, 0
		}

		at += from

		if width == 2 || b[at] == '\n' || !insideAny(base+at, php) {
			return at, width
		}

		from = at + 1
	}
}

// HasLineBreakIn is HasLineBreak over text holding PHP spans.
func HasLineBreakIn(b []byte, base int, php []parser.Span) bool {
	at, _ := LineBreakIn(b, base, php)

	return at >= 0
}

func insideAny(off int, spans []parser.Span) bool {
	for _, s := range spans {
		if off >= s.Start.Offset && off < s.End.Offset {
			return true
		}
	}

	return false
}

// NormalizeLineBreaks returns b with every HTML line break written as LF, for a
// span handed to a writer that splits on LF alone.
func NormalizeLineBreaks(b []byte) []byte {
	return NormalizeLineBreaksIn(b, 0, nil)
}

// NormalizeLineBreaksIn is NormalizeLineBreaks over text holding PHP spans,
// which keeps a lone CR inside one unless it ends a line. It returns b itself
// when b holds no CR.
func NormalizeLineBreaksIn(b []byte, base int, php []parser.Span) []byte {
	if bytes.IndexByte(b, '\r') < 0 {
		return b
	}

	return bytes.Join(splitHTMLLines(b, base, php), []byte("\n"))
}

// EmitMultilineAttrValue writes a value that spans lines. Line-trailing
// whitespace is kept, since an attribute value is a string. Only a value whose
// whitespace does not render is re-homed under its tag, each line keeping its
// indent relative to the attribute; any other value is written as authored,
// since moving its lines would change the string.
func EmitMultilineAttrValue(w *formatter.Writer, src []byte, a *parser.Attr) {
	value := NormalizeLineBreaks(a.Value)

	if !ValueReflows(a) {
		w.WithoutIndent(func() { formatter.EmitVerbatimStrippingKeepTrailing(w, value, 0) })

		return
	}

	attrIndent := formatter.NaturalIndentBefore(src, a.Start.Offset)

	// The blank line the closing quote sits on keeps its offset from the
	// lines above it, as Prettier leaves the value as written. Left of the
	// strip it would otherwise land on the attribute's column.
	if last := bytes.LastIndexByte(value, '\n'); last >= 0 && isBlankLine(value[last+1:]) {
		if offset := len(value[last+1:]) - uniformStrip(value[:last], attrIndent); offset < 0 {
			formatter.EmitVerbatimStrippingKeepTrailing(w, value[:last+1], attrIndent)

			column := max(w.IndentColumn()+offset, 0)
			w.WithoutIndent(func() { w.Write(strings.Repeat(" ", column)) })

			return
		}
	}

	formatter.EmitVerbatimStrippingKeepTrailing(w, value, attrIndent)
}

func isBlankLine(line []byte) bool {
	return len(bytes.TrimLeft(line, " \t")) == 0
}

// uniformStrip mirrors the strip EmitVerbatimStripping takes from value.
func uniformStrip(value []byte, natural int) int {
	strip := natural

	lines := bytes.Split(value, []byte("\n"))
	for _, line := range lines[1:] {
		if isBlankLine(line) {
			continue
		}

		strip = min(strip, len(line)-len(bytes.TrimLeft(line, " \t")))
	}

	return strip
}

// ValueReflows reports whether whitespace in a's value does not render.
func ValueReflows(a *parser.Attr) bool {
	name := a.Name
	js := a.Bound || alpine.IsAlpineJSAttr(name) || hasPrefixFold(name, "x-")

	if js && alpine.BreaksInTemplate(a.Value) {
		return false
	}

	return js || IsClassAttr(name) || bytes.EqualFold(name, []byte("style")) || hasPrefixFold(name, "wire:")
}

func hasPrefixFold(name []byte, prefix string) bool {
	return len(name) >= len(prefix) && bytes.EqualFold(name[:len(prefix)], []byte(prefix))
}

// WriteReindentedText writes a white-space:normal text run line by line,
// moving each continuation line to the Writer's indent; the whitespace around
// a line break collapses, so the run renders the same. The first line keeps
// its leading whitespace, the boundary with the node before it.
func WriteReindentedText(w *formatter.Writer, text []byte) {
	for {
		nl, width := LineBreak(text)
		if nl < 0 {
			if len(text) > 0 {
				w.WriteBytes(text)
			}

			return
		}

		if line := bytes.TrimRight(text[:nl], " \t"); len(line) > 0 {
			w.WriteBytes(line)
		}

		w.Newline()
		text = bytes.TrimLeft(text[nl+width:], " \t")
	}
}
