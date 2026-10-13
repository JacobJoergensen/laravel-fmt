package diff

import "strings"

// LineEdit replaces lines [Start, End) of strings.Split(old, "\n") with
// Lines; an insertion has Start == End.
type LineEdit struct {
	Start int
	End   int
	Lines []string
}

// LineEdits returns the minimal edits from old to updated, one per run of
// changed lines. Editing only those keeps an editor's cursor, folds and undo
// history, which replacing the whole document loses.
func LineEdits(old, updated string) []LineEdit {
	if old == updated {
		return nil
	}

	ops := diffLines(strings.Split(old, "\n"), strings.Split(updated, "\n"))

	var edits []LineEdit
	var lines []string

	hunkStart := -1
	oldIdx := 0

	flush := func() {
		if hunkStart < 0 {
			return
		}

		edits = append(edits, LineEdit{Start: hunkStart, End: oldIdx, Lines: lines})
		hunkStart = -1
		lines = nil
	}

	for _, op := range ops {
		switch op.kind {
		case opKeep:
			flush()
			oldIdx++
		case opDelete:
			if hunkStart < 0 {
				hunkStart = oldIdx
			}

			oldIdx++
		case opInsert:
			if hunkStart < 0 {
				hunkStart = oldIdx
			}

			lines = append(lines, op.line)
		}
	}

	flush()

	return edits
}

// LineStarts returns each line's first byte offset, indexed like LineEdit.
// Only "\n" ends a line, matching the split LineEdits works from.
func LineStarts(old []byte) []int {
	starts := []int{0}

	for i, b := range old {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}

	return starts
}

// Replacement returns the byte range of old that e replaces and its new text.
//
// CONTRACT: the newline travels with the span. At the end of a document with
// no trailing newline the edit takes the preceding "\n" instead of adding one,
// so no stray blank line is left.
func (e LineEdit) Replacement(old []byte, lineStarts []int) (start, end int, text string) {
	joined := strings.Join(e.Lines, "\n")
	empty := len(e.Lines) == 0

	switch {
	case e.End < len(lineStarts):
		start, end = lineStarts[e.Start], lineStarts[e.End]

		if !empty {
			text = joined + "\n"
		}
	case e.Start == 0:
		start, end = 0, len(old)
		text = joined
	case e.Start >= len(lineStarts):
		start, end = len(old), len(old)

		if !empty {
			text = "\n" + joined
		}
	default:
		start, end = lineStarts[e.Start]-1, len(old)

		if !empty {
			text = "\n" + joined
		}
	}

	return start, end, text
}

// lineSpan is the range e's own lines occupy. Replacement may reach one byte
// further back for a newline, and a range must not pull in a change just for
// touching that byte.
func (e LineEdit) lineSpan(old []byte, lineStarts []int) (int, int) {
	if e.Start >= len(lineStarts) {
		return len(old), len(old)
	}

	if e.End < len(lineStarts) {
		return lineStarts[e.Start], lineStarts[e.End]
	}

	return lineStarts[e.Start], len(old)
}

// ApplyInRange returns old with the changes in the byte range [start, end)
// taken from updated. The unit is a whole change, so a range reaching into a
// line takes every line that change covers.
//
// CONTRACT: a change is not always self-contained. Rewriting array( to [
// touches two lines apart, and taking one alone does not parse, so the
// selection widens until valid accepts it, at worst to all of updated.
//
// WARNING: widening cannot rescue a pass that moves code, since no selection
// makes half a move whole. Such a pass must be off; see
// config.DisableRelocatingPasses.
func ApplyInRange(old, updated []byte, start, end int, valid func([]byte) bool) []byte {
	edits := LineEdits(string(old), string(updated))
	if len(edits) == 0 {
		return old
	}

	lineStarts := LineStarts(old)
	lo, hi := hunkWindow(old, edits, lineStarts, start, end)

	if lo == hi {
		return old
	}

	if lo == 0 && hi == len(edits) {
		return updated
	}

	out := spliceEdits(old, edits, lineStarts, lo, hi)

	for step := 1; !valid(out); step *= 2 {
		if lo == 0 && hi == len(edits) {
			return updated
		}

		lo = max(lo-step, 0)
		hi = min(hi+step, len(edits))
		out = spliceEdits(old, edits, lineStarts, lo, hi)
	}

	return out
}

// Edits are ordered and disjoint, so the ones a range touches are adjacent.
func hunkWindow(old []byte, edits []LineEdit, lineStarts []int, start, end int) (int, int) {
	lo, hi := len(edits), 0

	for i, e := range edits {
		lineFrom, lineTo := e.lineSpan(old, lineStarts)

		if !inRange(lineFrom, lineTo, start, end) {
			continue
		}

		lo = min(lo, i)
		hi = i + 1
	}

	if lo > hi {
		return 0, 0
	}

	return lo, hi
}

func spliceEdits(old []byte, edits []LineEdit, lineStarts []int, lo, hi int) []byte {
	out := make([]byte, 0, len(old))
	cursor := 0

	// LineEdits flushes on every kept line, so consecutive edits always have a
	// line between them and never walk the cursor backwards.
	for _, e := range edits[lo:hi] {
		from, to, text := e.Replacement(old, lineStarts)

		out = append(out, old[cursor:from]...)
		out = append(out, text...)
		cursor = to
	}

	return append(out, old[cursor:]...)
}

// An insertion covers no bytes, so it counts wherever the range reaches it,
// either end included; otherwise a line appended at the end of the input
// would be dropped by a range ending there.
func inRange(lineFrom, lineTo, start, end int) bool {
	if lineFrom == lineTo {
		return start <= lineFrom && lineFrom <= end
	}

	return lineFrom < end && start < lineTo
}
