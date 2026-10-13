package html

import (
	"maps"
	"slices"
	"testing"
)

func TestStylePreservesWhitespaceReadsDeclarations(t *testing.T) {
	cases := []struct {
		style string
		want  bool
	}{
		{"white-space: pre-line !important", true},
		{"white-space : pre-line ! important", true},
		{"white-space:pre-line!important", true},
		{"color: red; white-space: preserve nowrap", true},
		{"white-space: pre-line /* unclosed", true},
		{"white-space: normal; white-space: pre-line", true},
		{"white-space: nowrap", false},
		{"white-space: collapse nowrap", false},
		{"white-space: inherit", false},
		{"white-space-collapse: collapse", false},
		{"--white-space: pre-line", false},
		{"text-white-space: pre", false},
		{"--mode: pre-line", false},
		{"/* white-space: pre */ color: red", false},
	}

	for _, tc := range cases {
		if got := stylePreservesWhitespace([]byte(tc.style)); got != tc.want {
			t.Errorf("stylePreservesWhitespace(%q) = %v, want %v", tc.style, got, tc.want)
		}
	}
}

func TestClassListPreservesWhitespaceStripsModifiers(t *testing.T) {
	cases := []struct {
		class string
		want  bool
	}{
		{"[&:has(p)]:whitespace-pre", true},
		{"print:!whitespace-break-spaces", true},
		{"[white-space:pre-line]!", true},
		{"whitespace-normal md:whitespace-nowrap", false},
		{"[white-space:nowrap]", false},
		{"[&:white-space]:p-4", false},
	}

	for _, tc := range cases {
		if got := classListPreservesWhitespace([]byte(tc.class)); got != tc.want {
			t.Errorf("classListPreservesWhitespace(%q) = %v, want %v", tc.class, got, tc.want)
		}
	}
}

func TestMentionsPreservingWhitespaceIgnoresCollapsingUtilities(t *testing.T) {
	cases := []struct {
		expr string
		want bool
	}{
		{"{ 'whitespace-break-spaces': wrap }", true},
		{"el.style.whiteSpace = mode", true},
		{"{ 'whitespace-normal': a, 'whitespace-nowrap': b }", false},
		{"['p-4' => $padded]", false},
		{"white-space", true},
		{"white - space", true},
		{"whitespace-normal", false},
		{"whitespace - normal", false},
	}

	for _, tc := range cases {
		if got := MentionsPreservingWhitespace([]byte(tc.expr)); got != tc.want {
			t.Errorf("MentionsPreservingWhitespace(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestScanPageStyleCreditsTheInnermostRule(t *testing.T) {
	src := []byte("<style>.outer { color: red; .inner { white-space: pre } }" +
		" .after { color: blue } @media print { .p1, .p2 { white-space: pre-wrap } }</style>")

	classes := ScanPageStyleWhitespaceClasses(src)
	names := slices.Sorted(maps.Keys(classes))

	want := []string{"inner", "p1", "p2"}
	if !slices.Equal(names, want) {
		t.Fatalf("classes = %v, want %v", names, want)
	}
}
