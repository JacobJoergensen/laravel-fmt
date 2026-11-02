package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseRawKeepsRegularStagedBlobs(t *testing.T) {
	out := []byte(
		":100644 100644 1111111111111111111111111111111111111111 2222222222222222222222222222222222222222 M\x00app/User.php\x00" +
			":000000 100755 0000000000000000000000000000000000000000 3333333333333333333333333333333333333333 A\x00bin/run\x00" +
			":100644 120000 4444444444444444444444444444444444444444 5555555555555555555555555555555555555555 M\x00link\x00",
	)

	got := parseRaw(out)

	want := []StagedFile{
		{Path: "app/User.php", Mode: "100644", SHA: "2222222222222222222222222222222222222222"},
		{Path: "bin/run", Mode: "100755", SHA: "3333333333333333333333333333333333333333"},
	}

	if len(got) != len(want) {
		t.Fatalf("parseRaw returned %d entries, want %d: %+v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSplitNULDropsEmptyTrailingRecord(t *testing.T) {
	got := splitNUL([]byte("a\x00b\x00"))

	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("splitNUL = %q, want [a b]", got)
	}

	if splitNUL(nil) != nil {
		t.Error("splitNUL(nil) should be nil")
	}
}

func TestRootFromNamesTheRootThroughTheLinkItWasReachedBy(t *testing.T) {
	toplevel := t.TempDir()
	sub := filepath.Join(toplevel, "sub")

	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	linked := filepath.Join(t.TempDir(), "linked")
	linkedSub := filepath.Join(t.TempDir(), "linked")

	if !linkDir(t, linked, toplevel) || !linkDir(t, linkedSub, sub) {
		t.Skip("this platform will not create a link without extra privileges")
	}

	tests := []struct {
		name string
		dir  string
		cdup string
		want string
	}{
		{"at the root", linked, "", linked},
		{"in a subfolder", filepath.Join(linked, "sub"), "../", linked},
		{"link to a subfolder has no path to the root", linkedSub, "../", toplevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rootFrom(tt.dir, toplevel, tt.cdup); got != tt.want {
				t.Fatalf("rootFrom(%s, %s) = %s, want %s", tt.dir, tt.cdup, got, tt.want)
			}
		})
	}
}

// Windows falls back to a directory junction, which needs no privilege.
// mklink is a cmd builtin and cannot be exec'd on its own.
func linkDir(t *testing.T, link, target string) bool {
	t.Helper()

	if os.Symlink(target, link) == nil {
		return true
	}

	if runtime.GOOS != "windows" {
		return false
	}

	//nolint:gosec // both paths are t.TempDir() output, not input
	return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run() == nil
}
