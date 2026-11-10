package detector

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/pelletier/go-toml/v2"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
	"github.com/JacobJoergensen/laravel-fmt/internal/walk"
)

const (
	prettierKindPackageJSON = iota
	prettierKindPackageYAML
	prettierKindConfig
	prettierKindJSON
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
	{".prettierrc.json", prettierKindJSON},
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
	options   prettierOptions
	overrides []prettierOverride
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
	PrintWidth  *int
	TabWidth    *int
	UseTabs     *bool
	EndOfLine   *string
	SingleQuote *bool
	ProseWrap   *string
	// TailwindStylesheet belongs to prettier-plugin-tailwindcss, not Prettier.
	TailwindStylesheet *string
}

type prettierOverride struct {
	Files        stringOrSlice
	ExcludeFiles stringOrSlice
	Options      prettierOptions
}

// stringOrSlice is Prettier's overrides[].files: one glob or many.
type stringOrSlice []string

func detectPrettier(root string) (Prettier, bool, error) {
	present, err := dirNames(root, prettierCandidateNames)
	if err != nil {
		return Prettier{}, false, err
	}

	for _, candidate := range prettierCandidates {
		if !present[nameKey(candidate.name)] {
			continue
		}

		configPath := filepath.Join(root, candidate.name)

		info, err := os.Stat(configPath)
		if err != nil {
			if upward.Absent(err) {
				continue
			}

			return Prettier{}, false, fmt.Errorf("detect Prettier: %w", err)
		}

		if info.IsDir() {
			continue
		}

		switch candidate.kind {
		case prettierKindPackageJSON, prettierKindPackageYAML:
			prettier, found, err := loadPrettierFromPackage(configPath, candidate.kind)
			if err != nil {
				return Prettier{}, false, err
			}

			if !found {
				continue
			}

			return prettier, true, nil
		case prettierKindConfig, prettierKindJSON, prettierKindTOML:
			return loadPrettierConfig(configPath, candidate.kind)
		default:
			// Prettier uses this file, which laravel-fmt cannot evaluate. It
			// counts as found with no options, so the search stops here as
			// Prettier's does.
			return Prettier{Path: configPath, Unread: true}, true, nil
		}
	}

	return Prettier{}, false, nil
}

func loadPrettierConfig(path string, kind int) (Prettier, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Prettier{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	value, err := decodePrettierConfig(path, raw, kind)
	if err != nil {
		return Prettier{}, false, err
	}

	prettier, err := prettierFromValue(path, value)
	if err != nil {
		return Prettier{}, false, err
	}

	return prettier, true, nil
}

// Node keeps a byte order mark, which the YAML parser accepts and JSON.parse
// refuses.
func decodePrettierConfig(path string, raw []byte, kind int) (any, error) {
	var value any

	switch kind {
	case prettierKindJSON:
		kept, err := lastKeyWins(raw)
		if err != nil {
			return nil, config.ParseJSONError(path, raw, err)
		}

		if err := json.Unmarshal(kept, &value); err != nil {
			return nil, config.ParseJSONError(path, kept, err)
		}
	case prettierKindTOML:
		var table map[string]any

		if err := toml.Unmarshal(raw, &table); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}

		value = table
	default:
		if err := yaml.Unmarshal(bytes.TrimPrefix(raw, []byte(walk.UTF8BOM)), &value); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}

	return value, nil
}

