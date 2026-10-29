package detector

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/pelletier/go-toml/v2"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

const (
	prettierKindPackageJSON = iota
	prettierKindPackageYAML
	prettierKindConfig
	prettierKindTOML
	prettierKindUnsupported
)

// prettierCandidates lists Prettier's config files, highest precedence first.
// An unsupported form that exists first makes detection adopt nothing, rather
// than falling through to a lower-priority file Prettier itself would ignore.
var prettierCandidates = []struct {
	name string
	kind int
}{
	{"package.json", prettierKindPackageJSON},
	{"package.yaml", prettierKindPackageYAML},
	{".prettierrc", prettierKindConfig},
	{".prettierrc.json", prettierKindConfig},
	{".prettierrc.yml", prettierKindConfig},
	{".prettierrc.yaml", prettierKindConfig},
	{".prettierrc.json5", prettierKindUnsupported},
	{".prettierrc.js", prettierKindUnsupported},
	{"prettier.config.js", prettierKindUnsupported},
	{".prettierrc.ts", prettierKindUnsupported},
	{"prettier.config.ts", prettierKindUnsupported},
	{".prettierrc.mjs", prettierKindUnsupported},
	{"prettier.config.mjs", prettierKindUnsupported},
	{".prettierrc.mts", prettierKindUnsupported},
	{"prettier.config.mts", prettierKindUnsupported},
	{".prettierrc.cjs", prettierKindUnsupported},
	{"prettier.config.cjs", prettierKindUnsupported},
	{".prettierrc.cts", prettierKindUnsupported},
	{"prettier.config.cts", prettierKindUnsupported},
	{".prettierrc.toml", prettierKindTOML},
}

var prettierCandidateNames = func() []string {
	names := make([]string, len(prettierCandidates))
	for i, candidate := range prettierCandidates {
		names[i] = candidate.name
	}

	return names
}()

// PrettierResolver finds a file's Prettier config as Prettier does: the
// nearest one, used whole. It is safe for concurrent use.
type PrettierResolver struct {
	base    *Prettier
	baseDir string
	stopAt  string
	memo    *probeMemo

	// One theme per Prettier config, so its files build it once.
	themes sync.Map
}

type tailwindTheme struct {
	enabled bool
	theme   *tailwind.Theme
	err     error
}

func (r *PrettierResolver) tailwindFor(prettier *Prettier, ctx Context) (bool, *tailwind.Theme, error) {
	key := ""
	if prettier != nil {
		key = prettier.Path
	}

	if cached, ok := r.themes.Load(key); ok {
		entry, _ := cached.(tailwindTheme)

		return entry.enabled, entry.theme, entry.err
	}

	ctx.Prettier = prettier

	var scratch config.File

	err := resolveTailwind(&scratch, ctx, "")
	entry := tailwindTheme{enabled: scratch.Tailwind, theme: scratch.TailwindTheme, err: err}

	stored, _ := r.themes.LoadOrStore(key, entry)
	entry, _ = stored.(tailwindTheme)

	return entry.enabled, entry.theme, entry.err
}

func newPrettierResolver(base *Prettier, baseDir, stopAt string, memo *probeMemo) *PrettierResolver {
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}

	return &PrettierResolver{base: base, baseDir: baseDir, stopAt: stopAt, memo: memo}
}

// ForFile returns the Prettier config for absPath, or nil.
func (r *PrettierResolver) ForFile(absPath string) (*Prettier, error) {
	var found *Prettier

	reachedBase := false

	err := upward.Walk(filepath.Dir(absPath), func(dir string) (bool, error) {
		if dir == r.baseDir {
			reachedBase = true

			return true, nil
		}

		value, ok, err := probeDir(r.memo, probePrettier, dir, detectPrettier)
		if err != nil {
			return false, err
		}

		if ok {
			found = &value

			return true, nil
		}

		return r.stopAt != "" && dir == r.stopAt, nil
	})
	if err != nil {
		return nil, err
	}

	if found == nil && reachedBase {
		return r.base, nil
	}

	return found, nil
}

// Prettier is the part of a Prettier config laravel-fmt applies.
type Prettier struct {
	Path      string
	Options   prettierOptions
	Overrides []prettierOverride
	Plugins   []string
	Unread    bool
}

// Prettier 3 loads only the plugins its config lists, so without one its
// options say nothing about a language it does not format on its own.
var prettierPlugins = map[formatter.Language][]string{
	formatter.LangPHP:   {"@prettier/plugin-php"},
	formatter.LangBlade: {"prettier-plugin-blade", "@shufo/prettier-plugin-blade"},
	formatter.LangXML:   {"@prettier/plugin-xml"},
}

// A plugin may be listed by package name or by a path into the package.
func (p *Prettier) formats(lang formatter.Language) bool {
	needed, needsPlugin := prettierPlugins[lang]
	if !needsPlugin {
		return true
	}

	return slices.ContainsFunc(p.Plugins, func(listed string) bool {
		listed = filepath.ToSlash(listed)

		return slices.ContainsFunc(needed, func(name string) bool { return strings.Contains(listed, name) })
	})
}

