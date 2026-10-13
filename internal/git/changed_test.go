package git

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRegularFilesKeepsEachFormattableFileOnce(t *testing.T) {
	root := t.TempDir()

	for _, rel := range []string{"app/User.php", "resources/views/a.blade.php", "notes.txt", "generated/x.php"} {
		path := filepath.Join(root, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte("<?php\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.MkdirAll(filepath.Join(root, "modules", "sub.php"), 0o750); err != nil {
		t.Fatal(err)
	}

	listed := []string{
		"resources/views/a.blade.php", "app/User.php", "app/User.php",
		"removed.php", "modules/sub.php", "notes.txt", "generated/x.php",
	}
	skip := func(rel string) (bool, error) { return rel == "generated/x.php", nil }

	got, err := regularFiles(root, listed, skip)
	if err != nil {
		t.Fatalf("regularFiles: %v", err)
	}

	want := []string{
		filepath.Join(root, "app", "User.php"),
		filepath.Join(root, "resources", "views", "a.blade.php"),
	}

	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestChangedFilesRefusesARefThatLooksLikeAnOption(t *testing.T) {
	if _, err := ChangedFiles(t.TempDir(), "--output=x", func(string) (bool, error) { return false, nil }); err == nil {
		t.Fatal("a ref starting with a dash was accepted")
	}
}
