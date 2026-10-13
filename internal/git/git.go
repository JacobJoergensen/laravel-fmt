// Package git runs the git operations behind --staged and --since.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git blob modes for the regular files the formatter handles; symlinks
// (120000) and gitlinks (160000) are filtered out.
const (
	modeRegular    = "100644"
	modeExecutable = "100755"
)

// StagedFile is one regular-file entry staged for commit.
type StagedFile struct {
	Path string
	Mode string
	SHA  string
}

// Repo is the git surface FormatStaged depends on.
type Repo interface {
	// Root returns the repository's absolute top-level directory.
	Root() string
	StagedFiles() ([]StagedFile, error)
	UnstagedPaths() (map[string]bool, error)
	// WorkingTreeMatches reports whether the working-tree copy of path, cleaned
	// the way git would stage it, is the blob named sha.
	WorkingTreeMatches(path, sha string) (bool, error)
	ReadBlob(sha string) ([]byte, error)
	WriteBlob(path string, content []byte) (string, error)
	// Stage points each entry's index path at its blob, in one index write.
	Stage(entries []StagedFile) error
	// Close stops the git processes the calls above started.
	Close() error
}

// Each process starts on first use, so a commit with nothing to format never
// starts the writer.
type execRepo struct {
	root    string
	reader  *process
	writer  *process
	hasher  *process
	blobDir string
}

// Open returns a Repo for the repository containing dir, or an error if dir
// is not in a git work tree.
func Open(dir string) (Repo, error) {
	out, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}

	return &execRepo{root: strings.TrimSpace(string(out))}, nil
}

func (r *execRepo) Root() string {
	return r.root
}

// --no-renames lists a rename by its new path, --diff-filter=ACM drops
// deletions, and parseRaw drops symlinks and submodules. One --raw call
// cannot overflow the command line on a big commit, and --no-abbrev keeps the
// full object name WorkingTreeMatches compares.
func (r *execRepo) StagedFiles() ([]StagedFile, error) {
	out, err := runGit(r.root, "diff", "--cached", "--no-renames", "--diff-filter=ACM", "-z", "--raw", "--no-abbrev")
	if err != nil {
		return nil, fmt.Errorf("list staged files: %w", err)
	}

	return parseRaw(out), nil
}

// Asks git rather than comparing working-tree bytes against the blob: under
// core.autocrlf or a text=auto gitattribute git checks out content it never
// stores verbatim, so an untouched file never matches its own blob.
func (r *execRepo) UnstagedPaths() (map[string]bool, error) {
	out, err := runGit(r.root, "diff", "--name-only", "-z")
	if err != nil {
		return nil, fmt.Errorf("list unstaged changes: %w", err)
	}

	paths := map[string]bool{}
	for _, path := range splitNUL(out) {
		paths[path] = true
	}

	return paths, nil
}

// Without --no-filters, --stdin-paths applies each path's clean and
// end-of-line filters, so the hash is what staging the file now would store.
func (r *execRepo) WorkingTreeMatches(path, sha string) (bool, error) {
	p, err := r.use(&r.hasher, "hash-object", "--stdin-paths")
	if err != nil {
		return false, err
	}

	got, err := p.request(quotePath(path))
	if err != nil {
		return false, fmt.Errorf("hash working tree %s: %w", path, drop(&r.hasher, err))
	}

	return got == sha, nil
}

func (r *execRepo) ReadBlob(sha string) ([]byte, error) {
	p, err := r.use(&r.reader, "cat-file", "--batch")
	if err != nil {
		return nil, err
	}

	if err := p.send(sha); err != nil {
		return nil, fmt.Errorf("read staged blob %s: %w", sha, drop(&r.reader, err))
	}

	content, err := readBatchBlob(p.stdout)
	if err != nil {
		return nil, fmt.Errorf("read staged blob %s: %w", sha, drop(&r.reader, err))
	}

	return content, nil
}

// hash-object cannot take several contents on stdin, so each passes through
// a scratch file. --no-filters stores the bytes verbatim, so the index holds
// exactly the formatter's output.
func (r *execRepo) WriteBlob(path string, content []byte) (string, error) {
	if r.blobDir == "" {
		dir, err := os.MkdirTemp("", "laravel-fmt-staged-")
		if err != nil {
			return "", fmt.Errorf("create blob scratch directory: %w", err)
		}

		r.blobDir = dir
	}

	p, err := r.use(&r.writer, "hash-object", "-w", "--no-filters", "--stdin-paths")
	if err != nil {
		return "", err
	}

	// One file serves every blob: git has read it by the time it replies.
	scratch := filepath.Join(r.blobDir, "blob")
	if err := os.WriteFile(scratch, content, 0o600); err != nil {
		return "", fmt.Errorf("write blob for %s: %w", path, err)
	}

	sha, err := p.request(quotePath(scratch))
	if err != nil {
		return "", fmt.Errorf("write blob for %s: %w", path, drop(&r.writer, err))
	}

	return sha, nil
}

func (r *execRepo) use(slot **process, args ...string) (*process, error) {
	if *slot == nil {
		p, err := startProcess(r.root, args...)
		if err != nil {
			return nil, err
		}

		*slot = p
	}

	return *slot, nil
}

// drop stops a process whose reply failed, so the next call starts a fresh one
// rather than reading replies meant for earlier requests.
func drop(slot **process, err error) error {
	err = errors.Join(err, (*slot).close())
	*slot = nil

	return err
}

func (r *execRepo) Stage(entries []StagedFile) error {
	var in bytes.Buffer

	for _, e := range entries {
		fmt.Fprintf(&in, "%s %s\t%s\x00", e.Mode, e.SHA, e.Path)
	}

	if _, err := runGitStdin(r.root, in.Bytes(), "update-index", "-z", "--index-info"); err != nil {
		return fmt.Errorf("re-stage %d files: %w", len(entries), err)
	}

	return nil
}

func (r *execRepo) Close() error {
	var errs []error

	for _, p := range []*process{r.reader, r.writer, r.hasher} {
		if p != nil {
			errs = append(errs, p.close())
		}
	}

	if r.blobDir != "" {
		if err := os.RemoveAll(r.blobDir); err != nil {
			errs = append(errs, fmt.Errorf("remove blob scratch directory: %w", err))
		}
	}

	return errors.Join(errs...)
}

// git diff --cached -z --raw emits alternating records: a
// ":<srcmode> <dstmode> <srcsha> <dstsha> <status>" entry, then the path.
func parseRaw(out []byte) []StagedFile {
	records := splitNUL(out)

	var files []StagedFile

	for i := 0; i+1 < len(records); i += 2 {
		fields := strings.Fields(strings.TrimPrefix(records[i], ":"))
		if len(fields) < 4 {
			continue
		}

		mode, sha := fields[1], fields[3]

		if mode != modeRegular && mode != modeExecutable {
			continue
		}

		files = append(files, StagedFile{Path: records[i+1], Mode: mode, SHA: sha})
	}

	return files
}

func splitNUL(out []byte) []string {
	var records []string

	for rec := range strings.SplitSeq(string(out), "\x00") {
		if rec != "" {
			records = append(records, rec)
		}
	}

	return records
}

func runGit(dir string, args ...string) ([]byte, error) {
	return runGitStdin(dir, nil, args...)
}

func runGitStdin(dir string, in []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // git subcommand with repo-relative paths the user asked to format
	cmd.Dir = dir

	if in != nil {
		cmd.Stdin = bytes.NewReader(in)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("git %s: %s: %w", args[0], msg, err)
		}

		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}

	return stdout.Bytes(), nil
}
