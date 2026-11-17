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
	cfg.IndentWidth = 4

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
	cfg.IndentWidth = 4

	out, err := Format(src, cfg)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if !bytes.Contains(out, []byte("\n  child: value")) {
		t.Fatalf("convention 2-space ignored; got:\n%s", out)
	}
}

func TestFormatLeavesAByteOrderMarkAfterBlankLinesButReportsErrors(t *testing.T) {
	kept := []byte("\n\xEF\xBB\xBFa: 1\n")

	out, err := Format(kept, config.Default())
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if !bytes.Equal(out, kept) {
		t.Fatalf("got %q, want it unchanged", out)
	}

	if _, err := Format([]byte("\n\xEF\xBB\xBFa: [1, 2\n"), config.Default()); err == nil {
		t.Fatal("an unterminated flow collection after a byte order mark was not reported")
	}
}
