package detector

import (
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
)

func TestDetectPrettierParsesJSON(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.json", `{
		"printWidth": 100,
		"tabWidth": 2,
		"useTabs": true,
		"endOfLine": "crlf",
		"singleQuote": true,
		"proseWrap": "always"
	}`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found {
		t.Fatal("detectPrettier reported not found, want found")
	}

	wantIntPtr(t, prettier.options.PrintWidth, 100, "PrintWidth")
	wantIntPtr(t, prettier.options.TabWidth, 2, "TabWidth")
	wantBoolPtr(t, prettier.options.UseTabs, true, "UseTabs")
	wantStringPtr(t, prettier.options.EndOfLine, "crlf", "EndOfLine")
	wantBoolPtr(t, prettier.options.SingleQuote, true, "SingleQuote")
	wantStringPtr(t, prettier.options.ProseWrap, "always", "ProseWrap")
}

func TestDetectPrettierUnwrittenKeyStaysNil(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.json", `{"printWidth": 80}`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found {
		t.Fatal("detectPrettier reported not found, want found")
	}

	if prettier.options.SingleQuote != nil {
		t.Errorf("SingleQuote = %v, want nil for an unwritten key", *prettier.options.SingleQuote)
	}
}

func TestDetectPrettierExtensionlessJSON(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc", `{"printWidth": 90}`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found {
		t.Fatal("detectPrettier reported not found, want found")
	}

	wantIntPtr(t, prettier.options.PrintWidth, 90, "PrintWidth")
}

func TestDetectPrettierYAML(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.yaml", `printWidth: 70
proseWrap: never
overrides:
  - files: "*.md"
    options:
      proseWrap: always
`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found {
		t.Fatal("detectPrettier reported not found, want found")
	}

	wantIntPtr(t, prettier.options.PrintWidth, 70, "PrintWidth")

	if len(prettier.overrides) != 1 {
		t.Fatalf("Overrides len = %d, want 1", len(prettier.overrides))
	}

	resolved := prettier.resolveForFile("README.md")
	wantStringPtr(t, resolved.ProseWrap, "always", "override ProseWrap")
}

func TestDetectPrettierReadsTOML(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.toml", `printWidth = 70
plugins = ["@prettier/plugin-php"]

[[overrides]]
files = "*.md"

[overrides.options]
proseWrap = "always"
`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found {
		t.Fatal("detectPrettier reported not found, want found")
	}

	wantIntPtr(t, prettier.options.PrintWidth, 70, "PrintWidth")

	if !prettier.formats(formatter.LangPHP) {
		t.Fatalf("Plugins = %q, want the PHP plugin read", prettier.Plugins)
	}

	resolved := prettier.resolveForFile("README.md")
	wantStringPtr(t, resolved.ProseWrap, "always", "override ProseWrap")
}

func TestDetectPrettierYAMLFilesArray(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.yml", `overrides:
  - files: ["*.md", "*.mdx"]
    options:
      proseWrap: always
