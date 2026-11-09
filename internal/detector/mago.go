package detector

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pelletier/go-toml/v2"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

const (
	magoBraceSameLine       = "same_line"
	magoBraceNextLine       = "next_line"
	magoBraceAlwaysNextLine = "always_next_line"
)

const (
	magoEndOfLineAuto = "auto"
	magoEndOfLineLF   = "lf"
	magoEndOfLineCRLF = "crlf"
	magoEndOfLineCR   = "cr"
)

const (
	magoPresetDefault = "default"
	magoPresetPsr12   = "psr-12"
	magoPresetPint    = "pint"
	magoPresetTempest = "tempest"
	magoPresetHack    = "hack"
	magoPresetDrupal  = "drupal"
)

// Mago 1.59.0's spellings, from crates/formatter/src/settings.rs and
// presets.rs. A preset name is matched lowercased; the others are not.
var (
	magoPresetNames = map[string]string{
		"default": magoPresetDefault, "per-cs": magoPresetDefault, "percs": magoPresetDefault,
		"psr-12": magoPresetPsr12, "psr12": magoPresetPsr12,
		"pint": magoPresetPint, "laravel-pint": magoPresetPint, "laravel_pint": magoPresetPint, "laravel": magoPresetPint,
		"tempest": magoPresetTempest, "tempest-php": magoPresetTempest, "tempest_php": magoPresetTempest,
		"hack": magoPresetHack, "hackfmt": magoPresetHack, "hhvm": magoPresetHack,
		"drupal": magoPresetDrupal,
	}
	magoBraceStyles = map[string]string{
		magoBraceSameLine: magoBraceSameLine, "SameLine": magoBraceSameLine, "same-line": magoBraceSameLine,
		magoBraceNextLine: magoBraceNextLine, "NextLine": magoBraceNextLine, "next-line": magoBraceNextLine,
		magoBraceAlwaysNextLine: magoBraceAlwaysNextLine, "AlwaysNextLine": magoBraceAlwaysNextLine,
		"always-next-line": magoBraceAlwaysNextLine,
	}
	magoEndOfLines = map[string]string{
		magoEndOfLineAuto: magoEndOfLineAuto, "Auto": magoEndOfLineAuto,
		magoEndOfLineLF: magoEndOfLineLF, "Lf": magoEndOfLineLF,
		magoEndOfLineCRLF: magoEndOfLineCRLF, "Crlf": magoEndOfLineCRLF,
		magoEndOfLineCR: magoEndOfLineCR, "Cr": magoEndOfLineCR,
	}
	// sort-uses takes a bool or a SortOrder. laravel-fmt sorts only ascending,
	// so any other order leaves the imports as Mago wrote them.
	magoSortsAscending = map[string]bool{
		"preserve": false, "as-is": false, "none": false, "keep": false,
		"alphanumeric-ascending": true, "alpha-ascending": true, "ascending": true,
		"alphanumeric-descending": false, "alpha-descending": false, "descending": false,
		"length-ascending": false, "length-descending": false,
	}
)

// Mago is the part of a Mago config's [formatter] table laravel-fmt applies.
type Mago struct {
	Path   string
	Preset string

	// LintOnly marks a config with no [formatter] table: Mago formats with it
	// all the same, but the project formats PHP some other way.
	LintOnly bool

	PrintWidth *int
	TabWidth   *int
	UseTabs    *bool
	EndOfLine  *string

	RemoveTrailingCloseTag *bool

	FunctionBraceStyle  *string
	MethodBraceStyle    *string
	ClasslikeBraceStyle *string
	ControlBraceStyle   *string

	ConcatSpace    *bool
	ClosureFnSpace *bool

	BlankLineBeforeReturn *bool
	IndentHeredoc         *bool

	SortUses        *bool
	ExpandUseGroups *bool

	InlineEmptyFunctionBodies  *bool
	InlineEmptyMethodBodies    *bool
	InlineEmptyClassBodies     *bool
	InlineEmptyAnonClassBodies *bool
	InlineEmptyClosureBodies   *bool
}

var (
	magoNames        = []string{"mago.toml", "mago.yaml", "mago.yml", "mago.json"}
	magoProjectNames = []string{
		"mago.toml", "mago.yaml", "mago.yml", "mago.json",
		"mago.dist.toml", "mago.dist.yaml", "mago.dist.yml", "mago.dist.json",
	}
)

