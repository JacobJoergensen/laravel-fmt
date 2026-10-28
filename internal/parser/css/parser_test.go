package css

import (
	"strings"
	"testing"
	"time"
)

func TestParseRecognizesEachBlockForm(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want any
	}{
		{"rule", ".foo { color: red; }", (*Rule)(nil)},
		{"at-rule with block", "@media (min-width: 640px) { .foo { color: red; } }", (*AtRule)(nil)},
		{"at-rule statement", `@import "a.css";`, (*AtRule)(nil)},
		{"comment", "/* hi */", (*Comment)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			if len(file.Nodes) != 1 {
				t.Fatalf("got %d top-level nodes, want 1", len(file.Nodes))
			}

			switch tc.want.(type) {
			case *Rule:
				if _, ok := file.Nodes[0].(*Rule); !ok {
					t.Fatalf("node is %T, want *Rule", file.Nodes[0])
				}
			case *AtRule:
				if _, ok := file.Nodes[0].(*AtRule); !ok {
					t.Fatalf("node is %T, want *AtRule", file.Nodes[0])
				}
			case *Comment:
				if _, ok := file.Nodes[0].(*Comment); !ok {
					t.Fatalf("node is %T, want *Comment", file.Nodes[0])
				}
			}
		})
	}
}

func TestNestedRuleAppearsInsideParentBlock(t *testing.T) {
	src := `.outer { color: red; .inner { color: blue; } }`

	file, errs := Parse([]byte(src))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	outer, ok := file.Nodes[0].(*Rule)

	if !ok {
		t.Fatalf("top-level node is %T, want *Rule", file.Nodes[0])
	}

	var sawNestedRule bool

	for _, child := range outer.Children {
		if _, isRule := child.(*Rule); isRule {
			sawNestedRule = true

			break
		}
	}

	if !sawNestedRule {
		t.Fatalf("nested Rule not found in outer.Children (%d children)", len(outer.Children))
	}
}

func TestSelectorDeclarationDisambiguation(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want any
	}{
		{"pseudo-class selector is a rule", "a:hover { color: red; }", (*Rule)(nil)},
		{"property is a declaration", ".x { color: red; }", (*Declaration)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			switch tc.want.(type) {
			case *Rule:
				if _, ok := file.Nodes[0].(*Rule); !ok {
					t.Fatalf("node is %T, want *Rule", file.Nodes[0])
				}
			case *Declaration:
				rule, ok := file.Nodes[0].(*Rule)

				if !ok {
					t.Fatalf("top node is %T, want *Rule", file.Nodes[0])
				}

				if len(rule.Children) == 0 {
					t.Fatalf("rule has no children")
				}

				if _, ok := rule.Children[0].(*Declaration); !ok {
					t.Fatalf("child is %T, want *Declaration", rule.Children[0])
				}
			}
		})
	}
}

