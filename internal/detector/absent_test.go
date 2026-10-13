package detector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAbsentCoversAnUnnamablePath(t *testing.T) {
	_, err := os.Stat(filepath.Join(t.TempDir(), "**", "composer.json"))
	if err == nil {
		t.Skip("the filesystem accepted the path")
	}

	if !absent(err) {
		t.Fatalf("absent(%v) = false, want true", err)
	}
}

func TestAbsentLeavesRealFailuresFatal(t *testing.T) {
	if absent(errors.New("disk on fire")) {
		t.Fatal("an unrelated error must not read as a missing file")
	}
}
