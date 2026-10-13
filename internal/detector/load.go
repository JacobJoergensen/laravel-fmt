package detector

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
)

// LoadConfig layers laravel-fmt.json over the detected settings, so the
// user's keys win.
func LoadConfig(workDir string, ctx Context) (config.Loaded, error) {
	dir := workDir

	if ctx.ConfigPath != "" {
		dir = filepath.Dir(ctx.ConfigPath)
	}

	base := config.Default()
	Apply(base, ctx)

	if err := adoptIgnores(base, ctx, dir); err != nil {
		return config.Loaded{}, err
	}

	loaded, err := config.LoadFrom(base, dir)
	if err != nil {
		return config.Loaded{}, fmt.Errorf("load config: %w", err)
	}

	if err := resolveTailwind(&loaded.File, ctx, dir); err != nil {
		return config.Loaded{}, err
	}

	return loaded, nil
}

// LoadDefaults is LoadConfig without laravel-fmt.json, for --no-config. The
// Tailwind stylesheet is still resolved: it is a fact about the project, not
// a setting.
func LoadDefaults(ctx Context) (config.Loaded, error) {
	cfg := config.Default()
	Apply(cfg, ctx)

	anchor := ctx.WorkDir

	if ctx.Laravel != nil {
		anchor = ctx.Laravel.Root
	}

	if err := adoptIgnores(cfg, ctx, anchor); err != nil {
		return config.Loaded{}, err
	}

	if err := resolveTailwind(cfg, ctx, ""); err != nil {
		return config.Loaded{}, err
	}

	return config.Loaded{File: *cfg}, nil
}

// LoadConfigFile is LoadConfig for --config; a missing file is an error,
// since the user named it.
func LoadConfigFile(configPath string, ctx Context) (config.Loaded, error) {
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return config.Loaded{}, fmt.Errorf("resolve %s: %w", configPath, err)
	}

	base := config.Default()
	Apply(base, ctx)

	if err := adoptIgnores(base, ctx, filepath.Dir(absConfig)); err != nil {
		return config.Loaded{}, err
	}

	loaded, err := config.LoadFile(base, configPath)
	if err != nil {
		return config.Loaded{}, fmt.Errorf("load config: %w", err)
	}

	if err := resolveTailwind(&loaded.File, ctx, filepath.Dir(absConfig)); err != nil {
		return config.Loaded{}, err
	}

	return loaded, nil
}

// resolveTailwind reads the Tailwind stylesheets and builds the project's
// theme. Prettier's tailwindStylesheet is honored so both tools sort alike. A
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
	if ctx.Prettier == nil || ctx.Prettier.Options.TailwindStylesheet == nil {
		return ""
	}

	return *ctx.Prettier.Options.TailwindStylesheet
}
