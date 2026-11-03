package walk

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

const (
	benchDirs        = 40
	benchFilesPerDir = 50
)

func BenchmarkFiles(b *testing.B) {
	root := b.TempDir()

	for d := range benchDirs {
		dir := filepath.Join(root, "app", fmt.Sprintf("dir%d", d))
		if err := os.MkdirAll(dir, 0o750); err != nil {
			b.Fatalf("mkdir: %v", err)
		}

		for f := range benchFilesPerDir {
			name := filepath.Join(dir, fmt.Sprintf("file%d.php", f))
			if err := os.WriteFile(name, []byte("<?php"), 0o600); err != nil {
				b.Fatalf("write: %v", err)
			}
		}
	}

	cfg := config.Default()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := Files(root, root, cfg, nil, nil); err != nil {
			b.Fatalf("Files: %v", err)
		}
	}
}
