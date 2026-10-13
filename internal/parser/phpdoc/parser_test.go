package phpdoc

import (
	"bytes"
	"testing"
)

func TestParseTypeBailsWithTheWholeSource(t *testing.T) {
	cases := []string{
		"[1, 2]",
		"int[",
		"array{foo:",
		"<int>",
		"array<int,",
		"callable(int",
		"Foo::",
	}

	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			invalid, ok := ParseType([]byte(in)).(*Invalid)
			if !ok {
				t.Fatalf("ParseType(%q) is not *Invalid", in)
			}

			if !bytes.Equal(invalid.Raw, []byte(in)) {
				t.Fatalf("Raw = %q, want %q", invalid.Raw, in)
			}
		})
	}
}

func BenchmarkParseType(b *testing.B) {
	inputs := [][]byte{
		[]byte("array{foo: int, bar: string, baz: array<int, Foo>}"),
		[]byte("callable(int, string, Foo &$x): (int|null)"),
		[]byte("non-empty-list<array{id: int, name: non-empty-string}>"),
		[]byte("Collection<covariant User>|null"),
		[]byte("$this is int ? string : bool"),
	}

	b.ReportAllocs()

	for b.Loop() {
		for _, in := range inputs {
			_ = ParseType(in)
		}
	}
}

func TestParseShapeRecordsALineBreakAfterTheBrace(t *testing.T) {
	cases := []struct {
		in     string
		broken bool
	}{
		{"array{a: int}", false},
		{"array{\na: int,\n}", true},
		{"array{\r\na: int}", true},
		{"array{a: int,\nb: int}", false},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			shape, ok := ParseType([]byte(tc.in)).(*ShapeType)
			if !ok {
				t.Fatalf("ParseType(%q) is not a *ShapeType", tc.in)
			}

			if shape.Broken != tc.broken {
				t.Fatalf("Broken = %v, want %v", shape.Broken, tc.broken)
			}
		})
	}
}
