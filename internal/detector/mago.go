package detector

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pelletier/go-toml/v2"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

const (
	magoBraceSameLine       = "same-line"
	magoBraceNextLine       = "next-line"
	magoBraceAlwaysNextLine = "always-next-line"
)

const (
	magoEOLAuto = "auto"
	magoEOLLf   = "lf"
	magoEOLCrlf = "crlf"
)

const (
	magoPresetPsr12   = "psr-12"
	magoPresetLaravel = "laravel"
	magoPresetDrupal  = "drupal"
)

// Mago is the part of a Mago config's [formatter] table laravel-fmt applies.
type Mago struct {
	Path   string `json:"-"`
	Preset string `json:"preset"`

	PrintWidth *int    `json:"print-width"`
	TabWidth   *int    `json:"tab-width"`
	UseTabs    *bool   `json:"use-tabs"`
	EndOfLine  *string `json:"end-of-line"`

	RemoveTrailingCloseTag *bool `json:"remove-trailing-close-tag"`

	FunctionBraceStyle  *string `json:"function-brace-style"`
	MethodBraceStyle    *string `json:"method-brace-style"`
	ClasslikeBraceStyle *string `json:"classlike-brace-style"`
	ControlBraceStyle   *string `json:"control-brace-style"`

	ConcatSpace    *bool `json:"space-around-concatenation-binary-operator"`
	ClosureFnSpace *bool `json:"space-before-arrow-function-parameter-list-parenthesis"`

	BlankLineBeforeReturn *bool `json:"empty-line-before-return"`
	IndentHeredoc         *bool `json:"indent-heredoc"`

	SortUses        *bool `json:"sort-uses"`
	ExpandUseGroups *bool `json:"expand-use-groups"`
}

var (
	magoNames        = []string{"mago.toml", "mago.yaml", "mago.yml", "mago.json"}
	magoProjectNames = []string{
		"mago.toml", "mago.yaml", "mago.yml", "mago.json",
		"mago.dist.toml", "mago.dist.yaml", "mago.dist.yml", "mago.dist.json",
	}
)

// A Mago config whose extends chain has no [formatter] table only configures
// linting or analysis, so it counts as not found.
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
		return Mago{}, false, nil
	}

	encoded, err := json.Marshal(formatter)
	if err != nil {
		return Mago{}, false, fmt.Errorf("encode %s formatter: %w", path, err)
	}

	var mago Mago

	if err := json.Unmarshal(encoded, &mago); err != nil {
		return Mago{}, false, fmt.Errorf("parse %s formatter: %w", path, err)
	}

	mago.Path = path

	return mago, true, nil
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
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, config.ParseJSONError(path, raw, err)
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

// An unknown preset gets Mago's defaults, since Mago has no unformatted state.
// Snapshot of Mago's crates/formatter/src/presets.rs.
func presetMago(name string) Mago {
	m := magoDefaultPreset()

	switch name {
	case magoPresetPsr12:
		m.EndOfLine = new(magoEOLLf)
		m.SortUses = new(false)
		m.ExpandUseGroups = new(false)
	case magoPresetLaravel:
		m.EndOfLine = new(magoEOLLf)
		m.ConcatSpace = new(false)
		m.ClosureFnSpace = new(true)
		m.BlankLineBeforeReturn = new(true)
	case magoPresetDrupal:
		m.EndOfLine = new(magoEOLLf)
		m.TabWidth = new(2)
		m.FunctionBraceStyle = new(magoBraceSameLine)
		m.MethodBraceStyle = new(magoBraceSameLine)
		m.ClasslikeBraceStyle = new(magoBraceSameLine)
		m.ClosureFnSpace = new(true)
	}

	return m
}

// Mago's FormatSettings defaults, from crates/formatter/src/settings.rs.
func magoDefaultPreset() Mago {
	return Mago{
		PrintWidth:             new(120),
		TabWidth:               new(4),
		UseTabs:                new(false),
		EndOfLine:              new(magoEOLAuto),
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
	}
}

// presetMago sets every modeled option, so detecting Mago moves PHP output to
// Mago's whole style, not only the keys mago.toml writes.
func applyMago(cfg *config.File, m *Mago) {
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
}

func applyMagoEndOfLine(cfg *config.File, value *string) {
	if value == nil {
		return
	}

	switch *value {
	case magoEOLLf:
		cfg.EndOfLine = config.EndOfLineLF
	case magoEOLCrlf:
		cfg.EndOfLine = config.EndOfLineCRLF
	case magoEOLAuto:
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
