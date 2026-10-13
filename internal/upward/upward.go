// Package upward walks from a directory up towards the filesystem root.
package upward

import "path/filepath"

var ceiling string

// SetCeiling stops every later Walk below dir. Only tests call it.
func SetCeiling(dir string) {
	ceiling = filepath.Clean(dir)
}

// Walk visits startDir and each directory above it until visit stops.
func Walk(startDir string, visit func(dir string) (stop bool, err error)) error {
	dir := startDir

	for {
		stop, err := visit(dir)
		if err != nil {
			return err
		}

		if stop {
			return nil
		}

		parent := filepath.Dir(dir)

		if parent == dir || parent == ceiling {
			return nil
		}

		dir = parent
	}
}
