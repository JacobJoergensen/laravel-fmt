package blade

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// genBlade builds a deterministic view of `components` nested blocks with what a
// real template carries: Tailwind classes, {{ }}, bound attrs, @if/@foreach.
// Profiling put the cost across parse and emit, allocation-heavy on nesting (not
// class sorting), so the benchmark stresses structure. Sizes bracket real files.
func genBlade(components int) []byte {
	blocks := []string{
		"    <div class=\"mt-4 flex items-center gap-2 rounded-lg bg-white p-4 shadow\" x-data=\"{ open: false }\">\n        <span class=\"text-sm font-medium text-gray-700\">{{ $item->label }}</span>\n    </div>\n",
		"    @if ($item->active)\n        <a href=\"{{ route('items.show', $item) }}\" class=\"block px-3 py-2 text-blue-600 hover:underline\">\n            {{ $item->name }}\n        </a>\n    @endif\n",
		"    <ul class=\"divide-y divide-gray-100\">\n        @foreach ($item->children as $child)\n            <li class=\"flex justify-between py-2\" :class=\"{ 'font-bold': $child->pinned }\">{{ $child->title }}</li>\n        @endforeach\n    </ul>\n",
	}

	var sb strings.Builder
	sb.WriteString("<div class=\"mx-auto max-w-4xl space-y-6 p-6\">\n")

	for i := range components {
		fmt.Fprintf(&sb, "    <section class=\"rounded border border-gray-200\" data-index=\"%d\">\n%s    </section>\n", i, blocks[i%len(blocks)])
	}

	sb.WriteString("</div>\n")

	return []byte(sb.String())
}

func canonicalBlade(b *testing.B, components int) []byte {
	b.Helper()

	out, err := Format(genBlade(components), config.Default())
	if err != nil {
		b.Fatalf("generated input did not format: %v", err)
	}

	return out
}

func BenchmarkFormat(b *testing.B) {
	cfg := config.Default()

	sizes := []struct {
		name       string
		components int
	}{
		{"typical", 10},
		{"large", 90},
	}

	for _, tc := range sizes {
		src := canonicalBlade(b, tc.components)

		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Format(src, cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// genNestedIf builds unclosed nested block openers, the shape that once made
// the block-body search exponential. Each opener still scans to EOF once, so
// doubling the levels roughly quadruples the time.
func genNestedIf(levels int) []byte {
	return []byte(strings.Repeat("@if(1)", levels) + "\n")
}

func BenchmarkFormatNestedBlockOpeners(b *testing.B) {
	cfg := config.Default()

	for _, levels := range []int{13, 26} {
		src := genNestedIf(levels)

		b.Run(fmt.Sprintf("levels_%d", levels), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Format(src, cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