// Mago reads only the nearest config, so one without a [formatter] table still
// ends the search.
func detectMago(root string) (Mago, bool, error) {
	path, err := findMagoFile(root, magoProjectNames)
	if err != nil || path == "" {
		return Mago{}, false, err
	}

	formatter, err := loadMagoFormatter(path, map[string]bool{})
	if err != nil {
		return Mago{}, false, err
	}

	if formatter == nil {
		return Mago{Path: path, LintOnly: true}, true, nil
	}

	mago, err := magoFromTable(path, formatter)
	if err != nil {
		return Mago{}, false, err
	}

	return mago, true, nil
}

// Unknown keys are left alone although Mago refuses them: Mago adds keys
// between releases, and an older list would refuse a newer config.
func magoFromTable(path string, table map[string]any) (Mago, error) {
	mago := Mago{Path: path}

	if value, present := table["preset"]; present {
		name, ok := value.(string)
		preset, known := magoPresetNames[strings.ToLower(name)]

		if !ok || !known {
			return Mago{}, fmt.Errorf("%s: formatter.preset must be one of default, psr-12, pint, tempest, hack or drupal, not %s",
				path, describeConfigValue(value))
		}

		mago.Preset = preset
	}

	err := errors.Join(
		magoWidth(path, table, "print-width", &mago.PrintWidth),
		magoWidth(path, table, "tab-width", &mago.TabWidth),
		magoBool(path, table, "use-tabs", &mago.UseTabs),
		magoSpelling(path, table, "end-of-line", magoEndOfLines, &mago.EndOfLine),
		magoBool(path, table, "remove-trailing-close-tag", &mago.RemoveTrailingCloseTag),
		magoSpelling(path, table, "function-brace-style", magoBraceStyles, &mago.FunctionBraceStyle),
		magoSpelling(path, table, "method-brace-style", magoBraceStyles, &mago.MethodBraceStyle),
		magoSpelling(path, table, "classlike-brace-style", magoBraceStyles, &mago.ClasslikeBraceStyle),
		magoSpelling(path, table, "control-brace-style", magoBraceStyles, &mago.ControlBraceStyle),
		magoBool(path, table, "space-around-concatenation-binary-operator", &mago.ConcatSpace),
		magoBool(path, table, "space-before-arrow-function-parameter-list-parenthesis", &mago.ClosureFnSpace),
		magoBool(path, table, "empty-line-before-return", &mago.BlankLineBeforeReturn),
		magoBool(path, table, "indent-heredoc", &mago.IndentHeredoc),
		magoSortUses(path, table, &mago.SortUses),
		magoBool(path, table, "expand-use-groups", &mago.ExpandUseGroups),
		magoBool(path, table, "inline-empty-function-braces", &mago.InlineEmptyFunctionBodies),
		magoBool(path, table, "inline-empty-method-braces", &mago.InlineEmptyMethodBodies),
		magoBool(path, table, "inline-empty-classlike-braces", &mago.InlineEmptyClassBodies),
		magoBool(path, table, "inline-empty-anonymous-class-braces", &mago.InlineEmptyAnonClassBodies),
		magoBool(path, table, "inline-empty-closure-braces", &mago.InlineEmptyClosureBodies),
	)

	return mago, err
}

// A width is a usize in Mago.
func magoWidth(path string, table map[string]any, key string, dst **int) error {
	value, present := table[key]
	if !present {
		return nil
	}

	n, isNumber := prettierNumber(value)
	if !isNumber || n < 0 || n != math.Trunc(n) || n > math.MaxInt32 {
		return fmt.Errorf("%s: formatter.%s must be a whole number, not %s", path, key, describeConfigValue(value))
	}

	*dst = new(int(n))

	return nil
}

func magoBool(path string, table map[string]any, key string, dst **bool) error {
	value, present := table[key]
	if !present {
		return nil
	}

	b, ok := value.(bool)
	if !ok {
		return fmt.Errorf("%s: formatter.%s must be true or false, not %s", path, key, describeConfigValue(value))
	}

	*dst = &b

	return nil
}

func magoSpelling(path string, table map[string]any, key string, spellings map[string]string, dst **string) error {
	value, present := table[key]
	if !present {
		return nil
	}

	s, _ := value.(string)

	canonical, ok := spellings[s]
	if !ok {
		return fmt.Errorf("%s: formatter.%s cannot be %s", path, key, describeConfigValue(value))
	}

	*dst = &canonical

	return nil
}