// Prettier passes over a manifest whose "prettier" value is falsy.
func loadPrettierFromPackage(path string, kind int) (Prettier, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Prettier{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	var pkg struct {
		Prettier any `json:"prettier" yaml:"prettier"`
	}

	if kind == prettierKindPackageYAML {
		err = yaml.Unmarshal(bytes.TrimPrefix(raw, []byte(walk.UTF8BOM)), &pkg)
	} else {
		// Node decodes the file as UTF-8 before parsing it, replacing bytes that
		// do not decode instead of failing.
		lenient := jsontext.AllowInvalidUTF8(true)

		var kept []byte
		if kept, err = lastKeyWins(raw, lenient); err == nil {
			err = json.Unmarshal(kept, &pkg, lenient)
		}
	}

	if err != nil || isFalsy(pkg.Prettier) {
		return Prettier{}, false, nil //nolint:nilerr // Prettier skips a manifest it cannot parse; formatting the file reports it
	}

	prettier, err := prettierFromValue(path, pkg.Prettier)
	if err != nil {
		return Prettier{}, false, err
	}

	return prettier, true, nil
}

func isFalsy(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case bool:
		return !v
	case string:
		return v == ""
	}

	n, isNumber := configNumber(value)

	return isNumber && n == 0
}

// A string is a shared config, which Prettier loads as a Node module and
// laravel-fmt cannot; Prettier stops at it, so the search does too.
func prettierFromValue(path string, value any) (Prettier, error) {
	switch v := value.(type) {
	case nil:
		return Prettier{Path: path}, nil
	case string:
		return Prettier{Path: path, Unread: true}, nil
	case map[string]any:
		return buildPrettier(path, v)
	}

	return Prettier{}, fmt.Errorf("%s: the config must be an object, not %s", path, describeConfigValue(value))
}

func buildPrettier(path string, m map[string]any) (Prettier, error) {
	options, err := prettierOptionsFrom(path, "", m)
	if err != nil {
		return Prettier{}, err
	}

	overrides, err := prettierOverridesFrom(path, m["overrides"])
	if err != nil {
		return Prettier{}, err
	}

	var plugins []string

	if list, ok := m["plugins"].([]any); ok {
		for _, entry := range list {
			if name, ok := entry.(string); ok {
				plugins = append(plugins, name)
			}
		}
	}

	return Prettier{Path: path, options: options, overrides: overrides, Plugins: plugins}, nil
}

func prettierOverridesFrom(path string, value any) ([]prettierOverride, error) {
	if value == nil {
		return nil, nil
	}

	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: overrides must be a list, not %s", path, describeConfigValue(value))
	}

	overrides := make([]prettierOverride, 0, len(list))

	for i, entry := range list {
		key := fmt.Sprintf("overrides[%d]", i)

		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: %s must be an object, not %s", path, key, describeConfigValue(entry))
		}

		override, err := prettierOverrideFrom(path, key, m)
		if err != nil {
			return nil, err
		}

		overrides = append(overrides, override)
	}

	return overrides, nil
}

func prettierOverrideFrom(path, key string, m map[string]any) (prettierOverride, error) {
	files, err := prettierGlobs(path, key+".files", m["files"])
	if err != nil {
		return prettierOverride{}, err
	}

	excludes, err := prettierGlobs(path, key+".excludeFiles", m["excludeFiles"])
	if err != nil {
		return prettierOverride{}, err
	}

	override := prettierOverride{Files: files, ExcludeFiles: excludes}

	raw := m["options"]
	if raw == nil {
		return override, nil
	}

	table, ok := raw.(map[string]any)
	if !ok {
		return prettierOverride{}, fmt.Errorf("%s: %s.options must be an object, not %s", path, key, describeConfigValue(raw))
	}

	override.Options, err = prettierOptionsFrom(path, key+".options.", table)

	return override, err
}

func prettierGlobs(path, key string, value any) (stringOrSlice, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		return stringOrSlice{v}, nil
	case []any:
		globs := make(stringOrSlice, 0, len(v))

		for _, entry := range v {
			glob, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("%s: %s must hold strings, not %s", path, key, describeConfigValue(entry))
			}

			globs = append(globs, glob)
		}

		return globs, nil
	}

	return nil, fmt.Errorf("%s: %s must be a string or a list, not %s", path, key, describeConfigValue(value))
}

