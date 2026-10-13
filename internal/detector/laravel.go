package detector

import (
	"fmt"
	"path/filepath"

	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// Laravel describes a detected Laravel project root.
type Laravel struct {
	Root string
}

func detectLaravel(workDir string, memo *probeMemo) (Laravel, bool, error) {
	var root string

	err := upward.Walk(workDir, func(dir string) (bool, error) {
		composerPath := filepath.Join(dir, "composer.json")

		hasComposer, err := memo.statExists(composerPath)
		if err != nil {
			return false, fmt.Errorf("stat %s: %w", composerPath, err)
		}

		if !hasComposer {
			return false, nil
		}

		artisanPath := filepath.Join(dir, "artisan")

		hasArtisan, err := memo.statExists(artisanPath)
		if err != nil {
			return false, fmt.Errorf("stat %s: %w", artisanPath, err)
		}

		if hasArtisan {
			root = dir

			return true, nil
		}

		return false, nil
	})

	if err != nil || root == "" {
		return Laravel{}, false, err
	}

	return Laravel{Root: root}, true, nil
}