func magoSortUses(path string, table map[string]any, dst **bool) error {
	value, present := table["sort-uses"]
	if !present {
		return nil
	}

	switch v := value.(type) {
	case bool:
		*dst = &v

		return nil
	case string:
		if ascending, ok := magoSortsAscending[v]; ok {
			*dst = &ascending

			return nil
		}
	}

	return fmt.Errorf("%s: formatter.sort-uses must be true, false or a sort order, not %s", path, describeConfigValue(value))
}

func findMagoFile(dir string, names []string) (string, error) {
	present, err := dirNames(dir, names)
	if err != nil {
		return "", err
	}

	for _, name := range names {
		if present[nameKey(name)] {
			return filepath.Join(dir, name), nil
		}
	}

	return "", nil
}

// Like Mago's load_layer, and unlike its docs, a file reached twice in one
// load is an error, even through two different parents.
func loadMagoFormatter(path string, visited map[string]bool) (map[string]any, error) {
	canonical, err := canonicalPath(path)
	if err != nil {
		return nil, err
	}

	if visited[canonical] {
		return nil, fmt.Errorf("mago extends cycles back on %s", canonical)
	}

	visited[canonical] = true

	doc, err := readMagoFile(canonical)
	if err != nil {
		return nil, err
	}

	entries, err := magoExtends(canonical, doc["extends"])
	if err != nil {
		return nil, err
	}

	// The folder as written, not the canonical one, as Mago's Path::parent
	// does: a symlinked config extends files next to the link.
	baseDir := strings.TrimSuffix(path, filepath.Base(path))

	var merged map[string]any

	for _, entry := range entries {
		parent, err := resolveMagoExtends(entry, baseDir)
		if err != nil {
			return nil, err
		}

		if parent == "" {
			continue
		}

		formatter, err := loadMagoFormatter(parent, visited)
		if err != nil {
			return nil, err
		}

		merged = mergeMagoTables(merged, formatter)
	}

	own, ok := doc["formatter"].(map[string]any)
	if !ok && doc["formatter"] != nil {
		return nil, fmt.Errorf("parse %s: formatter must be a table", canonical)
	}

	return mergeMagoTables(merged, own), nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}

	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}

	return canonical, nil
}

func readMagoFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var doc map[string]any

	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		err = toml.Unmarshal(raw, &doc)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(raw, &doc)
	case ".json":
		kept, err := lastKeyWins(raw)
		if err != nil {
			return nil, config.ParseJSONError(path, raw, err)
		}

		if err := json.Unmarshal(kept, &doc); err != nil {
			return nil, config.ParseJSONError(path, kept, err)
		}
	default:
		return nil, fmt.Errorf("read %s: a Mago config must be TOML, YAML or JSON", path)
	}

	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return doc, nil
}

func magoExtends(path string, value any) ([]string, error) {
	switch extends := value.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{extends}, nil
	case []any:
		entries := make([]string, 0, len(extends))

		for _, item := range extends {
			entry, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("parse %s: extends entries must be strings", path)
			}

			entries = append(entries, entry)
		}

		return entries, nil
	}

	return nil, fmt.Errorf("parse %s: extends must be a string or a list of strings", path)
}

// An empty result is a folder with no mago file, which Mago skips.
func resolveMagoExtends(entry, baseDir string) (string, error) {
	// Joined without cleaning, as Mago does, so `..` resolves through symlinks.
	target := entry
	if !filepath.IsAbs(target) {
		target = baseDir + target
	}

	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("resolve mago extends %q: %w", entry, err)
	}

	if !info.IsDir() {
		return target, nil
	}

	return findMagoFile(target, magoNames)
}

// Mago deep-merges layers, but every formatter key laravel-fmt reads is a
// scalar, where the deep merge is a plain overwrite.
func mergeMagoTables(base, overlay map[string]any) map[string]any {
	if overlay == nil {
		return base
	}

	if base == nil {
		return overlay
	}

	maps.Copy(base, overlay)

	return base
}

