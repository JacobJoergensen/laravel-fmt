package document

import (
	"strings"
	"testing"
)

func TestReindentHeredoc(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		targetWS string
		want     string
		bail     bool
	}{
		{"column-zero body indents to target", "<<<SQL\nselect 1\nSQL", "    ", "<<<SQL\n    select 1\n    SQL", false},
		{"deeper line keeps its relative depth", "<<<SQL\n    a\n        b\n    SQL", "        ", "<<<SQL\n        a\n            b\n        SQL", false},
		{"dedent to a shallower target", "<<<SQL\n        a\n        SQL", "    ", "<<<SQL\n    a\n    SQL", false},
		{"blank line stays blank with no trailing space", "<<<SQL\n    a\n\n    b\n    SQL", "  ", "<<<SQL\n  a\n\n  b\n  SQL", false},
		{"nowdoc reindents like a heredoc", "<<<'TXT'\nraw $value\nTXT", "    ", "<<<'TXT'\n    raw $value\n    TXT", false},
		{"empty body only moves the label", "<<<SQL\nSQL", "    ", "<<<SQL\n    SQL", false},
		{"line shallower than the label bails", "<<<SQL\n  a\n    SQL", "        ", "", true},
		{"tab body against a space label bails", "<<<SQL\n\ta\n    SQL", "        ", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := reindentHeredoc(tc.input, tc.targetWS)

			if tc.bail {
				if ok {
					t.Fatalf("reindentHeredoc returned ok=true, want bail; got %q", got)
				}

				return
			}

			if !ok {
				t.Fatalf("reindentHeredoc bailed, want %q", tc.want)
			}

			if got != tc.want {
				t.Errorf("reindentHeredoc = %q, want %q", got, tc.want)
			}

			if before, after := heredocValue(tc.input), heredocValue(got); before != after {
				t.Errorf("decoded value changed: before %q, after %q", before, after)
			}
		})
	}
}

// heredocValue is an independent decode oracle: PHP 7.3+ strips the closing
// label's whitespace from each body line.
func heredocValue(text string) string {
	lines := strings.Split(text, "\n")
	closeLine := strings.TrimSuffix(lines[len(lines)-1], "\r")
	closeWS := closeLine[:len(closeLine)-len(strings.TrimLeft(closeLine, " \t"))]

	var values []string

	for _, line := range lines[1 : len(lines)-1] {
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			values = append(values, "")

			continue
		}

		values = append(values, strings.TrimPrefix(line, closeWS))
	}

	return strings.Join(values, "\n")
}
