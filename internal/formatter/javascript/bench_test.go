package javascript

import "testing"

func BenchmarkNormalize(b *testing.B) {
	value := []byte("{ open: false, query: '', items: [], toggle() { this.open = ! this.open }, async load() { this.items = await (await fetch('/items')).json() } }")

	for b.Loop() {
		if _, ok := Normalize(value, true); !ok {
			b.Fatal("Normalize refused benchmark value")
		}
	}
}

func BenchmarkLineDepths(b *testing.B) {
	value := []byte(
		"{\n    open: false,\n    items: [],\n    toggle() {\n        this.open = ! this.open\n    },\n    async load() {\n        this.items = await (await fetch('/items')).json()\n    },\n}",
	)

	for b.Loop() {
		if _, ok := LineDepths(value); !ok {
			b.Fatal("LineDepths refused benchmark value")
		}
	}
}