// name is a canonical preset name, or empty for Mago's defaults.
func presetMago(name string) Mago {
	m := magoDefaultPreset()

	switch name {
	case magoPresetPsr12:
		m.EndOfLine = new(magoEndOfLineLF)
		m.SortUses = new(false)
		m.ExpandUseGroups = new(false)
		m.InlineEmptyFunctionBodies = new(false)
		m.InlineEmptyMethodBodies = new(false)
		m.InlineEmptyClassBodies = new(false)
	case magoPresetPint:
		m.EndOfLine = new(magoEndOfLineLF)
		m.ConcatSpace = new(false)
		m.ClosureFnSpace = new(true)
		m.BlankLineBeforeReturn = new(true)
	case magoPresetTempest:
		m.PrintWidth = new(180)
		m.ClosureFnSpace = new(true)
		m.IndentHeredoc = new(false)
		m.InlineEmptyFunctionBodies = new(false)
		m.InlineEmptyMethodBodies = new(false)
	case magoPresetHack:
		m.PrintWidth = new(80)
		m.TabWidth = new(2)
		m.EndOfLine = new(magoEndOfLineLF)
		m.FunctionBraceStyle = new(magoBraceSameLine)
		m.MethodBraceStyle = new(magoBraceSameLine)
		m.ClasslikeBraceStyle = new(magoBraceSameLine)
		m.ClosureFnSpace = new(true)
		m.BlankLineBeforeReturn = new(true)
		m.IndentHeredoc = new(false)
	case magoPresetDrupal:
		m.EndOfLine = new(magoEndOfLineLF)
		m.TabWidth = new(2)
		m.FunctionBraceStyle = new(magoBraceSameLine)
		m.MethodBraceStyle = new(magoBraceSameLine)
		m.ClasslikeBraceStyle = new(magoBraceSameLine)
		m.ClosureFnSpace = new(true)
		m.InlineEmptyFunctionBodies = new(false)
		m.InlineEmptyMethodBodies = new(false)
	}

	return m
}

// Mago's FormatSettings defaults, from crates/formatter/src/settings.rs.
func magoDefaultPreset() Mago {
	return Mago{
		PrintWidth:             new(120),
		TabWidth:               new(4),
		UseTabs:                new(false),
		EndOfLine:              new(magoEndOfLineAuto),
		RemoveTrailingCloseTag: new(true),
		FunctionBraceStyle:     new(magoBraceNextLine),
		MethodBraceStyle:       new(magoBraceNextLine),
		ClasslikeBraceStyle:    new(magoBraceAlwaysNextLine),
		ControlBraceStyle:      new(magoBraceSameLine),
		ConcatSpace:            new(true),
		ClosureFnSpace:         new(false),
		BlankLineBeforeReturn:  new(false),
		IndentHeredoc:          new(true),
		SortUses:               new(true),
		ExpandUseGroups:        new(true),

		InlineEmptyFunctionBodies:  new(true),
		InlineEmptyMethodBodies:    new(true),
		InlineEmptyClassBodies:     new(true),
		InlineEmptyAnonClassBodies: new(true),
		InlineEmptyClosureBodies:   new(true),
	}
}

// presetMago sets every modeled option, so detecting Mago moves PHP output to
// Mago's whole style, not only the keys mago.toml writes.
func applyMago(cfg *config.File, m *Mago) {
	if m.LintOnly {
		return
	}

	effective := presetMago(m.Preset)
	mergeMago(&effective, m)

	if effective.PrintWidth != nil && *effective.PrintWidth > 0 {
		cfg.PrintWidth = *effective.PrintWidth
	}

	if effective.TabWidth != nil && *effective.TabWidth > 0 {
		cfg.IndentWidth = *effective.TabWidth
	}

	if effective.UseTabs != nil {
		cfg.IndentStyle = config.IndentStyleFor(*effective.UseTabs)
	}

	applyMagoEndOfLine(cfg, effective.EndOfLine)

	setIfPresent(&cfg.PHP.RemoveTrailingCloseTag, effective.RemoveTrailingCloseTag)
	setIfPresent(&cfg.PHP.SortUses, effective.SortUses)
	setIfPresent(&cfg.PHP.ExpandUseGroups, effective.ExpandUseGroups)
	setIfPresent(&cfg.PHP.InlineEmptyFunctionBodies, effective.InlineEmptyFunctionBodies)
	setIfPresent(&cfg.PHP.InlineEmptyMethodBodies, effective.InlineEmptyMethodBodies)
	setIfPresent(&cfg.PHP.InlineEmptyClassBodies, effective.InlineEmptyClassBodies)
	setIfPresent(&cfg.PHP.InlineEmptyAnonClassBodies, effective.InlineEmptyAnonClassBodies)
	setIfPresent(&cfg.PHP.InlineEmptyClosureBodies, effective.InlineEmptyClosureBodies)

	// Mago has no setting for it and writes `new class(`.
	cfg.PHP.AnonClassParenSpace = false
	setIfPresent(&cfg.PHP.ConcatSpace, effective.ConcatSpace)
	setIfPresent(&cfg.PHP.ClosureFnSpace, effective.ClosureFnSpace)
	applyMagoBlankLineBeforeReturn(cfg, effective.BlankLineBeforeReturn)

	applyMagoBrace(&cfg.PHP.FunctionBraceStyle, effective.FunctionBraceStyle)
	applyMagoBrace(&cfg.PHP.MethodBraceStyle, effective.MethodBraceStyle)
	applyMagoBrace(&cfg.PHP.ClasslikeBraceStyle, effective.ClasslikeBraceStyle)
	applyMagoBrace(&cfg.PHP.ControlBraceStyle, effective.ControlBraceStyle)

	applyMagoHeredoc(cfg, effective.IndentHeredoc)
}

