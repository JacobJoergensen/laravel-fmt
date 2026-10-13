package json

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".json")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format("fuzz.json", b, cfg)
		}, src)
	})
}

func FuzzFormatJSONC(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".json")
	formattertest.AddFuzzSeeds(f, ".jsonc")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format("fuzz.jsonc", b, cfg)
		}, src)
	})
}
