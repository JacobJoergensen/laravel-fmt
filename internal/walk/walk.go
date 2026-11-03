// Package walk enumerates the files laravel-fmt will format.
package walk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
)

// os.Stat's *fs.PathError already names the op and path, and on Windows the
// op is a syscall (GetFileAttributesEx), so only the reason is wrapped, beside
// the argument as the user typed it.
func statError(arg string, err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return fmt.Errorf("stat %s: %w", arg, pathErr.Err)
	}

	return fmt.Errorf("stat %s: %w", arg, err)
}

// Files returns the files to format as absolute paths, in order. Ignore
// patterns match from ignoreRoot wherever traversal starts, since they are
// written from the file that declares them; projectOf, when set, also holds
// each file to its own project's patterns.
func Files(root, ignoreRoot string, cfg *config.File, args []string, projectOf ProjectIgnore) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", root, err)
	}

	patterns := compileRules(cfg, ignoreRoot)

	if len(args) > 0 {
		return fromArgs(ignoreRoot, args, patterns, projectOf)
	}

	return walkTree(absRoot, ignoreRoot, patterns, projectOf)
}

// ProjectIgnore returns the ignore anchor and config of the project dir is in.
type ProjectIgnore func(dir string) (anchor string, cfg *config.File, err error)

// IgnoreRoot returns the directory holding configPath when it contains root,
// else root: a config outside the tree being formatted cannot describe it.
func IgnoreRoot(root, configPath string) string {
	if configPath == "" {
		return root
	}

	dir := filepath.Dir(configPath)
	if !within(dir, root) {
		return root
	}

	return dir
}

// IgnoreFilter reports whether a --staged or --since path is ignored.
func IgnoreFilter(root, runAnchor string, runCfg *config.File, projectOf ProjectIgnore) (func(relPath string) (bool, error), error) {
	set := &ignoreSet{}
	if err := set.addGitignore(root, ""); err != nil {
		return nil, err
	}

	loaded := map[string]bool{"": true}

	run := &ignoreSet{}
	run.addProject(filepath.Clean(runAnchor), runCfg)

	return func(relPath string) (bool, error) {
		slash := filepath.ToSlash(relPath)

		if hasHiddenSegment(slash) {
			return true, nil
		}

		// git applies no .gitignore to tracked files, so the filter loads them.
		if err := set.addChainTo(root, path.Dir(slash), loaded); err != nil {
			return false, err
		}

		if set.matchFile(slash) {
			return true, nil
		}

		file := filepath.Join(root, filepath.FromSlash(slash))

		if run.matchProjectFile(file) {
			return true, nil
		}

		// A project that cannot be resolved is reported when the file is
		// formatted, which resolves it again, as that file's failure; raising
		// it here would abort the whole run instead.
		if err := set.enterProject(projectOf, filepath.Dir(file), false); err != nil {
			return false, nil //nolint:nilerr // reported per file when formatted, see above
		}

		return set.matchProjectFile(file), nil
	}, nil
}

// IgnoresFile reports whether a walk from anchor would leave out file by its
// ignore rules, every .gitignore down to it included. The walk's hidden-name
// skip does not apply: a dotfile being edited is formatted on save.
func IgnoresFile(anchor string, cfg *config.File, file string) (bool, error) {
	absPath, err := filepath.Abs(file)
	if err != nil {
		return false, fmt.Errorf("resolve %s: %w", file, err)
	}

	absPath = Precompose(absPath)
	dir := filepath.Dir(absPath)
	prefix := ignorePrefix(anchor, dir)
	set := &ignoreSet{cfg: compileRules(cfg, anchor)}

	if err := set.addChain(anchor, dir, prefix); err != nil {
		return false, err
	}

	return set.matchFile(prefix + filepath.Base(absPath)), nil
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func hasHiddenSegment(slashPath string) bool {
	for seg := range strings.SplitSeq(slashPath, "/") {
		if isHidden(seg) {
			return true
		}
	}

	return false
}

// Arguments overlap freely, so the union is taken per file: a named file stays
// in even when its directory's walk ignored it.
func fromArgs(ignoreRoot string, args []string, matcher ignoreRules, projectOf ProjectIgnore) ([]string, error) {
	var out []string

	seen := make(map[string][]string)

	walks := make(map[string][]string)

	for _, arg := range args {
		arg = onDiskCase(arg)

		if isGlobArg(arg) {
			base, segments, err := splitGlob(arg)
			if err != nil {
				return nil, err
			}

			candidates, walked := walks[base]
			if !walked {
				candidates, err = walkGlobBase(arg, base, ignoreRoot, matcher, projectOf)
				if err != nil {
					return nil, err
				}

				walks[base] = candidates
			}

			matched, err := matchGlob(arg, base, segments, candidates)
			if err != nil {
				return nil, err
			}

			out = appendUnseen(out, seen, base, matched...)

			continue
		}

		info, err := os.Stat(arg)
		if err != nil {
			return nil, statError(arg, err)
		}

		abs, err := filepath.Abs(arg)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", arg, err)
		}

		if info.IsDir() {
			more, err := walkTree(abs, ignoreRoot, matcher, projectOf)
			if err != nil {
				return nil, err
			}

			out = appendUnseen(out, seen, abs, more...)

			continue
		}

		if formatter.DetectLanguage(abs) == formatter.LangUnknown {
			continue
		}

		// An explicitly named file bypasses the ignore matcher; only
		// tool-assembled sets (the walk, globs, --staged) respect ignore.
		out = appendUnseen(out, seen, abs, abs)
	}

	return out, nil
}

