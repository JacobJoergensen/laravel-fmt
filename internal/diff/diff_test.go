package diff

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
)

// Snippet reads the global colors; off, its expected strings stay exact.
func init() {
	terminal.Disable()
}

// Minimal means N+M-2·LCS edits, checked against lcsLen.
func TestDiffLinesReconstructsAndIsMinimal(t *testing.T) {
	check := func(t *testing.T, a, b []string) {
		t.Helper()

		ops := diffLines(a, b)
		gotA, gotB := applyOps(ops)

		if !slices.Equal(gotA, a) {
			t.Fatalf("ops do not reconstruct old input:\n got %v\nwant %v", gotA, a)
		}

		if !slices.Equal(gotB, b) {
			t.Fatalf("ops do not reconstruct updated input:\n got %v\nwant %v", gotB, b)
		}

		edits := 0

		for _, op := range ops {
			if op.kind != opKeep {
				edits++
			}
		}

		if want := len(a) + len(b) - 2*lcsLen(a, b); edits != want {
			t.Fatalf("edit count %d is not minimal, want %d\na=%v\nb=%v", edits, want, a, b)
		}
	}

	t.Run("random small", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(1, 2))

		for range 3000 {
			check(t, randLines(rng), randLines(rng))
		}
	})

	t.Run("large structured", func(t *testing.T) {
		even := make([]int, 0, 200)
		for i := 0; i < 400; i += 2 {
			even = append(even, i)
		}

		cases := [][]int{
			{1, 200, 398},   // scattered
			{200, 201, 202}, // clustered
			even,            // half the lines changed
		}

		for _, changed := range cases {
			old, updated := nearlyIdentical(400, changed...)
			check(t, strings.Split(old, "\n"), strings.Split(updated, "\n"))
		}
	})
}

func applyOps(ops []editOp) (old, updated []string) {
	for _, op := range ops {
		switch op.kind {
		case opKeep:
			old = append(old, op.line)
			updated = append(updated, op.line)
		case opDelete:
			old = append(old, op.line)
		case opInsert:
			updated = append(updated, op.line)
		}
	}

	return old, updated
}

// lcsLen is the O(n×m) minimality reference; it lives here so production
// keeps no such table.
func lcsLen(a, b []string) int {
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	return dp[len(a)][len(b)]
}

func randLines(rng *rand.Rand) []string {
	alphabet := []string{"a", "b", "c", "d"}
	lines := make([]string, rng.IntN(7))

	for i := range lines {
		lines[i] = alphabet[rng.IntN(len(alphabet))]
	}

	return lines
}

func TestUnifiedReturnsEmptyForIdenticalInput(t *testing.T) {
	if got := Unified("a.php", "x\ny\nz\n", "x\ny\nz\n", terminal.Palette{}); got != "" {
		t.Fatalf("identical input produced a diff:\n%s", got)
	}
}

func TestUnifiedWritesAPatch(t *testing.T) {
	numbered := func(n int) string {
		var b strings.Builder
		for i := 1; i <= n; i++ {
			b.WriteString("l")
			b.WriteString(strconv.Itoa(i))
			b.WriteString("\n")
		}

		return b.String()
	}

	tests := []struct {
		name         string
		old, updated string
		want         string
	}{
		{
			"one change with context",
			"a\nb\nc\nd\ne\n", "a\nb\nX\nd\ne\n",
			"--- a/f.php\n+++ b/f.php\n@@ -1,5 +1,5 @@\n a\n b\n-c\n+X\n d\n e\n",
		},
		{
			"distant changes make two hunks",
			numbered(20), strings.Replace(strings.Replace(numbered(20), "l2\n", "X\n", 1), "l19\n", "Y\n", 1),
			"--- a/f.php\n+++ b/f.php\n@@ -1,5 +1,5 @@\n l1\n-l2\n+X\n l3\n l4\n l5\n@@ -16,5 +16,5 @@\n l16\n l17\n l18\n-l19\n+Y\n l20\n",
		},
		{
			"changes six lines apart share a hunk",
			numbered(12), strings.Replace(strings.Replace(numbered(12), "l3\n", "X\n", 1), "l10\n", "Y\n", 1),
			"--- a/f.php\n+++ b/f.php\n@@ -1,12 +1,12 @@\n l1\n l2\n-l3\n+X\n l4\n l5\n l6\n l7\n l8\n l9\n-l10\n+Y\n l11\n l12\n",
		},
		{
			"an insertion into an empty file",
			"", "a\n",
			"--- a/f.php\n+++ b/f.php\n@@ -0,0 +1 @@\n+a\n",
		},
		{
			"a missing final newline is marked",
			"a\nb", "a\nb\n",
			"--- a/f.php\n+++ b/f.php\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n",
		},
		{
			"tabs, trailing spaces and CR stay bytes",
			"\tfoo \r\n", "    foo\r\n",
			"--- a/f.php\n+++ b/f.php\n@@ -1 +1 @@\n-\tfoo \r\n+    foo\r\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Unified("f.php", tc.old, tc.updated, terminal.Palette{}); got != tc.want {
				t.Fatalf("patch differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}
		})
	}
}

