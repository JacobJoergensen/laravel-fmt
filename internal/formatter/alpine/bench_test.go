package alpine

import "testing"

func BenchmarkNormalize(b *testing.B) {
	value := []byte("{ open: false, query: '', items: [], toggle() { this.open = ! this.open }, async load() { this.items = await (await fetch('/items')).json() } }")

	for b.Loop() {
		if _, ok := Normalize(value, true); !ok {
			b.Fatal("Normalize refused benchmark value")
		}
	}
}
