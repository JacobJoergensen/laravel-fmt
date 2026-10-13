package php

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// genPHP builds a class with a real class's mix: properties, docblocks on half
// the methods, blank lines, light to heavy bodies. At about 4 KB and 90 KB the
// two sizes sit just above the corpus p50 (~2 KB) and p99 (~60 KB).
func genPHP(methods int) []byte {
	bodies := []string{
		"        return $this->name;\n",
		"        $this->count += $step;\n\n        return $this;\n",
		"        return $this->repo->query()->where('tenant_id', $id)->where('active', true)->orderBy('name')->limit(50)->get();\n",
		"        return ['total' => count($rows), 'items' => array_map(fn ($r) => $r->id, $rows), 'first' => $rows[0] ?? null];\n",
		"        return match ($kind) {\n            'a', 'b' => 'grouped',\n            'c' => 'single',\n            default => 'unknown',\n        };\n",
		"        if ($value === null) {\n            throw new InvalidArgumentException('value required');\n        }\n\n        return $value * 2;\n",
	}

	var sb strings.Builder
	sb.WriteString("<?php\n\nnamespace App\\Services;\n\nuse Illuminate\\Support\\Collection;\n\n")
	sb.WriteString("class GeneratedService\n{\n")
	sb.WriteString("    private string $name = 'default';\n\n    private int $count = 0;\n\n    protected array $config = [];\n\n")

	for i := range methods {
		if i%2 == 0 {
			sb.WriteString("    /**\n     * Handle unit " + strconv.Itoa(i) + ".\n     *\n     * @param  array<int, mixed>  $rows\n     * @return mixed\n     */\n")
		}

		fmt.Fprintf(&sb, "    public function method%d(int $id, array $rows, string $kind, int $step, ?int $value): mixed\n    {\n%s    }\n\n", i, bodies[i%len(bodies)])
	}

	sb.WriteString("}\n")

	return []byte(sb.String())
}

// canonicalPHP returns genPHP already formatted, so the benchmark measures the
// steady-state re-format an editor runs on every save, not one-off reshaping.
func canonicalPHP(b *testing.B, methods int) []byte {
	b.Helper()

	out, err := Format(genPHP(methods), config.Default())
	if err != nil {
		b.Fatalf("generated input did not format: %v", err)
	}

	return out
}

func BenchmarkFormat(b *testing.B) {
	cfg := config.Default()

	sizes := []struct {
		name    string
		methods int
	}{
		{"typical", 14},
		{"large", 340},
	}

	for _, tc := range sizes {
		src := canonicalPHP(b, tc.methods)

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

// BenchmarkParseVsBuild splits the stages so a regression is attributable to one
// side: parsing is byte-bound and cheap, BuildFile is the allocation-bound half.
func BenchmarkParseVsBuild(b *testing.B) {
	cfg := config.Default()
	src := canonicalPHP(b, 340)

	b.Run("parse", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()

		for b.Loop() {
			parser.Parse(src)
		}
	})

	file, _ := parser.Parse(src)

	b.Run("build", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()

		for b.Loop() {
			BuildFile(file, cfg)
		}
	})
}
