package yaml

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestExplicitIndentWins(t *testing.T) {
	src := []byte("parent:\n  child: value\n")

	cfg := config.Default()
	cfg.IndentExplicit = true
	cfg.TabWidth = 4

	out, err := Format(src, cfg)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if !bytes.Contains(out, []byte("\n    child: value")) {
		t.Fatalf("explicit indentWidth=4 ignored; got:\n%s", out)
	}
}

func TestConventionWithoutExplicitIndent(t *testing.T) {
	src := []byte("parent:\n  child: value\n")

	cfg := config.Default()
	cfg.TabWidth = 4

	out, err := Format(src, cfg)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if !bytes.Contains(out, []byte("\n  child: value")) {
		t.Fatalf("convention 2-space ignored; got:\n%s", out)
	}
}
