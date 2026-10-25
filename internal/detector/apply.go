package detector

import (
	"fmt"
	"path/filepath"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
)

// ResolveFileConfig returns cfg with path's own EditorConfig, Prettier and
// Mago layout applied, except for keys the user set in laravel-fmt.json.
func ResolveFileConfig(cfg *config.File, path string, ctx Context, userKeys map[string]bool) (config.File, error) {
	// Shallow copy: its slice fields still alias cfg's backing arrays, and this
	// runs concurrently per file. Overlays below may reassign whole fields but
	// must never mutate an aliased slice in place, or the goroutines would race.
	resolved := *cfg

	if userKeys["indentWidth"] {
		resolved.IndentExplicit = true
	}

	// The overlays match globs written from their config's directory, which
	// filepath.Rel cannot relate a relative path to.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return config.File{}, fmt.Errorf("resolve %s: %w", path, err)
	}

	prettier, err := filePrettier(absPath, ctx)
	if err != nil {
		return config.File{}, err
	}

	layout, err := fileLayout(absPath, ctx, prettier, nil)
	if err != nil {
		return config.File{}, err
	}

	copyLayout(&resolved, layout, userKeys)

	// A file's own Prettier config may name a Tailwind stylesheet; one set in
	// laravel-fmt.json outranks it.
	if ctx.PrettierResolver != nil && len(cfg.TailwindCS.Stylesheet) == 0 && !samePrettier(prettier, ctx.Prettier) {
		enabled, theme, err := ctx.PrettierResolver.tailwindFor(prettier, ctx)
		if err != nil {
			return config.File{}, err
		}

		resolved.Tailwind, resolved.TailwindTheme = enabled, theme
	}

	return resolved, nil
}

// LayoutSources maps each layout key to the detected config that set it.
func LayoutSources(path string, ctx Context) (map[string]string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
	}

	prettier, err := filePrettier(absPath, ctx)
	if err != nil {
		return nil, err
	}

	sources := map[string]string{}

	if _, err := fileLayout(absPath, ctx, prettier, sources); err != nil {
		return nil, err
	}

	return sources, nil
}

func filePrettier(absPath string, ctx Context) (*Prettier, error) {
	if ctx.PrettierResolver == nil {
		return ctx.Prettier, nil
	}

	return ctx.PrettierResolver.ForFile(absPath)
}

func samePrettier(a, b *Prettier) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Path == b.Path
}

// fileLayout replays one file's layout sources onto the defaults, lowest
// precedence first. Replayed onto the resolved config, a file's
// "indent_size = unset" could not fall back to the tool default, and its
// EditorConfig section would override Mago.
func fileLayout(absPath string, ctx Context, prettier *Prettier, sources map[string]string) (*config.File, error) {
	layout := config.Default()

	if ctx.EditorConfig != nil {
		section, err := ctx.EditorConfig.ResolveForFile(absPath)
		if err != nil {
			return nil, err
		}

		applyLayout(layout, sources, editorConfigFileName, func(cfg *config.File) {
			applyEditorConfigSection(cfg, section, nil)
		})
	}

	lang := formatter.DetectLanguage(absPath)

	if prettier != nil && prettier.formats(lang) {
		options := prettier.ResolveForFile(relativeToConfig(prettier.Path, absPath))

		applyLayout(layout, sources, prettier.Path, func(cfg *config.File) {
			applyPrettierOptions(cfg, options, nil)
		})
	}

	// Pint cannot be told to indent or end lines any other way, so a PHP file it
	// formats has this layout whatever the other sources say.
	if ctx.Pint != nil && lang == formatter.LangPHP {
		applyLayout(layout, sources, ctx.Pint.Path, func(cfg *config.File) {
			cfg.TabWidth = pintIndentWidth
			cfg.IndentStyle = config.IndentStyleSpace
			cfg.EndOfLine = config.EndOfLineLF
		})
	}

	if ctx.Mago != nil {
		applyLayout(layout, sources, ctx.Mago.Path, func(cfg *config.File) {
			applyMago(cfg, ctx.Mago)
		})
	}

	return layout, nil
}

// Keys are read off an empty config: a diff of the running layout would miss
// a value two sources agree on.
func applyLayout(layout *config.File, sources map[string]string, source string, apply func(*config.File)) {
	apply(layout)

	if sources == nil {
		return
	}

	var alone config.File

	apply(&alone)

	credit := func(key string, set bool) {
		if set {
			sources[key] = source
		}
	}

	credit("printWidth", alone.PrintWidth != 0)
	credit("indentWidth", alone.TabWidth != 0)
	credit("indentStyle", alone.IndentStyle != "")
	credit("endOfLine", alone.EndOfLine != "")
	credit("quoteStyle", alone.QuoteStyle != "")
	credit("markdown.proseWrap", alone.Markdown.ProseWrap != "")
}

func copyLayout(cfg, layout *config.File, userKeys map[string]bool) {
	if !userKeys["printWidth"] {
		cfg.PrintWidth = layout.PrintWidth
	}

	if !userKeys["indentWidth"] {
		cfg.TabWidth = layout.TabWidth
		cfg.IndentExplicit = layout.IndentExplicit
	}

	if !userKeys["indentStyle"] {
		cfg.IndentStyle = layout.IndentStyle
	}

	if !userKeys["endOfLine"] {
		cfg.EndOfLine = layout.EndOfLine
	}

	if !userKeys["quoteStyle"] {
		cfg.QuoteStyle = layout.QuoteStyle
	}

	if !userKeys["markdown.proseWrap"] {
		cfg.Markdown.ProseWrap = layout.Markdown.ProseWrap
	}
}

func relativeToConfig(configPath, path string) string {
	rel, err := filepath.Rel(filepath.Dir(configPath), path)
	if err != nil {
		return path
	}

	return rel
}

// Apply layers the detected sources onto cfg, lowest precedence first.
func Apply(cfg *config.File, ctx Context) {
	if ctx.EditorConfig != nil {
		applyEditorConfig(cfg, ctx.EditorConfig)
	}

	if ctx.Prettier != nil {
		applyPrettier(cfg, ctx.Prettier)
	}

	if ctx.PhpCsFixer != nil {
		applyPint(cfg, ctx.PhpCsFixer)
	}

	if ctx.Pint != nil {
		applyPint(cfg, ctx.Pint)
	}

	if ctx.Mago != nil {
		applyMago(cfg, ctx.Mago)
	}
}

func overlayPtr[T any](dst **T, src *T) {
	if src != nil {
		*dst = src
	}
}

func setIfPresent[T any](dst, src *T) {
	if src != nil {
		*dst = *src
	}
}
