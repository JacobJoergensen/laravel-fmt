package markdown

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".md")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)
	})
}