func TestImportantFlagRecognizedOnTrailingKeyword(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		important bool
		wantValue string
	}{
		{"plain flag", "red !important", true, "red"},
		{"uppercase keyword", "red !IMPORTANT", true, "red"},
		{"no space between bang and keyword", "red!important", true, "red"},
		{"comment between value and flag", "red /* c */ !important", true, "red /* c */"},
		{"no flag", "red", false, "red"},
		{"keyword inside string is not a flag", `"!important"`, false, `"!important"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(".x { color: " + tc.value + "; }"))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			rule, ok := file.Nodes[0].(*Rule)

			if !ok {
				t.Fatalf("node is %T, want *Rule", file.Nodes[0])
			}

			decl, ok := rule.Children[0].(*Declaration)

			if !ok {
				t.Fatalf("child is %T, want *Declaration", rule.Children[0])
			}

			if decl.Important != tc.important {
				t.Fatalf("Important=%v, want %v", decl.Important, tc.important)
			}

			if string(decl.Value) != tc.wantValue {
				t.Fatalf("Value=%q, want %q", decl.Value, tc.wantValue)
			}
		})
	}
}

func TestBalancedScanningIgnoresStructureInsideStringsAndBrackets(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"parens in value", ".x { width: calc(100% - 20px); }"},
		{"semicolon inside string value", `.x { content: ";"; }`},
		{"brace inside string value", `.x { content: "}"; }`},
		{"brackets in selector with quoted terminator", `.x[data-v=";"] { color: red; }`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			if len(file.Nodes) != 1 {
				t.Fatalf("got %d nodes, want 1, early terminator leaked", len(file.Nodes))
			}

			if _, ok := file.Nodes[0].(*Rule); !ok {
				t.Fatalf("node is %T, want *Rule", file.Nodes[0])
			}
		})
	}
}

func TestStrayTerminatorsDoNotDeadlockWalker(t *testing.T) {
	cases := []string{
		"}",
		";",
		"}}};",
		".foo { color: red",
		`/* unterminated`,
		`.x { color: "unterminated`,
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			done := make(chan struct{})

			go func() {
				_, _ = Parse([]byte(src))
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("Parse did not return on input %q", src)
			}
		})
	}
}

func TestUnbalancedBracketReportsErrorAndRecovers(t *testing.T) {
	src := ".a { width: calc(1 }\n.b { color: red; }\n"

	file, errs := Parse([]byte(src))

	if len(errs) == 0 {
		t.Fatalf("expected a diagnostic for the unbalanced (")
	}

	if findRule(file.Nodes, ".b") == nil {
		t.Fatalf(".b was not parsed; the unbalanced ( swallowed the rest of the file")
	}
}

func findRule(nodes []Node, selector string) *Rule {
	for _, n := range nodes {
		if r, ok := n.(*Rule); ok && string(r.Selector) == selector {
			return r
		}
	}

	return nil
}

func TestAtRuleBlockAndStatementForms(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		block bool
	}{
		{"block form sets LBrace/RBrace", "@media screen { }", true},
		{"statement form sets Semicolon", `@import "a.css";`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))
			at, ok := file.Nodes[0].(*AtRule)

			if !ok {
				t.Fatalf("node is %T, want *AtRule", file.Nodes[0])
			}

			if at.Block != tc.block {
				t.Fatalf("Block=%v, want %v", at.Block, tc.block)
			}

			if tc.block {
				if at.LBrace.End.Offset == 0 || at.RBrace.End.Offset == 0 {
					t.Fatalf("block form missing brace spans")
				}
			} else {
				if at.Semicolon.End.Offset == 0 {
					t.Fatalf("statement form missing semicolon span")
				}
			}
		})
	}
}

func TestParseCapsNestingDepth(t *testing.T) {
	cases := []struct {
		name       string
		depth      int
		wantErr    bool
		wantColumn int
	}{
		{"at the cap", maxNestingDepth, false, 0},
		{"past the cap", maxNestingDepth + 1, true, len("a{")*(maxNestingDepth+1) + 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Repeat("a{", tc.depth) + strings.Repeat("}", tc.depth)
			_, errs := Parse([]byte(src))

			if !tc.wantErr {
				if len(errs) != 0 {
					t.Fatalf("errors = %v, want none", errs)
				}

				return
			}

			if len(errs) != 1 || errs[0].Msg != "maximum nesting depth exceeded" || errs[0].Pos.Column != tc.wantColumn {
				t.Fatalf("errors = %v, want one depth error at column %d", errs, tc.wantColumn)
			}
		})
	}
}

func TestUnterminatedConstructIsReportedAtItsOpener(t *testing.T) {
	cases := []struct {
		name, src, wantMsg string
		wantLine, wantCol  int
	}{
		{"comment", "a {}\n/* c\n\n", "unterminated comment", 2, 1},
		{"string", "a { b: \"x;\n}\n", "unterminated string", 1, 8},
		{"rule block", "a {\n  b: c;\n\n", "unclosed rule block", 1, 3},
		{"at-rule block", "@media x {\n  a {}\n\n", "unclosed at-rule block", 1, 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src))

			for _, err := range errs {
				if err.Msg == tc.wantMsg {
					if err.Pos.Line != tc.wantLine || err.Pos.Column != tc.wantCol {
						t.Fatalf("%q at %d:%d, want %d:%d", err.Msg, err.Pos.Line, err.Pos.Column, tc.wantLine, tc.wantCol)
					}

					return
				}
			}

			t.Fatalf("errors = %v, want %q", errs, tc.wantMsg)
		})
	}
}
