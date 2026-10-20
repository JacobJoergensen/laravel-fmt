package formatter

import "bytes"

// NormalizeLineTerminators returns raw with every line terminator rewritten to
// eol. A run of CRs closed by a line feed collapses into one terminator, which
// is what emitVerbatim's per-line CR trim already does, so a span reachable by
// both paths comes out the same either way. A CR no line feed closes is left
// alone: there it is content, a byte inside a string literal, not a terminator.
//
// The collapse is what makes this a fixpoint. Replacing only `\r\n` is not:
// the replacement can pair with a preceding CR and match again on the next
// pass, so `\r\r\n` loses one CR per run of the formatter.
func NormalizeLineTerminators(raw []byte, eol string) []byte {
	if eol == "\n" && bytes.IndexByte(raw, '\r') < 0 {
		return raw
	}

	out := make([]byte, 0, len(raw))

	for i := 0; i < len(raw); {
		switch raw[i] {
		case '\n':
			out = append(out, eol...)
			i++
		case '\r':
			run := i
			for run < len(raw) && raw[run] == '\r' {
				run++
			}

			if run < len(raw) && raw[run] == '\n' {
				out = append(out, eol...)
				i = run + 1

				continue
			}

			out = append(out, raw[i:run]...)
			i = run
		default:
			out = append(out, raw[i])
			i++
		}
	}

	return out
}

// EmitVerbatim writes src[startOffset:endOffset] to w with two adjustments:
// line endings normalize to the Writer's eol, and the source's natural indent
// at startOffset is stripped from continuation lines so the Writer's indent
// controls absolute placement. Relative indentation is preserved, so a
// hand-aligned block stays aligned.
//
// A zero-length, inverted, or out-of-bounds range is silently ignored so
// callers can pass spans from best-effort parses without crashing.
func EmitVerbatim(w *Writer, src []byte, startOffset, endOffset int) {
	emitVerbatim(w, src, startOffset, endOffset, NaturalIndentBefore(src, startOffset), false)
}

// EmitVerbatimRebased is EmitVerbatim but strips the indent of the line at
// indentOffset rather than at startOffset, and drops each line's trailing
// whitespace. Use it when the span begins mid-line on a continuation of a
// multi-line construct (an element opened by a tag whose attributes span lines),
// where the start line's leading whitespace is the continuation's, not the
// structural indent, and the span holds code rather than a value.
func EmitVerbatimRebased(w *Writer, src []byte, startOffset, endOffset, indentOffset int) {
	emitVerbatim(w, src, startOffset, endOffset, NaturalIndentBefore(src, indentOffset), true)
}

// EmitVerbatimRebasedKeepTrailing is EmitVerbatimRebased for a span whose
// line-trailing whitespace is content, such as an attribute value or an
// element's text, where trimming would delete bytes the document carries.
func EmitVerbatimRebasedKeepTrailing(w *Writer, src []byte, startOffset, endOffset, indentOffset int) {
	emitVerbatim(w, src, startOffset, endOffset, NaturalIndentBefore(src, indentOffset), false)
}

// EmitVerbatimAtIndent is EmitVerbatim for a span the Writer re-homes at its own
// indent (a multi-line YAML flow or anchored value): it strips the Writer's
// indent and per-line trailing whitespace, not the source's, so re-emission does
// not compound the indent or flip trailing whitespace across passes.
func EmitVerbatimAtIndent(w *Writer, src []byte, startOffset, endOffset int) {
	emitVerbatim(w, src, startOffset, endOffset, w.IndentColumn(), true)
}

// EmitVerbatimStripping writes raw with stripCols columns of indent removed from
// each continuation line, for a caller that knows the span's structural indent
// but has had to normalize the bytes before emitting, so the source line the
// indent came from is no longer reachable from the slice.
func EmitVerbatimStripping(w *Writer, raw []byte, stripCols int) {
	emitVerbatim(w, raw, 0, len(raw), stripCols, true)
}

// EmitVerbatimStrippingKeepTrailing is EmitVerbatimStripping for a span whose
// line-trailing whitespace is content rather than layout: a YAML literal block
// scalar preserves trailing spaces in its value, so trimming them would rewrite
// what the document means.
func EmitVerbatimStrippingKeepTrailing(w *Writer, raw []byte, stripCols int) {
	emitVerbatim(w, raw, 0, len(raw), stripCols, false)
}

