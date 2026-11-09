package detector

import (
	"fmt"
	"path/filepath"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// LoadConfig layers laravel-fmt.json over the detected settings, so the
// user's keys win. Its first argument, the Resolver's project root, goes
// unused: ignores are read where the detected tools are, ctx.ConfigDir, which
// is the Laravel root for a package nested in an application.
func LoadConfig(_ string, ctx Context) (config.Loaded, error) {
	dir := ctx.ConfigDir()

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

	loaded.IgnoreAnchor = dir

	return loaded, nil
}

// LoadDefaults is LoadConfig without laravel-fmt.json, for --no-config. The
// Tailwind stylesheet is still resolved: it is a fact about the project, not
// a setting.
func LoadDefaults(ctx Context) (config.Loaded, error) {
	cfg := config.Default()
	Apply(cfg, ctx)

	anchor := ctx.ConfigDir()

	if err := adoptIgnores(cfg, ctx, anchor); err != nil {
		return config.Loaded{}, err
	}

	if err := resolveTailwind(cfg, ctx, ""); err != nil {
		return config.Loaded{}, err
	}

	return config.Loaded{File: *cfg, IgnoreAnchor: anchor}, nil
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

	loaded.IgnoreAnchor = filepath.Dir(absConfig)

	return loaded, nil
}
