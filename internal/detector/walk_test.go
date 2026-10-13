package detector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// A fixture must not pick up the host's files above the temp directory.
func init() {
	upward.SetCeiling(os.TempDir())
}

func TestFindConfigStopsAtBound(t *testing.T) {
	root := t.TempDir()
	bound := filepath.Join(root, "project")
	deep := filepath.Join(bound, "app", "Models")

	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	parentYml := filepath.Join(root, config.FileName)

	if err := os.WriteFile(parentYml, nil, 0o644); err != nil {
		t.Fatalf("seed parent yml: %v", err)
	}

	got, err := findConfig(deep, bound, &probeMemo{})
	if err != nil {
		t.Fatalf("findConfig: %v", err)
	}

	if got != "" {
		t.Fatalf("findConfig returned %q from above the bound", got)
	}
}

func TestFindConfigSearchesBoundDirectoryItself(t *testing.T) {
	root := t.TempDir()
	bound := filepath.Join(root, "project")
	deep := filepath.Join(bound, "app", "Models")

	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	boundYml := filepath.Join(bound, config.FileName)

	if err := os.WriteFile(boundYml, nil, 0o644); err != nil {
		t.Fatalf("seed bound yml: %v", err)
	}

	got, err := findConfig(deep, bound, &probeMemo{})
	if err != nil {
		t.Fatalf("findConfig: %v", err)
	}

	if got != boundYml {
		t.Fatalf("findConfig = %q, want %q", got, boundYml)
	}
}

func TestFindConfigFoundAtWorkDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)

	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("seed yml: %v", err)
	}

	got, err := findConfig(dir, "", &probeMemo{})
	if err != nil {
		t.Fatalf("findConfig: %v", err)
	}

	if got != path {
		t.Fatalf("findConfig = %q, want %q", got, path)
	}
}

func TestFindConfigEmptyStopAtUnboundedWalks(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "deeper")

	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	parentYml := filepath.Join(root, config.FileName)

	if err := os.WriteFile(parentYml, nil, 0o644); err != nil {
		t.Fatalf("seed parent yml: %v", err)
	}

	got, err := findConfig(deep, "", &probeMemo{})
	if err != nil {
		t.Fatalf("findConfig: %v", err)
	}

	if got != parentYml {
		t.Fatalf("findConfig = %q, want %q (unbounded walk should reach parent)", got, parentYml)
	}
}
