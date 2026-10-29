package json

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
)

func TestExplicitIndentWins(t *testing.T) {
	src := []byte("{\n\"a\": 1,\n\"b\": 2\n}\n")

	cfg := config.Default()
	cfg.IndentExplicit = true
	cfg.IndentWidth = 4

	out, err := Format("settings.json", src, cfg)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if !bytes.Contains(out, []byte("\n    \"a\": 1")) {
		t.Fatalf("explicit indentWidth=4 ignored; got:\n%s", out)
	}
}

func TestConventionWithoutExplicitIndent(t *testing.T) {
	src := []byte("{\n\"a\": 1,\n\"b\": 2\n}\n")

	cases := []struct {
		filename, wantLine string
	}{
		{"settings.json", "\n  \"a\": 1"},
		{"composer.json", "\n    \"a\": 1"},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			cfg := config.Default()
			cfg.IndentWidth = 3

			out, err := Format(tc.filename, src, cfg)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}

			if !bytes.Contains(out, []byte(tc.wantLine)) {
				t.Fatalf("convention indent ignored; got:\n%s", out)
			}
		})
	}
}

func TestEmitScriptBodyRejectsWhatIsNotStrictJSON(t *testing.T) {
	cases := []struct{ name, src string }{
		{"comment", "{\n  // a\n  \"a\": 1\n}"},
		{"trailing comma", "{\"a\": 1,}"},
		{"empty", "  \n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := formatter.NewWriter(config.Default())

			if EmitScriptBody(w, []byte(tc.src)) {
				t.Fatal("EmitScriptBody accepted a body that is not strict JSON")
			}

			if out := w.Bytes(); len(out) != 0 {
				t.Fatalf("EmitScriptBody wrote %q for a body it rejected", out)
			}
		})
	}
}
