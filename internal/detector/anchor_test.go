package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnchorDirFollowsThePathsBeingFormatted(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "mono", "pkg-b")
	other := filepath.Join(root, "mono", "pkg-a")

	for _, dir := range []string{pkg, other} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	file := filepath.Join(pkg, "y.php")
	if err := os.WriteFile(file, []byte("<?php\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	elsewhere := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o750); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		paths []string
		want  string
	}{
		{"a file anchors on its directory", []string{file}, pkg},
		{"a directory anchors on itself", []string{pkg}, pkg},
		{"two paths anchor on their common directory", []string{pkg, other}, filepath.Join(root, "mono")},
		{"no paths fall back to the working directory", nil, elsewhere},
		{"a missing path still anchors on its directory", []string{filepath.Join(pkg, "gone.php")}, pkg},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AnchorDir(elsewhere, tc.paths)
			if err != nil {
				t.Fatalf("AnchorDir: %v", err)
			}

			wantAbs, err := filepath.Abs(tc.want)
			if err != nil {
				t.Fatal(err)
			}

			if got != wantAbs {
				t.Fatalf("AnchorDir(%q, %v) = %q, want %q", elsewhere, tc.paths, got, wantAbs)
			}
		})
	}
}

func TestAnchorDirUsesTheDirectoryBeforeAGlob(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "resources", "views"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cases := []struct {
		name  string
		paths []string
		want  string
	}{
		{"double star", []string{filepath.Join(root, "resources") + "/**/*.blade.php"}, filepath.Join(root, "resources")},
		{"single star", []string{filepath.Join(root, "resources") + "/*.php"}, filepath.Join(root, "resources")},
		{"character class", []string{filepath.Join(root, "resources") + "/[ab].php"}, filepath.Join(root, "resources")},
		{"pattern deeper down", []string{filepath.Join(root, "resources", "views") + "/**/*.php"}, filepath.Join(root, "resources", "views")},
		{"no pattern at all", []string{filepath.Join(root, "resources", "x.php")}, filepath.Join(root, "resources")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AnchorDir(root, tc.paths)
			if err != nil {
				t.Fatalf("AnchorDir: %v", err)
			}

			if got != tc.want {
				t.Fatalf("AnchorDir(%v) = %q, want %q", tc.paths, got, tc.want)
			}
		})
	}
}
