package yaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	yamlv3 "go.yaml.in/yaml/v3"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".yml")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		checkFormatKeepsValues(t, cfg, src)
	})
}

// A width other than the two columns after `- ` separates indent-level from
// marker-aligned layout, which the default width cannot tell apart.
func FuzzFormatWideIndent(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".yml")
	cfg := config.Default()
	cfg.IndentWidth = 4
	cfg.IndentExplicit = true

	f.Fuzz(func(t *testing.T, src []byte) {
		checkFormatKeepsValues(t, cfg, src)
	})
}

// yaml.v3 is the oracle: what it reads from src it must read from the output.
// It is not spec-exact, but its misreadings do not depend on layout, so both
// sides share them. Measured on yaml-test-suite, it read every formatted case
// as its input except a file that ends inside a block scalar with no final line
// break: the suite reads the file's end as a line break (L24T), yaml.v3 does
// not, and the formatter adds the break. Such inputs are skipped.
func checkFormatKeepsValues(t *testing.T, cfg *config.File, src []byte) {
	t.Helper()

	format := func(b []byte) ([]byte, error) { return Format(b, cfg) }

	out, err := formattertest.FormatIdempotent(t, format, src)
	if err != nil || endsWithoutLineBreak(src) {
		return
	}

	want, err := yamlValues(src)
	if err != nil {
		return
	}

	got, err := yamlValues(out)
	if err != nil {
		t.Fatalf("output is not YAML yaml.v3 reads: %v\ninput  %q\noutput %q", err, src, out)
	}

	if want != got {
		t.Fatalf("values differ\ninput  %q\noutput %q\nwant %s\ngot  %s", src, out, want, got)
	}
}

func endsWithoutLineBreak(src []byte) bool {
	return len(src) > 0 && src[len(src)-1] != '\n' && src[len(src)-1] != '\r'
}

// %#v prints map keys sorted and NaN as NaN, so equal documents print equal
// where reflect.DeepEqual would call a NaN unequal to itself.
func yamlValues(src []byte) (values string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("yaml.v3 panicked: %v", recovered)
		}
	}()

	dec := yamlv3.NewDecoder(bytes.NewReader(src))

	var docs []any

	for {
		var doc any

		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return fmt.Sprintf("%#v", docs), nil
		}

		if err != nil {
			return "", fmt.Errorf("decode: %w", err)
		}

		docs = append(docs, doc)
	}
}
