// Package diff computes line-level diffs and renders source snippets.
package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
)

// Three lines of context, the GNU diff and git default.
const contextLines = 3

const noNewlineMarker = `\ No newline at end of file`

// Unified returns a unified diff from old to updated that git apply and
// patch -p1 accept, or "" when they are equal. The zero Palette writes no
// color.
func Unified(path, old, updated string, p terminal.Palette) string {
	if old == updated {
		return ""
	}

	ops := diffLines(splitLines(old), splitLines(updated))

	var out strings.Builder

	fmt.Fprintf(&out, "%s--- a/%s%s\n", p.Bold, path, p.Reset)
	fmt.Fprintf(&out, "%s+++ b/%s%s\n", p.Bold, path, p.Reset)

	for _, h := range hunks(ops) {
		writeHunk(&out, ops, h, p)
	}

	return out.String()
}

// Lines keep their terminator, so a last line without one differs from the
// same text with one, as it does on disk.
func splitLines(s string) []string {
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// hunk is a run of ops, [start, end), and the line where it begins on each side.
type hunk struct {
	start, end       int
	oldLine, newLine int
}

func hunks(ops []editOp) []hunk {
	var out []hunk

	oldLine, newLine := 1, 1

	for i := 0; i < len(ops); {
		if ops[i].kind == opKeep {
			oldLine++
			newLine++
			i++

			continue
		}

		start := max(i-contextLines, 0)
		last := i

		// Merge while the unchanged lines since the last change, j-last-1 of
		// them, are few enough that the two contexts would meet.
		for j := i + 1; j < len(ops) && j-last-1 <= 2*contextLines; j++ {
			if ops[j].kind != opKeep {
				last = j
			}
		}

		end := min(last+1+contextLines, len(ops))
		lead := i - start

		out = append(out, hunk{start: start, end: end, oldLine: oldLine - lead, newLine: newLine - lead})

		for _, op := range ops[i:end] {
			if op.kind != opInsert {
				oldLine++
			}

			if op.kind != opDelete {
				newLine++
			}
		}

		i = end
	}

	return out
}

func writeHunk(out *strings.Builder, ops []editOp, h hunk, p terminal.Palette) {
	oldCount, newCount := 0, 0

	for _, op := range ops[h.start:h.end] {
		if op.kind != opInsert {
			oldCount++
		}

		if op.kind != opDelete {
			newCount++
		}
	}

	fmt.Fprintf(out, "%s@@ -%s +%s @@%s\n", p.Cyan, hunkRange(h.oldLine, oldCount), hunkRange(h.newLine, newCount), p.Reset)

	for _, op := range ops[h.start:h.end] {
		switch op.kind {
		case opKeep:
			writeLine(out, " ", "", op.line, p)
		case opDelete:
			writeLine(out, "-", p.Red, op.line, p)
		case opInsert:
			writeLine(out, "+", p.Green, op.line, p)
		}
	}
}

// hunkRange formats one side of an @@ line the way GNU diff does: the count is
// left out when it is 1, and an empty side names the line before it.
func hunkRange(line, count int) string {
	switch count {
	case 0:
		return fmt.Sprintf("%d,0", line-1)
	case 1:
		return strconv.Itoa(line)
	default:
		return fmt.Sprintf("%d,%d", line, count)
	}
}

func writeLine(out *strings.Builder, prefix, color, line string, p terminal.Palette) {
	content, hasNewline := strings.CutSuffix(line, "\n")

	out.WriteString(color)
	out.WriteString(prefix)
	out.WriteString(markTrailingWhitespace(content, prefix != " ", p))
	out.WriteString(p.Reset)
	out.WriteString("\n")

	if !hasNewline {
		out.WriteString(noNewlineMarker)
		out.WriteString("\n")
	}
}

// The mark is empty without color, so a plain patch stays byte-exact.
func markTrailingWhitespace(content string, changed bool, p terminal.Palette) string {
	if !changed || p.WhitespaceMark == "" {
		return content
	}

	body := strings.TrimRight(content, " \t\r")
	trailing := strings.TrimRight(content[len(body):], "\r")

	if trailing == "" {
		return content
	}

	return body + p.WhitespaceMark + trailing + p.Reset + content[len(body)+len(trailing):]
}

type opKind int

const (
	opKeep opKind = iota
	opDelete
	opInsert
)

type editOp struct {
	kind opKind
	line string
}

// diffLines is linear-space Myers: a reformatted file can change lines
// throughout, so a full O(n×m) LCS table is not affordable.
func diffLines(a, b []string) []editOp {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}

	ta, tb := a[prefix:], b[prefix:]

	suffix := 0
	for suffix < len(ta) && suffix < len(tb) && ta[len(ta)-1-suffix] == tb[len(tb)-1-suffix] {
		suffix++
	}

	aMid := len(ta) - suffix
	bMid := len(tb) - suffix

	ops := make([]editOp, 0, len(a)+len(b))

	for _, line := range a[:prefix] {
		ops = append(ops, editOp{opKeep, line})
	}

	ops = append(ops, diffMiddle(ta[:aMid], tb[:bMid])...)

	for _, line := range ta[aMid:] {
		ops = append(ops, editOp{opKeep, line})
	}

	return ops
}

