package document

import "testing"

func row(scope int32, key, value string) *Doc {
	return Concat(Text(key), AlignGap(scope, 0), Text("=> "), Text(value), Text(","), HardLine())
}

func TestAlignGapSharesAColumnAcrossConsecutiveLines(t *testing.T) {
	d := Concat(row(1, "'a'", "1"), row(1, "'bbbbbb'", "2"), row(1, "'cc'", "3"))

	got := print80(d)
	want := "'a'      => 1,\n'bbbbbb' => 2,\n'cc'     => 3,\n"

	if got != want {
		t.Fatalf("Print =\n%q\nwant\n%q", got, want)
	}
}

func TestAlignGapRunEndsAtALineWithoutOne(t *testing.T) {
	cases := []struct {
		name      string
		separator *Doc
	}{
		{"blank line", Concat(HardLine())},
		{"a line of its own", Concat(Text("...$rest,"), HardLine())},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Concat(row(1, "'a'", "1"), row(1, "'bbbbbb'", "2"), tc.separator, row(1, "'cc'", "3"))

			got := print80(d)
			if want := "'a'      => 1,\n'bbbbbb' => 2,\n"; got[:len(want)] != want {
				t.Fatalf("first run =\n%q\nwant it to start\n%q", got, want)
			}

			if want := "'cc' => 3,\n"; got[len(got)-len(want):] != want {
				t.Fatalf("second run =\n%q\nwant it to end\n%q", got, want)
			}
		})
	}
}

func TestAlignGapDoesNotJoinTwoScopes(t *testing.T) {
	d := Concat(row(1, "'a'", "1"), row(2, "'bbbbbb'", "2"), row(1, "'cc'", "3"))

	got := print80(d)
	want := "'a' => 1,\n'bbbbbb' => 2,\n'cc' => 3,\n"

	if got != want {
		t.Fatalf("Print =\n%q\nwant\n%q", got, want)
	}
}

func TestAlignGapCutsARunOnlyForPaddingFromBelow(t *testing.T) {
	call := func(arg string) *Doc {
		return Group(Concat(Text("f("), Indent(Concat(SoftLine(), Text(arg))), SoftLine(), Text(")")))
	}

	entry := func(key string, reserve int, value *Doc) *Doc {
		return Concat(Text(key), AlignGap(1, reserve), Text("=> "), value, Text(","), HardLine())
	}

	cases := []struct {
		name string
		doc  *Doc
		want string
	}{
		{
			"an entry that fits only unpadded stands alone",
			Concat(row(1, "'aVeryLongKeyIndeed'", "0"), entry("'k'", 17, call("arg"))),
			"'aVeryLongKeyIndeed' => 0,\n'k' => f(arg),\n",
		},
		{
			"padding only from a key below",
			Concat(entry("'k'", 17, call("arg")), row(1, "'aVeryLongKeyIndeed'", "0")),
			"'k' => f(arg),\n'aVeryLongKeyIndeed' => 0,\n",
		},
		{
			"a cut entry keeps the padding from above",
			Concat(entry("'a'", 2, Text("0")), entry("'kkk'", 13, call("ar")), row(1, "'aVeryLongKeyIndeed'", "0")),
			"'a'   => 0,\n'kkk' => f(ar),\n'aVeryLongKeyIndeed' => 0,\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := printW(tc.doc, 20); got != tc.want {
				t.Fatalf("Print =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func commentedRow(key, value string, reserve int, comment string) *Doc {
	return Concat(Text(key), AlignGap(1, reserve), Text("=> "), Text(value), Text(","),
		AlignGap(CommentScope(0), 0), Text(comment), HardLine())
}

func TestCommentGapAlignsAfterTheLinePadding(t *testing.T) {
	d := Concat(
		commentedRow("'a'", "'one'", 5, "// first"),
		commentedRow("'bbbbbb'", "'two'", 0, "// second"),
		commentedRow("'cc'", "'three'", 4, "// third"),
	)

	got := print80(d)
	want := "'a'      => 'one',   // first\n'bbbbbb' => 'two',   // second\n'cc'     => 'three', // third\n"

	if got != want {
		t.Fatalf("Print =\n%q\nwant\n%q", got, want)
	}
}

func TestSecondCommentGapOnALineAlignsAfterTheFirstIsPadded(t *testing.T) {
	line := func(code string) *Doc {
		return Concat(
			Text(code), AlignGap(CommentScope(1), 0), Text("/* x */"),
			AlignGap(CommentScope(2), 0), Text("/* y */"), HardLine(),
		)
	}

	got := print80(Concat(line("$a = 1;"), line("$bb = 2;")))
	want := "$a = 1;  /* x */ /* y */\n$bb = 2; /* x */ /* y */\n"

	if got != want {
		t.Fatalf("Print =\n%q\nwant\n%q", got, want)
	}
}
