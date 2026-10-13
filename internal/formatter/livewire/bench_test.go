package livewire

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

const (
	sfcHead = "<?php\n\nuse Livewire\\Component;\n\nnew class extends Component\n{\n" +
		"    public string $search = '';\n\n    public array $items = [];\n\n"
	sfcMethod = "    public function action%d(int $id): void\n    {\n" +
		"        $this->items[$id] = collect($this->items)->where('id', $id)->first() ?? ['id' => $id];\n" +
		"    }\n\n"
	sfcTemplateHead = "};\n?>\n\n<div class=\"space-y-4 p-6\">\n" +
		"    <input type=\"text\" wire:model.live=\"search\" class=\"rounded border px-3 py-2\">\n"
	sfcRow = "    <div wire:key=\"row-%d\" class=\"flex items-center justify-between rounded bg-white p-4 shadow\">\n" +
		"        <span class=\"text-sm text-gray-700\">{{ $items[%d]['id'] ?? '' }}</span>\n" +
		"        <button wire:click=\"action%d(%d)\" class=\"text-blue-600 hover:underline\">Open</button>\n" +
		"    </div>\n"
)

// genSFC builds a component whose template dwarfs its frontmatter, as in a
// real app, so lexing past the frontmatter's `?>` would dominate the cost.
func genSFC(methods, rows int) []byte {
	var sb strings.Builder

	sb.WriteString(sfcHead)

	for i := range methods {
		fmt.Fprintf(&sb, sfcMethod, i)
	}

	sb.WriteString(sfcTemplateHead)

	for i := range rows {
		fmt.Fprintf(&sb, sfcRow, i, i, i%methods, i)
	}

	sb.WriteString("</div>\n")

	return []byte(sb.String())
}

func BenchmarkFormat(b *testing.B) {
	cfg := config.Default()

	src, err := Format(genSFC(20, 120), cfg)
	if err != nil {
		b.Fatalf("generated component did not format: %v", err)
	}

	b.SetBytes(int64(len(src)))
	b.ReportAllocs()

	for b.Loop() {
		if _, err := Format(src, cfg); err != nil {
			b.Fatal(err)
		}
	}
}
