package detector

import (
	"path/filepath"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestApplyPrettierOverridesEditorConfig(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".editorconfig", "root = true\n[*]\nindent_style = space\nindent_size = 2\n")

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	cfg := config.Default()
	Apply(cfg, Context{
		EditorConfig: resolver,
		Prettier:     &Prettier{Options: prettierOptions{TabWidth: new(4)}},
	})

	if cfg.TabWidth != 4 {
		t.Errorf("TabWidth = %d, want 4 (Prettier overrides EditorConfig)", cfg.TabWidth)
	}
}

func TestResolveFileConfigEditorConfigUnsetFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".editorconfig", "root = true\n"+
		"[*]\nindent_style = tab\nindent_size = 2\nend_of_line = crlf\nmax_line_length = 100\n\n"+
		"[*.php]\nindent_style = unset\nindent_size = unset\nend_of_line = unset\nmax_line_length = unset\n")

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	base := config.Default()
	got, err := ResolveFileConfig(base, filepath.Join(dir, "User.php"), Context{EditorConfig: resolver}, nil)
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	if got.IndentStyle != base.IndentStyle {
		t.Errorf("IndentStyle = %q, want default %q", got.IndentStyle, base.IndentStyle)
	}

	if got.TabWidth != base.TabWidth {
		t.Errorf("TabWidth = %d, want default %d", got.TabWidth, base.TabWidth)
	}

	if got.EndOfLine != base.EndOfLine {
		t.Errorf("EndOfLine = %q, want default %q", got.EndOfLine, base.EndOfLine)
	}

	if got.PrintWidth != base.PrintWidth {
		t.Errorf("PrintWidth = %d, want default %d", got.PrintWidth, base.PrintWidth)
	}

	if got.IndentExplicit {
		t.Error("IndentExplicit = true, want false (a cleared indent is not an explicit one)")
	}
}

func TestResolveFileConfigAppliesPrettierOverrideToARelativePath(t *testing.T) {
	root := t.TempDir()
	seedFileAt(t, root, "resources/css/app.css", "a {}\n")
	t.Chdir(filepath.Join(root, "resources"))

	prettier := &Prettier{
		Path: filepath.Join(root, ".prettierrc.json"),
		Overrides: []prettierOverride{{
			Files:   stringOrSlice{"resources/css/*.css"},
			Options: prettierOptions{TabWidth: new(8)},
		}},
	}

	got, err := ResolveFileConfig(config.Default(), filepath.Join("css", "app.css"), Context{Prettier: prettier}, nil)
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	if got.TabWidth != 8 {
		t.Errorf("TabWidth = %d, want 8 from the override matching resources/css/app.css", got.TabWidth)
	}
}

func TestResolveFileConfigUnsetIndentSizeKeepsTabWidth(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".editorconfig", "root = true\n[*]\nindent_size = 2\ntab_width = 8\n\n[*.php]\nindent_size = unset\n")

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}

	got, err := ResolveFileConfig(config.Default(), filepath.Join(dir, "User.php"), Context{EditorConfig: resolver}, nil)
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	if got.TabWidth != 8 {
		t.Errorf("TabWidth = %d, want 8 (tab_width stands once indent_size is cleared)", got.TabWidth)
	}

	if !got.IndentExplicit {
		t.Error("IndentExplicit = false, want true (tab_width is still an explicit indent)")
	}
}

func TestApplyMagoWinsOverPint(t *testing.T) {
	cfg := config.Default()
	Apply(cfg, Context{
		Pint: &Pint{Preset: "laravel"},
		Mago: &Mago{Preset: magoPresetDrupal},
	})

	if cfg.PHP.FunctionBraceStyle != config.BraceStyleSameLine {
		t.Errorf("FunctionBraceStyle = %q, want %q (Mago wins over Pint)", cfg.PHP.FunctionBraceStyle, config.BraceStyleSameLine)
	}

	if cfg.TabWidth != 2 {
		t.Errorf("TabWidth = %d, want 2 (Mago drupal wins)", cfg.TabWidth)
	}
}

