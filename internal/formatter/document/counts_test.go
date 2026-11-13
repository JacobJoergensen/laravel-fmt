package document

import (
	"reflect"
	"testing"
)

func TestPrintCountsOverThePrintersChoices(t *testing.T) {
	note := Text("// note")
	other := Text("// other")
	tags := map[*Doc][]Tag{
		note:  {{ID: 1, Count: Count{Min: 1, Max: 1}}},
		other: {{ID: 2, Count: Count{Min: 1, Max: 1}}},
	}

	cases := []struct {
		name string
		doc  *Doc
		want map[int]Count
	}{
		{"printed once", Concat(Text("a"), Indent(note)), map[int]Count{1: {1, 1}}},
		{"in every variant", ConditionalGroup(Concat(note, Text("a")), Group(note)), map[int]Count{1: {1, 1}}},
		{"in one variant only", ConditionalGroup(Concat(note, Text("a")), Text("b")), map[int]Count{1: {0, 1}}},
		{"in one branch only", IfBreak(Text("a"), Concat(Text("b"), other)), map[int]Count{2: {0, 1}}},
		{"printed twice", Concat(note, LineSuffix(note)), map[int]Count{1: {2, 2}}},
		{"a tagged leaf counts as its tags", Concat(note, other), map[int]Count{1: {1, 1}, 2: {1, 1}}},
		{"nothing tagged", Concat(Text("a"), Line()), nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrintCounts(tc.doc, tags); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