// Mago's toggle covers return only.
func applyMagoBlankLineBeforeReturn(cfg *config.File, enabled *bool) {
	if enabled == nil {
		return
	}

	if *enabled {
		cfg.PHP.BlankLineBeforeStatements = []string{config.StmtReturn}
	} else {
		cfg.PHP.BlankLineBeforeStatements = nil
	}
}

func mergeMago(base, overlay *Mago) {
	if overlay == nil {
		return
	}

	base.Path = overlay.Path

	overlayPtr(&base.PrintWidth, overlay.PrintWidth)
	overlayPtr(&base.TabWidth, overlay.TabWidth)
	overlayPtr(&base.UseTabs, overlay.UseTabs)
	overlayPtr(&base.EndOfLine, overlay.EndOfLine)
	overlayPtr(&base.RemoveTrailingCloseTag, overlay.RemoveTrailingCloseTag)
	overlayPtr(&base.FunctionBraceStyle, overlay.FunctionBraceStyle)
	overlayPtr(&base.MethodBraceStyle, overlay.MethodBraceStyle)
	overlayPtr(&base.ClasslikeBraceStyle, overlay.ClasslikeBraceStyle)
	overlayPtr(&base.ControlBraceStyle, overlay.ControlBraceStyle)
	overlayPtr(&base.ConcatSpace, overlay.ConcatSpace)
	overlayPtr(&base.ClosureFnSpace, overlay.ClosureFnSpace)
	overlayPtr(&base.BlankLineBeforeReturn, overlay.BlankLineBeforeReturn)
	overlayPtr(&base.IndentHeredoc, overlay.IndentHeredoc)
	overlayPtr(&base.SortUses, overlay.SortUses)
	overlayPtr(&base.ExpandUseGroups, overlay.ExpandUseGroups)
	overlayPtr(&base.InlineEmptyFunctionBodies, overlay.InlineEmptyFunctionBodies)
	overlayPtr(&base.InlineEmptyMethodBodies, overlay.InlineEmptyMethodBodies)
	overlayPtr(&base.InlineEmptyClassBodies, overlay.InlineEmptyClassBodies)
	overlayPtr(&base.InlineEmptyAnonClassBodies, overlay.InlineEmptyAnonClassBodies)
	overlayPtr(&base.InlineEmptyClosureBodies, overlay.InlineEmptyClosureBodies)
}

func applyMagoEndOfLine(cfg *config.File, value *string) {
	if value == nil {
		return
	}

	switch *value {
	case magoEndOfLineLF:
		cfg.EndOfLine = config.EndOfLineLF
	case magoEndOfLineCRLF:
		cfg.EndOfLine = config.EndOfLineCRLF
	case magoEndOfLineAuto:
		cfg.EndOfLine = config.EndOfLineAuto
	}
}

func applyMagoBrace(target, value *string) {
	if value == nil {
		return
	}

	switch *value {
	case magoBraceSameLine:
		*target = config.BraceStyleSameLine
	case magoBraceNextLine:
		*target = config.BraceStyleNextLine
	case magoBraceAlwaysNextLine:
		*target = config.BraceStyleAlwaysNextLine
	}
}

// Mago's boolean indent-heredoc maps to start_plus_one (true) or none (false);
// the middle same_as_start has no Mago equivalent.
func applyMagoHeredoc(cfg *config.File, indent *bool) {
	if indent == nil {
		return
	}

	if *indent {
		cfg.PHP.HeredocIndentation = config.HeredocIndentStartPlusOne
	} else {
		cfg.PHP.HeredocIndentation = config.HeredocIndentNone
	}
}
