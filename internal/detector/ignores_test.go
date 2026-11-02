package detector

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/walk"
)

// Each row is what Symfony Finder, which Pint walks with, does with the file.
func TestPintIgnoresSkipWhatPintSkips(t *testing.T) {
	cases := []struct {
		name     string
		pint     Pint
		file     string
		wantSkip bool
	}{
		{"exclude by name matches that directory at any depth", Pint{Exclude: []string{"build/"}}, "src/build/a.php", true},
		{"exclude by path matches that path at any depth", Pint{Exclude: []string{"resources/views/flux"}}, "pkg/resources/views/flux/a.php", true},
		{"exclude is literal", Pint{Exclude: []string{"a*b"}}, "axb/a.php", false},
		{"exclude starting with # is not a comment", Pint{Exclude: []string{"#tmp"}}, "#tmp/a.php", true},
		{"notName globs the file name", Pint{NotName: []string{"*-my-file.php"}}, "app/x-my-file.php", true},
		{"notName matching a directory does not prune its children", Pint{NotName: []string{"legacy*"}}, "app/legacy_helpers/Support.php", false},
		{"exclude still prunes a directory notName matches", Pint{NotName: []string{"legacy*"}, Exclude: []string{"legacy_helpers"}}, "app/legacy_helpers/Support.php", true},
		{"notName expands braces", Pint{NotName: []string{"*.{stub,tpl}.php"}}, "app/x.tpl.php", true},
		{"notName regex without regex syntax matches part of the name", Pint{NotName: []string{"/legacy/"}}, "app/old_legacy_x.php", true},
		{"notName with a slash matches no name", Pint{NotName: []string{"sub/x.php"}}, "sub/x.php", false},
		{"notPath matches a file path", Pint{NotPath: []string{"path/to/excluded-file.php"}}, "src/path/to/excluded-file.php", true},
		{"notPath is a substring match across a segment", Pint{NotPath: []string{"tests/Fixtures"}}, "tests/FixturesTest.php", true},
		{"notPath is a substring match within a segment", Pint{NotPath: []string{"Fixtures/bad"}}, "tests/Fixtures/bad_input.php", true},
		{"notPath with a leading slash needs a directory before it", Pint{NotPath: []string{"/fixtures"}}, "fixtures/a.php", false},
		{"notPath regex without regex syntax matches anywhere", Pint{NotPath: []string{"/Models/"}}, "app/Models/User.php", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			seedFileAt(t, root, tc.file, "<?php\n")

			cfg := config.Default()
			cfg.AdoptedIgnore = []config.IgnoreLayer{{Patterns: pintIgnores(&tc.pint)}}

			paths, err := walk.Files(root, root, cfg, nil, nil)
			if err != nil {
				t.Fatalf("Files: %v", err)
			}

			skipped := len(paths) == 0
			if skipped != tc.wantSkip {
				t.Fatalf("%s skipped = %v, want %v (patterns %q)", tc.file, skipped, tc.wantSkip, pintIgnores(&tc.pint))
			}
		})
	}
}

