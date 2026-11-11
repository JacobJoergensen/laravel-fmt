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

func TestParseTypePrefixEndsAtASpaceWherePHPStanOrPsalmDoes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"int [] list", "int"},
		{"array<int []> list", "array<int []>"},
		{"non-empty-array {0: ?string}", "non-empty-array"},
		{"Closure (int): void The closure", "Closure"},
		{"array<Closure (int): void> x", "array<Closure (int): void>"},
		{"callable(int):  void x", "callable(int)"},
		{"callable(int) :void x", "callable(int) :void"},
		{"array <int> The list", "array"},
		{"Foo ::BAR x", "Foo"},
		{"A |B x", "A"},
		{"A|  B x", "A"},
		{"A | B x", "A | B"},
		{"A| B x", "A| B"},
		{"? int $x", ""},
		{"T is int?string:bool", "T"},
		{"array<string, array\n{a: int}> x", "array<string, array\n{a: int}>"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			_, n := ParseTypePrefix([]byte(tc.in))
			if got := tc.in[:n]; got != tc.want {
				t.Fatalf("type = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseBoundPrefixEndsAtASpaceOnlyWherePHPStanDoes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Collection <int> The list", "Collection <int>"},
		{"int |string x", "int |string"},
		{"int [] list", "int []"},
		{"string [optional] The name", "string"},
		{"callable (string) Handler", "callable"},
		{"Closure (int): void The closure", "Closure (int): void"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			_, n := ParseBoundPrefix([]byte(tc.in))
			if got := tc.in[:n]; got != tc.want {
				t.Fatalf("type = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseBoundPrefixKeepsAFailureFromBeforeACallable(t *testing.T) {
	const in = "Foo::|callable (string) $x"

	if _, n := ParseBoundPrefix([]byte(in)); n != 0 {
		t.Fatalf("ParseBoundPrefix(%q) read %q, want no type", in, in[:n])
	}
}

func TestParseShapeAcceptsACommaAfterTheRestMarker(t *testing.T) {
	shape, ok := ParseType([]byte("array{\na: int,\n...,\n}")).(*ShapeType)
	if !ok || !shape.HasRest {
		t.Fatalf("ParseType = %#v, want an unsealed *ShapeType", shape)
	}
}
