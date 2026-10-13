package detector

import (
	"fmt"
	"path/filepath"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// stopAt, inclusive, keeps a stray laravel-fmt.json above the Laravel root
// from applying; an empty stopAt walks to the filesystem root.
func findConfig(workDir, stopAt string, memo *probeMemo) (string, error) {
	var found string

	err := upward.Walk(workDir, func(dir string) (bool, error) {
		candidate := filepath.Join(dir, config.FileName)

		exists, statErr := memo.statExists(candidate)
		if statErr != nil {
			return false, fmt.Errorf("stat %s: %w", candidate, statErr)
		}

		if exists {
			found = candidate

			return true, nil
		}

		return stopAt != "" && dir == stopAt, nil
	})

	return found, err
}

// nestedConfig finds a laravel-fmt.json between workDir and the project root,
// which the run does not use.
func nestedConfig(workDir, root, chosen string, memo *probeMemo) (string, error) {
	if workDir == root {
		return "", nil
	}

	found, err := findConfig(workDir, root, memo)
	if err != nil {
		return "", err
	}

	if found == chosen {
		return "", nil
	}

	return found, nil
}
