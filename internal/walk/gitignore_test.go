package walk

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestIgnoreMatcher(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		path     string
		isDir    bool
		want     bool
	}{
		{"unanchored matches at root", []string{"foo"}, "foo", false, true},
		{"unanchored matches at depth", []string{"foo"}, "bar/foo", false, true},
		{"unanchored does not match substring", []string{"foo"}, "foobar", false, false},

		{"anchored via leading slash matches root", []string{"/foo"}, "foo", false, true},
		{"anchored via leading slash does not match deep", []string{"/foo"}, "bar/foo", false, false},

		{"anchored via internal slash", []string{"foo/bar"}, "foo/bar", false, true},
		{"anchored via internal slash rejects depth prefix", []string{"foo/bar"}, "baz/foo/bar", false, false},

		{"trailing slash dir-only matches dir", []string{"foo/"}, "foo", true, true},
		{"trailing slash dir-only rejects file", []string{"foo/"}, "foo", false, false},

		{"double star matches deep", []string{"foo/**/baz"}, "foo/a/b/baz", false, true},
		{"double star matches zero segments", []string{"foo/**/baz"}, "foo/baz", false, true},

		{"glob within segment", []string{"*.tmp"}, "a/b/c.tmp", false, true},
		{"glob within segment rejects wrong ext", []string{"*.tmp"}, "a/b/c.log", false, false},

		{"negation re-includes", []string{"vendor/**", "!vendor/keep/**"}, "vendor/keep/file.php", false, false},
		{"negation does not re-include other path", []string{"vendor/**", "!vendor/keep/**"}, "vendor/drop/file.php", false, true},

		{"last match wins on conflicting rules", []string{"!foo", "foo"}, "foo", false, true},

		{"blank lines and comments are skipped", []string{"", "  ", "# comment", "foo"}, "foo", false, true},

		{"escaped hash names a file", []string{`\#notes.md`}, "#notes.md", false, true},
		{"escaped star is literal", []string{`a\*.php`}, "a*.php", false, true},
		{"escaped star matches no other name", []string{`a\*.php`}, "ab.php", false, false},

		{"trailing double star leaves the directory itself", []string{"foo/**"}, "foo", true, false},
		{"trailing double star matches inside the directory", []string{"foo/**"}, "foo/a/b.php", false, true},

		{"leading space is part of the pattern", []string{" lead.php"}, " lead.php", false, true},
		{"leading space does not match the unspaced name", []string{" lead.php"}, "lead.php", false, false},
		{"trailing spaces are dropped", []string{"foo.php  "}, "foo.php", false, true},
		{"escaped trailing space is kept", []string{`foo\ `}, "foo ", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := compileMatcher(tc.patterns)

			got := m.match(tc.path, tc.isDir)

			if got != tc.want {
				t.Fatalf("match(%q, isDir=%v) with %v = %v, want %v", tc.path, tc.isDir, tc.patterns, got, tc.want)
			}
		})
	}
}

// Layered as Prettier keeps .prettierignore apart from .gitignore, checked
// against prettier --file-info.
func TestAdoptedIgnoreListsAreLayersOfTheirOwn(t *testing.T) {
	root := buildTree(t, map[string]string{
		".gitignore":        "gitkept/x.php\n",
		"keep/manual.php":   "<?php",
		"keep/gen.php":      "<?php",
		"keep/pint.php":     "<?php",
		"gitkept/x.php":     "<?php",
		"vendor/pkg/a.php":  "<?php",
		"dirout/manual.php": "<?php",
		"own/a.php":         "<?php",
	})

	cfg := config.Default()
	cfg.Ignore = append(cfg.Ignore, "!own/a.php")
	cfg.AdoptedIgnore = [][]string{
		{
			"keep/*.php", "!keep/manual.php", "!keep/pint.php",
			"!gitkept/x.php", "!vendor/pkg/a.php",
			"dirout/", "!dirout/manual.php",
			"own/a.php",
		},
		{"keep/pint.php"},
	}

	paths, err := Files(root, root, cfg, nil, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	got := make([]string, 0, len(paths))

	for _, path := range paths {
		got = append(got, relSlash(root, path))
	}

	want := []string{"keep/manual.php", "own/a.php"}

	if !slices.Equal(got, want) {
		t.Fatalf("walk selected %v, want %v", got, want)
	}

	skip, err := IgnoreFilter(root, root, cfg, nil)
	if err != nil {
		t.Fatalf("IgnoreFilter: %v", err)
	}

	for _, rel := range []string{
		"keep/manual.php", "keep/gen.php", "keep/pint.php", "gitkept/x.php",
		"vendor/pkg/a.php", "dirout/manual.php", "own/a.php",
	} {
		ignored, err := IgnoresFile(root, cfg, filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("IgnoresFile(%s): %v", rel, err)
		}

		if ignored == slices.Contains(want, rel) {
			t.Errorf("IgnoresFile(%s) = %v, the walk selected it = %v", rel, ignored, !ignored)
		}

		if skipped := mustSkip(t, skip, rel); skipped == slices.Contains(want, rel) {
			t.Errorf("IgnoreFilter(%s) = %v, the walk selected it = %v", rel, skipped, !skipped)
		}
	}
}
