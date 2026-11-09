package detector

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
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

// Prettier's tailwindStylesheet is honored so both tools sort alike. A
// configured path resolves against the file that set it and must exist, while
// discovery finding nothing just means no Tailwind. Several entry points
// merge: a class the sorter does not know sorts first, so picking one would
// misplace the others' classes.
//
// configDir is the loaded laravel-fmt.json's directory, not ctx.ConfigPath's,
// which under --config is the auto-detected file.
func resolveTailwind(cfg *config.File, ctx Context, configDir string) error {
	var paths []string
	var base string

	switch {
	case len(cfg.TailwindCS.Stylesheet) > 0:
		paths, base = cfg.TailwindCS.Stylesheet, configDir
	case prettierStylesheet(ctx) != "":
		paths, base = []string{prettierStylesheet(ctx)}, filepath.Dir(ctx.Prettier.Path)
	case ctx.Tailwind != nil:
		paths = ctx.Tailwind.Paths
	default:
		return nil
	}

	resolved := make([]string, 0, len(paths))

	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, path)
		}

		resolved = append(resolved, path)
	}

	theme, err := tailwind.LoadTheme(resolved...)
	if err != nil {
		return fmt.Errorf("tailwind stylesheet %s: %w", strings.Join(resolved, ", "), err)
	}

	cfg.Tailwind = true
	cfg.TailwindTheme = theme

	return nil
}

func prettierStylesheet(ctx Context) string {
	if ctx.Prettier == nil || ctx.Prettier.options.TailwindStylesheet == nil {
		return ""
	}

	return *ctx.Prettier.options.TailwindStylesheet
}
