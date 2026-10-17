package document

import "testing"

func TestFlatWidthRefusesMultiLineText(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"blank line carrying a tab", "0\n\t \n0"},
		{"plain two-liner", "ab\ncd"},
		{"trailing newline", "ab\n"},
		{"leading newline", "\nab"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FlatWidth(Text(tc.text), 4); got != ForcedBreakWidth {
				t.Fatalf("FlatWidth(%q) = %d, want ForcedBreakWidth (%d)", tc.text, got, ForcedBreakWidth)
			}
		})
	}

	if got := FlatWidth(Text("abcd"), 4); got != 4 {
		t.Fatalf("single-line text must keep its measured width: got %d, want 4", got)
	}

	kept := FlatWidth(Text("0\n\t \n0"), 4)
	stripped := FlatWidth(Text("0\n\n0"), 4)

	if kept != stripped {
		t.Fatalf("stripped whitespace changed the measurement: %d vs %d", kept, stripped)
	}
}

func TestClosingLineWidthMeasuresLastBrokenLine(t *testing.T) {
	array := func(group func(*Doc) *Doc) *Doc {
		return group(Concat(Text("["), Indent(Concat(SoftLine(), Text("1"))), SoftLine(), Text("]")))
	}

	authored := Group(Concat(Text("("), Indent(Concat(SoftLine(), Text("a"))), SoftLine(), Text(")"), BreakParent()))
	HasForcedBreak(authored)

	cases := []struct {
		name  string
		doc   *Doc
		width int
		broke bool
	}{
		{"must-break array then a call", Concat(Text("f("), array(GroupBreak), Text(")"), Text("->g()")), len("])->g()"), true},
		{"flat group measures whole", Concat(Text("f("), array(Group), Text(")")), len("f([1])"), false},
		{"propagated forced break", Concat(Text("f"), authored, Text("->g()")), len(")->g()"), true},
		{"multi-line text keeps its last line", Concat(Text("'a\nbc'"), Text(")")), len("bc')"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			width, broke := ClosingLineWidth(tc.doc, 4)
			if width != tc.width || broke != tc.broke {
				t.Fatalf("ClosingLineWidth = (%d, %v), want (%d, %v)", width, broke, tc.width, tc.broke)
			}
		})
	}
}

func TestBrokenPrefixWidthStopsAtTheFirstPossibleBreak(t *testing.T) {
	call := Group(Concat(Text("f("), Indent(Concat(SoftLine(), Text("a"))), SoftLine(), Text(")")))
	operand := Concat(Text("$a"), Indent(Concat(Line(), Text("?? $b"))))

	cases := []struct {
		name  string
		doc   *Doc
		width int
		broke bool
	}{
		{"a nested group breaks", Concat(Text("x => "), Group(Concat(Text("g"), call))), len("x => gf("), true},
		{"narrowest variant", ConditionalGroup(Text("glued ?? $b"), operand), len("$a"), true},
		{"line outside every group", Concat(Text("'a'."), Line(), Text("'b'")), len("'a'."), true},
		{"nothing to break", Text("$a->b"), len("$a->b"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			width, broke := BrokenPrefixWidth(tc.doc, 4)
			if width != tc.width || broke != tc.broke {
				t.Fatalf("BrokenPrefixWidth = (%d, %v), want (%d, %v)", width, broke, tc.width, tc.broke)
			}
		})
	}
}

func TestClosingLineWidthOfVerbatimIsItsLastLine(t *testing.T) {
	d := Concat(Verbatim("((0)#", []string{"%(0))"}), Text("()"))

	if w, broke := ClosingLineWidth(d, 4); w != 7 || !broke {
		t.Fatalf("ClosingLineWidth = %d, %v; want 7, true", w, broke)
	}
}
