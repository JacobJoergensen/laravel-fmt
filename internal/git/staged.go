package git

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/rewrite"
)

// FormatFunc formats the bytes of the file at the given repo-relative path.
type FormatFunc func(relPath string, src []byte) ([]byte, error)

// FileError pairs a path with the error that stopped it being formatted.
type FileError struct {
	Path string
	Err  error
}

// Result summarizes a FormatStaged run; Reformatted and PartiallyStaged are
// both re-staged, differing only in whether the working-tree copy was
// rewritten.
type Result struct {
	Reformatted     []string
	PartiallyStaged []string
	Failed          []FileError
}

// FormatStaged formats and re-stages each staged file, leaving working-tree
// copies with unstaged edits untouched. A non-nil skip excludes paths the
// project ignores.
func FormatStaged(repo Repo, format FormatFunc, skip func(relPath string) (bool, error)) (Result, error) {
	files, err := repo.StagedFiles()
	if err != nil {
		return Result{}, err
	}

	// Read before the first re-stage, while the index still holds what the user
	// staged.
	unstaged, err := repo.UnstagedPaths()
	if err != nil {
		return Result{}, err
	}

	var (
		res     Result
		restage []StagedFile
	)

	for _, f := range files {
		if formatter.DetectLanguage(f.Path) == formatter.LangUnknown {
			continue
		}

		if skip != nil {
			skipped, err := skip(f.Path)
			if err != nil {
				return Result{}, err
			}

			if skipped {
				continue
			}
		}

		sha, err := formatStagedFile(repo, format, f, unstaged, &res)
		if err != nil {
			res.Failed = append(res.Failed, FileError{Path: f.Path, Err: err})

			continue
		}

		if sha != "" {
			restage = append(restage, StagedFile{Path: f.Path, Mode: f.Mode, SHA: sha})
		}
	}

	if len(restage) == 0 {
		return res, nil
	}

	// One index write at the end holds index.lock for a moment rather than for
	// the whole run.
	if err := repo.Stage(restage); err != nil {
		return Result{}, err
	}

	return res, nil
}

// formatStagedFile returns the formatted blob to re-stage, or "" when the
// staged copy is already formatted.
func formatStagedFile(repo Repo, format FormatFunc, f StagedFile, unstaged map[string]bool, res *Result) (string, error) {
	staged, err := repo.ReadBlob(f.SHA)
	if err != nil {
		return "", err
	}

	out, err := format(f.Path, staged)
	if err != nil {
		return "", err
	}

	if bytes.Equal(out, staged) {
		return "", nil
	}

	sha, err := repo.WriteBlob(f.Path, out)
	if err != nil {
		return "", err
	}

	if unstaged[f.Path] {
		res.PartiallyStaged = append(res.PartiallyStaged, f.Path)

		return sha, nil
	}

	wtPath := filepath.Join(repo.Root(), filepath.FromSlash(f.Path))

	// The working-tree bytes can differ from the blob (line-ending conversion),
	// so the original a failed write restores is read from disk.
	current, err := os.ReadFile(wtPath)
	if err != nil {
		return "", fmt.Errorf("read working tree: %w", err)
	}

	// The unstaged snapshot is as old as the run. A file edited since then
	// would lose that edit to the formatted staged copy, so it is left alone
	// like any other partially staged file.
	edited, err := editedSinceStaged(repo, f, staged, current)
	if err != nil {
		return "", err
	}

	if edited {
		res.PartiallyStaged = append(res.PartiallyStaged, f.Path)

		return sha, nil
	}

	if err := rewrite.Rewrite(wtPath, current, out); err != nil {
		return "", fmt.Errorf("write working tree: %w", err)
	}

	res.Reformatted = append(res.Reformatted, f.Path)

	return sha, nil
}

// Equal bytes settle it without starting git; otherwise git decides, since
// under line-ending conversion an untouched copy never matches its blob byte
// for byte.
func editedSinceStaged(repo Repo, f StagedFile, staged, current []byte) (bool, error) {
	if bytes.Equal(current, staged) {
		return false, nil
	}

	matches, err := repo.WorkingTreeMatches(f.Path, f.SHA)
	if err != nil {
		return false, err
	}

	return !matches, nil
}
