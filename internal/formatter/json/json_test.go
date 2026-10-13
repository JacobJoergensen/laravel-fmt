package json

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestExplicitIndentWins(t *testing.T) {
	src := []byte("{\n\"a\": 1,\n\"b\": 2\n}\n")

	cfg := config.Default()
	cfg.IndentExplicit = true
	cfg.TabWidth = 4

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

	cfg := config.Default()
	cfg.TabWidth = 4

	out, err := Format("settings.json", src, cfg)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if !bytes.Contains(out, []byte("\n  \"a\": 1")) {
		t.Fatalf("convention 2-space ignored; got:\n%s", out)
	}
}
