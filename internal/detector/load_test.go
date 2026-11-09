package detector

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
)

func TestLoadConfigUserJSONBeatsDetector(t *testing.T) {
	dir := t.TempDir()

	seedFileAt(t, dir, ".editorconfig", "[*]\nmax_line_length = 200\n")

	cfgJSON := []byte(`{"printWidth": 120}`)

	seedFileAt(t, dir, config.FileName, string(cfgJSON))

	ctx, err := Detect(dir)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	if ctx.EditorConfig.Global().MaxLineLength != 200 {
		t.Fatal("detector did not read .editorconfig, precondition of the test failed")
	}

	loaded, err := LoadConfig(dir, ctx)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.File.PrintWidth != 120 {
		t.Fatalf("user's printWidth=120 was overridden by detector (got %d), precedence regressed", loaded.File.PrintWidth)
	}

	if !loaded.UserKeys["printWidth"] {
		t.Fatalf("UserKeys missing printWidth, per-file overlays would clobber the user's value")
	}
}

func TestLoadConfigFileHonorsExplicitPath(t *testing.T) {
	workDir := t.TempDir()
	configDir := t.TempDir()

	seedFileAt(t, workDir, ".editorconfig", "[*]\nmax_line_length = 200\n")

	configPath := filepath.Join(configDir, "explicit.json")
	cfgJSON := []byte(`{"printWidth": 120}`)

	if err := os.WriteFile(configPath, cfgJSON, 0o644); err != nil {
		t.Fatalf("seed explicit json: %v", err)
	}

	ctx, err := Detect(workDir)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	loaded, err := LoadConfigFile(configPath, ctx)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}

	if loaded.File.PrintWidth != 120 {
		t.Fatalf("explicit --config printWidth=120 was overridden by detector (got %d), precedence regressed", loaded.File.PrintWidth)
	}
}

func TestLoadConfigFileResolvesTheStylesheetBesideItself(t *testing.T) {
	const theme = "@import \"tailwindcss\";\n@theme { --color-brand: #f00; }\n"

	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			"no auto-detected config",
			map[string]string{"config/theme.css": theme},
			"",
		},
		{
			"an auto-detected root config beside it",
			map[string]string{"laravel-fmt.json": "{}", "config/theme.css": theme},
			"",
		},
		{
			"a root theme.css is not taken for the missing one",
			map[string]string{"laravel-fmt.json": "{}", "theme.css": theme},
			filepath.Join("config", "theme.css"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			seedFileAt(t, root, "composer.json", "{}")
			seedFileAt(t, root, "config/laravel-fmt.json", `{"tailwind": {"stylesheet": "theme.css"}}`)

			for name, body := range tc.files {
				seedFileAt(t, root, name, body)
			}

			ctx, err := Detect(root)
			if err != nil {
				t.Fatalf("detect: %v", err)
			}

			loaded, err := LoadConfigFile(filepath.Join(root, "config", "laravel-fmt.json"), ctx)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("LoadConfigFile err = %v, want it to name %s", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("LoadConfigFile: %v", err)
			}

			if !loaded.File.Tailwind {
				t.Fatal("the stylesheet loaded but sorting stayed off")
			}
		})
	}
}

func TestLoadConfigFileMissingPathErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")
	_, err := LoadConfigFile(missing, Context{})

	if err == nil {
		t.Fatal("LoadConfigFile returned nil error for a missing path")
	}

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadConfigFile error %v does not wrap os.ErrNotExist", err)
	}
}

func TestLoadDefaultsBuildsTheDiscoveredTheme(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "composer.json", "{}")
	seedFileAt(t, dir, "artisan", "")
	seedFileAt(t, dir, "resources/css/app.css", `@import "tailwindcss";`+"\n@theme { --text-brand: 2rem; }")

	ctx, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	loaded, err := LoadDefaults(ctx)
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}

	// `text-brand` is a font size in this theme, which sorts before a weight;
	// read as a color it would sort after one.
	const want = "text-brand font-bold"

	if got := tailwind.Sort(loaded.File.TailwindTheme, "text-brand font-bold"); got != want {
		t.Fatalf("sorted %q, want %q", got, want)
	}
}

func loadTailwind(t *testing.T, dir string) config.File {
	t.Helper()

	ctx, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	loaded, err := LoadConfig(dir, ctx)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	return loaded.File
}