func TestApplyMagoBeatsPhpCsFixer(t *testing.T) {
	cfg := config.Default()

	concat := true
	phpCsFixer := &Pint{ConcatSpace: &ConcatSpace{Spacing: pintSpacingNone}}
	mago := &Mago{ConcatSpace: &concat}

	Apply(cfg, Context{PhpCsFixer: phpCsFixer, Mago: mago})

	if !cfg.PHP.ConcatSpace {
		t.Errorf("ConcatSpace = false, want true (Mago must win over PHP-CS-Fixer)")
	}
}

func TestApplyPintBeatsPhpCsFixer(t *testing.T) {
	cfg := config.Default()

	phpCsFixer := &Pint{ConcatSpace: &ConcatSpace{Spacing: pintSpacingOne}}
	pint := &Pint{ConcatSpace: &ConcatSpace{Spacing: pintSpacingNone}}

	Apply(cfg, Context{PhpCsFixer: phpCsFixer, Pint: pint})

	if cfg.PHP.ConcatSpace {
		t.Errorf("ConcatSpace = true, want false (pint.json must win over PHP-CS-Fixer)")
	}
}

func TestApplyPrettierLosesToMago(t *testing.T) {
	cfg := config.Default()

	width := 120
	magoWidth := 100
	ctx := Context{
		Prettier: &Prettier{Options: prettierOptions{PrintWidth: &width}},
		Mago:     &Mago{PrintWidth: &magoWidth},
	}

	Apply(cfg, ctx)

	if cfg.PrintWidth != 100 {
		t.Errorf("PrintWidth = %d, want 100 (Mago over Prettier)", cfg.PrintWidth)
	}
}

func TestApplyPrettierWinsOverEditorConfig(t *testing.T) {
	cfg := config.Default()
	cfg.PrintWidth = 80

	width := 120
	editorConfig := &EditorConfigResolver{
		baseChain: []*EditorConfig{{Sections: []EditorConfigSection{{Pattern: "*", MaxLineLength: 80}}}},
	}
	ctx := Context{
		EditorConfig: editorConfig,
		Prettier:     &Prettier{Options: prettierOptions{PrintWidth: &width}},
	}

	Apply(cfg, ctx)

	if cfg.PrintWidth != 120 {
		t.Errorf("PrintWidth = %d, want 120 (Prettier over EditorConfig)", cfg.PrintWidth)
	}
}

func TestResolveFileConfigAppliesPrettierOverride(t *testing.T) {
	dir := t.TempDir()
	never := config.ProseWrapNever
	always := config.ProseWrapAlways
	ctx := Context{
		Prettier: &Prettier{
			Path:    filepath.Join(dir, ".prettierrc.json"),
			Options: prettierOptions{ProseWrap: &always},
			Overrides: []prettierOverride{
				{Files: stringOrSlice{"*.md"}, Options: prettierOptions{ProseWrap: &never}},
			},
		},
	}

	cfg := config.Default()

	md, err := ResolveFileConfig(cfg, filepath.Join(dir, "README.md"), ctx, nil)
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	if md.Markdown.ProseWrap != config.ProseWrapNever {
		t.Errorf("README.md ProseWrap = %q, want %q (override)", md.Markdown.ProseWrap, config.ProseWrapNever)
	}

	other, err := ResolveFileConfig(cfg, filepath.Join(dir, "app.css"), ctx, nil)
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	if other.Markdown.ProseWrap != config.ProseWrapAlways {
		t.Errorf("app.css ProseWrap = %q, want %q (top-level)", other.Markdown.ProseWrap, config.ProseWrapAlways)
	}
}

func TestResolveFileConfigUserProseWrapWins(t *testing.T) {
	dir := t.TempDir()
	never := config.ProseWrapNever
	ctx := Context{
		Prettier: &Prettier{
			Path: filepath.Join(dir, ".prettierrc.json"),
			Overrides: []prettierOverride{
				{Files: stringOrSlice{"*.md"}, Options: prettierOptions{ProseWrap: &never}},
			},
		},
	}

	cfg := config.Default()
	cfg.Markdown.ProseWrap = config.ProseWrapAlways
	userKeys := map[string]bool{"markdown.proseWrap": true}

	md, err := ResolveFileConfig(cfg, filepath.Join(dir, "README.md"), ctx, userKeys)
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	if md.Markdown.ProseWrap != config.ProseWrapAlways {
		t.Errorf("ProseWrap = %q, want %q (user laravel-fmt.json wins)", md.Markdown.ProseWrap, config.ProseWrapAlways)
	}
}

