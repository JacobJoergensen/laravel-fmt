package diff

import (
	"strings"
	"testing"
)

func TestSnippetPlacesCaretUnderColumn(t *testing.T) {
	src := []byte("line one\n$x = [1, 2, 3\n")

	got := Snippet(src, 2, 14)

	want := "2 │ $x = [1, 2, 3\n" +
		"  │ " + strings.Repeat(" ", 13) + "^\n"

	if got != want {
		t.Fatalf("snippet differs.\n=== got ===\n%q\n=== want ===\n%q", got, want)
	}
}

func TestSnippetPreservesTabsInCaretIndent(t *testing.T) {
	src := []byte("\t\treturn $x\n")

	got := Snippet(src, 1, 10)

	want := "1 │ \t\treturn $x\n" +
		"  │ \t\t" + strings.Repeat(" ", 7) + "^\n"

	if got != want {
		t.Fatalf("snippet differs.\n=== got ===\n%q\n=== want ===\n%q", got, want)
	}
}

func TestSnippetClampsColumnPastEnd(t *testing.T) {
	got := Snippet([]byte("abc\n"), 1, 99)

	want := "1 │ abc\n" +
		"  │ " + strings.Repeat(" ", 3) + "^\n"

	if got != want {
		t.Fatalf("snippet differs.\n=== got ===\n%q\n=== want ===\n%q", got, want)
	}
}

func TestSnippetReturnsEmptyForUnavailableLine(t *testing.T) {
	if got := Snippet([]byte("a\nb\n"), 9, 1); got != "" {
		t.Fatalf("out-of-range line: want empty, got %q", got)
	}

	if got := Snippet(nil, 1, 1); got != "" {
		t.Fatalf("nil src: want empty, got %q", got)
	}
}
