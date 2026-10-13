package yaml

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".yml")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)
	})
}

// A width other than the two columns after `- ` separates indent-level from
// marker-aligned layout, which the default width cannot tell apart.
func FuzzFormatWideIndent(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".yml")
	cfg := config.Default()
	cfg.TabWidth = 4
	cfg.IndentExplicit = true

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)
	})
}
