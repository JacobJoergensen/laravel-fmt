package detector

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
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

	wantIntPtr(t, prettier.Options.PrintWidth, 100, "PrintWidth")
	wantIntPtr(t, prettier.Options.TabWidth, 2, "TabWidth")
	wantBoolPtr(t, prettier.Options.UseTabs, true, "UseTabs")
	wantStringPtr(t, prettier.Options.EndOfLine, "crlf", "EndOfLine")
	wantBoolPtr(t, prettier.Options.SingleQuote, true, "SingleQuote")
	wantStringPtr(t, prettier.Options.ProseWrap, "always", "ProseWrap")
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

	if prettier.Options.SingleQuote != nil {
		t.Errorf("SingleQuote = %v, want nil for an unwritten key", *prettier.Options.SingleQuote)
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

	wantIntPtr(t, prettier.Options.PrintWidth, 90, "PrintWidth")
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

	wantIntPtr(t, prettier.Options.PrintWidth, 70, "PrintWidth")

	if len(prettier.Overrides) != 1 {
		t.Fatalf("Overrides len = %d, want 1", len(prettier.Overrides))
	}

	resolved := prettier.ResolveForFile("README.md")
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

	resolved := prettier.ResolveForFile("docs/guide.mdx")
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

	wantIntPtr(t, prettier.Options.PrintWidth, 100, "PrintWidth")
	wantBoolPtr(t, prettier.Options.SingleQuote, true, "SingleQuote")
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

	if prettier.Path != "" || prettier.Overrides != nil {
		t.Errorf("Prettier = %+v, want zero value", prettier)
	}
}

func TestDetectPrettierPackageJSONStringReferenceNotDetected(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "package.json", `{"prettier": "./my-config.json"}`)

	_, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if found {
		t.Fatal("detectPrettier reported found for a string reference, want not found")
	}
}

func TestDetectPrettierUnsupportedFormAdoptsNothing(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".prettierrc.js", `module.exports = { printWidth: 100 }`)

	prettier, found, err := detectPrettier(dir)
	if err != nil {
		t.Fatalf("detectPrettier: %v", err)
	}

	if !found || prettier.Options != (prettierOptions{}) || prettier.Overrides != nil {
		t.Fatalf("detectPrettier = %+v, found %v; want found with no options", prettier, found)
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

	if prettier.Path != "" || prettier.Overrides != nil {
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

	wantIntPtr(t, prettier.Options.PrintWidth, 100, "PrintWidth")
}

func TestPrettierResolveForFileExcludeFiles(t *testing.T) {
	always := "always"
	dir := t.TempDir()
	prettier := Prettier{
		Path: filepath.Join(dir, ".prettierrc.json"),
		Overrides: []prettierOverride{
			{
				Files:        stringOrSlice{"*.md"},
				ExcludeFiles: stringOrSlice{"CHANGELOG.md"},
				Options:      prettierOptions{ProseWrap: &always},
			},
		},
	}

	matched := prettier.ResolveForFile("README.md")
	wantStringPtr(t, matched.ProseWrap, "always", "matched ProseWrap")

	excluded := prettier.ResolveForFile("CHANGELOG.md")
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

		applyPrettier(cfg, &Prettier{Options: prettierOptions{EndOfLine: &value}})

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
		Options: prettierOptions{
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

	if cfg.TabWidth != 2 {
		t.Errorf("TabWidth = %d, want 2", cfg.TabWidth)
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

	if !found || prettier.Options.TabWidth == nil || *prettier.Options.TabWidth != 3 {
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
			if got := resolveLayout(t, tc.files, tc.file).TabWidth; got != tc.want {
				t.Fatalf("TabWidth = %d, want %d", got, tc.want)
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
