package phpdoc

import (
	"bytes"
	"testing"
)

// A type ParseType cannot read must come back as *Invalid holding all of src,
// which the formatter writes back unchanged.
func FuzzParseType(f *testing.F) {
	for _, seed := range []string{
		"array{foo: int, bar?: string}",
		"int|string|null",
		"Collection<int, User>",
		"callable(int, string): bool",
		"array{",
		"<<<",
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		invalid, ok := ParseType(src).(*Invalid)

		if ok && !bytes.Equal(invalid.Raw, src) {
			t.Fatalf("Invalid.Raw = %q, want all of src %q", invalid.Raw, src)
		}
	})
}