type prettierOptions struct {
	PrintWidth  *int    `yaml:"printWidth"`
	TabWidth    *int    `yaml:"tabWidth"`
	UseTabs     *bool   `yaml:"useTabs"`
	EndOfLine   *string `yaml:"endOfLine"`
	SingleQuote *bool   `yaml:"singleQuote"`
	ProseWrap   *string `yaml:"proseWrap"`
	// TailwindStylesheet belongs to prettier-plugin-tailwindcss, not Prettier.
	TailwindStylesheet *string `yaml:"tailwindStylesheet"`
}

type prettierOverride struct {
	Files        stringOrSlice   `yaml:"files"`
	ExcludeFiles stringOrSlice   `yaml:"excludeFiles"`
	Options      prettierOptions `yaml:"options"`
}

// stringOrSlice is Prettier's overrides[].files: one glob or many.
type stringOrSlice []string

func (s *stringOrSlice) UnmarshalYAML(unmarshal func(any) error) error {
	var single string

	if err := unmarshal(&single); err == nil {
		*s = stringOrSlice{single}

		return nil
	}

	var many []string

	if err := unmarshal(&many); err != nil {
		return err
	}

	*s = many

	return nil
}

func detectPrettier(root string) (Prettier, bool, error) {
	present, err := dirNames(root, prettierCandidateNames)
	if err != nil {
		return Prettier{}, false, err
	}

	for _, candidate := range prettierCandidates {
		if !present[nameKey(candidate.name)] {
			continue
		}

		path := filepath.Join(root, candidate.name)

		info, err := os.Stat(path)
		if err != nil {
			if upward.Absent(err) {
				continue
			}

			return Prettier{}, false, fmt.Errorf("stat %s: %w", path, err)
		}

		if info.IsDir() {
			continue
		}

		switch candidate.kind {
		case prettierKindPackageJSON, prettierKindPackageYAML:
			prettier, found, err := loadPrettierFromPackage(path, candidate.kind)
			if err != nil {
				return Prettier{}, false, err
			}

			if !found {
				continue
			}

			return prettier, true, nil
		case prettierKindConfig, prettierKindTOML:
			return loadPrettierConfig(path, candidate.kind)
		default:
			// Prettier uses this file, which laravel-fmt cannot evaluate. It
			// counts as found with no options, so the search stops here as
			// Prettier's does.
			return Prettier{Path: path, Unread: true}, true, nil
		}
	}

	return Prettier{}, false, nil
}

// One listing per directory is cheaper than a stat per candidate name, which
// added 7% to a monorepo run. A directory that can be entered but not listed,
// like a 0711 home folder, falls back to the stat per candidate.
func dirNames(dir string, candidates []string) (map[string]bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		switch {
		case upward.Absent(err):
			return map[string]bool{}, nil
		case errors.Is(err, os.ErrPermission):
			return statNames(dir, candidates)
		}

		return nil, fmt.Errorf("open %s: %w", dir, err)
	}

	names, err := f.Readdirnames(-1)
	if closeErr := f.Close(); err == nil && closeErr != nil {
		err = closeErr
	}

	// A Windows ACL can allow opening the folder and deny reading it.
	if errors.Is(err, os.ErrPermission) {
		return statNames(dir, candidates)
	}

	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}

	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[nameKey(name)] = true
	}

	return set, nil
}

// Lstat, not Stat, so a dangling symlink counts as present, as a listing
// reports it.
func statNames(dir string, candidates []string) (map[string]bool, error) {
	set := make(map[string]bool, len(candidates))

	for _, name := range candidates {
		path := filepath.Join(dir, name)

		_, err := os.Lstat(path)
		if err == nil {
			set[nameKey(name)] = true

			continue
		}

		if !upward.Absent(err) {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
	}

	return set, nil
}

// nameKey folds case where the platform's default filesystem does, so a listed
// name matches exactly the files an os.Stat of the candidate would have found.
func nameKey(name string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(name)
	}

	return name
}

