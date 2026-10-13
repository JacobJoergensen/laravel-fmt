package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
)

// ChangedFiles lists the files that differ from where HEAD branched off ref:
// committed, staged and unstaged changes plus untracked files git does not
// ignore. skip gets root-relative slash paths.
func ChangedFiles(root, ref string, skip func(relPath string) (bool, error)) ([]string, error) {
	base, err := mergeBase(root, ref)
	if err != nil {
		return nil, err
	}

	// Against the work tree, so staged and unstaged edits count. Filtering out
	// only deletions keeps a renamed file under its new name.
	tracked, err := runGit(root, "diff", "--name-only", "-z", "--diff-filter=d", base)
	if err != nil {
		return nil, fmt.Errorf("list files changed since %s: %w", ref, err)
	}

	untracked, err := runGit(root, "ls-files", "-z", "--others", "--exclude-standard", "--full-name")
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}

	return regularFiles(root, append(splitNUL(tracked), splitNUL(untracked)...), skip)
}

// mergeBase resolves ref first so an unknown one is told apart from one with
// no history in common with HEAD, which is what a shallow CI checkout gives.
func mergeBase(root, ref string) (string, error) {
	// git would read a leading dash as one of its own options.
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("--since %q: not a git revision", ref)
	}

	if _, err := runGit(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return "", fmt.Errorf("--since %s: not a commit in this repository; a shallow CI checkout has to fetch it first", ref)
	}

	out, err := runGit(root, "merge-base", ref, "HEAD")
	if err != nil {
		return "", fmt.Errorf("--since %s: no common history with HEAD; a shallow clone needs the full history (actions/checkout: fetch-depth: 0): %w", ref, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// Only regular files count: a submodule, a symlink or a file removed without
// git knowing is not something to format.
func regularFiles(root string, relPaths []string, skip func(relPath string) (bool, error)) ([]string, error) {
	slices.Sort(relPaths)
	relPaths = slices.Compact(relPaths)

	var out []string

	for _, rel := range relPaths {
		if formatter.DetectLanguage(rel) == formatter.LangUnknown {
			continue
		}

		skipped, err := skip(rel)
		if err != nil {
			return nil, err
		}

		if skipped {
			continue
		}

		path := filepath.Join(root, filepath.FromSlash(rel))

		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}

		if info.Mode().IsRegular() {
			out = append(out, path)
		}
	}

	return out, nil
}
