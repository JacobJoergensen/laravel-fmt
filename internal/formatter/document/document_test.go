package document

import (
	"strings"
	"testing"
)

func print80(d *Doc) string {
	return string(Print(d, Options{PrintWidth: 80, TabWidth: 4, EOL: "\n"}))
}

func printW(d *Doc, w int) string {
	return string(Print(d, Options{PrintWidth: w, TabWidth: 4, EOL: "\n"}))
}

func call(name string, args ...*Doc) *Doc {
	body := Concat(SoftLine(), Join(Concat(Text(","), Line()), args))

	return Group(Concat(
		Text(name+"("),
		Indent(body),
		IfBreak(Text(","), Text("")),
		SoftLine(),
		Text(")"),
	))
}

func TestGroupStaysFlatWhenItFits(t *testing.T) {
	d := call("foo", Text("a"), Text("b"), Text("c"))

	if got, want := print80(d), "foo(a, b, c)"; got != want {
		t.Fatalf("Print = %q, want %q", got, want)
	}
}

func TestGroupBreaksWhenTooWide(t *testing.T) {
	d := call("foo", Text("argumentOne"), Text("argumentTwo"), Text("argumentThree"))

	got := printW(d, 20)
	want := "foo(\n    argumentOne,\n    argumentTwo,\n    argumentThree,\n)"

	if got != want {
		t.Fatalf("Print =\n%q\nwant\n%q", got, want)
	}
}

func TestTrailingContentCountedAgainstWidth(t *testing.T) {
	expr := call("foo", Text("a"), Text("b"))

	if got := printW(expr, 9); got != "foo(a, b)" {
		t.Fatalf("at width 9 without trailing: got %q, want flat", got)
	}

	stmt := Concat(expr, Text(";"))

	if got := printW(stmt, 9); got == "foo(a, b);" {
		t.Fatalf("trailing `;` was not counted: stayed flat at %q over width 9", got)
	}
}

func TestHardLineForcesEnclosingGroupToBreak(t *testing.T) {
	d := Group(Concat(Text("a"), HardLine(), Text("b")))

	if got, want := printW(d, 80), "a\nb"; got != want {
		t.Fatalf("Print = %q, want %q (hardline must force break)", got, want)
	}
}

func TestIndentAppliesOnBreakOnly(t *testing.T) {
	flat := Group(Concat(Text("("), Indent(Concat(SoftLine(), Text("x"))), SoftLine(), Text(")")))
	if got := print80(flat); got != "(x)" {
		t.Fatalf("flat indent: got %q, want %q", got, "(x)")
	}

	broken := GroupBreak(Concat(Text("("), Indent(Concat(SoftLine(), Text("x"))), SoftLine(), Text(")")))
	if got, want := print80(broken), "(\n    x\n)"; got != want {
		t.Fatalf("broken indent: got %q, want %q", got, want)
	}
}

func TestIfBreakSelectsByGroupMode(t *testing.T) {
	build := func() *Doc {
		return Concat(Text("["), Join(Concat(Text(","), Line()), []*Doc{Text("a"), Text("b")}), IfBreak(Text(","), Text("")), Text("]"))
	}

	if got := printW(Group(build()), 80); got != "[a, b]" {
		t.Fatalf("flat ifBreak: got %q, want %q", got, "[a, b]")
	}

	if got, want := printW(GroupBreak(build()), 80), "[a,\nb,]"; got != want {
		t.Fatalf("broken ifBreak: got %q, want %q", got, want)
	}
}

func TestLineSuffixTrailsBeforeNextBreak(t *testing.T) {
	d := Concat(Text("a"), LineSuffix(Text(" // note")), HardLine(), Text("b"))

	if got, want := print80(d), "a // note\nb"; got != want {
		t.Fatalf("Print = %q, want %q", got, want)
	}
}

