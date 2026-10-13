package detector

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func wantBoolPtr(t *testing.T, got *bool, want bool, field string) {
	t.Helper()

	if got == nil {
		t.Fatalf("%s = nil, want %v", field, want)
	} else if *got != want {
		t.Errorf("%s = %v, want %v", field, *got, want)
	}
}

func wantStringPtr(t *testing.T, got *string, want, field string) {
	t.Helper()

	if got == nil {
		t.Fatalf("%s = nil, want %q", field, want)
	} else if *got != want {
		t.Errorf("%s = %q, want %q", field, *got, want)
	}
}

func wantIntPtr(t *testing.T, got *int, want int, field string) {
	t.Helper()

	if got == nil {
		t.Fatalf("%s = nil, want %d", field, want)
	} else if *got != want {
		t.Errorf("%s = %d, want %d", field, *got, want)
	}
}

func TestDetectMagoParsesFormatterTable(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "mago.toml", `[formatter]
preset = "laravel"
print-width = 100
classlike-brace-style = "same-line"
sort-uses = false
indent-heredoc = false
`)

	mago, found, err := detectMago(dir)
	if err != nil {
		t.Fatalf("detectMago: %v", err)
	}

	if !found {
		t.Fatal("detectMago reported not found, want found")
	}

	if mago.Preset != "laravel" {
		t.Errorf("Preset = %q, want %q", mago.Preset, "laravel")
	}

	wantIntPtr(t, mago.PrintWidth, 100, "PrintWidth")
	wantStringPtr(t, mago.ClasslikeBraceStyle, magoBraceSameLine, "ClasslikeBraceStyle")
	wantBoolPtr(t, mago.SortUses, false, "SortUses")
	wantBoolPtr(t, mago.IndentHeredoc, false, "IndentHeredoc")

	if mago.ConcatSpace != nil {
		t.Errorf("ConcatSpace = %v, want nil for an unwritten key", *mago.ConcatSpace)
	}
}

func TestDetectMagoMissingFile(t *testing.T) {
	mago, found, err := detectMago(t.TempDir())
	if err != nil {
		t.Fatalf("detectMago: %v", err)
	}

	if found {
		t.Fatal("detectMago reported found, want not found")
	}

	if mago != (Mago{}) {
		t.Fatalf("Mago = %+v, want zero value", mago)
	}
}

func TestDetectMagoWithoutFormatterTable(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "mago.toml", `[linter]
level = "strict"
`)

	mago, found, err := detectMago(dir)
	if err != nil {
		t.Fatalf("detectMago: %v", err)
	}

	if found {
		t.Fatal("detectMago reported found for a [formatter]-less file, want not found")
	}

	if mago != (Mago{}) {
		t.Fatalf("Mago = %+v, want zero value", mago)
	}
}

func TestDetectMagoMalformedTOMLErrors(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "mago.toml", "[formatter\nprint-width = ")

	if _, _, err := detectMago(dir); err == nil {
		t.Fatal("detectMago returned nil error on malformed TOML, want an error")
	}
}

func TestPresetMagoLaravelDefaults(t *testing.T) {
	preset := presetMago(magoPresetLaravel)

	wantStringPtr(t, preset.EndOfLine, magoEOLLf, "EndOfLine")
	wantBoolPtr(t, preset.ConcatSpace, false, "ConcatSpace")
	wantBoolPtr(t, preset.ClosureFnSpace, true, "ClosureFnSpace")
	wantBoolPtr(t, preset.BlankLineBeforeReturn, true, "BlankLineBeforeReturn")
	wantStringPtr(t, preset.FunctionBraceStyle, magoBraceNextLine, "FunctionBraceStyle")
	wantStringPtr(t, preset.ClasslikeBraceStyle, magoBraceAlwaysNextLine, "ClasslikeBraceStyle")
	wantBoolPtr(t, preset.SortUses, true, "SortUses")
}

func TestPresetMagoUnknownReturnsDefault(t *testing.T) {
	for _, name := range []string{"", "not-a-real-preset"} {
		preset := presetMago(name)

		wantStringPtr(t, preset.EndOfLine, magoEOLAuto, "EndOfLine")
		wantBoolPtr(t, preset.ConcatSpace, true, "ConcatSpace")
		wantBoolPtr(t, preset.SortUses, true, "SortUses")
		wantStringPtr(t, preset.ClasslikeBraceStyle, magoBraceAlwaysNextLine, "ClasslikeBraceStyle")
		wantIntPtr(t, preset.TabWidth, 4, "TabWidth")
	}
}

