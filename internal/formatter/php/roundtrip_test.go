package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// The EOF token carries file-trailing trivia, so the loop needs no tail.
func TestTokenStreamReconstructsSource(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".php", false) {
		t.Run(name, func(t *testing.T) {
			src := formattertest.ReadFixture(t, name+".php")
			file, _ := parser.Parse(src)

			var rebuilt bytes.Buffer
			rebuilt.Grow(len(src))

			for _, tok := range file.Tokens {
				for _, tr := range tok.Leading {
					rebuilt.Write(tr.Text)
				}

				rebuilt.Write(tok.Text)
			}

			diff := formattertest.FirstDiff(rebuilt.Bytes(), src)

			if diff >= 0 {
				t.Fatalf("token stream diverges from source at offset %d.\n=== rebuilt ===\n%s\n=== source ===\n%s", diff, rebuilt.Bytes(), src)
			}
		})
	}
}
