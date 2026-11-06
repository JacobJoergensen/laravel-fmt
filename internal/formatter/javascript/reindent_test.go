package javascript

import (
	"slices"
	"testing"
)

func TestLineDepthsFollowBracketNesting(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  []int
	}{
		{
			"object with method",
			"{\n count: 0,\n        inc() {\n this.count++\n },\n}",
			[]int{0, 1, 1, 2, 1, 0},
		},
		{
			"brackets opened on one line are one level",
			"init() {\nfetch('/a').then((r) => {\nthis.r = r\n})\n}",
			[]int{0, 1, 2, 1, 0},
		},
		{
			"line starting with a chain continues the line above",
			"items\n.filter(i => i.on)\n.map(i => i.id)",
			[]int{0, 1, 1},
		},
		{
			"trailing operator continues once, not stacked",
			"ok = a &&\nb &&\nc",
			[]int{0, 1, 1},
		},
		{
			"value after its key continues once, ternary included",
			"{\nlabel:\nx ?\n'on' :\n'off'\n}",
			[]int{0, 1, 2, 2, 2, 0},
		},
		{
			"case label colon does not continue",
			"switch (a) {\ncase 1:\nb()\n}",
			[]int{0, 1, 1, 0},
		},
		{
			"condition spanning lines keeps one level and its block hangs from the if",
			"if (a &&\nb &&\nc) {\nd()\n}",
			[]int{0, 1, 1, 1, 0},
		},
		{
			"condition on its own lines does not continue",
			"if (\na\n&& b\n) {\nc()\n}",
			[]int{0, 1, 1, 0, 1, 0},
		},
		{
			"expression started after the opener line continues",
			"f(\na &&\nb,\n)",
			[]int{0, 1, 2, 0},
		},
		{
			"value opening with a line break holds its body one level in",
			"\nopen = false\n$dispatch('closed')\n",
			[]int{0, 1, 1, 1},
		},
		{
			"bracket opened on the operator's line indents once",
			"all = items.map((el) =>\nparseInt(el.id),\n)",
			[]int{0, 1, 0},
		},
		{
			"braceless control body indents once",
			"if (! a.closest('b'))\nopen();\nelse\nclose();\nnext()",
			[]int{0, 1, 0, 1, 0},
		},
		{
			"brace on the line after a condition does not indent",
			"if (a)\n{\nb()\n}",
			[]int{0, 0, 1, 0},
		},
		{
			"call parens are not a condition",
			"run(a)\nnext()",
			[]int{0, 0},
		},
		{
			"comment after a trailing operator still continues",
			"ok = a && // why\nb",
			[]int{0, 1},
		},
		{
			"blank line inside keeps the depth",
			"{\na: 1,\n\nb: 2\n}",
			[]int{0, 1, 1, 1, 0},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := LineDepths([]byte(c.value))
			if !ok {
				t.Fatalf("LineDepths(%q) refused, want %v", c.value, c.want)
			}

			if !slices.Equal(got, c.want) {
				t.Fatalf("LineDepths(%q) = %v, want %v", c.value, got, c.want)
			}
		})
	}
}

func TestLineDepthsRefusesValuesItCannotMeasure(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"template literal across lines", "{\nmsg: `a\n  b`\n}"},
		{"string continued across lines", "{\nmsg: 'a\\\n  b'\n}"},
		{"block comment across lines", "{\n/* a\n  b */\nc: 1\n}"},
		{"entity that decodes to a bracket", "{\na: 1 &#125;\n}"},
		{"Annex B line comment opener", "{\n--> a\n}"},
		{"unclosed bracket", "{\na: 1"},
		{"stray closer", "a\n}"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, ok := LineDepths([]byte(c.value)); ok {
				t.Fatalf("LineDepths(%q) = %v, want refusal", c.value, got)
			}
		})
	}
}