func TestApplyMagoLaravelPreset(t *testing.T) {
	cfg := config.Default()
	Apply(cfg, Context{Mago: &Mago{Preset: magoPresetLaravel}})

	if cfg.PHP.ConcatSpace {
		t.Error("ConcatSpace = true, want false (laravel: $a.$b)")
	}

	if !cfg.PHP.ClosureFnSpace {
		t.Error("ClosureFnSpace = false, want true (laravel: fn ())")
	}

	if got := cfg.PHP.BlankLineBeforeStatements; len(got) != 1 || got[0] != config.StmtReturn {
		t.Errorf("BlankLineBeforeStatements = %v, want [return] (mago empty-line-before-return)", got)
	}

	if cfg.PHP.FunctionBraceStyle != config.BraceStyleNextLine {
		t.Errorf("FunctionBraceStyle = %q, want %q", cfg.PHP.FunctionBraceStyle, config.BraceStyleNextLine)
	}

	if cfg.PHP.ClasslikeBraceStyle != config.BraceStyleAlwaysNextLine {
		t.Errorf("ClasslikeBraceStyle = %q, want %q", cfg.PHP.ClasslikeBraceStyle, config.BraceStyleAlwaysNextLine)
	}

	if cfg.PHP.ControlBraceStyle != config.BraceStyleSameLine {
		t.Errorf("ControlBraceStyle = %q, want %q", cfg.PHP.ControlBraceStyle, config.BraceStyleSameLine)
	}

	if cfg.PHP.HeredocIndentation != config.HeredocIndentStartPlusOne {
		t.Errorf("HeredocIndentation = %q, want %q", cfg.PHP.HeredocIndentation, config.HeredocIndentStartPlusOne)
	}
}

func TestApplyMagoDrupalPreset(t *testing.T) {
	cfg := config.Default()
	Apply(cfg, Context{Mago: &Mago{Preset: magoPresetDrupal}})

	if cfg.TabWidth != 2 {
		t.Errorf("TabWidth = %d, want 2", cfg.TabWidth)
	}

	for _, tc := range []struct {
		field string
		got   string
	}{
		{"FunctionBraceStyle", cfg.PHP.FunctionBraceStyle},
		{"MethodBraceStyle", cfg.PHP.MethodBraceStyle},
		{"ClasslikeBraceStyle", cfg.PHP.ClasslikeBraceStyle},
		{"ControlBraceStyle", cfg.PHP.ControlBraceStyle},
	} {
		if tc.got != config.BraceStyleSameLine {
			t.Errorf("%s = %q, want %q", tc.field, tc.got, config.BraceStyleSameLine)
		}
	}
}

func TestApplyMagoOverrideBeatsPreset(t *testing.T) {
	cfg := config.Default()
	Apply(cfg, Context{Mago: &Mago{
		Preset:        magoPresetLaravel,
		PrintWidth:    new(80),
		IndentHeredoc: new(false),
	}})

	if cfg.PrintWidth != 80 {
		t.Errorf("PrintWidth = %d, want 80 (explicit override beats preset)", cfg.PrintWidth)
	}

	if cfg.PHP.HeredocIndentation != config.HeredocIndentNone {
		t.Errorf("HeredocIndentation = %q, want %q", cfg.PHP.HeredocIndentation, config.HeredocIndentNone)
	}
}

func TestApplyMagoEndOfLineCrLeavesBaseline(t *testing.T) {
	cfg := config.Default()
	baseline := cfg.EndOfLine

	Apply(cfg, Context{Mago: &Mago{Preset: magoPresetLaravel, EndOfLine: new("cr")}})

	if cfg.EndOfLine != baseline {
		t.Errorf("EndOfLine = %q, want baseline %q for unsupported `cr`", cfg.EndOfLine, baseline)
	}
}

func TestApplyMagoAlwaysNextLineBrace(t *testing.T) {
	cfg := config.Default()
	Apply(cfg, Context{Mago: &Mago{
		Preset:             magoPresetDrupal,
		FunctionBraceStyle: new(magoBraceAlwaysNextLine),
	}})

	if cfg.PHP.FunctionBraceStyle != config.BraceStyleAlwaysNextLine {
		t.Errorf("FunctionBraceStyle = %q, want %q (always-next-line is no longer collapsed)", cfg.PHP.FunctionBraceStyle, config.BraceStyleAlwaysNextLine)
	}
}
