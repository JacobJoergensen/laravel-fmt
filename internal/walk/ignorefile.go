package walk

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// A missing .gitignore returns (nil, nil), since most directories have none.
// ENOTDIR counts as missing: a path git lists can name a directory the work
// tree has since replaced with a file.
func readGitignore(dir string) ([]string, error) {
	gitignore := filepath.Join(dir, ".gitignore")

	file, err := os.Open(gitignore)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil, nil
		}

		return nil, fmt.Errorf("read %s: %w", gitignore, err)
	}

	defer func() { _ = file.Close() }()

	var lines []string

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", gitignore, err)
	}

	return lines, nil
}

// relDir is absDir's slash path from the ignore root, empty for the root
// itself.
func (s *ignoreSet) addGitignore(absDir, relDir string) error {
	lines, err := readGitignore(absDir)
	if err != nil {
		return err
	}

	repo, nested := s.repoOf(relDir + "/")

	for _, raw := range lines {
		p, ok := compileScopedPattern(relDir, raw)

		switch {
		case !ok:
		case nested:
			s.repos[repo] = append(s.repos[repo], p)
		default:
			s.git = append(s.git, p)
		}
	}

	return nil
}

func (s *ignoreSet) enterGitignoreDir(absDir, relDir string) error {
	nested, err := hasGitEntry(absDir)
	if err != nil {
		return err
	}

	if nested {
		if s.repos == nil {
			s.repos = make(map[string]ignoreMatcher)
		}

		s.repos[relDir+"/"] = nil
	}

	return s.addGitignore(absDir, relDir)
}

// .git is a directory in a clone and a file in a worktree or submodule.
func hasGitEntry(dir string) (bool, error) {
	entry := filepath.Join(dir, ".git")

	_, err := os.Lstat(entry)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("stat %s: %w", entry, err)
}

// One set serves files tested in any order: patterns are scoped to their
// subtree and a directory loads after its ancestors, so deeper files win.
func (s *ignoreSet) addChainTo(root, dir string, loaded map[string]bool) error {
	if dir == "." {
		dir = ""
	}

	if loaded[dir] {
		return nil
	}

	if err := s.addChainTo(root, path.Dir(dir), loaded); err != nil {
		return err
	}

	loaded[dir] = true

	return s.addGitignore(filepath.Join(root, filepath.FromSlash(dir)), dir)
}

func (s *ignoreSet) addOuterGitignores(base string) error {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", base, err)
	}

	top, err := gitTop(absBase)
	if err != nil || top == "" || top == absBase {
		return err
	}

	rel, _ := relFromAbs(top, absBase)
	below := strings.Split(filepath.ToSlash(rel), "/")

	var outer ignoreMatcher

	for i := range below {
		dir := strings.Join(below[:i], "/")

		lines, err := readGitignore(filepath.Join(top, filepath.FromSlash(dir)))
		if err != nil {
			return err
		}

		for _, raw := range lines {
			if p, ok := compileScopedPattern(dir, raw); ok {
				outer = append(outer, p)
			}
		}
	}

	for i := range below {
		if outer.match(strings.Join(below[:i+1], "/"), true) {
			s.excluded = true

			break
		}
	}

	s.above = outer
	s.aboveRoot = strings.Join(below, "/") + "/"

	return nil
}

func gitTop(dir string) (string, error) {
	var top string

	err := upward.Walk(dir, func(at string) (bool, error) {
		found, err := hasGitEntry(at)
		if found {
			top = at
		}

		return found, err
	})

	return top, err
}

// addChain loads every .gitignore from ignoreRoot down to dir, shallowest
// first so deeper files win; a walk starting below the root would otherwise
// miss the files above it, which git applies to everything underneath.
func (s *ignoreSet) addChain(ignoreRoot, dir, prefix string) error {
	rel, ok := relFromAbs(ignoreRoot, dir)
	atRoot := ok && rel == "."

	base := ignoreRoot
	if prefix == "" && !atRoot {
		base = dir
	}

	if err := s.addOuterGitignores(base); err != nil {
		return err
	}

	if err := s.addGitignore(ignoreRoot, ""); err != nil {
		return err
	}

	if prefix == "" {
		// dir is the ignore root, or lies outside it. Outside, it has no
		// root-relative name to scope by, so its own file applies unscoped.
		if atRoot {
			return nil
		}

		return s.addGitignore(dir, "")
	}

	segments := strings.Split(strings.TrimSuffix(prefix, "/"), "/")

	for i := range segments {
		rel := strings.Join(segments[:i+1], "/")

		if err := s.enterGitignoreDir(filepath.Join(ignoreRoot, filepath.FromSlash(rel)), rel); err != nil {
			return err
		}
	}

	return nil
}