func loadPrettierConfig(path string, kind int) (Prettier, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Prettier{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	if kind == prettierKindTOML {
		raw, err = prettierTOMLAsJSON(path, raw)
		if err != nil {
			return Prettier{}, false, err
		}
	}

	prettier, err := parsePrettierConfig(path, raw)
	if err != nil {
		return Prettier{}, false, err
	}

	return prettier, true, nil
}

func prettierTOMLAsJSON(path string, raw []byte) ([]byte, error) {
	var parsed map[string]any

	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	encoded, err := json.Marshal(parsed)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return encoded, nil
}

func loadPrettierFromPackage(path string, kind int) (Prettier, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Prettier{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	var pkg struct {
		Prettier any `json:"prettier" yaml:"prettier"`
	}

	if kind == prettierKindPackageYAML {
		err = yaml.Unmarshal(raw, &pkg)
	} else {
		err = json.Unmarshal(raw, &pkg)
	}

	if err != nil {
		return Prettier{}, false, nil //nolint:nilerr // Prettier skips a manifest it cannot parse; formatting the file reports it
	}

	switch pkg.Prettier.(type) {
	case nil:
		return Prettier{}, false, nil
	case string:
		// A shared config Prettier resolves as a Node module, which laravel-fmt
		// cannot. Prettier stops here, so the search does too.
		return Prettier{Path: path, Unread: true}, true, nil
	}

	encoded, err := json.Marshal(pkg.Prettier)
	if err != nil {
		return Prettier{}, false, fmt.Errorf("parse %s: %w", path, err)
	}

	prettier, err := parsePrettierConfig(path, encoded)
	if err != nil {
		return Prettier{}, false, err
	}

	return prettier, true, nil
}

// Options and overrides are decoded in two passes because goccy cannot set an
// unexported embedded struct (JSON parses fine, being a YAML subset).
func parsePrettierConfig(path string, raw []byte) (Prettier, error) {
	var options prettierOptions

	if err := yaml.Unmarshal(raw, &options); err != nil {
		return Prettier{}, fmt.Errorf("parse %s: %w", path, err)
	}

	var wrapper struct {
		Overrides []prettierOverride `yaml:"overrides"`
		Plugins   []string           `yaml:"plugins"`
	}

	if err := yaml.Unmarshal(raw, &wrapper); err != nil {
		return Prettier{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return Prettier{
		Path:      path,
		Options:   options,
		Overrides: wrapper.Overrides,
		Plugins:   wrapper.Plugins,
	}, nil
}

// ResolveForFile merges every override matching rel over the top-level
// options, last match winning. Globs use the EditorConfig matcher.
func (p *Prettier) ResolveForFile(rel string) prettierOptions {
	rel = filepath.ToSlash(rel)
	merged := p.Options

	for _, override := range p.Overrides {
		if prettierFilesMatch(override.Files, rel) && !prettierFilesMatch(override.ExcludeFiles, rel) {
			mergePrettierOptions(&merged, override.Options)
		}
	}

	return merged
}

func prettierFilesMatch(patterns stringOrSlice, rel string) bool {
	for _, pattern := range patterns {
		if editorConfigGlobMatches(pattern, rel) {
			return true
		}
	}

	return false
}

func mergePrettierOptions(base *prettierOptions, overlay prettierOptions) {
	overlayPtr(&base.PrintWidth, overlay.PrintWidth)
	overlayPtr(&base.TabWidth, overlay.TabWidth)
	overlayPtr(&base.UseTabs, overlay.UseTabs)
	overlayPtr(&base.EndOfLine, overlay.EndOfLine)
	overlayPtr(&base.SingleQuote, overlay.SingleQuote)
	overlayPtr(&base.ProseWrap, overlay.ProseWrap)
}

func applyPrettier(cfg *config.File, p *Prettier) {
	applyPrettierOptions(cfg, p.Options, nil)
}

func applyPrettierOptions(cfg *config.File, o prettierOptions, userKeys map[string]bool) {
	if o.PrintWidth != nil && *o.PrintWidth > 0 && !userKeys["printWidth"] {
		cfg.PrintWidth = *o.PrintWidth
	}

	if o.TabWidth != nil && *o.TabWidth > 0 && !userKeys["indentWidth"] {
		cfg.IndentWidth = *o.TabWidth
		cfg.IndentExplicit = true
	}

	if o.UseTabs != nil && !userKeys["indentStyle"] {
		cfg.IndentStyle = config.IndentStyleFor(*o.UseTabs)
	}

	if o.EndOfLine != nil && !userKeys["endOfLine"] {
		applyPrettierEndOfLine(cfg, *o.EndOfLine)
	}

	if o.SingleQuote != nil && !userKeys["quoteStyle"] {
		applyPrettierQuoteStyle(cfg, *o.SingleQuote)
	}

	if o.ProseWrap != nil && !userKeys["markdown.proseWrap"] {
		applyPrettierProseWrap(cfg, *o.ProseWrap)
	}
}

// Prettier's "cr" has no laravel-fmt counterpart and is left unmapped.
func applyPrettierEndOfLine(cfg *config.File, value string) {
	switch value {
	case "lf":
		cfg.EndOfLine = config.EndOfLineLF
	case "crlf":
		cfg.EndOfLine = config.EndOfLineCRLF
	case "auto":
		cfg.EndOfLine = config.EndOfLineAuto
	}
}

func applyPrettierQuoteStyle(cfg *config.File, single bool) {
	if single {
		cfg.QuoteStyle = config.QuoteStyleSingle
	} else {
		cfg.QuoteStyle = config.QuoteStyleDouble
	}
}

func applyPrettierProseWrap(cfg *config.File, value string) {
	switch value {
	case config.ProseWrapAlways, config.ProseWrapNever, config.ProseWrapPreserve:
		cfg.Markdown.ProseWrap = value
	}
}
