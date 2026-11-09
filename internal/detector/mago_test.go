package detector

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

	if mago.Preset != magoPresetPint {
		t.Errorf("Preset = %q, want laravel read as %q", mago.Preset, magoPresetPint)
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
	seedFileAt(t, dir, "mago.toml", `extends = "base.toml"

[linter]
level = "strict"
`)
	seedFileAt(t, dir, "base.toml", "[analyzer]\n")

	mago, found, err := detectMago(dir)
	if err != nil {
		t.Fatalf("detectMago: %v", err)
	}

	if !found || !mago.LintOnly {
		t.Fatalf("found = %v, LintOnly = %v; want a lint-only config that ends the search", found, mago.LintOnly)
	}

	cfg := config.Default()
	applyMago(cfg, &mago)

	if !reflect.DeepEqual(cfg, config.Default()) {
		t.Fatalf("a lint-only Mago config changed the settings: %+v", cfg.PHP)
	}
}

func TestDetectMagoReadsTheProjectFileAndItsExtends(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  int
	}{
		{
			name: "the file's own key beats its parent's",
			files: map[string]string{
				"mago.toml": "extends = \"base.toml\"\n[formatter]\nprint-width = 100\n",
				"base.toml": "[formatter]\nprint-width = 90\n",
			},
			want: 100,
		},
		{
			name: "a later extends entry beats an earlier one",
			files: map[string]string{
				"mago.toml": `extends = ["a.toml", "b.toml"]`,
				"a.toml":    "[formatter]\nprint-width = 80\n",
				"b.toml":    "[formatter]\nprint-width = 90\n",
			},
			want: 90,
		},
		{
			name: "a parent's extends resolves next to the parent, before its own keys",
			files: map[string]string{
				"mago.toml":        `extends = "shared/base.toml"`,
				"shared/base.toml": "extends = \"root.toml\"\n[formatter]\nprint-width = 90\n",
				"shared/root.toml": "[formatter]\nprint-width = 80\n",
			},
			want: 90,
		},
		{
			name: "a folder entry reads the mago file inside it",
			files: map[string]string{
				"mago.toml":                    `extends = "vendor/acme/config"`,
				"vendor/acme/config/mago.yaml": "formatter:\n  print-width: 90\n",
			},
			want: 90,
		},
		{
			name: "a folder entry without a mago file is skipped",
			files: map[string]string{
				"mago.toml":       `extends = ["empty", "base.json"]`,
				"empty/README.md": "",
				"base.json":       `{"formatter": {"print-width": 90}}`,
			},
			want: 90,
		},
		{
			name: "mago.dist.toml is read when there is no mago file",
			files: map[string]string{
				"mago.dist.toml": "[formatter]\nprint-width = 90\n",
			},
			want: 90,
		},
		{
			name:  "a key written twice in mago.json keeps the last, as serde_json does",
			files: map[string]string{"mago.json": `{"formatter": {"print-width": 80, "print-width": 90}}`},
			want:  90,
		},
		{
			name: "mago.toml beats mago.dist.toml",
			files: map[string]string{
				"mago.toml":      "[formatter]\nprint-width = 90\n",
				"mago.dist.toml": "[formatter]\nprint-width = 80\n",
			},
			want: 90,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, body := range test.files {
				seedFileAt(t, dir, rel, body)
			}

			mago, found, err := detectMago(dir)
			if err != nil {
				t.Fatalf("detectMago: %v", err)
			}

			if !found {
				t.Fatal("detectMago reported not found, want found")
			}

			wantIntPtr(t, mago.PrintWidth, test.want, "PrintWidth")
		})
	}
}

func TestDetectMagoResolvesExtendsNextToASymlinkedConfig(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "shared/mago.toml", `extends = "./base.toml"`)
	seedFileAt(t, dir, "project/base.toml", "[formatter]\nprint-width = 90\n")

	link := filepath.Join(dir, "project", "mago.toml")
	if err := os.Symlink(filepath.Join(dir, "shared", "mago.toml"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	mago, found, err := detectMago(filepath.Join(dir, "project"))
	if err != nil {
		t.Fatalf("detectMago: %v", err)
	}

	if !found {
		t.Fatal("detectMago reported not found, want found")
	}

	wantIntPtr(t, mago.PrintWidth, 90, "PrintWidth")
}

func TestDetectMagoRejectsBrokenExtends(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "missing target",
			files: map[string]string{"mago.toml": `extends = "missing.toml"`},
			want:  `resolve mago extends "missing.toml"`,
		},
		{
			name: "cycle",
			files: map[string]string{
				"mago.toml": `extends = "base.toml"`,
				"base.toml": `extends = "mago.toml"`,
			},
			want: "cycles back on",
		},
		{
			name: "file reached twice",
			files: map[string]string{
				"mago.toml":   `extends = ["a.toml", "b.toml"]`,
				"a.toml":      `extends = "shared.toml"`,
				"b.toml":      `extends = "shared.toml"`,
				"shared.toml": "[formatter]\nprint-width = 90\n",
			},
			want: "cycles back on",
		},
		{
			name: "unsupported extension",
			files: map[string]string{
				"mago.toml": `extends = "base.ini"`,
				"base.ini":  "",
			},
			want: "must be TOML, YAML or JSON",
		},
		{
			name:  "extends is not a string",
			files: map[string]string{"mago.toml": "extends = 3\n"},
			want:  "extends must be a string",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, body := range test.files {
				seedFileAt(t, dir, rel, body)
			}

			_, _, err := detectMago(dir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("detectMago err = %v, want one containing %q", err, test.want)
			}
		})
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
	preset := presetMago(magoPresetPint)

	wantStringPtr(t, preset.EndOfLine, magoEndOfLineLF, "EndOfLine")
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

		wantStringPtr(t, preset.EndOfLine, magoEndOfLineAuto, "EndOfLine")
		wantBoolPtr(t, preset.ConcatSpace, true, "ConcatSpace")
		wantBoolPtr(t, preset.SortUses, true, "SortUses")
		wantStringPtr(t, preset.ClasslikeBraceStyle, magoBraceAlwaysNextLine, "ClasslikeBraceStyle")
		wantIntPtr(t, preset.TabWidth, 4, "TabWidth")
	}
}

