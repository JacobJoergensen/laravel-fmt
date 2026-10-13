package detector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AnchorDir returns the deepest directory holding every path, or workDir when
// there are none.
//
// A path that does not exist still anchors on its parent directory: the caller
// reports the missing file itself, and guessing a different anchor would only pick
// the wrong config for the error message.
func AnchorDir(workDir string, paths []string) (string, error) {
	var common string

	for _, path := range paths {
		dir, err := anchorFor(path)
		if err != nil {
			return "", err
		}

		if common == "" {
			common = dir

			continue
		}

		common = commonDir(common, dir)
	}

	if common == "" {
		abs, err := filepath.Abs(workDir)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", workDir, err)
		}

		return abs, nil
	}

	return common, nil
}

// A glob anchors above its first pattern character: a pattern names no
// directory, and an upward search through one fails outright on Windows.
func anchorFor(path string) (string, error) {
	abs, err := filepath.Abs(globPrefix(path))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}

	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return abs, nil
	}

	return filepath.Dir(abs), nil
}

// The pattern characters are walk's own, so the anchor reads an argument the
// way the walk will.
func globPrefix(path string) string {
	idx := strings.IndexAny(path, "*?[")
	if idx < 0 {
		return path
	}

	cut := strings.LastIndexAny(path[:idx], `/\`)

	switch {
	case cut < 0:
		return "."
	case cut == 0:
		return path[:1]
	default:
		return path[:cut]
	}
}

// Paths on different volumes share no directory, so a is kept: a real directory
// beats an empty anchor.
func commonDir(a, b string) string {
	if a == b {
		return a
	}

	if filepath.VolumeName(a) != filepath.VolumeName(b) {
		return a
	}

	aParts := strings.Split(a, string(filepath.Separator))
	bParts := strings.Split(b, string(filepath.Separator))

	shared := 0

	for shared < len(aParts) && shared < len(bParts) && aParts[shared] == bParts[shared] {
		shared++
	}

	if shared == 0 {
		return a
	}

	return strings.Join(aParts[:shared], string(filepath.Separator))
}