`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found {
		t.Fatal("detectPrettier reported not found, want found")
	}

	resolved := prettier.resolveForFile("docs/guide.mdx")
	wantStringPtr(t, resolved.ProseWrap, "always", "override ProseWrap for array files")
}

func TestDetectPrettierPackageJSON(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "package.json", `{
		"name": "app",
		"prettier": { "printWidth": 100, "singleQuote": true }
	}`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found {
		t.Fatal("detectPrettier reported not found, want found")
	}

	wantIntPtr(t, prettier.options.PrintWidth, 100, "PrintWidth")
	wantBoolPtr(t, prettier.options.SingleQuote, true, "SingleQuote")
}

func TestDetectPrettierPackageJSONWithoutKeyNotDetected(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "package.json", `{"name": "app", "version": "1.0.0"}`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if found {
		t.Fatal("detectPrettier reported found, want not found")
	}

	if prettier.Path != "" || prettier.overrides != nil {
		t.Errorf("Prettier = %+v, want zero value", prettier)
	}
}

func TestDetectPrettierUnsupportedFormIsUnreadAndAdoptsNothing(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.js", `module.exports = { printWidth: 100 }`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found || !prettier.Unread || prettier.options != (prettierOptions{}) || prettier.overrides != nil {
		t.Fatalf("detectPrettier = %+v, found %v; want found, unread, with no options", prettier, found)
	}
}

func TestDetectPrettierMissingFile(t *testing.T) {
	prettier, found, err := detectPrettier(t.TempDir())
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if found {
		t.Fatal("detectPrettier reported found, want not found")
	}

	if prettier.Path != "" || prettier.overrides != nil {
		t.Errorf("Prettier = %+v, want zero value", prettier)
	}
}

func TestDetectPrettierMalformedErrors(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.json", `{"printWidth": [1, 2`)

	if _, _, err := detectPrettier(dir); err == nil {
		t.Fatal("detectPrettier: want error for malformed config, got nil")
	}
}

func TestDetectPrettierPackageJSONWinsOverPrettierrc(t *testing.T) {
	dir := t.TempDir()

	pkg := `{"prettier": {"printWidth": 100}}`
	seedFileAt(t, dir, "package.json", pkg)

	rc := `{"printWidth": 40}`
	seedFileAt(t, dir, ".prettierrc", rc)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found {
		t.Fatal("detectPrettier reported not found, want found")
	}

	wantIntPtr(t, prettier.options.PrintWidth, 100, "PrintWidth")
}

func TestPrettierResolveForFileExcludeFiles(t *testing.T) {
	always := "always"
	dir := t.TempDir()
	prettier := Prettier{
		Path: filepath.Join(dir, ".prettierrc.json"),
		overrides: []prettierOverride{
			{
				Files:        stringOrSlice{"*.md"},
				ExcludeFiles: stringOrSlice{"CHANGELOG.md"},
				Options:      prettierOptions{ProseWrap: &always},
			},
		},
	}

	matched := prettier.resolveForFile("README.md")
	wantStringPtr(t, matched.ProseWrap, "always", "matched ProseWrap")

	excluded := prettier.resolveForFile("CHANGELOG.md")
	if excluded.ProseWrap != nil {
		t.Errorf("excluded ProseWrap = %q, want nil", *excluded.ProseWrap)
	}
}

func TestApplyPrettierMapsEndOfLine(t *testing.T) {
	cases := map[string]string{
		"lf":   config.EndOfLineLF,
		"crlf": config.EndOfLineCRLF,
		"auto": config.EndOfLineAuto,
		"cr":   "",
	}

	for value, want := range cases {
		cfg := config.Default()
		cfg.EndOfLine = ""

		applyPrettier(cfg, &Prettier{options: prettierOptions{EndOfLine: &value}})

		if cfg.EndOfLine != want {
			t.Errorf("endOfLine %q: EndOfLine = %q, want %q", value, cfg.EndOfLine, want)
		}
	}
}

func TestApplyPrettierMapsOptions(t *testing.T) {
	cfg := config.Default()

	width := 100
	tab := 2
	useTabs := true
	single := true
	prose := config.ProseWrapAlways
	prettier := &Prettier{
		options: prettierOptions{
			PrintWidth:  &width,
			TabWidth:    &tab,
			UseTabs:     &useTabs,
			SingleQuote: &single,
			ProseWrap:   &prose,
		},
	}

	applyPrettier(cfg, prettier)

	if cfg.PrintWidth != 100 {
		t.Errorf("PrintWidth = %d, want 100", cfg.PrintWidth)
	}

	if cfg.IndentWidth != 2 {
		t.Errorf("IndentWidth = %d, want 2", cfg.IndentWidth)
	}

	if !cfg.IndentExplicit {
		t.Error("IndentExplicit = false, want true after Prettier set tabWidth")
	}

	if cfg.IndentStyle != config.IndentStyleTab {
		t.Errorf("IndentStyle = %q, want %q", cfg.IndentStyle, config.IndentStyleTab)
	}

	if cfg.QuoteStyle != config.QuoteStyleSingle {
		t.Errorf("QuoteStyle = %q, want %q", cfg.QuoteStyle, config.QuoteStyleSingle)
	}

	if cfg.Markdown.ProseWrap != config.ProseWrapAlways {
		t.Errorf("ProseWrap = %q, want %q", cfg.Markdown.ProseWrap, config.ProseWrapAlways)
	}
}

func TestDetectPrettierFindsACaseVariantWhereTheFilesystemFoldsCase(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("the default filesystem here is case-sensitive, where a stat would not match either")
	}

	dir := t.TempDir()
	seedFileAt(t, dir, ".PrettierRC", `{"tabWidth": 3}`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found || prettier.options.TabWidth == nil || *prettier.options.TabWidth != 3 {
		t.Fatalf("detectPrettier = %+v, found %v; want the .PrettierRC config", prettier, found)
	}
}

func TestPrettierConfigComesFromTheNearestDirectory(t *testing.T) {
	const root = `{"tabWidth": 2, "plugins": ["@prettier/plugin-php", "prettier-plugin-blade"]}`

	tests := []struct {
		name  string
		files map[string]string
		file  string
		want  int
	}{
		{
			"a nested config wins below it",
			map[string]string{".prettierrc": root, "resources/views/.prettierrc": `{"tabWidth": 8, "plugins": ["prettier-plugin-blade"]}`},
			"resources/views/a.blade.php",
			8,
		},
		{
			"the root config still answers elsewhere",
			map[string]string{".prettierrc": root, "resources/views/.prettierrc": `{"tabWidth": 8, "plugins": ["prettier-plugin-blade"]}`},
			"app/a.php",
			2,
		},
		{
			"a nested config's overrides match from its own directory",
			map[string]string{
				".prettierrc":                 root,
				"resources/views/.prettierrc": `{"plugins": ["prettier-plugin-blade"], "overrides": [{"files": "*.blade.php", "options": {"tabWidth": 6}}]}`,
			},
			"resources/views/a.blade.php",
			6,
		},
		{
			"a nested JS config adopts nothing rather than the root's",
			map[string]string{".prettierrc": root, "resources/views/.prettierrc.js": "module.exports = { tabWidth: 8 }"},
			"resources/views/a.blade.php",
			4,
		},
		{
			"a package.json without a prettier key does not stop the search",
			map[string]string{".prettierrc": root, "resources/views/package.json": `{"name": "views"}`},
			"resources/views/a.blade.php",
			2,
		},
		{
			"a package.json prettier key is a config",
			map[string]string{".prettierrc": root, "resources/views/package.json": `{"prettier": {"tabWidth": 3, "plugins": ["prettier-plugin-blade"]}}`},
			"resources/views/a.blade.php",
			3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveLayout(t, tc.files, tc.file).IndentWidth; got != tc.want {
				t.Fatalf("IndentWidth = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNestedPrettierStylesheetSortsTheFilesBelowIt(t *testing.T) {
	files := map[string]string{
		"resources/views/.prettierrc": `{"tailwindStylesheet": "theme.css"}`,
		"resources/views/theme.css":   "@import \"tailwindcss\";\n@theme { --color-brand: #f00; }\n",
	}

	if !resolveLayout(t, files, "resources/views/a.blade.php").Tailwind {
		t.Fatal("a view below the config was not sorted with its stylesheet")
	}

	if resolveLayout(t, files, "app/a.blade.php").Tailwind {
		t.Fatal("a file outside the config was sorted with its stylesheet")
	}
}

func TestDirNamesFindsCandidatesInAnUnlistableFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod cannot make a folder unlistable on Windows")
	}

	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.json", "{}")

	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, ".prettierrc.yml")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	//nolint:gosec // a folder mode: entering it needs the execute bit
	if err := os.Chmod(dir, 0o311); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	//nolint:gosec // restores the folder so t.TempDir can remove it
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("the folder is still listable, as it is for root")
	}

	present, err := dirNames(dir, []string{".prettierrc.json", ".prettierrc.yml", ".prettierrc.yaml"})
	if err != nil {
		t.Fatalf("dirNames: %v", err)
	}

	want := map[string]bool{
		nameKey(".prettierrc.json"): true,
		nameKey(".prettierrc.yml"):  true,
	}

	if !maps.Equal(present, want) {
		t.Errorf("present = %v, want %v (a dangling symlink is listed, so it counts)", present, want)
	}
}

func TestDetectPrettierFollowsPrettiersSearchOrder(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		wantFile   string
		wantUnread bool
		wantWidth  int
	}{
		{
			name: "a JS config wins over TOML",
			files: map[string]string{
				"prettier.config.js": "export default {}",
				".prettierrc.toml":   "printWidth = 100\n",
			},
			wantFile:   "prettier.config.js",
			wantUnread: true,
		},
		{
			name: ".yml wins over .yaml",
			files: map[string]string{
				".prettierrc.yml":  "printWidth: 70\n",
				".prettierrc.yaml": "printWidth: 90\n",
			},
			wantFile:  ".prettierrc.yml",
			wantWidth: 70,
		},
		{
			name:       "prettier.config.mts is found",
			files:      map[string]string{"prettier.config.mts": "export default {}"},
			wantFile:   "prettier.config.mts",
			wantUnread: true,
		},
		{
			name: "an invalid package.json is passed over",
			files: map[string]string{
				"package.json": `{"prettier": {"printWidth": 100}`,
				".prettierrc":  `{"printWidth": 70}`,
			},
			wantFile:  ".prettierrc",
			wantWidth: 70,
		},
		{
			name: "a shared config in package.json stops the search",
			files: map[string]string{
				"package.json": `{"prettier": "@vendor/prettier-config"}`,
				".prettierrc":  `{"printWidth": 70}`,
			},
			wantFile:   "package.json",
			wantUnread: true,
		},
		{
			name: "package.json wins over package.yaml",
			files: map[string]string{
				"package.json": `{"prettier": {"printWidth": 100}}`,
				"package.yaml": "prettier:\n  printWidth: 90\n",
			},
			wantFile:  "package.json",
			wantWidth: 100,
		},
		{
			name: "package.yaml wins over .prettierrc",
			files: map[string]string{
				"package.yaml": "prettier:\n  printWidth: 90\n",
				".prettierrc":  `{"printWidth": 70}`,
			},
			wantFile:  "package.yaml",
			wantWidth: 90,
		},
		{
			name: "package.yaml without a prettier key is passed over",
			files: map[string]string{
				"package.yaml": "name: app\n",
				".prettierrc":  `{"printWidth": 70}`,
			},
			wantFile:  ".prettierrc",
			wantWidth: 70,
		},
		{
			name: "an invalid package.yaml is passed over",
			files: map[string]string{
				"package.yaml": "prettier: [\n",
				".prettierrc":  `{"printWidth": 70}`,
			},
			wantFile:  ".prettierrc",
			wantWidth: 70,
		},
		{
			name: "a shared config in package.yaml stops the search",
			files: map[string]string{
				"package.yaml": "prettier: \"@vendor/prettier-config\"\n",
				".prettierrc":  `{"printWidth": 70}`,
			},
			wantFile:   "package.yaml",
			wantUnread: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			for rel, body := range tc.files {
				seedFileAt(t, dir, rel, body)
			}

			got, found, err := detectPrettier(dir)
			if err != nil {
				t.Fatalf("detectPrettier: %v", err)
			}

			if !found {
				t.Fatalf("found = false, want %s", tc.wantFile)
			}

			if filepath.Base(got.Path) != tc.wantFile || got.Unread != tc.wantUnread {
				t.Fatalf("detectPrettier = %s, unread %v; want %s, unread %v", got.Path, got.Unread, tc.wantFile, tc.wantUnread)
			}

			if tc.wantWidth != 0 {
				wantIntPtr(t, got.options.PrintWidth, tc.wantWidth, "PrintWidth")
			}
		})
	}
}
