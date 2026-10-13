package upward

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestWalkStopsBelowTheCeiling(t *testing.T) {
	root := t.TempDir()
	start := filepath.Join(root, "a", "b")

	SetCeiling(root)
	t.Cleanup(func() { ceiling = "" })

	var visited []string

	err := Walk(start, func(dir string) (bool, error) {
		visited = append(visited, dir)

		return false, nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	want := []string{start, filepath.Join(root, "a")}

	if !slices.Equal(visited, want) {
		t.Fatalf("visited %q, want %q", visited, want)
	}
}