func TestUnifiedPatchAppliesBack(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	words := []string{"a", "b", "c", "\tx", "y ", ""}

	text := func() string {
		var b strings.Builder
		for range r.IntN(40) {
			b.WriteString(words[r.IntN(len(words))])
			b.WriteString("\n")
		}

		if r.IntN(4) == 0 {
			b.WriteString("tail")
		}

		return b.String()
	}

	for i := range 500 {
		old, updated := text(), text()

		patch := Unified("f", old, updated, terminal.Palette{})
		if old == updated {
			continue
		}

		if got := applyPatch(t, old, patch); got != updated {
			t.Fatalf("case %d: the patch does not apply back.\nold %q\nnew %q\npatch:\n%s\ngot %q", i, old, updated, patch, got)
		}
	}
}

func applyPatch(t *testing.T, old, patch string) string {
	t.Helper()

	src := splitLines(old)
	lines := strings.SplitAfter(patch, "\n")

	var out []string

	next := 0

	for i := 2; i < len(lines) && lines[i] != ""; {
		var oldStart, oldCount, newStart, newCount int
		if !parseRange(lines[i], &oldStart, &oldCount, &newStart, &newCount) {
			t.Fatalf("bad hunk header %q", lines[i])
		}

		begin := oldStart - 1
		if oldCount == 0 {
			begin = oldStart
		}

		out = append(out, src[next:begin]...)
		next = begin
		i++

		for i < len(lines) && lines[i] != "" && !strings.HasPrefix(lines[i], "@@") {
			body := lines[i][1:]
			noEOL := i+1 < len(lines) && strings.HasPrefix(lines[i+1], `\ `)

			if noEOL {
				body = strings.TrimSuffix(body, "\n")
			}

			switch lines[i][0] {
			case ' ', '-':
				if next >= len(src) || src[next] != body {
					t.Fatalf("patch line %q does not match source line %d %q", lines[i], next+1, src[min(next, len(src)-1)])
				}

				if lines[i][0] == ' ' {
					out = append(out, body)
				}

				next++
			case '+':
				out = append(out, body)
			}

			i++
			if noEOL {
				i++
			}
		}
	}

	return strings.Join(append(out, src[next:]...), "")
}

func parseRange(header string, oldStart, oldCount, newStart, newCount *int) bool {
	var oldPart, newPart string
	if _, err := fmt.Sscanf(header, "@@ %s %s @@", &oldPart, &newPart); err != nil {
		return false
	}

	return side(strings.TrimPrefix(oldPart, "-"), oldStart, oldCount) && side(strings.TrimPrefix(newPart, "+"), newStart, newCount)
}

func side(part string, start, count *int) bool {
	first, rest, hasCount := strings.Cut(part, ",")

	var err error
	if *start, err = strconv.Atoi(first); err != nil {
		return false
	}

	*count = 1
	if hasCount {
		*count, err = strconv.Atoi(rest)
	}

	return err == nil
}

func TestUnifiedMarksTrailingWhitespaceOnlyInColor(t *testing.T) {
	colors := terminal.Colors()
	colored := Unified("f.php", "foo  \n", "foo\n", colors)

	if !strings.Contains(colored, colors.WhitespaceMark+"  "+colors.Reset) {
		t.Fatalf("trailing whitespace not marked:\n%q", colored)
	}

	if plain := Unified("f.php", "foo  \n", "foo\n", terminal.Palette{}); stripEscapes(colored) != plain {
		t.Fatalf("color changed the patch bytes:\n%q\n%q", stripEscapes(colored), plain)
	}
}

func stripEscapes(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] == '\033' {
			for i < len(s) && s[i] != 'm' {
				i++
			}

			continue
		}

		b.WriteByte(s[i])
	}

	return b.String()
}
