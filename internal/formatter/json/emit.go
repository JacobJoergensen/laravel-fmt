package json

import (
	"bytes"
	"unicode/utf8"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/json"
)

func emitFile(w *formatter.Writer, file *parser.File) {
	if file.Root == nil && len(file.Leading) == 0 && len(file.Trailing) == 0 {
		return
	}

	for _, c := range file.Leading {
		emitComment(w, file.Src, c)
		w.Newline()
	}

	if file.Root == nil {
		return
	}

	emitValue(w, file.Src, file.Root)

	for _, c := range file.Trailing {
		w.Newline()
		emitComment(w, file.Src, c)
	}

	w.Newline()
}

func emitValue(w *formatter.Writer, src []byte, v parser.Value) {
	switch n := v.(type) {
	case *parser.Object:
		emitObject(w, src, n)
	case *parser.Array:
		emitArray(w, src, n)
	case *parser.String:
		emitString(w, n)
	case *parser.Number:
		w.WriteBytes(n.Raw)
	case *parser.Bool:
		if n.Value {
			w.Write("true")
		} else {
			w.Write("false")
		}
	case *parser.Null:
		w.Write("null")
	}
}

func emitObject(w *formatter.Writer, src []byte, o *parser.Object) {
	emitContainer(w, src, "{", "}", " ", o.Members, o.Dangling, fitsFlat(w, o),
		func(m *parser.Member) []parser.Comment { return m.Leading },
		func(m *parser.Member) *parser.Comment { return m.Trailing },
		func(m *parser.Member) bool { return m.BlankBefore },
		func(m *parser.Member) { emitMember(w, src, m) })
}

// emitMember puts the value on a new line after a line comment that follows
// the colon, since the comment would otherwise swallow it.
func emitMember(w *formatter.Writer, src []byte, m *parser.Member) {
	emitString(w, m.Key)
	w.Write(":")

	lineStart := false

	for _, c := range m.BeforeValue {
		if !lineStart {
			w.Write(" ")
		}

		emitComment(w, src, c)

		lineStart = isLineComment(c)
		if lineStart {
			w.Newline()
		}
	}

	if !lineStart {
		w.Write(" ")
	}

	emitValue(w, src, m.Value)
}

func emitArray(w *formatter.Writer, src []byte, a *parser.Array) {
	emitContainer(w, src, "[", "]", "", a.Elements, a.Dangling, fitsFlat(w, a),
		func(e *parser.Element) []parser.Comment { return e.Leading },
		func(e *parser.Element) *parser.Comment { return e.Trailing },
		func(e *parser.Element) bool { return e.BlankBefore },
		func(e *parser.Element) { emitValue(w, src, e.Value) })
}

// emitContainer's flat layout writes no comments, so flatFits must be false
// whenever an item or the dangling run carries one.
func emitContainer[T any](
	w *formatter.Writer,
	src []byte,
	open, closer, flatPad string,
	items []T,
	dangling []parser.Comment,
	flatFits bool,
	leading func(T) []parser.Comment,
	trailing func(T) *parser.Comment,
	blankBefore func(T) bool,
	body func(T),
) {
	if len(items) == 0 && len(dangling) == 0 {
		w.Write(open + closer)

		return
	}

	if flatFits {
		w.Write(open + flatPad)

		for i, it := range items {
			if i > 0 {
				w.Write(", ")
			}

			body(it)
		}

		w.Write(flatPad + closer)

		return
	}

	w.Write(open)
	w.Indent()

	first := true
	line := func(blank bool) {
		if blank && !first {
			w.Newline()
		}

		w.Newline()

		first = false
	}

	for i, it := range items {
		for _, c := range leading(it) {
			line(c.BlankBefore)
			emitComment(w, src, c)
		}

		line(blankBefore(it))
		body(it)

		if i < len(items)-1 {
			w.Write(",")
		}

		emitTrailingComment(w, src, trailing(it))
	}

	for _, c := range dangling {
		line(c.BlankBefore)
		emitComment(w, src, c)
	}

	w.Dedent()
	w.Newline()
	w.Write(closer)
}

