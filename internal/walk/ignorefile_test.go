package walk

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestReadGitignoreMissing(t *testing.T) {
	root := t.TempDir()

	lines, err := readGitignore(root)
	if err != nil {
		t.Fatalf("readGitignore(missing) = %v, want nil error", err)
	}

	if lines != nil {
		t.Fatalf("readGitignore(missing) returned %v, want nil", lines)
	}
}

func TestIgnoreRootBelowTheWorkTreeTopAppliesTheGitignoresAbove(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		project string
		want    []string
	}{
		{
			name: "monorepo",
			files: map[string]string{
				".git/HEAD":              "",
				".gitignore":             "web/app/Gen.php\nCache.php\n/app/Keep.php\nweb/app/Back.php\n",
				"web/.gitignore":         "!app/Back.php\n",
				"web/app/Gen.php":        "<?php",
				"web/app/Keep.php":       "<?php",
				"web/app/Back.php":       "<?php",
				"web/app/deep/Cache.php": "<?php",
			},
			project: "web",
			want:    []string{"web/app/Back.php", "web/app/Keep.php"},
		},
		{
			name: "project directory excluded from above",
			files: map[string]string{
				".git/HEAD":              "",
				".gitignore":             "sandbox/\n",
				"sandbox/proj/app/A.php": "<?php",
			},
			project: "sandbox/proj",
			want:    nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := buildTree(t, tc.files)
			project := filepath.Join(root, filepath.FromSlash(tc.project))

			paths, err := Files(project, project, config.Default(), nil, nil)
			if err != nil {
				t.Fatalf("Files: %v", err)
			}

			var got []string

			for _, path := range paths {
				got = append(got, relSlash(root, path))
			}

			if !slices.Equal(got, tc.want) {
				t.Fatalf("walk selected %v, want %v", got, tc.want)
			}

			for rel := range tc.files {
				if filepath.Ext(rel) != ".php" {
					continue
				}

				ignored, err := IgnoresFile(project, nil, filepath.Join(root, filepath.FromSlash(rel)))
				if err != nil {
					t.Fatalf("IgnoresFile(%s): %v", rel, err)
				}

				if ignored == slices.Contains(tc.want, rel) {
					t.Errorf("IgnoresFile(%s) = %v, the walk selected it = %v", rel, ignored, !ignored)
				}
			}
		})
	}
}

func TestNestedRepoAnswersToItsOwnGitignores(t *testing.T) {
	root := buildTree(t, map[string]string{
		".git/HEAD":                  "",
		".gitignore":                 "Gen.php\npackages/bar/\n",
		"app/Gen.php":                "<?php",
		"app/Keep.php":               "<?php",
		"packages/foo/.git/HEAD":     "",
		"packages/foo/.gitignore":    "Local.php\n",
		"packages/foo/src/Gen.php":   "<?php",
		"packages/foo/src/Keep.php":  "<?php",
		"packages/foo/src/Local.php": "<?php",
		"packages/bar/.git/HEAD":     "",
		"packages/bar/src/X.php":     "<?php",
		"packages/sub/.git":          "gitdir: ../../.git/modules/sub\n",
		"packages/sub/src/Gen.php":   "<?php",
	})
	foo := filepath.Join(root, "packages", "foo")

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "walk from the outer root",
			want: []string{
				"app/Keep.php",
				"packages/foo/src/Gen.php",
				"packages/foo/src/Keep.php",
				"packages/sub/src/Gen.php",
			},
		},
		{
			name: "walk starting inside the nested repo",
			args: []string{foo},
			want: []string{"packages/foo/src/Gen.php", "packages/foo/src/Keep.php"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths, err := Files(root, root, config.Default(), tc.args, nil)
			if err != nil {
				t.Fatalf("Files: %v", err)
			}

			var got []string

			for _, path := range paths {
				got = append(got, relSlash(root, path))
			}

			if !slices.Equal(got, tc.want) {
				t.Fatalf("walk selected %v, want %v", got, tc.want)
			}
		})
	}

	selected := cases[0].want

	for _, rel := range []string{
		"app/Gen.php", "app/Keep.php",
		"packages/foo/src/Gen.php", "packages/foo/src/Keep.php", "packages/foo/src/Local.php",
		"packages/bar/src/X.php", "packages/sub/src/Gen.php",
	} {
		ignored, err := IgnoresFile(root, nil, filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("IgnoresFile(%s): %v", rel, err)
		}

		if ignored == slices.Contains(selected, rel) {
			t.Errorf("IgnoresFile(%s) = %v, the walk selected it = %v", rel, ignored, !ignored)
		}
	}
}

func TestIgnoreRootOutsideAWorkTreeReadsNothingAbove(t *testing.T) {
	root := buildTree(t, map[string]string{
		".gitignore":     "proj/app/A.php\n",
		"proj/app/A.php": "<?php",
	})

	project := filepath.Join(root, "proj")

	got, err := Files(project, project, config.Default(), nil, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if !containsRel(got, root, "proj/app/A.php") {
		t.Fatalf("a .gitignore above a project outside any work tree was applied: %v", got)
	}
}
