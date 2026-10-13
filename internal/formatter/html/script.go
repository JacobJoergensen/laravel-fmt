package html

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/alpine"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

// scriptTypes are the `type` values whose body is JavaScript or JSON, where the
// whitespace starting a line is insignificant outside a string or template
// literal. Any other type (text/template, text/x-template, text/html, …) holds
// content whose whitespace can be the point, so its body stays verbatim.
var scriptTypes = map[string]bool{
	"":                       true,
	"module":                 true,
	"text/javascript":        true,
	"application/javascript": true,
	"text/ecmascript":        true,
	"application/ecmascript": true,
	"application/json":       true,
	"application/ld+json":    true,
	"importmap":              true,
	"speculationrules":       true,
}

func isShiftableScript(el *parser.Element) bool {
	if !bytes.EqualFold(el.Name, []byte("script")) {
		return false
	}

	for _, a := range el.Attributes {
		if !bytes.EqualFold(a.Name, []byte("type")) {
			continue
		}

		if a.Bound || a.Parts != nil {
			return false
		}

		return scriptTypes[string(bytes.ToLower(bytes.TrimSpace(a.Value)))]
	}

	return true
}

// emitShiftedScript lays a script body out one level under its tag, keeping
// its relative indentation, and reports whether it did. A line that starts
// inside a multi-line JavaScript or PHP literal is part of that value, so it
// keeps its source column.
func emitShiftedScript(w *formatter.Writer, raw []byte, base int, php, phpLiterals []parser.Span) bool {
	raw = loneCRAsLF(raw, base, php)

	// A lone CR left now sits inside PHP, where it is a byte of data, not a
	// line break this layout could re-home. A run of them ending a line goes,
	// as the verbatim path drops it.
	if hasInnerLoneCR(raw) {
		return false
	}

	frozen, ok := scriptFrozenSpans(raw, base, php, phpLiterals)
	if !ok {
		return false
	}

	lines := scriptLines(raw)

	for len(lines) > 0 && isDroppableRawLine(raw[lines[0][0]:lines[0][1]]) {
		lines = lines[1:]
	}

	for len(lines) > 0 && isDroppableRawLine(raw[lines[len(lines)-1][0]:lines[len(lines)-1][1]]) {
		lines = lines[:len(lines)-1]
	}

	if len(lines) == 0 {
		return false
	}

	var shifted [][]byte

	for _, l := range lines {
		if !insideSpan(l[0], frozen) {
			shifted = append(shifted, raw[l[0]:l[1]])
		}
	}

	common := commonRawIndent(shifted)

	w.Indent()

	for _, l := range lines {
		// A CR ending a line is a CRLF's, or one in PHP the verbatim path drops.
		line := bytes.TrimRight(raw[l[0]:l[1]], "\r")

		w.Newline()

		if insideSpan(l[0], frozen) {
			w.WithoutIndent(func() { w.WriteBytes(line) })

			continue
		}

		body := line[dropIndent(line, common):]
		if !insideSpan(l[1], frozen) {
			body = bytes.TrimRight(body, " \t\r")
		}

		if len(body) > 0 {
			w.WriteBytes(body)
		}
	}

	w.Dedent()
	w.Newline()

	return true
}

// scriptFrozenSpans returns the ranges of raw whose line starts must not move:
// multi-line JavaScript literals and the PHP literals the parser found in
// Blade constructs. A Blade construct is masked to identifier bytes before
// lexing, since `@json(...)` or a quote inside `{{ }}` is not JavaScript the
// lexer could read. So is every `@`: a bare directive (`@endif`) has no
// recorded span, and in JavaScript the byte only ever sits inside a literal or
// comment or starts a decorator, where an identifier byte in its place moves no
// literal boundary.
func scriptFrozenSpans(raw []byte, base int, php, phpLiterals []parser.Span) ([][2]int, bool) {
	masked := raw
	if len(php) > 0 || bytes.IndexByte(raw, '@') >= 0 {
		masked = bytes.ReplaceAll(raw, []byte("@"), []byte("_"))
	}

	for _, s := range php {
		start, end := s.Start.Offset-base, s.End.Offset-base
		if start < 0 || end > len(raw) || start >= end {
			continue
		}

		for i := start; i < end; i++ {
			if masked[i] != '\n' && masked[i] != '\r' {
				masked[i] = '_'
			}
		}
	}

	var frozen [][2]int

	for _, s := range phpLiterals {
		start, end := s.Start.Offset-base, s.End.Offset-base
		if start >= 0 && end <= len(raw) && start < end {
			frozen = append(frozen, [2]int{start, end})
		}
	}

	literals, ok := alpine.MultilineLiterals(masked)
	if !ok {
		return nil, false
	}

	return append(frozen, literals...), true
}

// scriptLines returns the [start, end) offsets of raw's lines split at LF; a
// CRLF leaves its CR on the line.
func scriptLines(raw []byte) [][2]int {
	var lines [][2]int

	start := 0

	for i, c := range raw {
		if c == '\n' {
			lines = append(lines, [2]int{start, i})
			start = i + 1
		}
	}

	return append(lines, [2]int{start, len(raw)})
}

// insideSpan reports whether off falls strictly inside one of spans, so a line
// starting (or ending) there is part of that span's value.
func insideSpan(off int, spans [][2]int) bool {
	for _, s := range spans {
		if off > s[0] && off < s[1] {
			return true
		}
	}

	return false
}

// loneCRAsLF rewrites each lone CR outside PHP as LF, the break HTML reads it as
// before tokenizing, so a body and its LF twin lay out alike. The length is
// unchanged, so every offset into raw still holds.
func loneCRAsLF(raw []byte, base int, php []parser.Span) []byte {
	out := raw

	for i, c := range raw {
		if c != '\r' || (i+1 < len(raw) && raw[i+1] == '\n') || insideAny(base+i, php) {
			continue
		}

		if &out[0] == &raw[0] {
			out = bytes.Clone(raw)
		}

		out[i] = '\n'
	}

	return out
}

// hasInnerLoneCR reports whether b holds a lone CR with more than CRs after it
// on its line.
func hasInnerLoneCR(b []byte) bool {
	for i := 0; i < len(b); i++ {
		if b[i] != '\r' {
			continue
		}

		end := i
		for end < len(b) && b[end] == '\r' {
			end++
		}

		if end < len(b) && b[end] != '\n' {
			return true
		}

		i = end
	}

	return false
}