func TestApplyMagoLaravelPreset(t *testing.T) {
	cfg := config.Default()
	Apply(cfg, Context{Mago: &Mago{Preset: magoPresetPint}})

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

	if cfg.IndentWidth != 2 {
		t.Errorf("IndentWidth = %d, want 2", cfg.IndentWidth)
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
		Preset:        magoPresetPint,
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

func TestApplyMagoEndOfLineMapsEachValue(t *testing.T) {
	cases := []struct {
		value, from, want string
	}{
		{magoEndOfLineLF, config.EndOfLineCRLF, config.EndOfLineLF},
		{magoEndOfLineCRLF, config.EndOfLineLF, config.EndOfLineCRLF},
		{magoEndOfLineAuto, config.EndOfLineLF, config.EndOfLineAuto},
		{"cr", config.EndOfLineCRLF, config.EndOfLineCRLF},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			cfg := config.Default()
			cfg.EndOfLine = tc.from

			Apply(cfg, Context{Mago: &Mago{Preset: magoPresetPint, EndOfLine: new(tc.value)}})

			if cfg.EndOfLine != tc.want {
				t.Errorf("EndOfLine = %q, want %q", cfg.EndOfLine, tc.want)
			}
		})
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

func TestDetectMagoReadsTheFormatterTableAsMagoDoes(t *testing.T) {
	cases := []struct {
		name    string
		table   string
		wantErr bool
		check   func(config.PHP, *config.File) bool
	}{
		{"a snake_case brace style", `function-brace-style = "same_line"`, false, func(p config.PHP, _ *config.File) bool {
			return p.FunctionBraceStyle == config.BraceStyleSameLine
		}},
		{"a PascalCase end of line", `end-of-line = "Crlf"`, false, func(_ config.PHP, f *config.File) bool {
			return f.EndOfLine == config.EndOfLineCRLF
		}},
		{"a preset name in another case", `preset = "Laravel"`, false, func(p config.PHP, _ *config.File) bool {
			return !p.ConcatSpace && p.ClosureFnSpace
		}},
		{"the tempest preset", `preset = "tempest"`, false, func(_ config.PHP, f *config.File) bool {
			return f.PrintWidth == 180
		}},
		{"the hack preset", `preset = "hhvm"`, false, func(_ config.PHP, f *config.File) bool {
			return f.PrintWidth == 80 && f.IndentWidth == 2
		}},
		{"sort-uses as a sort order to keep", `sort-uses = "preserve"`, false, func(p config.PHP, _ *config.File) bool {
			return !p.SortUses
		}},
		{"sort-uses as ascending", `sort-uses = "alphanumeric-ascending"`, false, func(p config.PHP, _ *config.File) bool {
			return p.SortUses
		}},
		{"an unknown preset", `preset = "nope"`, true, nil},
		{"a negative width", `print-width = -2`, true, nil},
		{"a fractional width", `print-width = 80.5`, true, nil},
		{"a string for a bool", `use-tabs = "yes"`, true, nil},
		{"an end of line in the wrong case", `end-of-line = "LF"`, true, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFileAt(t, dir, "mago.toml", "[formatter]\n"+tc.table+"\n")

			mago, _, err := detectMago(dir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}

			if tc.wantErr {
				return
			}

			cfg := config.Default()
			applyMago(cfg, &mago)

			if !tc.check(cfg.PHP, cfg) {
				t.Fatalf("%s gave %+v", tc.table, cfg)
			}
		})
	}
}

func TestDetectStopsAtTheNearestMagoConfigWithoutAFormatter(t *testing.T) {
	root := t.TempDir()
	seedFileAt(t, root, "mago.toml", "[formatter]\nprint-width = 80\n")
	seedFileAt(t, root, "child/mago.toml", "[linter]\n")

	ctx, err := Detect(filepath.Join(root, "child"))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	if ctx.Mago == nil || !ctx.Mago.LintOnly {
		t.Fatalf("Mago = %+v, want the child's lint-only config", ctx.Mago)
	}
}

func TestApplyMagoLaysEmptyBodiesOutAsMago(t *testing.T) {
	cases := []struct {
		name                          string
		mago                          Mago
		functions, methods, classes   bool
		anonClasses, closures, spaced bool
	}{
		{"the default preset", Mago{}, true, true, true, true, true, false},
		{"the psr-12 preset", Mago{Preset: magoPresetPsr12}, false, false, false, true, true, false},
		{"the drupal preset", Mago{Preset: magoPresetDrupal}, false, false, true, true, true, false},
		{"a key over its preset", Mago{InlineEmptyClosureBodies: new(false)}, true, true, true, true, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.InlineEmptyFunctionBodies = false
			cfg.PHP.AnonClassParenSpace = true

			applyMago(cfg, &tc.mago)

			p := cfg.PHP
			got := []bool{
				p.InlineEmptyFunctionBodies, p.InlineEmptyMethodBodies, p.InlineEmptyClassBodies,
				p.InlineEmptyAnonClassBodies, p.InlineEmptyClosureBodies, p.AnonClassParenSpace,
			}
			want := []bool{tc.functions, tc.methods, tc.classes, tc.anonClasses, tc.closures, tc.spaced}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("functions, methods, classes, anonymous classes, closures inline and paren space = %v, want %v", got, want)
			}
		})
	}
}
