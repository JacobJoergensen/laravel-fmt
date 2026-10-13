package document

import (
	"bytes"
	"slices"
)

// A builder cannot know the column an AlignGap will print at: that depends on
// break decisions the printer has yet to make. So the gap prints one space,
// the printer records where it landed, and the padding is computed here from
// the finished lines. Consecutive lines that each carry a gap of the same
// construct share a column, and any other line ends the run.

// CommentScope is the AlignGap scope of one trailing-comment group; it pads
// after the others, so its column includes their padding.
func CommentScope(id int32) int32 {
	return -1 - id
}

func isCommentScope(scope int32) bool {
	return scope < 0
}

type alignGap struct {
	offset  int
	column  int
	scope   int32
	reserve int  // columns the layout counted for the gap but did not print
	cut     bool // the run ends after this gap: more padding would have broken what follows
	alone   bool // the gap takes no run: any padding would have broken what follows
}

// expandAlignGaps needs gaps in increasing offset order, since the newlines
// between two neighbors decide whether they sit on consecutive lines.
func expandAlignGaps(out []byte, gaps []alignGap) []byte {
	extra := make([]int, len(gaps))
	columns := make([]int, len(gaps))

	for i, g := range gaps {
		columns[i] = g.column
	}

	padRuns(out, gaps, columns, extra, func(scope int32) bool { return !isCommentScope(scope) })

	// A comment gap moves with every gap padded before it on its line, and an
	// earlier comment's padding waits on its own run. Each round settles at
	// least the leftmost unsettled comment of every line, so rounds stop once
	// no padding changes.
	base := slices.Clone(columns)
	settled := make([]int, len(gaps))

	for {
		for i, g := range gaps {
			if !isCommentScope(g.scope) {
				continue
			}

			columns[i] = base[i]
			for j := i - 1; j >= 0 && !bytes.Contains(out[gaps[j].offset:g.offset], []byte{'\n'}); j-- {
				columns[i] += extra[j] - gaps[j].reserve
			}
		}

		copy(settled, extra)
		padRuns(out, gaps, columns, extra, isCommentScope)

		if slices.Equal(settled, extra) {
			break
		}
	}

	total := 0
	for _, n := range extra {
		total += n
	}

	if total == 0 {
		return out
	}

	padded := make([]byte, 0, len(out)+total)
	prev := 0

	for i, g := range gaps {
		padded = append(padded, out[prev:g.offset]...)
		padded = append(padded, bytes.Repeat([]byte{' '}, extra[i])...)
		prev = g.offset
	}

	return append(padded, out[prev:]...)
}

func padRuns(out []byte, gaps []alignGap, columns, extra []int, selected func(int32) bool) {
	byScope := make(map[int32][]int)

	var scopes []int32

	for i, g := range gaps {
		if !selected(g.scope) {
			continue
		}

		if _, seen := byScope[g.scope]; !seen {
			scopes = append(scopes, g.scope)
		}

		byScope[g.scope] = append(byScope[g.scope], i)
	}

	for _, scope := range scopes {
		idx := byScope[scope]
		start := 0

		for k := 1; k <= len(idx); k++ {
			if k < len(idx) && !gaps[idx[k-1]].cut && !gaps[idx[k]].alone && continuesRun(out, gaps[idx[k-1]], gaps[idx[k]]) {
				continue
			}

			widest := 0
			for _, i := range idx[start:k] {
				widest = max(widest, columns[i])
			}

			for _, i := range idx[start:k] {
				extra[i] = widest - columns[i]
			}

			start = k
		}
	}
}

// Two gaps of one scope on one line come only from a flat construct, where
// nothing aligns, so a run needs exactly one newline between neighbors.
func continuesRun(out []byte, a, b alignGap) bool {
	return bytes.Count(out[a.offset:b.offset], []byte{'\n'}) == 1
}