// diffMiddle diffs two slices that share no common first or last line.
func diffMiddle(a, b []string) []editOp {
	switch {
	case len(a) == 0 && len(b) == 0:
		return nil
	case len(a) == 0:
		return runOps(opInsert, b)
	case len(b) == 0:
		return runOps(opDelete, a)
	default:
		return bisect(a, b)
	}
}

func runOps(kind opKind, lines []string) []editOp {
	ops := make([]editOp, len(lines))

	for i, line := range lines {
		ops[i] = editOp{kind, line}
	}

	return ops
}

// With no snake inside the edit-distance bound the inputs share nothing, so
// bisect replaces them wholesale.
func bisect(a, b []string) []editOp {
	n, m := len(a), len(b)
	maxD := (n + m + 1) / 2
	offset := maxD
	length := 2*maxD + 2

	v1 := make([]int, length)
	v2 := make([]int, length)

	for i := range v1 {
		v1[i] = -1
		v2[i] = -1
	}

	v1[offset+1] = 0
	v2[offset+1] = 0

	delta := n - m
	front := delta%2 != 0
	k1start, k1end, k2start, k2end := 0, 0, 0, 0

	for d := range maxD {
		for k1 := -d + k1start; k1 <= d-k1end; k1 += 2 {
			k1Offset := offset + k1

			var x1 int
			if k1 == -d || (k1 != d && v1[k1Offset-1] < v1[k1Offset+1]) {
				x1 = v1[k1Offset+1]
			} else {
				x1 = v1[k1Offset-1] + 1
			}

			y1 := x1 - k1
			for x1 < n && y1 < m && a[x1] == b[y1] {
				x1++
				y1++
			}

			v1[k1Offset] = x1

			switch {
			case x1 > n:
				k1end += 2
			case y1 > m:
				k1start += 2
			case front:
				k2Offset := offset + delta - k1
				if k2Offset >= 0 && k2Offset < length && v2[k2Offset] != -1 && x1 >= n-v2[k2Offset] {
					return bisectSplit(a, b, x1, y1)
				}
			}
		}

		for k2 := -d + k2start; k2 <= d-k2end; k2 += 2 {
			k2Offset := offset + k2

			var x2 int
			if k2 == -d || (k2 != d && v2[k2Offset-1] < v2[k2Offset+1]) {
				x2 = v2[k2Offset+1]
			} else {
				x2 = v2[k2Offset-1] + 1
			}

			y2 := x2 - k2
			for x2 < n && y2 < m && a[n-x2-1] == b[m-y2-1] {
				x2++
				y2++
			}

			v2[k2Offset] = x2

			switch {
			case x2 > n:
				k2end += 2
			case y2 > m:
				k2start += 2
			case !front:
				k1Offset := offset + delta - k2
				if k1Offset >= 0 && k1Offset < length && v1[k1Offset] != -1 && v1[k1Offset] >= n-x2 {
					return bisectSplit(a, b, v1[k1Offset], v1[k1Offset]-(delta-k2))
				}
			}
		}
	}

	return append(runOps(opDelete, a), runOps(opInsert, b)...)
}

func bisectSplit(a, b []string, x, y int) []editOp {
	left := diffLines(a[:x], b[:y])
	right := diffLines(a[x:], b[y:])

	return append(left, right...)
}
