package walk

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// A fixture must not pick up the host's files above the temp directory.
func init() {
	upward.SetCeiling(os.TempDir())
}

func TestFilesHonorsCfgIgnore(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/User.php":          "<?php",
		"vendor/foo/Bar.php":    "<?php",
		"resources/views/x.php": "<?php",
	})

	cfg := config.Default()

	got := mustFiles(t, root, cfg)

	if containsRel(got, root, "vendor/foo/Bar.php") {
		t.Fatalf("vendor file leaked into walk result: %v", got)
	}

	if !containsRel(got, root, "app/User.php") {
		t.Fatalf("app file missing from walk result: %v", got)
	}
}

func TestIgnoreFilter(t *testing.T) {
	root := buildTree(t, map[string]string{
		".gitignore": "secret.php\n",
	})

	skip, err := IgnoreFilter(root, root, config.Default(), nil)
	if err != nil {
		t.Fatalf("IgnoreFilter: %v", err)
	}

	cases := []struct {
		path string
		want bool
	}{
		{"vendor/foo/Bar.php", true},
		{"secret.php", true},
		{"app/User.php", false},
	}

	for _, tc := range cases {
		if got := mustSkip(t, skip, tc.path); got != tc.want {
			t.Errorf("IgnoreFilter(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIgnoreFilterExcludesFilesUnderDirOnlyPattern(t *testing.T) {
	root := buildTree(t, nil)

	cfg := config.Default()
	cfg.Ignore = []string{"build/"}

	skip, err := IgnoreFilter(root, root, cfg, nil)
	if err != nil {
		t.Fatalf("IgnoreFilter: %v", err)
	}

	if !mustSkip(t, skip, "build/x.php") {
		t.Error("dir-only pattern build/ did not exclude build/x.php")
	}

	if mustSkip(t, skip, "app/x.php") {
		t.Error("dir-only pattern build/ wrongly excluded app/x.php")
	}
}

func TestIgnoreFilterParentExclusionBeatsNegation(t *testing.T) {
	root := buildTree(t, nil)

	cfg := config.Default()
	cfg.Ignore = []string{"build/", "!build/keep.php"}

	skip, err := IgnoreFilter(root, root, cfg, nil)
	if err != nil {
		t.Fatalf("IgnoreFilter: %v", err)
	}

	if !mustSkip(t, skip, "build/keep.php") {
		t.Error("file under an excluded directory must stay excluded despite a file-level negation")
	}
}

// The cases run in order: a deep file loads its chain before a shallow one.
func TestIgnoreFilterAppliesNestedGitignores(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/legacy/.gitignore": "Old.php\n",
		"app/.gitignore":        "gen/*\n",
		"app/gen/.gitignore":    "!keep.php\n",
		"app/Keep.php":          "<?php",
	})

	skip, err := IgnoreFilter(root, root, nil, nil)
	if err != nil {
		t.Fatalf("IgnoreFilter: %v", err)
	}

	cases := []struct {
		path string
		want bool
	}{
		{"app/gen/keep.php", false},
		{"app/gen/drop.php", true},
		{"app/legacy/Old.php", true},
		{"app/Old.php", false},
		{"app/Keep.php/x.php", false},
	}

	for _, tc := range cases {
		if got := mustSkip(t, skip, tc.path); got != tc.want {
			t.Errorf("skip(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIgnoreFilterReportsAnUnreadableNestedGitignore(t *testing.T) {
	root := buildTree(t, nil)

	if err := os.MkdirAll(filepath.Join(root, "app", ".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}

	skip, err := IgnoreFilter(root, root, nil, nil)
	if err != nil {
		t.Fatalf("IgnoreFilter: %v", err)
	}

	if _, err := skip("app/User.php"); err == nil {
		t.Fatal("skip read past an unreadable app/.gitignore; want an error")
	}
}

func mustSkip(t *testing.T, skip func(string) (bool, error), relPath string) bool {
	t.Helper()

	skipped, err := skip(relPath)
	if err != nil {
		t.Fatalf("skip(%q): %v", relPath, err)
	}

	return skipped
}

func TestFilesSkipsHiddenDirs(t *testing.T) {
	root := buildTree(t, map[string]string{
		".github/workflows/ci.yml": "on: push",
		".cursor/rules.json":       "{}",
		"app/User.php":             "<?php",
	})

	got := mustFiles(t, root, config.Default())

	if containsRel(got, root, ".github/workflows/ci.yml") || containsRel(got, root, ".cursor/rules.json") {
		t.Fatalf("hidden directory file leaked into walk result: %v", got)
	}

	if !containsRel(got, root, "app/User.php") {
		t.Fatalf("app file missing from walk result: %v", got)
	}
}

func TestFilesSkipsRootDotfile(t *testing.T) {
	root := buildTree(t, map[string]string{
		".prettierrc.json": "{}",
		"app/User.php":     "<?php",
	})

	got := mustFiles(t, root, config.Default())

	if containsRel(got, root, ".prettierrc.json") {
		t.Fatalf("root dotfile leaked into walk result: %v", got)
	}
}

func TestFilesExplicitHiddenDirArgIncluded(t *testing.T) {
	root := buildTree(t, map[string]string{
		".github/workflows/ci.yml": "on: push",
	})

	got, err := Files(root, root, config.Default(), []string{filepath.Join(root, ".github")}, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if !containsRel(got, root, ".github/workflows/ci.yml") {
		t.Fatalf("explicitly named hidden directory was not formatted: %v", got)
	}
}

func TestIgnoreFilterSkipsHiddenSegments(t *testing.T) {
	root := buildTree(t, nil)

	skip, err := IgnoreFilter(root, root, config.Default(), nil)
	if err != nil {
		t.Fatalf("IgnoreFilter: %v", err)
	}

	if !mustSkip(t, skip, ".github/workflows/ci.yml") {
		t.Error("IgnoreFilter did not skip a file under a hidden directory")
	}

	if mustSkip(t, skip, "app/User.php") {
		t.Error("IgnoreFilter wrongly skipped a non-hidden file")
	}
}

func TestFilesRespectsGitignore(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/User.php":   "<?php",
		"app/Secret.php": "<?php",
		".gitignore":     "app/Secret.php\n",
	})

	got := mustFiles(t, root, config.Default())

	if containsRel(got, root, "app/Secret.php") {
		t.Fatalf("gitignored file leaked into walk: %v", got)
	}

	if !containsRel(got, root, "app/User.php") {
		t.Fatalf("non-ignored file missing: %v", got)
	}
}

// Fixture uses a file-level rule, not a directory rule: a !pattern cannot
// re-include a file inside an excluded directory.
func TestFilesCfgIgnoreNegationOverridesGitignore(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/Secret.php": "<?php",
		"app/Public.php": "<?php",
		".gitignore":     "app/Secret.php\n",
	})

	cfg := config.Default()
	cfg.Ignore = []string{"!app/Secret.php"}

	got := mustFiles(t, root, cfg)

	if !containsRel(got, root, "app/Secret.php") {
		t.Fatalf("cfg.Ignore negation did not re-include file: %v", got)
	}

	if !containsRel(got, root, "app/Public.php") {
		t.Fatalf("non-ignored file missing: %v", got)
	}
}

func TestGitignoreNegationReIncludesAFileUnderATrailingDoubleStar(t *testing.T) {
	root := buildTree(t, map[string]string{
		"reports/keep.php": "<?php",
		"reports/drop.php": "<?php",
		".gitignore":       "reports/**\n!reports/keep.php\n",
	})

	got := mustFiles(t, root, config.Default())

	if !containsRel(got, root, "reports/keep.php") {
		t.Fatalf("re-included file missing: %v", got)
	}

	if containsRel(got, root, "reports/drop.php") {
		t.Fatalf("ignored file leaked into walk: %v", got)
	}
}

func TestFilesExpandsGlobArg(t *testing.T) {
	root := buildTree(t, map[string]string{
		"resources/views/home.blade.php":    "<div></div>",
		"resources/views/admin/x.blade.php": "<div></div>",
		"resources/views/home.php":          "<?php",
		"app/User.php":                      "<?php",
	})
	t.Chdir(root)

	got, err := Files(root, root, config.Default(), []string{"resources/**/*.blade.php"}, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if !containsRel(got, root, "resources/views/home.blade.php") ||
		!containsRel(got, root, "resources/views/admin/x.blade.php") {
		t.Fatalf("glob did not match expected blade files: %v", got)
	}

	if containsRel(got, root, "resources/views/home.php") || containsRel(got, root, "app/User.php") {
		t.Fatalf("glob matched files outside the pattern: %v", got)
	}
}

func TestFilesGlobArgRespectsIgnore(t *testing.T) {
	root := buildTree(t, map[string]string{
		"resources/views/keep.blade.php": "<div></div>",
		"resources/views/skip.blade.php": "<div></div>",
		".gitignore":                     "resources/views/skip.blade.php\n",
	})
	t.Chdir(root)

	got, err := Files(root, root, config.Default(), []string{"resources/**/*.blade.php"}, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if containsRel(got, root, "resources/views/skip.blade.php") {
		t.Fatalf("glob expansion included an ignored file: %v", got)
	}

	if !containsRel(got, root, "resources/views/keep.blade.php") {
		t.Fatalf("glob expansion dropped a non-ignored match: %v", got)
	}
}

func TestFilesListsEachFileOnceWhenArgumentsOverlap(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/Models/User.php":         "<?php",
		"app/Other.php":               "<?php",
		"app/Ignored.php":             "<?php",
		"resources/views/x.blade.php": "<div></div>",
	})
	t.Chdir(root)

	cfg := config.Default()
	cfg.Ignore = append(cfg.Ignore, "app/Ignored.php")

	absUser := filepath.Join(root, "app", "Models", "User.php")

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "overlapping globs",
			args: []string{"**/*.blade.php", "resources/**/*.blade.php"},
			want: []string{"resources/views/x.blade.php"},
		},
		{
			name: "directory and a file inside it",
			args: []string{"app", "app/Models/User.php"},
			want: []string{"app/Models/User.php", "app/Other.php"},
		},
		{
			name: "file and the directory holding it",
			args: []string{"app/Other.php", "app"},
			want: []string{"app/Other.php", "app/Models/User.php"},
		},
		{
			name: "directory and its subdirectory",
			args: []string{"app", "app/Models"},
			want: []string{"app/Models/User.php", "app/Other.php"},
		},
		{
			name: "same directory twice",
			args: []string{"app", "./app"},
			want: []string{"app/Models/User.php", "app/Other.php"},
		},
		{
			name: "file spelled relative and absolute",
			args: []string{"app/Models/User.php", absUser},
			want: []string{"app/Models/User.php"},
		},
		{
			name: "directory and a glob inside it",
			args: []string{"app", "app/**/*.php"},
			want: []string{"app/Models/User.php", "app/Other.php"},
		},
		{
			name: "glob and a file it matches",
			args: []string{"app/**/*.php", "./app/Models/User.php"},
			want: []string{"app/Models/User.php", "app/Other.php"},
		},
		{
			name: "ignored file named next to its directory",
			args: []string{"app", "app/Ignored.php"},
			want: []string{"app/Models/User.php", "app/Other.php", "app/Ignored.php"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths, err := Files(root, root, cfg, tc.args, nil)
			if err != nil {
				t.Fatalf("Files: %v", err)
			}

			got := make([]string, 0, len(paths))

			for _, path := range paths {
				rel, ok := relFromAbs(root, path)
				if !ok {
					t.Fatalf("cannot relate %s to %s", path, root)
				}

				got = append(got, rel)
			}

			if !slices.Equal(got, tc.want) {
				t.Fatalf("Files(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestFilesReturnsAbsolutePaths(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/User.php":                "<?php",
		"resources/views/x.blade.php": "<div></div>",
		"routes/web.php":              "<?php",
	})
	t.Chdir(root)

	user := filepath.Join(root, "app", "User.php")
	view := filepath.Join(root, "resources", "views", "x.blade.php")
	routes := filepath.Join(root, "routes", "web.php")

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "no arguments", want: []string{user, view, routes}},
		{
			name: "relative directory, file and glob",
			args: []string{"app", "routes/web.php", "resources/**/*.blade.php"},
			want: []string{user, routes, view},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Files(".", ".", config.Default(), tc.args, nil)
			if err != nil {
				t.Fatalf("Files: %v", err)
			}

			if !slices.Equal(got, tc.want) {
				t.Fatalf("Files(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestFilesGlobArgNoMatchErrors(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/User.php": "<?php",
	})
	t.Chdir(root)

	if _, err := Files(root, root, config.Default(), []string{"resources/**/*.blade.php"}, nil); err == nil {
		t.Fatal("Files accepted a glob matching no files; want an error")
	}
}

func TestFilesResolvesGlobArgsFromTheWorkingDirectory(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/A.php":     "<?php",
		"app/sub/B.php": "<?php",
	})
	app := filepath.Join(root, "app")
	t.Chdir(app)

	cases := []struct {
		name    string
		pattern string
		want    []string
	}{
		{name: "relative", pattern: "sub/*.php", want: []string{"app/sub/B.php"}},
		{name: "dot prefix", pattern: "./sub/*.php", want: []string{"app/sub/B.php"}},
		{name: "parent segment", pattern: "../app/*.php", want: []string{"app/A.php"}},
		{name: "absolute", pattern: filepath.Join(app, "**", "*.php"), want: []string{"app/A.php", "app/sub/B.php"}},
		{name: "star stays in its directory", pattern: "*.php", want: []string{"app/A.php"}},
		{name: "double star descends", pattern: "**/*.php", want: []string{"app/A.php", "app/sub/B.php"}},
		{name: "glob in a middle segment", pattern: "*/B.php", want: []string{"app/sub/B.php"}},
	}

	if runtime.GOOS == "windows" {
		cases = append(cases, struct {
			name    string
			pattern string
			want    []string
		}{name: "backslash separators", pattern: `sub\*.php`, want: []string{"app/sub/B.php"}})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths, err := Files(root, root, config.Default(), []string{tc.pattern}, nil)
			if err != nil {
				t.Fatalf("Files(%q): %v", tc.pattern, err)
			}

			got := make([]string, 0, len(paths))

			for _, path := range paths {
				got = append(got, relSlash(root, path))
			}

			if !slices.Equal(got, tc.want) {
				t.Fatalf("Files(%q) = %v, want %v", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestFilesRejectsGlobArgsThatDoNotNameAPath(t *testing.T) {
	root := buildTree(t, map[string]string{
		"app/A.php": "<?php",
	})
	t.Chdir(filepath.Join(root, "app"))

	for _, pattern := range []string{"!*.php", "app/*.php", "[.php"} {
		if got, err := Files(root, root, config.Default(), []string{pattern}, nil); err == nil {
			t.Errorf("Files(%q) = %v, want an error", pattern, got)
		}
	}
}

func buildTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()

	for rel, contents := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}

		if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	return root
}

func mustFiles(t *testing.T, root string, cfg *config.File) []string {
	t.Helper()

	got, err := Files(root, root, cfg, nil, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	return got
}

func containsRel(paths []string, root, rel string) bool {
	want := filepath.Join(root, filepath.FromSlash(rel))

	return slices.Contains(paths, want)
}

func TestFilesReportsAMissingPathOnce(t *testing.T) {
	root := buildTree(t, map[string]string{"app/User.php": "<?php"})
	missing := filepath.Join(root, "no-such-path")

	_, err := Files(root, root, config.Default(), []string{missing}, nil)
	if err == nil {
		t.Fatal("expected an error for a path that does not exist")
	}

	msg := err.Error()

	if n := strings.Count(msg, missing); n != 1 {
		t.Fatalf("path appears %d times, want 1: %s", n, msg)
	}

	if n := strings.Count(msg, "stat "); n != 1 {
		t.Fatalf("%q appears %d times, want 1: %s", "stat ", n, msg)
	}
}

func TestFilesHonorsCfgIgnoreWhenGivenASubdirectory(t *testing.T) {
	const (
		ignored = "resources/views/generated/gen.blade.php"
		kept    = "resources/views/pages/page.blade.php"
	)

	check := func(t *testing.T, root, ignoreRoot, arg string) {
		t.Helper()

		cfg := config.Default()
		cfg.Ignore = append(cfg.Ignore, "resources/views/generated/**")

		got, err := Files(root, ignoreRoot, cfg, []string{arg}, nil)
		if err != nil {
			t.Fatalf("Files: %v", err)
		}

		for _, path := range got {
			if strings.Contains(filepath.ToSlash(path), "generated/") {
				t.Fatalf("ignored file leaked in with argument %q: %v", arg, got)
			}
		}

		if len(got) != 1 {
			t.Fatalf("argument %q selected %v, want only the one non-ignored file", arg, got)
		}
	}

	t.Run("absolute argument", func(t *testing.T) {
		root := buildTree(t, map[string]string{ignored: "<div></div>", kept: "<div></div>"})
		check(t, root, root, filepath.Join(root, "resources"))
	})

	t.Run("relative argument from the project root", func(t *testing.T) {
		root := buildTree(t, map[string]string{ignored: "<div></div>", kept: "<div></div>"})
		t.Chdir(root)
		check(t, root, root, "resources")
	})

	t.Run("relative argument from a subdirectory", func(t *testing.T) {
		root := buildTree(t, map[string]string{ignored: "<div></div>", kept: "<div></div>"})
		t.Chdir(filepath.Join(root, "resources"))
		check(t, root, root, "views")
	})

	// Outside a Laravel project the traversal root is the working directory,
	// which can sit below the config's directory.
	t.Run("traversal root below the ignore root", func(t *testing.T) {
		root := buildTree(t, map[string]string{ignored: "<div></div>", kept: "<div></div>"})
		check(t, filepath.Join(root, "resources"), root, filepath.Join(root, "resources", "views"))
	})

	// A glob resolves from the working directory, like a path argument, while
	// its candidates still come from a walk that ignores from the config's
	// directory.
	t.Run("glob with the traversal root below the ignore root", func(t *testing.T) {
		root := buildTree(t, map[string]string{ignored: "<div></div>", kept: "<div></div>"})
		t.Chdir(filepath.Join(root, "resources"))
		check(t, filepath.Join(root, "resources"), root, "views/**/*.blade.php")
	})
}

func TestIgnoreRootPrefersTheConfigDirectoryThatContainsTheRoot(t *testing.T) {
	root := buildTree(t, nil)
	sub := filepath.Join(root, "resources")

	cases := []struct {
		name       string
		root       string
		configPath string
		want       string
	}{
		{"config above the root", sub, filepath.Join(root, "laravel-fmt.json"), root},
		{"config at the root", root, filepath.Join(root, "laravel-fmt.json"), root},
		{"config below the root", root, filepath.Join(sub, "laravel-fmt.json"), root},
		{"no config", sub, "", sub},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IgnoreRoot(tc.root, tc.configPath); got != tc.want {
				t.Fatalf("IgnoreRoot(%q, %q) = %q, want %q", tc.root, tc.configPath, got, tc.want)
			}
		})
	}
}

func TestFilesHonorsNestedGitignore(t *testing.T) {
	root := buildTree(t, map[string]string{
		".gitignore":                      "root-only.blade.php\n",
		"root-only.blade.php":             "<div></div>",
		"resources/views/.gitignore":      "local.blade.php\n",
		"resources/views/local.blade.php": "<div></div>",
		"resources/views/keep.blade.php":  "<div></div>",
		"resources/other/local.blade.php": "<div></div>",
	})

	got := mustFiles(t, root, config.Default())

	for _, rel := range []string{"root-only.blade.php", "resources/views/local.blade.php"} {
		if containsRel(got, root, rel) {
			t.Fatalf("%s is ignored but was selected: %v", rel, got)
		}
	}

	// A sibling directory names the same file and must be untouched by the
	// other directory's ignore.
	for _, rel := range []string{"resources/views/keep.blade.php", "resources/other/local.blade.php"} {
		if !containsRel(got, root, rel) {
			t.Fatalf("%s is not ignored anywhere but was dropped: %v", rel, got)
		}
	}
}

// A lone * is what Laravel ships in storage/framework/views/.gitignore.
func TestNestedGitignoreCannotReachOutsideItsDirectory(t *testing.T) {
	root := buildTree(t, map[string]string{
		"build/.gitignore":       "*\n",
		"build/out.blade.php":    "<div></div>",
		"build/deep/x.blade.php": "<div></div>",
		"app/User.php":           "<?php",
		"index.blade.php":        "<div></div>",
	})

	got := mustFiles(t, root, config.Default())

	for _, rel := range []string{"build/out.blade.php", "build/deep/x.blade.php"} {
		if containsRel(got, root, rel) {
			t.Fatalf("%s is under a `*` ignore but was selected: %v", rel, got)
		}
	}

	for _, rel := range []string{"app/User.php", "index.blade.php"} {
		if !containsRel(got, root, rel) {
			t.Fatalf("a nested `*` reached outside its directory and dropped %s: %v", rel, got)
		}
	}
}

func TestNestedGitignoreNegationReIncludesWithinItsScope(t *testing.T) {
	root := buildTree(t, map[string]string{
		"build/.gitignore":     "*\n!keep.blade.php\n",
		"build/drop.blade.php": "<div></div>",
		"build/keep.blade.php": "<div></div>",
	})

	got := mustFiles(t, root, config.Default())

	if containsRel(got, root, "build/drop.blade.php") {
		t.Fatalf("the `*` did not apply: %v", got)
	}

	if !containsRel(got, root, "build/keep.blade.php") {
		t.Fatalf("the negation did not re-include its file: %v", got)
	}
}

func TestCfgIgnoreOverridesANestedGitignore(t *testing.T) {
	root := buildTree(t, map[string]string{
		"build/.gitignore":     "*\n!keep.blade.php\n",
		"build/keep.blade.php": "<div></div>",
	})

	cfg := config.Default()
	cfg.Ignore = append(cfg.Ignore, "build/**")

	got, err := Files(root, root, cfg, nil, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if containsRel(got, root, "build/keep.blade.php") {
		t.Fatalf("an explicit ignore lost to a .gitignore negation: %v", got)
	}
}

func TestNestedGitignoreAppliesToASubdirectoryArgument(t *testing.T) {
	root := buildTree(t, map[string]string{
		"resources/.gitignore":            "generated/\n",
		"resources/views/x.blade.php":     "<div></div>",
		"resources/generated/y.blade.php": "<div></div>",
	})

	got, err := Files(root, root, config.Default(), []string{filepath.Join(root, "resources")}, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if containsRel(got, root, "resources/generated/y.blade.php") {
		t.Fatalf("an ignore above the argument was skipped: %v", got)
	}

	if !containsRel(got, root, "resources/views/x.blade.php") {
		t.Fatalf("the non-ignored file was dropped: %v", got)
	}
}

func TestBuiltInIgnoresCoverNestedDependencyTrees(t *testing.T) {
	root := buildTree(t, map[string]string{
		"packages/a/node_modules/dep/package.json": "{}",
		"packages/a/vendor/lib/composer.json":      "{}",
		"packages/a/src/storage/keep.json":         "{}",
		"packages/a/own.json":                      "{}",
		"storage/framework/app.json":               "{}",
	})

	got := mustFiles(t, root, config.Default())

	for _, rel := range []string{
		"packages/a/node_modules/dep/package.json",
		"packages/a/vendor/lib/composer.json",
		"storage/framework/app.json",
	} {
		if containsRel(got, root, rel) {
			t.Fatalf("%s should be ignored by the built-ins: %v", rel, got)
		}
	}

	for _, rel := range []string{"packages/a/own.json", "packages/a/src/storage/keep.json"} {
		if !containsRel(got, root, rel) {
			t.Fatalf("%s is the project's own code and must be formatted: %v", rel, got)
		}
	}
}

// Creating a symlink needs a privilege Windows does not grant by default, so
// this reports whether it worked.
func symlinkOutsideFile(t *testing.T, root, rel string) bool {
	t.Helper()

	target := filepath.Join(t.TempDir(), "Target.php")

	if err := os.WriteFile(target, []byte("<?php\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	return os.Symlink(target, filepath.Join(root, filepath.FromSlash(rel))) == nil
}

// Windows falls back to a directory junction, which needs no privilege;
// either way the link is not a regular file. mklink is a cmd builtin and
// cannot be exec'd on its own.
func linkOutside(t *testing.T, root, rel string) bool {
	t.Helper()

	if symlinkOutsideFile(t, root, rel) {
		return true
	}

	if runtime.GOOS != "windows" {
		return false
	}

	link := filepath.Join(root, filepath.FromSlash(rel))

	//nolint:gosec // both paths are t.TempDir() output, not input
	return exec.Command("cmd", "/c", "mklink", "/J", link, t.TempDir()).Run() == nil
}

func TestFilesSkipsLinksFoundByTraversal(t *testing.T) {
	root := buildTree(t, map[string]string{"app/User.php": "<?php"})

	if !linkOutside(t, root, "app/Escaped.php") {
		t.Skip("this platform will not create a link without extra privileges")
	}

	got := mustFiles(t, root, config.Default())

	if containsRel(got, root, "app/Escaped.php") {
		t.Fatalf("a link reached the walk result: %v", got)
	}

	if !containsRel(got, root, "app/User.php") {
		t.Fatalf("regular file missing from walk result: %v", got)
	}
}

func TestFilesExplicitLinkArgIsFormatted(t *testing.T) {
	root := buildTree(t, map[string]string{"app/User.php": "<?php"})
	link := filepath.Join(root, "app", "Named.php")

	if !symlinkOutsideFile(t, root, "app/Named.php") {
		t.Skip("this platform will not create a file symlink without extra privileges")
	}

	got, err := Files(root, root, config.Default(), []string{link}, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if !slices.Contains(got, link) {
		t.Fatalf("explicitly named link was dropped: %v", got)
	}
}

func TestIgnoresFileAgreesWithTheWalk(t *testing.T) {
	root := buildTree(t, map[string]string{
		".gitignore":                      "app/Secret.php\npublic/build/\n",
		"app/User.php":                    "<?php",
		"app/Secret.php":                  "<?php",
		"app/Kept.php":                    "<?php",
		"generated/a.php":                 "<?php",
		"vendor/foo/Bar.php":              "<?php",
		"composer.lock":                   "{}",
		"public/build/manifest.json":      "{}",
		"resources/views/.gitignore":      "local.blade.php\n",
		"resources/views/local.blade.php": "<div></div>",
		"resources/views/keep.blade.php":  "<div></div>",
	})

	cfg := config.Default()
	cfg.Ignore = append(cfg.Ignore, "generated/**", "!app/Kept.php")

	selected := mustFiles(t, root, cfg)
	candidates := []string{
		"app/User.php", "app/Secret.php", "app/Kept.php", "generated/a.php",
		"vendor/foo/Bar.php", "composer.lock", "public/build/manifest.json",
		"resources/views/local.blade.php", "resources/views/keep.blade.php",
	}

	for _, rel := range candidates {
		ignored, err := IgnoresFile(root, cfg, filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("IgnoresFile %s: %v", rel, err)
		}

		if walked := containsRel(selected, root, rel); ignored == walked {
			t.Errorf("%s: IgnoresFile says ignored=%v, the walk selected=%v", rel, ignored, walked)
		}
	}
}

func TestIgnoresFileDoesNotSkipDotfiles(t *testing.T) {
	root := buildTree(t, map[string]string{".php-cs-fixer.dist.php": "<?php"})

	ignored, err := IgnoresFile(root, config.Default(), filepath.Join(root, ".php-cs-fixer.dist.php"))
	if err != nil {
		t.Fatalf("IgnoresFile: %v", err)
	}

	if ignored {
		t.Fatal("a dotfile was treated as ignored")
	}
}

func TestFilesSkipsGeneratedIdeHelperFiles(t *testing.T) {
	root := buildTree(t, map[string]string{
		"_ide_helper.php":        "<?php",
		"_ide_helper_models.php": "<?php",
		"app/Helpers/helper.php": "<?php",
	})

	got := mustFiles(t, root, config.Default())

	for _, rel := range []string{"_ide_helper.php", "_ide_helper_models.php"} {
		if containsRel(got, root, rel) {
			t.Fatalf("%s is generated but was selected: %v", rel, got)
		}
	}

	if !containsRel(got, root, "app/Helpers/helper.php") {
		t.Fatalf("an ordinary helper was dropped: %v", got)
	}
}

// projectsAt fakes the resolver: each directory belongs to the deepest listed
// anchor at or above it, and the tree root is the run's own project.
func projectsAt(root string, anchors map[string][]string) ProjectIgnore {
	return func(dir string) (string, *config.File, error) {
		best, patterns := root, []string(nil)

		for rel, p := range anchors {
			anchor := filepath.Join(root, filepath.FromSlash(rel))
			if within(anchor, dir) && len(anchor) > len(best) {
				best, patterns = anchor, p
			}
		}

		return best, &config.File{Ignore: patterns}, nil
	}
}

func relFiles(root, sub string, files []string) []string {
	var out []string

	for _, f := range files {
		rel := relSlash(root, f)
		if sub == "" || strings.HasPrefix(rel, sub+"/") {
			out = append(out, rel)
		}
	}

	slices.Sort(out)

	return out
}

func TestProjectIgnoreSelectsTheSameFromAnyStart(t *testing.T) {
	tree := map[string]string{
		"a/laravel-fmt.json":            "{}",
		"a/generated/a.php":             "<?php",
		"a/generated/keep.php":          "<?php",
		"a/src/b.php":                   "<?php",
		"a/src/legacy/old.php":          "<?php",
		"a/resources/views/x.blade.php": "<div></div>",
		"top.php":                       "<?php",
	}

	for name, patterns := range map[string][]string{
		"directory glob":              {"generated/**"},
		"nested path":                 {"src/legacy/**"},
		"negation under an exclusion": {"generated/**", "!generated/keep.php"},
		"negation of a file":          {"*.php", "!src/b.php"},
		"dir-only pattern":            {"generated/"},
	} {
		t.Run(name, func(t *testing.T) {
			root := buildTree(t, tree)
			sub := filepath.Join(root, "a")
			projectOf := projectsAt(root, map[string][]string{"a": patterns})

			inside := &config.File{Ignore: patterns}

			fromInside, err := Files(sub, sub, inside, nil, nil)
			if err != nil {
				t.Fatalf("Files inside: %v", err)
			}

			fromParent, err := Files(root, root, &config.File{}, nil, projectOf)
			if err != nil {
				t.Fatalf("Files from the parent: %v", err)
			}

			namingSub, err := Files(root, root, &config.File{}, []string{sub}, projectOf)
			if err != nil {
				t.Fatalf("Files naming the project: %v", err)
			}

			want := relFiles(root, "a", fromInside)

			if got := relFiles(root, "a", fromParent); !slices.Equal(got, want) {
				t.Errorf("from the parent = %v, inside the project = %v", got, want)
			}

			if got := relFiles(root, "a", namingSub); !slices.Equal(got, want) {
				t.Errorf("naming the project = %v, inside the project = %v", got, want)
			}
		})
	}
}

func TestProjectIgnoreAppliesWhenTheWalkStartsBelowTheProjectRoot(t *testing.T) {
	root := buildTree(t, map[string]string{
		"a/src/legacy/old.php": "<?php",
		"a/src/b.php":          "<?php",
	})

	projectOf := projectsAt(root, map[string][]string{"a": {"src/legacy/**"}})

	got, err := Files(root, root, &config.File{}, []string{filepath.Join(root, "a", "src")}, projectOf)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if want := []string{"a/src/b.php"}; !slices.Equal(relFiles(root, "", got), want) {
		t.Fatalf("Files = %v, want %v", relFiles(root, "", got), want)
	}
}

// Project a's pattern names a path inside project a/b, which has its own
// configuration.
func TestProjectIgnoreComesFromTheNearestProject(t *testing.T) {
	root := buildTree(t, map[string]string{
		"a/b/generated/x.php": "<?php",
		"a/generated/y.php":   "<?php",
	})

	projectOf := projectsAt(root, map[string][]string{
		"a":   {"b/generated/**", "generated/**"},
		"a/b": nil,
	})

	got, err := Files(root, root, &config.File{}, nil, projectOf)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if want := []string{"a/b/generated/x.php"}; !slices.Equal(relFiles(root, "", got), want) {
		t.Fatalf("Files = %v, want %v", relFiles(root, "", got), want)
	}
}

func TestRunIgnoreStillExcludesASubproject(t *testing.T) {
	root := buildTree(t, map[string]string{
		"a/src/b.php": "<?php",
		"top.php":     "<?php",
	})

	projectOf := projectsAt(root, map[string][]string{"a": nil})

	got, err := Files(root, root, &config.File{Ignore: []string{"a/**"}}, nil, projectOf)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if want := []string{"top.php"}; !slices.Equal(relFiles(root, "", got), want) {
		t.Fatalf("Files = %v, want %v", relFiles(root, "", got), want)
	}
}

// generated/** ignores what is inside generated, not the directory, so the
// walk lists it and prunes every directory in it.
func TestProjectIgnorePrunesIgnoredDirectories(t *testing.T) {
	root := buildTree(t, map[string]string{
		"a/generated/deep/x.php": "<?php",
		"a/src/b.php":            "<?php",
	})

	inner := projectsAt(root, map[string][]string{"a": {"generated/**"}})
	asked := map[string]bool{}

	projectOf := func(dir string) (string, *config.File, error) {
		asked[relSlash(root, dir)] = true

		return inner(dir)
	}

	if _, err := Files(root, root, &config.File{}, nil, projectOf); err != nil {
		t.Fatalf("Files: %v", err)
	}

	if asked["a/generated/deep"] {
		t.Fatalf("the walk entered an ignored directory: asked about %v", asked)
	}
}

func TestIgnoreFilterAnchorsTheRunAndHoldsFilesToTheirProject(t *testing.T) {
	root := buildTree(t, nil)
	projectOf := projectsAt(root, map[string][]string{"packages/b": {"generated/**"}})

	skip, err := IgnoreFilter(root, filepath.Join(root, "app"), &config.File{Ignore: []string{"legacy/**"}}, projectOf)
	if err != nil {
		t.Fatalf("IgnoreFilter: %v", err)
	}

	cases := map[string]bool{
		"app/legacy/x.php":             true,
		"legacy/x.php":                 false,
		"packages/b/generated/y.php":   true,
		"packages/b/src/z.php":         false,
		"packages/b/generated/d/w.php": true,
	}

	for path, want := range cases {
		if got := mustSkip(t, skip, path); got != want {
			t.Errorf("skip(%q) = %v, want %v", path, got, want)
		}
	}
}