const noFlat = -1

// fitsFlat reports whether v's flat form fits on the current line. It measures
// rather than emitting speculatively: a nested container probed inside its
// parent's probe repeats that emission at every level, which costs 2^depth.
func fitsFlat(w *formatter.Writer, v parser.Value) bool {
	return flatWidth(v, w.Remaining()) != noFlat
}

// flatWidth returns v's flat width, or noFlat when it cannot go flat. It stops
// once the width passes budget, which keeps a deeply nested document linear.
func flatWidth(v parser.Value, budget int) int {
	switch n := v.(type) {
	case *parser.Object:
		return objectFlatWidth(n, budget)
	case *parser.Array:
		return arrayFlatWidth(n, budget)
	case *parser.String:
		return withinBudget(stringFlatWidth(n), budget)
	case *parser.Number:
		return withinBudget(utf8.RuneCount(n.Raw), budget)
	case *parser.Bool:
		if n.Value {
			return withinBudget(len("true"), budget)
		}

		return withinBudget(len("false"), budget)
	case *parser.Null:
		return withinBudget(len("null"), budget)
	}

	return noFlat
}

func objectFlatWidth(o *parser.Object, budget int) int {
	if len(o.Members) == 0 {
		return emptyContainerFlatWidth(o.Dangling, "{}", budget)
	}

	if len(o.Dangling) > 0 || wasAuthoredMultiLine(o.Range) {
		return noFlat
	}

	width := len("{  }")

	for i, m := range o.Members {
		if len(m.Leading) > 0 || len(m.BeforeValue) > 0 || m.Trailing != nil {
			return noFlat
		}

		if i > 0 {
			width += len(", ")
		}

		width += stringFlatWidth(m.Key) + len(": ")

		valueWidth := flatWidth(m.Value, budget-width)
		if valueWidth == noFlat {
			return noFlat
		}

		width += valueWidth
	}

	return withinBudget(width, budget)
}

func arrayFlatWidth(a *parser.Array, budget int) int {
	if len(a.Elements) == 0 {
		return emptyContainerFlatWidth(a.Dangling, "[]", budget)
	}

	if len(a.Dangling) > 0 || wasAuthoredMultiLine(a.Range) {
		return noFlat
	}

	width := len("[]")

	for i, e := range a.Elements {
		if len(e.Leading) > 0 || e.Trailing != nil {
			return noFlat
		}

		if i > 0 {
			width += len(", ")
		}

		elementWidth := flatWidth(e.Value, budget-width)
		if elementWidth == noFlat {
			return noFlat
		}

		width += elementWidth
	}

	return withinBudget(width, budget)
}

// emptyContainerFlatWidth ignores the authored shape: an empty container goes
// flat unless it holds a dangling comment.
func emptyContainerFlatWidth(dangling []parser.Comment, delimiters string, budget int) int {
	if len(dangling) > 0 {
		return noFlat
	}

	return withinBudget(len(delimiters), budget)
}

func stringFlatWidth(s *parser.String) int {
	return len(`""`) + utf8.RuneCount(s.Raw)
}

func withinBudget(width, budget int) int {
	if width > budget {
		return noFlat
	}

	return width
}

func wasAuthoredMultiLine(s parser.Span) bool {
	return s.Start.Line != s.End.Line
}

func emitTrailingComment(w *formatter.Writer, src []byte, comment *parser.Comment) {
	if comment == nil {
		return
	}

	w.Write(" ")
	emitComment(w, src, *comment)
}

// emitComment writes c through EmitVerbatim because a block comment can span
// lines, which WriteBytes must never be handed.
func emitComment(w *formatter.Writer, src []byte, c parser.Comment) {
	start := c.Range.Start.Offset

	formatter.EmitVerbatim(w, src, start, start+len(c.Text))
}

func isLineComment(c parser.Comment) bool {
	return bytes.HasPrefix(c.Text, []byte("//"))
}

func emitString(w *formatter.Writer, s *parser.String) {
	w.Write(`"`)
	w.WriteBytes(s.Raw)
	w.Write(`"`)
}
