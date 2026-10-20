package yaml

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestFrozenSpanKeepsAKeepChompedScalarsBlankLines(t *testing.T) {
	cases := []struct{ name, src string }{
		{"bailed document root", "|+\n  k\n\n\n"},
		{"ignored entry", "top: 1\n# laravel-fmt-ignore\na: |+\n  k\n\n\n"},
		{"ignored document", "# laravel-fmt-ignore\n---\na: |+\n  k\n\n\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mustFormat(t, []byte(tc.src), config.Default()); string(got) != tc.src {
				t.Fatalf("the frozen span changed.\n=== in ===\n%q\n=== out ===\n%q", tc.src, got)
			}
		})
	}
}
