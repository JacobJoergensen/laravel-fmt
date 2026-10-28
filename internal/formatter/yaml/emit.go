package yaml

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/yaml"
)

func emitFile(w *formatter.Writer, file *parser.File) {
	// Freeze the WHOLE file byte-for-byte when it cannot be safely modeled.
	// Re-emitting such input as structure yields cleaner output that reparses
	// into a different, normalized shape; freezing is the only fixed point.
	if shouldFreezeFile(file) {
		// Start at the first content line: leading blank lines are trivia the
		// document path never re-emits, so freezing them would leave output the
		// next pass strips, and the emission would not be a fixed point.
		start := parser.Position{Offset: contentLineStart(file.Src)}

		var last parser.Node

		for _, doc := range file.Docs {
			if doc.Body != nil {
				last = doc.Body
			}
		}

		emitFrozen(w, file.Src, parser.Span{Start: start, End: parser.Position{Offset: len(file.Src)}}, last)

		return
	}

	for _, doc := range file.Docs {
		emitDocument(w, file.Src, doc)
	}
}

func contentLineStart(src []byte) int {
	start := 0

	for i := 0; i < len(src); i++ {
		switch src[i] {
		case ' ', '\t':
		case '\n':
			start = i + 1
		case '\r':
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}

			start = i + 1
		default:
			return start
		}
	}

	return start
}

// shouldFreezeFile reports input the formatter must keep verbatim: a document
// holding a Bail node (a region the parser could not resolve), a non-first
// document with no `---` (a split the parser made from inconsistent
// indentation, which reparses differently), or a key that reads as a
// document marker at column 0.
func shouldFreezeFile(file *parser.File) bool {
	for i, doc := range file.Docs {
		if doc.Body != nil && bodyHasBail(doc.Body) {
			return true
		}

		if i > 0 && !hasSpan(doc.StartMarker) && docHasContent(doc) {
			return true
		}

		if docHasColumnZeroMarkerScalar(doc) {
			return true
		}
	}

	return false
}

func docHasColumnZeroMarkerScalar(doc *parser.Document) bool {
	mapping, ok := doc.Body.(*parser.Mapping)
	if !ok {
		return false
	}

	for _, e := range mapping.Entries {
		if e.Key != nil && keyReparsesAsDocMarker(e.Key.Raw) {
			return true
		}
	}

	return false
}

// keyReparsesAsDocMarker reports whether a mapping key, emitted unquoted at column
// 0, would be read as a document marker rather than a scalar. A bare `---`/`...` is
// a marker outright; a `--- `-prefixed key becomes a directives-end marker with
// inline content (`--- 0:`) that the next pass splits onto two lines, and a
// `... `-prefixed key becomes a document-end marker whose rest of line starts a
// NEW document, which loses the key entirely.
func keyReparsesAsDocMarker(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)

	if bytes.Equal(trimmed, []byte("---")) || bytes.Equal(trimmed, []byte("...")) {
		return true
	}

	for _, marker := range [...]string{"--- ", "---\t", "... ", "...\t"} {
		if bytes.HasPrefix(trimmed, []byte(marker)) {
			return true
		}
	}

	return false
}

// docHasContent reports whether a document is more than whitespace and comments.
// A trailing whitespace-only line parses into a content-less phantom document
// that must not trip the marker-less non-first-document freeze.
func docHasContent(doc *parser.Document) bool {
	return doc.Body != nil || hasSpan(doc.StartMarker) || hasSpan(doc.EndMarker) || len(doc.Directives) > 0
}

func bodyHasBail(node parser.Node) bool {
	switch n := node.(type) {
	case *parser.Verbatim:
		return n.Bail
	case *parser.Mapping:
		for _, e := range n.Entries {
			if e.Value != nil && bodyHasBail(e.Value) {
				return true
			}
		}
	case *parser.Sequence:
		for _, item := range n.Items {
			if item.Value != nil && bodyHasBail(item.Value) {
				return true
			}
		}
	}

	return false
}

func emitDocument(w *formatter.Writer, src []byte, doc *parser.Document) {
	emitTrivia(w, doc.Leading)

	for _, directive := range doc.Directives {
		emitVerbatimSpan(w, src, directive)
		w.Newline()
	}

	if hasSpan(doc.StartMarker) {
		w.Write("---")
		w.Newline()
	}

	if doc.Body != nil {
		if leadingHasIgnore(doc.Leading) {
			// Freeze from the body through the document end so trailing trivia is
			// covered by the frozen span, not emitted again below as a blank line.
			// The span starts at the body's line, or an indented body's first line
			// alone would lose its indent.
			bodyStart := doc.Body.Span().Start
			lineStart := parser.Position{Offset: bodyStart.Offset - (bodyStart.Column - 1), Line: bodyStart.Line, Column: 1}
			emitFrozen(w, src, parser.Span{Start: lineStart, End: doc.Range.End}, doc.Body)

			return
		}

		emitNode(w, src, doc.Body)
	}

	if hasSpan(doc.EndMarker) {
		w.Write("...")
		w.Newline()
	}

	emitTrivia(w, doc.Trailing)
}

