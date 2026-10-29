package markdown

import (
	"strings"
	"testing"
)

func TestOnlyADocumentOpensWithFrontmatter(t *testing.T) {
	src := []byte("---\na\n---\n")

	document, _ := Parse(src)
	if _, ok := document.Blocks[0].(*Frontmatter); !ok {
		t.Fatalf("Parse: block 0 is %T, want *Frontmatter", document.Blocks[0])
	}

	body := parse(src, false)
	if _, ok := body.Blocks[0].(*ThematicBreak); !ok {
		t.Fatalf("body: block 0 is %T, want *ThematicBreak", body.Blocks[0])
	}
}

func TestParseRefusesContainersNestedPastTheCap(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int
	}{
		{"quotes at the cap", strings.Repeat(">", maxNestingDepth) + " 0\n", 0},
		{"quotes past the cap", "intro\n" + strings.Repeat(">", maxNestingDepth+1) + " 0\n", 2},
		{"list markers past the cap", strings.Repeat("- ", maxNestingDepth+1) + "x\n", 1},
		{"indentation past the cap", "- a\n" + strings.Repeat(" ", 2*maxNestingDepth) + "- x\n", 2},
		{"a long thematic break is not nesting", strings.Repeat("- ", 2*maxNestingDepth) + "-\n", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))
			if file == nil {
				t.Fatal("Parse returned a nil File")
			}

			if tc.line == 0 {
				if len(errs) > 0 {
					t.Fatalf("errors = %v, want none", errs)
				}

				return
			}

			if len(errs) != 1 || errs[0].Pos.Line != tc.line || errs[0].Pos.Column != 1 {
				t.Fatalf("errors = %v, want one at %d:1", errs, tc.line)
			}
		})
	}
}
