package diff

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestApplyInRangeCoveringTheWholeInputReproducesTheOutput(t *testing.T) {
	check := func(t *testing.T, old, updated string) {
		t.Helper()

		got := ApplyInRange([]byte(old), []byte(updated), 0, len(old), alwaysValid)
		if string(got) != updated {
			t.Fatalf("ApplyInRange(%q) over the whole input = %q, want %q", old, got, updated)
		}
	}

	cases := []struct {
		name    string
		old     string
		updated string
	}{
		{"unchanged", "a\nb\n", "a\nb\n"},
		{"line changed in the middle", "a\nb\nc\n", "a\nB\nc\n"},
		{"two separate changes", "a\nb\nc\nd\n", "A\nb\nC\nd\n"},
		{"line inserted", "a\nc\n", "a\nb\nc\n"},
		{"line deleted", "a\nb\nc\n", "a\nc\n"},
		{"last line changed without trailing newline", "a\nb", "a\nB"},
		{"last line deleted without trailing newline", "a\nb\nc", "a\nb"},
		{"line appended without trailing newline", "a\nb", "a\nb\nc"},
		{"trailing newline added", "a\nb", "a\nb\n"},
		{"trailing newline removed", "a\nb\n", "a\nb"},
		{"whole document rewritten", "one\ntwo\n", "uno\ndos\ntres\n"},
		{"empty input", "", "a\nb\n"},
		{"emptied output", "a\nb\n", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check(t, tc.old, tc.updated)
		})
	}

	t.Run("random documents", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(3, 4))

		for range 3000 {
			check(t, randDocument(rng), randDocument(rng))
		}
	})
}

// The trailing newline is random because it decides which end-of-document
// branch a change takes.
func randDocument(rng *rand.Rand) string {
	doc := strings.Join(randLines(rng), "\n")

	if rng.IntN(2) == 0 {
		doc += "\n"
	}

	return doc
}

func TestApplyInRangeLeavesChangesOutsideTheRangeAlone(t *testing.T) {
	const (
		old     = "a\nb\nc\nd\n"
		updated = "A\nb\nC\nd\n"
	)

	cases := []struct {
		name  string
		start int
		end   int
		want  string
	}{
		{"first line", 0, 2, "A\nb\nc\nd\n"},
		{"part of the first line", 1, 2, "A\nb\nc\nd\n"},
		{"second line, which did not change", 2, 4, old},
		{"third line", 4, 6, "a\nb\nC\nd\n"},
		{"the two middle lines", 2, 6, "a\nb\nC\nd\n"},
		{"the whole input", 0, len(old), updated},
		{"an empty range", 3, 3, old},
		{"past the last change", 6, len(old), old},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyInRange([]byte(old), []byte(updated), tc.start, tc.end, alwaysValid)
			if string(got) != tc.want {
				t.Fatalf("ApplyInRange(%d, %d) = %q, want %q", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

func TestApplyInRangeIgnoresTheNewlineBorrowedByAFinalChange(t *testing.T) {
	old, updated := []byte("a\nb"), []byte("a\nB")

	if got := ApplyInRange(old, updated, 0, 2, alwaysValid); string(got) != "a\nb" {
		t.Fatalf("range over the first line = %q, want the input unchanged", got)
	}

	if got := ApplyInRange(old, updated, 2, 3, alwaysValid); string(got) != "a\nB" {
		t.Fatalf("range over the last line = %q, want %q", got, updated)
	}
}

// alwaysValid turns widening off, so a test sees the selection alone.
func alwaysValid([]byte) bool { return true }

func TestApplyInRangeWidensUntilTheResultIsValid(t *testing.T) {
	old := []byte("$a = array(\n    1,\n    2,\n);\n")
	updated := []byte("$a = [\n    1,\n    2,\n];\n")

	balanced := func(b []byte) bool {
		return bytes.Count(b, []byte("[")) == bytes.Count(b, []byte("]")) &&
			bytes.Count(b, []byte("(")) == bytes.Count(b, []byte(")"))
	}

	// The range covers only `$a = array(`, whose change opens a bracket the
	// closing line has to finish.
	const firstLine = 11

	got := ApplyInRange(old, updated, 0, firstLine, balanced)
	if !bytes.Equal(got, updated) {
		t.Fatalf("ApplyInRange = %q, want the whole change applied (%q)", got, updated)
	}

	unwidened := ApplyInRange(old, updated, 0, firstLine, alwaysValid)
	if want := "$a = [\n    1,\n    2,\n);\n"; string(unwidened) != want {
		t.Fatalf("without widening = %q, want the unbalanced splice %q", unwidened, want)
	}
}

func TestFirstChangedLineFindsTheLineOfTheFirstDifference(t *testing.T) {
	cases := []struct {
		name    string
		old     string
		updated string
		want    int
	}{
		{"later line", "a\nb\nc\n", "a\nb\nC\n", 3},
		{"trailing newline added", "a\nb", "a\nb\n", 2},
		{"text added after the final newline", "a\nb\n", "a\nb\n\nc\n", 2},
		{"empty input", "", "a\n", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FirstChangedLine([]byte(tc.old), []byte(tc.updated)); got != tc.want {
				t.Fatalf("FirstChangedLine(%q, %q) = %d, want %d", tc.old, tc.updated, got, tc.want)
			}
		})
	}
}
