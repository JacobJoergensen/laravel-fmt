//go:build !windows

package walk

import (
	"fmt"
	"path/filepath"
)

func resolveLinks(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}

	return resolved, nil
}