func emitVerbatim(w *Writer, src []byte, startOffset, endOffset, naturalIndent int, trimTrailing bool) {
	if endOffset <= startOffset || endOffset > len(src) {
		return
	}

	raw := src[startOffset:endOffset]
	strip := uniformIndent(raw, naturalIndent)

	start := 0

	for i := range raw {
		if raw[i] != '\n' {
			continue
		}

		// Skip the write on an empty line: WriteBytes flushes the pending indent
		// even for no content, which would leave indent-only trailing whitespace.
		if c := raw[start:lineEnd(raw, start, i, trimTrailing)]; len(c) > 0 {
			w.WriteBytes(c)
		}

		w.Newline()
		start = i + 1

		start += skipIndent(raw[start:], strip)
	}

	if start < len(raw) {
		if c := raw[start:lineEnd(raw, start, len(raw), trimTrailing)]; len(c) > 0 {
			w.WriteBytes(c)
		}
	}
}

func lineEnd(raw []byte, start, end int, trimTrailing bool) int {
	for end > start {
		c := raw[end-1]

		if c == '\r' || (trimTrailing && (c == ' ' || c == '\t')) {
			end--

			continue
		}

		break
	}

	return end
}

// EmitVerbatimUnmoved writes a span at the column it already occupies, adding
// no indent and stripping none. For a span whose opener has not moved it is the
// only fixpoint: re-homing strips the source line's leading whitespace and
// re-adds the Writer's indent, which never converge when the Writer does not
// lay that line out (a comment glued to a multi-line `<pre>`).
func EmitVerbatimUnmoved(w *Writer, src []byte, startOffset, endOffset int) {
	if endOffset <= startOffset || endOffset > len(src) {
		return
	}

	w.WithoutIndent(func() {
		emitVerbatim(w, src[startOffset:endOffset], 0, endOffset-startOffset, 0, false)
	})
}

// EmitVerbatimExact writes raw at the column it already occupies with every byte
// kept, trailing whitespace and a lone CR included; only a CRLF line end becomes
// the Writer's own. It is for text the formatter does not model at all, where
// even the CR that EmitVerbatimUnmoved trims at a line end is content.
func EmitVerbatimExact(w *Writer, raw []byte) {
	lines := bytes.Split(raw, []byte("\n"))
	last := len(lines) - 1

	w.WithoutIndent(func() {
		for i, line := range lines {
			if i > 0 {
				w.Newline()
			}

			// Each CR still ending the line after its CRLF's own is a line break of
			// its own: written as a byte, the last of them and the break after it
			// would read back as one CRLF, and the next pass would lose it.
			loneBreaks := 0

			if i < last && bytes.HasSuffix(line, []byte("\r")) {
				line = line[:len(line)-1]

				for bytes.HasSuffix(line, []byte("\r")) {
					line = line[:len(line)-1]
					loneBreaks++
				}
			}

			if len(line) > 0 {
				w.WriteBytes(line)
			}

			for range loneBreaks {
				w.Newline()
			}
		}
	})
}

// ColumnUnmoved reports whether the span starting at offset is being written at
// the column it occupies in the source, i.e. its line is reproduced byte for byte
// and its continuation lines have nowhere to move to. Both sides are measured the
// Writer's way, so a tab counts the same in each.
func ColumnUnmoved(w *Writer, src []byte, offset int) bool {
	if offset < 0 || offset > len(src) {
		return false
	}

	lineStart := offset

	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	return w.Column() == VisualWidthBytes(src[lineStart:offset], w.tabWidth)
}

// NaturalIndentBefore returns the count of leading whitespace bytes on the source
// line containing offset, up to offset: the structural indent an emitter strips
// from a construct's continuation lines before re-homing them under the Writer's
// current indent.
func NaturalIndentBefore(src []byte, offset int) int {
	if offset <= 0 || offset > len(src) {
		return 0
	}

	lineStart := offset

	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	n := 0

	for i := lineStart; i < offset; i++ {
		if src[i] != ' ' && src[i] != '\t' {
			break
		}

		n++
	}

	return n
}

// uniformIndent returns how much leading whitespace every continuation line in
// raw can give up: the smallest non-blank continuation indent, capped at
// natural. It has to be one number for the whole block; stripping per line
// lets a shallower line give up all it has while its siblings give up natural,
// flattening a hand-aligned block under laravel-fmt-ignore or inside <pre>.
func uniformIndent(raw []byte, natural int) int {
	strip := natural
	first := true

	for line := range bytes.SplitSeq(raw, []byte("\n")) {
		if first {
			first = false

			continue
		}

		ws := skipIndent(line, len(line))
		if ws == lineEnd(line, 0, len(line), false) {
			continue
		}

		strip = min(strip, ws)
	}

	return strip
}

func skipIndent(line []byte, limit int) int {
	n := 0

	for n < limit && n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}

	return n
}
