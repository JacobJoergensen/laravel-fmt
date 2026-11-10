package detector

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectLaravelNamesAnUnreadablePathOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits")
	}

	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}

	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // restores the folder so t.TempDir can remove it
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	_, _, err := detectLaravel(locked, &probeMemo{})
	if err == nil {
		t.Fatal("detectLaravel: want a permission error")
	}

	composer := filepath.Join(locked, "composer.json")
	if n := strings.Count(err.Error(), composer); n != 1 {
		t.Fatalf("error names %s %d times: %v", composer, n, err)
	}
}
