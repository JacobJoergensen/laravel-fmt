package phpdoc

import "testing"

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
