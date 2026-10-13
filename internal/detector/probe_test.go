package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProbeMemoReadsADirectoryOnce(t *testing.T) {
	memo := &probeMemo{}
	dir := t.TempDir()

	calls := 0
	detect := func(string) (string, bool, error) {
		calls++

		return "config", true, nil
	}

	for range 3 {
		value, found, err := probeDir(memo, probePint, dir, detect)
		if err != nil || !found || value != "config" {
			t.Fatalf("probeDir = %q, %v, %v; want \"config\", true, nil", value, found, err)
		}
	}

	if calls != 1 {
		t.Fatalf("the probe read %s %d times, want 1", dir, calls)
	}

	if _, _, err := probeDir(memo, probeMago, dir, detect); err != nil {
		t.Fatalf("probeDir: %v", err)
	}

	if calls != 2 {
		t.Fatalf("a second probe answered from the first one's entry for %s", dir)
	}
}

func TestProbeMemoStatsAPathOnce(t *testing.T) {
	memo := &probeMemo{}
	path := filepath.Join(t.TempDir(), "composer.json")

	seedFileAt(t, filepath.Dir(path), "composer.json", "{}")

	exists, err := memo.statExists(path)
	if err != nil || !exists {
		t.Fatalf("statExists = %v, %v; want true, nil", exists, err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}

	// The file is gone, so only a cached answer can still say true.
	exists, err = memo.statExists(path)
	if err != nil || !exists {
		t.Fatalf("statExists = %v, %v after the file went away; the path was stat'd twice", exists, err)
	}

	exists, err = (&probeMemo{}).statExists(path)
	if err != nil || exists {
		t.Fatalf("a fresh memo read %v, %v; it must see the filesystem as it now is", exists, err)
	}
}