// emitNode emits a node that stands on its own at the current indent: a
// document body or a block value already positioned on a fresh line. Only a
// collection gets here, a flow one only as a whole document; any other body
// bails, which freezes the file.
func emitNode(w *formatter.Writer, src []byte, node parser.Node) {
	switch n := node.(type) {
	case *parser.Mapping:
		emitMapping(w, src, n)
	case *parser.Sequence:
		emitSequence(w, src, n)
	case *parser.Flow:
		formatter.EmitVerbatimStripping(w, src[n.Range.Start.Offset:n.Range.End.Offset], n.Indent)
		w.Newline()
	}
}

func emitMapping(w *formatter.Writer, src []byte, m *parser.Mapping) {
	for _, e := range m.Entries {
		emitTrivia(w, e.Leading)

		if leadingHasIgnore(e.Leading) {
			emitFrozen(w, src, e.Range, e.Value)

			continue
		}

		emitMappingEntry(w, src, e)
	}
}

func emitMappingEntry(w *formatter.Writer, src []byte, e *parser.MappingEntry) {
	w.WriteBytes(e.Key.Raw)
	w.Write(":")

	if emitLeafValue(w, src, e.Value, e.Comment, 0) {
		return
	}

	emitInlineComment(w, e.Comment)
	w.Newline()
	w.Indent()
	emitNode(w, src, e.Value)
	w.Dedent()
}

func emitSequence(w *formatter.Writer, src []byte, s *parser.Sequence) {
	for _, item := range s.Items {
		emitTrivia(w, item.Leading)

		if leadingHasIgnore(item.Leading) {
			emitFrozen(w, src, item.Range, item.Value)

			continue
		}

		w.Write("-")
		emitItemValue(w, src, item.Value, item.Comment)
	}
}

func emitItemValue(w *formatter.Writer, src []byte, value parser.Node, comment *parser.Trivia) {
	if emitLeafValue(w, src, value, comment, seqValueOffset) {
		return
	}

	switch v := value.(type) {
	case *parser.Mapping:
		// A same-line comment ends the dash line, so the mapping drops to the
		// next line; without this the comment is silently lost. Trivia leading
		// the mapping's first entry ends it too: written after the dash it comes
		// back as the ITEM's inline comment, which moves what a comment annotates
		// and stops a `laravel-fmt-ignore` from freezing the entry.
		if comment != nil || firstEntryHasTrivia(v) {
			emitInlineComment(w, comment)
			w.Newline()
			w.Indent()
			emitMapping(w, src, v)
			w.Dedent()

			return
		}

		// The first key sits after the `- `, so the keys below line up with it
		// whatever the indent width: at a level in they would not be its siblings.
		w.Space()
		w.WithAlign(seqValueOffset, func() { emitMapping(w, src, v) })
	case *parser.Sequence:
		emitInlineComment(w, comment)
		w.Newline()
		w.Indent()
		emitSequence(w, src, v)
		w.Dedent()
	}
}

func firstEntryHasTrivia(m *parser.Mapping) bool {
	return len(m.Entries) > 0 && len(m.Entries[0].Leading) > 0
}

// seqValueOffset is how far past the Writer's indent a sequence item's value
// starts: the `- ` the emitter writes before it. The parser reads such a value
// against that column, so a span re-homed under it must be measured from there
// and not from the Writer's indent.
const seqValueOffset = len("- ")

