package detector

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// Mago is the part of mago.toml's [formatter] table laravel-fmt applies.
type Mago struct {
	Path   string `toml:"-"`
	Preset string `toml:"preset"`

	PrintWidth *int    `toml:"print-width"`
	TabWidth   *int    `toml:"tab-width"`
	UseTabs    *bool   `toml:"use-tabs"`
	EndOfLine  *string `toml:"end-of-line"`

	RemoveTrailingCloseTag *bool `toml:"remove-trailing-close-tag"`

	FunctionBraceStyle  *string `toml:"function-brace-style"`
	MethodBraceStyle    *string `toml:"method-brace-style"`
	ClasslikeBraceStyle *string `toml:"classlike-brace-style"`
	ControlBraceStyle   *string `toml:"control-brace-style"`

	ConcatSpace    *bool `toml:"space-around-concatenation-binary-operator"`
	ClosureFnSpace *bool `toml:"space-before-arrow-function-parameter-list-parenthesis"`

	BlankLineBeforeReturn *bool `toml:"empty-line-before-return"`
	IndentHeredoc         *bool `toml:"indent-heredoc"`

	SortUses        *bool `toml:"sort-uses"`
	ExpandUseGroups *bool `toml:"expand-use-groups"`
}

type magoFile struct {
	Formatter *Mago `toml:"formatter"`
}

// A mago.toml without a [formatter] table only configures linting or
// analysis, so it counts as not found.
func detectMago(root string) (Mago, bool, error) {
	path := filepath.Join(root, "mago.toml")

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Mago{}, false, nil
		}

		return Mago{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	var parsed magoFile

	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return Mago{}, false, fmt.Errorf("parse %s: %w", path, err)
	}

	if parsed.Formatter == nil {
		return Mago{}, false, nil
	}

	parsed.Formatter.Path = path

	return *parsed.Formatter, true, nil
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
		cfg.TabWidth = *effective.TabWidth
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