// Answers taken from Symfony's own isRegex, run from Pint's phar.
func TestIsFinderRegexFollowsSymfony(t *testing.T) {
	cases := map[string]bool{
		"/foo/": true, "/foo/i": true, "/x/y/": true, "#tests/.*#i": true, "#x#im": true,
		"~x~": true, "{foo}": true, "(x)": true, "[x]": true, "<x>": true,
		"#x#z": false, "/a": false, "ab": false, "aba": false, "1x1": false, "x/y/x": false,
		`\x\`: false, "??": false, "*.php": false, "*foo*": false, "foo.php": false,
		"{a,b}.php": false, "build": false, "resources/views/flux": false,
	}

	for in, want := range cases {
		if got := isFinderRegex(in); got != want {
			t.Errorf("isFinderRegex(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLoadConfigAdoptsExclusionsBeneathTheUsersList(t *testing.T) {
	root := t.TempDir()

	for name, body := range map[string]string{
		".prettierignore":  "# generated\nCLAUDE.md\n\n!keep.md\n",
		"pint.json":        `{"exclude": "resources/views/flux"}`,
		"laravel-fmt.json": `{"ignore": ["!CLAUDE.md", "extra/"]}`,
	} {
		seedFileAt(t, root, name, body)
	}

	ctx, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	loaded, err := LoadConfig(root, ctx)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	builtIn := config.Default().Ignore
	want := slices.Concat(builtIn, []string{"!CLAUDE.md", "extra/"})

	if !slices.Equal(loaded.File.Ignore, want) {
		t.Fatalf("Ignore =\n%q\nwant\n%q", loaded.File.Ignore, want)
	}

	wantAdopted := []config.IgnoreLayer{
		{Patterns: []string{"# generated", "CLAUDE.md", "", "!keep.md"}, IgnoreCase: true},
		{Patterns: []string{"**/resources/views/flux/"}},
	}

	sameLayer := func(a, b config.IgnoreLayer) bool {
		return a.IgnoreCase == b.IgnoreCase && slices.Equal(a.Patterns, b.Patterns)
	}

	if !slices.EqualFunc(loaded.File.AdoptedIgnore, wantAdopted, sameLayer) {
		t.Fatalf("AdoptedIgnore = %+v, want %+v", loaded.File.AdoptedIgnore, wantAdopted)
	}
}

func TestAdoptIgnoresSkipsAPintConfigFromAnotherDirectory(t *testing.T) {
	cfg := config.Default()
	ctx := Context{Pint: &Pint{Path: filepath.Join(t.TempDir(), "pint.json"), Exclude: []string{"build"}}}

	if err := adoptIgnores(cfg, ctx, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	if cfg.AdoptedIgnore != nil {
		t.Fatalf("adopted from another directory: %+v", cfg.AdoptedIgnore)
	}
}

func TestOxfmtOwnsTheFileTypesItFormatsWhenTheProjectDeclaresIt(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		body  string
		owned bool
	}{
		{"vite+ with a fmt block", "vite.config.ts", "import { defineConfig } from 'vite-plus';\n\nexport default defineConfig({\n    fmt: {\n        singleQuote: true,\n    },\n});\n", true},
		{"vite+ that only builds", "vite.config.js", "import { defineConfig } from 'vite-plus';\n\nexport default defineConfig({\n    plugins: [],\n});\n", false},
		{"plain vite", "vite.config.ts", "import { defineConfig } from 'vite';\n\nexport default defineConfig({\n    fmt: {},\n});\n", false},
		{"oxfmt config", ".oxfmtrc.json", "{}\n", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			seedFileAt(t, root, tc.file, tc.body)

			cfg := config.Default()
			if err := adoptIgnores(cfg, Context{}, root); err != nil {
				t.Fatal(err)
			}

			owned := slices.ContainsFunc(cfg.AdoptedIgnore, func(layer config.IgnoreLayer) bool {
				return slices.Equal(layer.Patterns, oxfmtLanguages)
			})

			if owned != tc.owned {
				t.Fatalf("Oxfmt owns the frontend files = %v, want %v (adopted %+v)", owned, tc.owned, cfg.AdoptedIgnore)
			}
		})
	}
}

func TestAUserNegationKeepsAFileTypeOxfmtOwns(t *testing.T) {
	root := t.TempDir()

	for name, body := range map[string]string{
		".oxfmtrc.json":    "{}\n",
		"laravel-fmt.json": `{"ignore": ["!*.md"]}`,
		"app/Model.php":    "<?php\n",
		"README.md":        "# Title\n",
		"resources/a.css":  "a {}\n",
	} {
		seedFileAt(t, root, name, body)
	}

	ctx, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	loaded, err := LoadConfig(root, ctx)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	paths, err := walk.Files(root, root, &loaded.File, nil, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	got := make([]string, 0, len(paths))

	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}

		got = append(got, filepath.ToSlash(rel))
	}

	if want := []string{"README.md", "app/Model.php"}; !slices.Equal(got, want) {
		t.Fatalf("formatted %q, want %q", got, want)
	}
}

func TestPrettierIgnoreIsReadAsGitignoreSyntax(t *testing.T) {
	root := t.TempDir()
	seedFileAt(t, root, ".prettierignore", "\uFEFFfirst.md\n lead.md\ntrail.md\\ \n")

	ctx, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	loaded, err := LoadConfig(root, ctx)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	cases := []struct {
		name        string
		file        string
		want        bool
		trailsSpace bool
	}{
		{"the pattern after a BOM", "first.md", true, false},
		{"a leading space is part of the name", " lead.md", true, false},
		{"a leading space does not match without it", "lead.md", false, false},
		{"an escaped trailing space is kept", "trail.md ", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.trailsSpace && runtime.GOOS == "windows" {
				t.Skip("Windows drops a trailing space from a path")
			}

			got, err := walk.IgnoresFile(root, &loaded.File, filepath.Join(root, tc.file))
			if err != nil {
				t.Fatalf("IgnoresFile: %v", err)
			}

			if got != tc.want {
				t.Fatalf("IgnoresFile(%q) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}