// emitLeafValue emits an inline value, closing its line, and reports true.
// Mapping and sequence values return false for the caller to lay out as a block.
// baseOffset is the caller's own prefix width past the Writer's indent.
func emitLeafValue(w *formatter.Writer, src []byte, value parser.Node, comment *parser.Trivia, baseOffset int) bool {
	switch v := value.(type) {
	case nil:
		emitInlineComment(w, comment)
		w.Newline()
	case *parser.Scalar:
		w.Space()
		// A quoted scalar may span lines. Strip the block indent it was read
		// against, less what the caller already wrote past the Writer's
		// indent, as the Verbatim case below does; the source line's own
		// indent is not that distance, since a `- ` marker is structural.
		// Trailing whitespace stays, being inside the quotes.
		formatter.EmitVerbatimStrippingKeepTrailing(
			w, src[v.Range.Start.Offset:v.Range.End.Offset], max(v.Indent-baseOffset, 0),
		)
		emitInlineComment(w, comment)
		w.Newline()
	case *parser.Flow:
		w.Space()
		// Continuation lines move with the key, as a quoted scalar's do: kept at
		// their source column they could end up no deeper than the key.
		formatter.EmitVerbatimStripping(w, src[v.Range.Start.Offset:v.Range.End.Offset], max(v.Indent-baseOffset, 0))
		emitInlineComment(w, comment)
		w.Newline()
	case *parser.BlockScalar:
		w.Space()
		emitBlockScalar(w, src, v)
		w.Newline()
	case *parser.Verbatim:
		w.Space()
		// The value's span starts at the block indent and may end with trailing
		// whitespace, both structure the emitter owns; emitting them after the
		// separator space would double the gap and collapse when the value
		// reparses inline on the next pass.
		start := v.Range.Start.Offset
		end := v.Range.End.Offset

		for start < end && (src[start] == ' ' || src[start] == '\t') {
			start++
		}

		for end > start && (src[end-1] == ' ' || src[end-1] == '\t' || src[end-1] == '\r' || src[end-1] == '\n') {
			end--
		}

		// Strip the block indent the span was read against, less what the
		// caller already wrote past the Writer's indent: only that distance
		// keeps the node's continuation lines inside it once re-homed.
		formatter.EmitVerbatimStripping(w, src[start:end], max(v.Indent-baseOffset, 0))
		w.Newline()
	default:
		return false
	}

	return true
}

func emitTrivia(w *formatter.Writer, trivia []parser.Trivia) {
	for _, tr := range trivia {
		switch tr.Kind {
		case parser.TriviaBlank:
			w.RequestBlankLine()
		case parser.TriviaComment:
			w.WriteBytes(bytes.Trim(tr.Text, " \t\r\n"))
			w.Newline()
		}
	}
}

func emitInlineComment(w *formatter.Writer, comment *parser.Trivia) {
	if comment == nil {
		return
	}

	w.Space()
	w.WriteBytes(bytes.TrimRight(comment.Text, " \t\r\n"))
}

func emitVerbatimSpan(w *formatter.Writer, src []byte, span parser.Span) {
	formatter.EmitVerbatim(w, src, span.Start.Offset, span.End.Offset)
}

// emitBlockScalar re-homes a `|`/`>` body under the Writer's indent. It cannot
// strip the header line's own indent instead: a block scalar nested in a
// sequence item sits on a `- key: >` line whose leading whitespace is zero,
// while the Writer re-adds the item's structural indent, so the body would gain
// that indent again on every pass.
//
// A body without an indent digit moves to one indent under its owner, every
// line shifted alike, so its value is unchanged. A body with one keeps its
// distance from its owner: moving it would change the value.
func emitBlockScalar(w *formatter.Writer, src []byte, n *parser.BlockScalar) {
	raw := src[n.Range.Start.Offset:n.Range.End.Offset]

	if n.ContentIndent > 0 {
		w.Indent()
		formatter.EmitVerbatimStrippingKeepTrailing(w, raw, n.ContentIndent)
		w.Dedent()

		return
	}

	formatter.EmitVerbatimStrippingKeepTrailing(w, raw, n.Indent)
}

// loneCRNormalized rewrites a `\r` that is not part of a `\r\n` to `\n`,
// returning raw untouched when there is none. YAML 1.2.2 requires it: [28]
// b-break admits a lone CR, and 5.4 reads a break inside scalar content as one
// line feed. The parser already ends a line on a lone CR while the verbatim
// writer breaks only on `\n`, so a span holding one would otherwise come back
// as a single line.
//
// Other languages must not share this: a lone CR in a PHP string is data.
func loneCRNormalized(raw []byte) []byte {
	if !hasLoneCR(raw) {
		return raw
	}

	out := make([]byte, len(raw))
	copy(out, raw)

	for i, c := range out {
		if c == '\r' && (i+1 >= len(out) || out[i+1] != '\n') {
			out[i] = '\n'
		}
	}

	return out
}

func hasLoneCR(raw []byte) bool {
	for i, c := range raw {
		if c == '\r' && (i+1 >= len(raw) || raw[i+1] != '\n') {
			return true
		}
	}

	return false
}

func hasSpan(s parser.Span) bool {
	return s.End.Offset > s.Start.Offset
}
