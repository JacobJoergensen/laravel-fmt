package detector

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// Tailwind is a detected Tailwind v4 install. v3 is never detected: the
// sorter's registry is generated from v4.
type Tailwind struct {
	Paths []string
}

// detectTailwind collects every v4 entry point under resources/css. The walk
// recurses because Filament writes resources/css/filament/<panel>/theme.css,
// and WalkDir's lexical order keeps the merge order stable.
func detectTailwind(root string) (Tailwind, bool, error) {
	dir := filepath.Join(root, "resources", "css")

	var paths []string

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".css") {
			return nil
		}

		if tailwind.ImportsTailwind(path) {
			paths = append(paths, path)
		}

		return nil
	})
	if err != nil {
		if upward.Absent(err) {
			return Tailwind{}, false, nil
		}

		return Tailwind{}, false, fmt.Errorf("walk %s: %w", dir, err)
	}

	if len(paths) == 0 {
		return Tailwind{}, false, nil
	}

	return Tailwind{Paths: paths}, true, nil
}
