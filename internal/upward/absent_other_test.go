//go:build !windows

package upward

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAbsentCoversAMissingFile(t *testing.T) {
	_, err := os.Stat(filepath.Join(t.TempDir(), "composer.json"))

	if !Absent(err) {
		t.Fatalf("Absent(%v) = false, want true", err)
	}
}

func TestAbsentLeavesRealFailuresFatal(t *testing.T) {
	if Absent(errors.New("disk on fire")) {
		t.Fatal("an unrelated error must not read as a missing file")
	}
}