// Paths under base, the argument they came from, are keyed by where its links
// lead, so a file named through a linked folder and its target is listed once.
// The walk follows no link below base, so resolving base alone is exact; a base
// that does not resolve keys as typed. On a case-insensitive file system two
// spellings name one file, so keys equal but for case are compared by identity.
func appendUnseen(out []string, seen map[string][]string, base string, paths ...string) []string {
	resolved, err := resolveLinks(base)
	if err != nil {
		resolved = base
	}

	for _, abs := range paths {
		key := strings.ToLower(resolved + strings.TrimPrefix(abs, base))

		isSame := func(earlier string) bool {
			return sameFile(earlier, abs)
		}

		if slices.ContainsFunc(seen[key], isSame) {
			continue
		}

		seen[key] = append(seen[key], abs)
		out = append(out, abs)
	}

	return out
}

// onDiskCase spells each name in path as its directory lists it, so a path
// typed in another case is reported as stored. It keeps the rest as typed
// from the first name it cannot find, such as a glob segment.
func onDiskCase(path string) string {
	volume := filepath.VolumeName(path)
	prefix, dir, rest := "", ".", path[len(volume):]

	if filepath.IsAbs(path) {
		prefix = path[:len(volume)+1]
		dir, rest = prefix, rest[1:]
	} else if volume != "" {
		return path
	}

	sep := string(filepath.Separator)
	names := strings.Split(filepath.ToSlash(rest), "/")

	// Joined without cleaning, so a .. after a symlink resolves as the file
	// system resolves it.
	for i, name := range names {
		if name != "" && name != "." && name != ".." {
			spelled, ok := listedName(dir, name)
			if !ok {
				break
			}

			names[i] = spelled
		}

		dir += sep + names[i]
	}

	return Precompose(prefix + strings.Join(names, sep))
}

// A spelling that differs only in case counts only if it is the same file, so
// a case-sensitive file system never swaps one file for another.
func listedName(dir, name string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}

	for _, entry := range entries {
		if entry.Name() == name {
			return name, true
		}
	}

	composed := composedName(name)

	for _, entry := range entries {
		listed := entry.Name()

		if strings.EqualFold(composedName(listed), composed) && sameFile(filepath.Join(dir, name), filepath.Join(dir, listed)) {
			return listed, true
		}
	}

	return "", false
}

// git precomposes the names it reads only on macOS (core.precomposeunicode),
// and only a volume that opens a name in either form can take the composed one.
var precomposeNames = runtime.GOOS == "darwin"

// Precompose returns path in Unicode NFC on macOS, the form git records there.
func Precompose(path string) string {
	composed := composedName(path)
	if composed == path || !sameFile(composed, path) {
		return path
	}

	return composed
}

func composedName(name string) string {
	if !precomposeNames {
		return name
	}

	return norm.NFC.String(name)
}

func sameFile(a, b string) bool {
	if a == b {
		return true
	}

	infoA, err := os.Stat(a)
	if err != nil {
		return false
	}

	infoB, err := os.Stat(b)
	if err != nil {
		return false
	}

	return os.SameFile(infoA, infoB)
}

// Windows shells do not expand globs, so laravel-fmt does, for the
// metacharacters path.Match reads, which keeps every platform the same.
func isGlobPattern(arg string) bool {
	return strings.ContainsAny(arg, "*?[")
}

// A path that exists is taken as named, so a Folio page like [id].blade.php is
// never read as a pattern.
func isGlobArg(arg string) bool {
	return isGlobPattern(arg) && !exists(arg)
}

func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// GlobBase returns the absolute directory above the first segment of arg that
// holds a glob character and does not exist, or arg made absolute when there is
// none. On Windows \ is a separator there, not an escape.
func GlobBase(arg string) (string, error) {
	base, _, err := globBaseAndSegments(arg)

	return base, err
}