func TestLineSuffixFlushedAtEnd(t *testing.T) {
	tests := []struct {
		name string
		doc  *Doc
		want string
	}{
		{"text", Concat(Text("a"), LineSuffix(Text(" // end"))), "a // end"},
		{"concat", Concat(Text("a"), LineSuffix(Concat(Text(" "), Text("// end")))), "a // end"},
		{"two in order", Concat(Text("a"), LineSuffix(Concat(Text(" "), Text("/* 1 */"))), LineSuffix(Concat(Text(" "), Text("/* 2 */")))), "a /* 1 */ /* 2 */"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := print80(tc.doc); got != tc.want {
				t.Fatalf("Print = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrailingWhitespaceTrimmedAtBreak(t *testing.T) {
	d := GroupBreak(Concat(Text("a"), Line(), Text("b")))

	if got, want := print80(d), "a\nb"; got != want {
		t.Fatalf("Print = %q, want %q (trailing space before break must be trimmed)", got, want)
	}
}

func TestUseTabsIndents(t *testing.T) {
	d := GroupBreak(Concat(Text("("), Indent(Concat(SoftLine(), Text("x"))), SoftLine(), Text(")")))

	got := string(Print(d, Options{PrintWidth: 80, TabWidth: 4, UseTabs: true, EOL: "\n"}))
	if want := "(\n\tx\n)"; got != want {
		t.Fatalf("Print = %q, want %q", got, want)
	}
}

func TestNestedGroupsBreakOutermostFirst(t *testing.T) {
	inner := call("inner", Text("x"), Text("y"))
	outer := call("outer", inner, Text("z"))

	got := printW(outer, 18)
	want := "outer(\n    inner(x, y),\n    z,\n)"

	if got != want {
		t.Fatalf("Print =\n%q\nwant\n%q", got, want)
	}
}

func TestLinePrefixEmittedAfterIndentOnBreak(t *testing.T) {
	body := GroupBreak(Concat(
		Text("array{"),
		HardLine(), Text("     a: int,"),
		HardLine(), Text("     b: int,"),
		HardLine(), Text(" }"),
	))

	d := Indent(Concat(
		HardLine(),
		Text("/**"),
		LinePrefix(" *", Concat(
			HardLine(), Text(" @return "), body,
			HardLine(), Text("/"),
		)),
	))

	got := printW(d, 40)
	want := "\n    /**\n     * @return array{\n     *     a: int,\n     *     b: int,\n     * }\n     */"

	if got != want {
		t.Fatalf("Print =\n%q\nwant\n%q", got, want)
	}
}

func TestLinePrefixGroupStaysFlatNoPrefix(t *testing.T) {
	shape := Group(Concat(
		Text("array{"),
		Indent(Concat(SoftLine(), Join(Concat(Text(","), Line()), []*Doc{Text("a: int"), Text("b: int")}))),
		SoftLine(),
		Text("}"),
	))

	d := Concat(Text("/**"), LinePrefix(" *", Concat(HardLine(), Text(" @return "), shape, HardLine(), Text("/"))))

	got := print80(d)
	want := "/**\n * @return array{a: int, b: int}\n */"

	if got != want {
		t.Fatalf("Print =\n%q\nwant\n%q", got, want)
	}
}

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

func TestIfBreakOfFollowsNamedGroup(t *testing.T) {
	const headID = 1

	build := func() *Doc {
		head := GroupID(headID, Concat(
			Text("preg_match("),
			Indent(Concat(
				SoftLine(), Text("$pattern,"), Line(), Text("$cue"),
				IfBreak(Text(","), Text("")),
			)),
			SoftLine(),
			Text(")"),
		))

		sep := IfBreakOf(headID, Text(" "), Line())

		return Group(Concat(head, Indent(Concat(sep, Text("=== 1")))))
	}

	t.Run("head flat keeps the operator breakable", func(t *testing.T) {
		got := string(Print(build(), Options{PrintWidth: 80, TabWidth: 4, EOL: "\n"}))
		want := "preg_match($pattern, $cue) === 1"

		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("head broken glues the operator to its closing paren", func(t *testing.T) {
		got := string(Print(build(), Options{PrintWidth: 20, TabWidth: 4, EOL: "\n"}))
		want := "preg_match(\n    $pattern,\n    $cue,\n) === 1"

		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	// A flat head ends in no closing paren, so nothing is stranded and PSR-12 puts
	// the operator at the start of the continuation line.
	t.Run("short head with a long operand still breaks at the operator", func(t *testing.T) {
		const shortID = 2

		head := GroupID(shortID, Concat(
			Text("foo("),
			Indent(Concat(SoftLine(), Text("$a"), IfBreak(Text(","), Text("")))),
			SoftLine(),
			Text(")"),
		))
		sep := IfBreakOf(shortID, Text(" "), Line())
		doc := Group(Concat(head, Indent(Concat(sep, Text("&& someVeryLongCondition($b)")))))

		got := string(Print(doc, Options{PrintWidth: 30, TabWidth: 4, EOL: "\n"}))
		want := "foo($a)\n    && someVeryLongCondition($b)"

		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("a plain IfBreak strands the operator instead", func(t *testing.T) {
		head := Concat(
			Text("preg_match("),
			Indent(Concat(
				SoftLine(), Text("$pattern,"), Line(), Text("$cue"),
				IfBreak(Text(","), Text("")),
			)),
			SoftLine(),
			Text(")"),
		)
		doc := Group(Concat(head, Indent(Concat(IfBreak(HardLine(), Line()), Text("=== 1")))))

		got := string(Print(doc, Options{PrintWidth: 20, TabWidth: 4, EOL: "\n"}))
		want := "preg_match(\n    $pattern,\n    $cue,\n)\n    === 1"

		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	})
}

func TestStartColumnMeasuresFirstLineFromColumn(t *testing.T) {
	doc := Group(Concat(
		Text("f("),
		Indent(Concat(SoftLine(), Text("aaaa,"), Line(), Text("bbbb"))),
		SoftLine(),
		Text(")"),
	))

	flat := string(Print(doc, Options{PrintWidth: 20, TabWidth: 4, EOL: "\n", StartColumn: 0}))
	if strings.Contains(flat, "\n") {
		t.Fatalf("fit at column 0 but broke:\n%s", flat)
	}

	broken := string(Print(doc, Options{PrintWidth: 20, TabWidth: 4, EOL: "\n", StartColumn: 12}))
	if !strings.Contains(broken, "\n") {
		t.Fatalf("start column 12 leaves 8 of 20 columns; should break:\n%s", broken)
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

func TestOpenerJudgesItsVariantByTheMeasuredWidth(t *testing.T) {
	glued := Concat(Text("call(["), Indent(Concat(SoftLine(), Text("'a' => 1"))), SoftLine(), Text("])"))
	broken := Concat(Text("call("), Indent(Concat(HardLine(), Text("['a' => 1]"))), HardLine(), Text(")"))

	for _, tc := range []struct {
		name   string
		opener int
		want   string
	}{
		{"opening line fits", 6, "call([\n    'a' => 1\n])"},
		{"opening line overflows", 30, "call(\n    ['a' => 1]\n)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := ConditionalGroup(Opener(tc.opener, glued), broken)
			if got := string(Print(doc, Options{PrintWidth: 10, TabWidth: 4, EOL: "\n"})); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
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
