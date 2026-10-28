package yaml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// genWorkflow builds a CI workflow of `jobs` jobs; the sizes bracket real
// workflow and compose files.
func genWorkflow(jobs int) []byte {
	var sb strings.Builder
	sb.WriteString("name: ci\n\non:\n  push:\n    branches: [main]\n  pull_request:\n\njobs:\n")

	for i := range jobs {
		fmt.Fprintf(&sb, "  job-%d:\n", i)
		sb.WriteString("    runs-on: ubuntu-latest\n")
		sb.WriteString("    strategy:\n      matrix:\n        php: ['8.3', '8.4']\n        stability: [prefer-lowest, prefer-stable]\n")
		sb.WriteString("    steps:\n")
		sb.WriteString("      # Check out the code before anything else.\n")
		sb.WriteString("      - uses: actions/checkout@v4\n")
		sb.WriteString("      - name: Setup PHP\n        uses: shivammathur/setup-php@v2\n        with:\n          php-version: ${{ matrix.php }}\n          coverage: none\n")
		fmt.Fprintf(&sb, "      - name: Run tests %d\n        run: |\n          composer install --no-interaction\n          vendor/bin/pest --parallel\n", i)
	}

	return []byte(sb.String())
}

func canonicalWorkflow(b *testing.B, jobs int) []byte {
	b.Helper()

	out, err := Format(genWorkflow(jobs), config.Default())
	if err != nil {
		b.Fatalf("generated input did not format: %v", err)
	}

	return out
}

func BenchmarkFormat(b *testing.B) {
	cfg := config.Default()

	sizes := []struct {
		name string
		jobs int
	}{
		{"typical", 5},
		{"large", 100},
	}

	for _, tc := range sizes {
		src := canonicalWorkflow(b, tc.jobs)

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