func globBaseAndSegments(arg string) (string, []string, error) {
	base, err := filepath.Abs(arg)
	if err != nil {
		return "", nil, fmt.Errorf("resolve %s: %w", arg, err)
	}

	var segments []string

	for isGlobArg(base) {
		segments = append(segments, filepath.Base(base))
		base = filepath.Dir(base)
	}

	slices.Reverse(segments)

	return base, segments, nil
}

func splitGlob(pattern string) (string, []string, error) {
	base, segments, err := globBaseAndSegments(pattern)
	if err != nil {
		return "", nil, err
	}

	for _, segment := range segments {
		if _, err := path.Match(segment, ""); err != nil {
			return "", nil, fmt.Errorf("invalid pattern: %s", pattern)
		}
	}

	return base, segments, nil
}

func walkGlobBase(pattern, base, ignoreRoot string, matcher ignoreRules, projectOf ProjectIgnore) ([]string, error) {
	info, err := os.Stat(base)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return nil, nil
	}

	if err != nil {
		return nil, statError(pattern, err)
	}

	return walkTree(base, ignoreRoot, matcher, projectOf)
}

// A pattern matching nothing is an error, like any argument that selects no
// work.
func matchGlob(pattern, base string, segments, candidates []string) ([]string, error) {
	var out []string

	for _, candidate := range candidates {
		rel := relSlash(base, candidate)

		if matchSegments(segments, strings.Split(rel, "/")) {
			out = append(out, candidate)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no files match pattern: %s", pattern)
	}

	return out, nil
}

// Paths match from ignoreRoot, not dir, since patterns are written from the
// project root. Each directory adds its .gitignore as it is entered, before
// anything inside is tested, and a pruned directory is never read.
func walkTree(dir, ignoreRoot string, cfg ignoreRules, projectOf ProjectIgnore) ([]string, error) {
	prefix := ignorePrefix(ignoreRoot, dir)
	set := &ignoreSet{cfg: cfg}

	if err := set.addChain(ignoreRoot, dir, prefix); err != nil {
		return nil, err
	}

	// The project the walk starts in may be rooted above it (`a/src` inside
	// project `a`), where entering directories would never reach its anchor.
	if err := set.enterProject(projectOf, dir, false); err != nil {
		return nil, err
	}

	var out []string

	// WalkDir does not follow a link it starts at; a trailing separator makes
	// the start resolve to the folder it links to.
	start := dir
	if !os.IsPathSeparator(start[len(start)-1]) {
		start += string(filepath.Separator)
	}

	err := filepath.WalkDir(start, func(abs string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel := relSlash(dir, abs)

		if composed := Precompose(abs); composed != abs {
			abs, rel = composed, composedName(rel)
		}

		if d.IsDir() {
			if rel == "." {
				return nil
			}

			if isHidden(d.Name()) {
				return filepath.SkipDir
			}

			if set.match(prefix+rel, true) || set.matchProject(abs, true) {
				return filepath.SkipDir
			}

			if err := set.enterProject(projectOf, abs, true); err != nil {
				return err
			}

			return set.enterGitignoreDir(abs, prefix+rel)
		}

		// Only regular files: a write follows a link, so a symlink found by
		// traversal would rewrite its target. IsRegular, not a ModeSymlink
		// test, because a Windows junction reports ModeIrregular; it also keeps
		// a FIFO from blocking the read. Naming one on the command line still
		// formats it.
		if !d.Type().IsRegular() {
			return nil
		}

		if isHidden(d.Name()) {
			return nil
		}

		if formatter.DetectLanguage(abs) == formatter.LangUnknown {
			return nil
		}

		if set.match(prefix+rel, false) || set.matchProject(abs, false) {
			return nil
		}

		out = append(out, abs)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}

	return out, nil
}

// A dir outside ignoreRoot has no root-relative name, so it yields "" and its
// files match on their own paths.
func ignorePrefix(ignoreRoot, dir string) string {
	rel, ok := relFromAbs(ignoreRoot, dir)
	if !ok || rel == "." || !within(ignoreRoot, dir) {
		return ""
	}

	return rel + "/"
}

func within(parent, child string) bool {
	rel, ok := relFromAbs(parent, child)

	return ok && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Both sides are made absolute first, so a relative argument gets the same
// answer as the absolute one naming the same directory.
func relFromAbs(parent, child string) (string, bool) {
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return "", false
	}

	absChild, err := filepath.Abs(child)
	if err != nil {
		return "", false
	}

	rel, err := filepath.Rel(absParent, absChild)
	if err != nil {
		return "", false
	}

	return filepath.ToSlash(rel), true
}

func relSlash(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}

	return filepath.ToSlash(rel)
}