func TestResolveFileConfigRespectsUserKeys(t *testing.T) {
	dir := t.TempDir()
	body := "[*]\nmax_line_length = 200\n\n[*.php]\nmax_line_length = 100\n"

	seedFileAt(t, dir, ".editorconfig", body)

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("newEditorConfigResolver: %v", err)
	}

	ctx := Context{EditorConfig: resolver}
	phpPath := filepath.Join(dir, "app", "User.php")

	t.Run("user printWidth blocks overlay", func(t *testing.T) {
		cfg := config.Default()
		cfg.PrintWidth = 120

		resolved, err := ResolveFileConfig(cfg, phpPath, ctx, map[string]bool{"printWidth": true})
		if err != nil {
			t.Fatalf("ResolveFileConfig: %v", err)
		}

		if resolved.PrintWidth != 120 {
			t.Fatalf("PrintWidth = %d, want 120 (user config should win)", resolved.PrintWidth)
		}
	})

	t.Run("no user override applies the section", func(t *testing.T) {
		cfg := config.Default()

		resolved, err := ResolveFileConfig(cfg, phpPath, ctx, map[string]bool{})
		if err != nil {
			t.Fatalf("ResolveFileConfig: %v", err)
		}

		if resolved.PrintWidth != 100 {
			t.Fatalf("PrintWidth = %d, want 100 (last matching section wins)", resolved.PrintWidth)
		}
	})

	t.Run("no editorconfig keeps what the user set", func(t *testing.T) {
		empty := t.TempDir()

		resolver, err := newEditorConfigResolver(empty, &probeMemo{})
		if err != nil {
			t.Fatalf("newEditorConfigResolver: %v", err)
		}

		cfg := config.Default()
		cfg.PrintWidth = 80

		resolved, err := ResolveFileConfig(cfg, filepath.Join(empty, "app", "User.php"), Context{EditorConfig: resolver}, map[string]bool{"printWidth": true})
		if err != nil {
			t.Fatalf("ResolveFileConfig: %v", err)
		}

		if resolved.PrintWidth != 80 {
			t.Fatalf("PrintWidth = %d, want 80 (no config should not change a user key)", resolved.PrintWidth)
		}
	})
}

func TestUserIndentWidthSetsExplicit(t *testing.T) {
	cfg := config.Default()

	resolved, err := ResolveFileConfig(cfg, "settings.json", Context{}, map[string]bool{"indentWidth": true})
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	if !resolved.IndentExplicit {
		t.Fatal("IndentExplicit not set when laravel-fmt.json sets indentWidth")
	}
}

func TestNoIndentConfigLeavesImplicit(t *testing.T) {
	cfg := config.Default()

	resolved, err := ResolveFileConfig(cfg, "settings.json", Context{}, map[string]bool{})
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	if resolved.IndentExplicit {
		t.Fatal("IndentExplicit set without any explicit indent config")
	}
}

