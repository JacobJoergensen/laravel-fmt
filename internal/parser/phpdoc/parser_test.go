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

func TestParseTypePrefixEndsAtASpaceWherePHPStanDoes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"string [optional] The name", "string"},
		{"int [] list", "int []"},
		{"array{a: int} [0] First", "array{a: int}"},
		{"non-empty-array {0: ?string}", "non-empty-array"},
		{"callable (string) Handler", "callable"},
		{"Closure (int): void The closure", "Closure (int): void"},
		{"array <int> The list", "array <int>"},
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

func TestParseTypePrefixKeepsAFailureFromBeforeACallable(t *testing.T) {
	const in = "Foo::|callable (string) $x"

	if _, n := ParseTypePrefix([]byte(in)); n != 0 {
		t.Fatalf("ParseTypePrefix(%q) read %q, want no type", in, in[:n])
	}
}

func TestParseShapeAcceptsACommaAfterTheRestMarker(t *testing.T) {
	shape, ok := ParseType([]byte("array{\na: int,\n...,\n}")).(*ShapeType)
	if !ok || !shape.HasRest {
		t.Fatalf("ParseType = %#v, want an unsealed *ShapeType", shape)
	}
}