// The options are checked as Prettier's own option schema checks them, so a
// value Prettier refuses is an error here too rather than quietly ignored.
func prettierOptionsFrom(path, prefix string, m map[string]any) (prettierOptions, error) {
	var o prettierOptions

	err := errors.Join(
		prettierInt(path, prefix, m, "printWidth", &o.PrintWidth),
		prettierInt(path, prefix, m, "tabWidth", &o.TabWidth),
		prettierBool(path, prefix, m, "useTabs", &o.UseTabs),
		prettierBool(path, prefix, m, "singleQuote", &o.SingleQuote),
		prettierChoice(path, prefix, m, "endOfLine", &o.EndOfLine, "lf", "crlf", "cr", "auto"),
		prettierChoice(path, prefix, m, "proseWrap", &o.ProseWrap,
			config.ProseWrapAlways, config.ProseWrapNever, config.ProseWrapPreserve),
	)

	if stylesheet, ok := m["tailwindStylesheet"].(string); ok {
		o.TailwindStylesheet = &stylesheet
	}

	return o, err
}

func prettierInt(path, prefix string, m map[string]any, key string, dst **int) error {
	value, present := m[key]
	if !present {
		return nil
	}

	n, isNumber := configNumber(value)
	if !isNumber || n != math.Trunc(n) || math.Abs(n) > math.MaxInt32 {
		return fmt.Errorf("%s: %s%s must be an integer, not %s", path, prefix, key, describeConfigValue(value))
	}

	*dst = new(int(n))

	return nil
}

func prettierBool(path, prefix string, m map[string]any, key string, dst **bool) error {
	value, present := m[key]
	if !present {
		return nil
	}

	b, ok := value.(bool)
	if !ok {
		return fmt.Errorf("%s: %s%s must be true or false, not %s", path, prefix, key, describeConfigValue(value))
	}

	*dst = &b

	return nil
}

func prettierChoice(path, prefix string, m map[string]any, key string, dst **string, choices ...string) error {
	value, present := m[key]
	if !present {
		return nil
	}

	s, ok := value.(string)
	if !ok || !slices.Contains(choices, s) {
		return fmt.Errorf("%s: %s%s must be one of %s, not %s",
			path, prefix, key, strings.Join(choices, ", "), describeConfigValue(value))
	}

	*dst = &s

	return nil
}

// resolveForFile merges every override matching rel over the top-level
// options, last match winning. Globs use the EditorConfig matcher.
func (p *Prettier) resolveForFile(rel string) prettierOptions {
	rel = filepath.ToSlash(rel)
	merged := p.options

	for _, override := range p.overrides {
		if prettierOverrideMatches(rel, override.Files, override.ExcludeFiles) {
			mergePrettierOptions(&merged, override.Options)
		}
	}

	return merged
}

// Prettier's pathMatchesGlobs: patterns without a slash match the base name,
// patterns with one the path, and excludeFiles is read the same way as the
// group it filters.
func prettierOverrideMatches(rel string, files, excludes []string) bool {
	for _, baseName := range []bool{true, false} {
		matches := func(pattern string) bool { return micromatchGlob(pattern, rel, baseName) }

		inGroup := func(pattern string) bool {
			return strings.Contains(pattern, "/") != baseName && matches(pattern)
		}

		if slices.ContainsFunc(files, inGroup) && !slices.ContainsFunc(excludes, matches) {
			return true
		}
	}

	return false
}

func micromatchGlob(pattern, rel string, baseName bool) bool {
	if pattern == rel {
		return true
	}

	pattern = strings.TrimPrefix(pattern, "./")

	negated := strings.HasPrefix(pattern, "!") && !strings.HasPrefix(pattern, "!(")
	if negated {
		pattern = pattern[1:]
	}

	subject := rel
	if baseName {
		subject = path.Base(rel)
	}

	return matchEditorConfigGlob(pattern, subject) != negated
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
	applyPrettierOptions(cfg, p.options, nil)
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