func TestFileLayoutFollowsThePrecedenceChain(t *testing.T) {
	def := config.Default()

	tests := []struct {
		name  string
		files map[string]string
		file  string
		check func(c config.File) (got, want any)
	}{
		{
			"unset indent_size drops to the default",
			map[string]string{".editorconfig": "root = true\n[*]\nindent_size = 2\n[*.php]\nindent_size = unset\n"},
			"a.php",
			func(c config.File) (any, any) { return c.TabWidth, def.TabWidth },
		},
		{
			"unset indent_size leaves other files their width",
			map[string]string{".editorconfig": "root = true\n[*]\nindent_size = 2\n[*.php]\nindent_size = unset\n"},
			"a.json",
			func(c config.File) (any, any) { return c.TabWidth, 2 },
		},
		{
			"unset indent_style drops to the default",
			map[string]string{".editorconfig": "root = true\n[*]\nindent_style = tab\n[*.php]\nindent_style = unset\n"},
			"a.php",
			func(c config.File) (any, any) { return c.IndentStyle, def.IndentStyle },
		},
		{
			"unset end_of_line drops to the default",
			map[string]string{".editorconfig": "root = true\n[*]\nend_of_line = crlf\n[*.php]\nend_of_line = unset\n"},
			"a.php",
			func(c config.File) (any, any) { return c.EndOfLine, def.EndOfLine },
		},
		{
			"unset max_line_length drops to the default",
			map[string]string{".editorconfig": "root = true\n[*]\nmax_line_length = 100\n[*.php]\nmax_line_length = unset\n"},
			"a.php",
			func(c config.File) (any, any) { return c.PrintWidth, def.PrintWidth },
		},
		{
			"unset under a Prettier width keeps Prettier's",
			map[string]string{
				".editorconfig": "root = true\n[*]\nindent_size = 2\n[*.php]\nindent_size = unset\n",
				".prettierrc":   `{"tabWidth": 3, "plugins": ["@prettier/plugin-php"]}`,
			},
			"a.php",
			func(c config.File) (any, any) { return c.TabWidth, 3 },
		},
		{
			"Mago outranks a file's EditorConfig section",
			map[string]string{
				".editorconfig": "root = true\n[*.php]\nindent_size = 2\nmax_line_length = 60\n",
				"mago.toml":     "[formatter]\ntab-width = 8\nprint-width = 100\n",
			},
			"a.php",
			func(c config.File) (any, any) { return [2]int{c.TabWidth, c.PrintWidth}, [2]int{8, 100} },
		},
		{
			"Mago outranks the global section",
			map[string]string{
				".editorconfig": "root = true\n[*]\nindent_size = 2\n",
				"mago.toml":     "[formatter]\ntab-width = 8\n",
			},
			"a.php",
			func(c config.File) (any, any) { return c.TabWidth, 8 },
		},
		{
			"laravel-fmt.json outranks every detected source",
			map[string]string{
				".editorconfig":    "root = true\n[*]\nindent_size = 2\n",
				"mago.toml":        "[formatter]\ntab-width = 8\n",
				"laravel-fmt.json": `{"indentWidth": 6}`,
			},
			"a.php",
			func(c config.File) (any, any) { return c.TabWidth, 6 },
		},
		{
			"indent_size = tab defers to tab_width",
			map[string]string{".editorconfig": "root = true\n[*]\nindent_size = 2\n[*.php]\nindent_size = tab\ntab_width = 3\n"},
			"a.php",
			func(c config.File) (any, any) { return c.TabWidth, 3 },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, want := tc.check(resolveLayout(t, tc.files, tc.file))
			if got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func TestLayoutSourcesNameTheWinningConfig(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		file  string
		key   string
		want  string
	}{
		{
			"a value two sources agree on goes to the later",
			map[string]string{".editorconfig": "root = true\n[*]\nmax_line_length = 100\n", ".prettierrc": `{"printWidth": 100}`},
			"a.css", "printWidth", ".prettierrc",
		},
		{
			"Pint fixes the PHP indent",
			map[string]string{".editorconfig": "root = true\n[*]\nindent_size = 2\n", "pint.json": `{"preset": "laravel"}`},
			"a.php", "indentWidth", "pint.json",
		},
		{
			"a later source keeps no credit for a key it leaves alone",
			map[string]string{".prettierrc": `{"printWidth": 100, "plugins": ["@prettier/plugin-php"]}`, "pint.json": `{"preset": "laravel"}`},
			"a.php", "printWidth", ".prettierrc",
		},
		{
			"Mago outranks Pint",
			map[string]string{"pint.json": `{"preset": "laravel"}`, "mago.toml": "[formatter]\ntab-width = 8\n"},
			"a.php", "indentWidth", "mago.toml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, project := seedProject(t, tc.files, tc.file)

			sources, err := LayoutSources(path, project.Ctx)
			if err != nil {
				t.Fatalf("LayoutSources: %v", err)
			}

			want := filepath.Join(project.Root, tc.want)

			if got := sources[tc.key]; got != want {
				t.Fatalf("%s from %q, want %q", tc.key, got, want)
			}
		})
	}
}

func seedProject(t *testing.T, files map[string]string, rel string) (string, *Project) {
	t.Helper()

	root := t.TempDir()
	seedFileAt(t, root, "composer.json", "{}")

	for name, body := range files {
		seedFileAt(t, root, name, body)
	}

	path := filepath.Join(root, filepath.FromSlash(rel))

	project, err := NewResolver(LoadConfig).For(path)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	return path, project
}

func resolveLayout(t *testing.T, files map[string]string, rel string) config.File {
	t.Helper()

	path, project := seedProject(t, files, rel)

	resolved, err := ResolveFileConfig(&project.Cfg, path, project.Ctx, project.UserKeys)
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	return resolved
}

func TestPrettierShapesOnlyTheLanguagesItFormats(t *testing.T) {
	const jsOnly = `{"tabWidth": 2, "printWidth": 80, "singleQuote": false}`

	def := config.Default()
	layout := func(c config.File) any { return [3]any{c.TabWidth, c.PrintWidth, c.QuoteStyle} }

	tests := []struct {
		name  string
		files map[string]string
		file  string
		want  any
	}{
		{"a JS-only config leaves PHP alone", map[string]string{".prettierrc": jsOnly}, "a.php", [3]any{def.TabWidth, def.PrintWidth, def.QuoteStyle}},
		{"a JS-only config shapes CSS", map[string]string{".prettierrc": jsOnly}, "a.css", [3]any{2, 80, config.QuoteStyleDouble}},
		{"a JS-only config leaves Blade alone", map[string]string{".prettierrc": jsOnly}, "a.blade.php", [3]any{def.TabWidth, def.PrintWidth, def.QuoteStyle}},
		{
			"the PHP plugin brings PHP in",
			map[string]string{".prettierrc": `{"tabWidth": 2, "printWidth": 80, "singleQuote": false, "plugins": ["@prettier/plugin-php"]}`},
			"a.php",
			[3]any{2, 80, config.QuoteStyleDouble},
		},
		{
			"a plugin listed by path counts",
			map[string]string{".prettierrc": `{"tabWidth": 2, "plugins": ["./node_modules/@prettier/plugin-php/standalone.js"]}`},
			"a.php",
			[3]any{2, def.PrintWidth, def.QuoteStyle},
		},
		{
			"the Blade plugin brings Blade in",
			map[string]string{".prettierrc": `{"tabWidth": 2, "plugins": ["@shufo/prettier-plugin-blade"]}`},
			"a.blade.php",
			[3]any{2, def.PrintWidth, def.QuoteStyle},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := layout(resolveLayout(t, tc.files, tc.file)); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPintFixesTheLayoutOfThePHPItFormats(t *testing.T) {
	files := map[string]string{
		"pint.json":     `{"preset": "laravel"}`,
		".editorconfig": "root = true\n[*]\nindent_size = 2\nindent_style = tab\nend_of_line = crlf\n",
		".prettierrc":   `{"tabWidth": 3, "plugins": ["@prettier/plugin-php"]}`,
	}

	php := resolveLayout(t, files, "a.php")
	if got, want := [3]any{php.TabWidth, php.IndentStyle, php.EndOfLine}, [3]any{4, config.IndentStyleSpace, config.EndOfLineLF}; got != want {
		t.Fatalf("PHP layout = %v, want Pint's %v", got, want)
	}

	json := resolveLayout(t, files, "a.json")
	if got, want := [3]any{json.TabWidth, json.IndentStyle, json.EndOfLine}, [3]any{3, config.IndentStyleTab, config.EndOfLineCRLF}; got != want {
		t.Fatalf("JSON layout = %v, want Prettier and EditorConfig's %v", got, want)
	}
}
