package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const (
	phpPath      = "a.php"
	stagedPHP    = "<?php  echo 1;"
	formattedPHP = "<?php echo 1;\n"
	oldSHA       = "old"
)

type mockRepo struct {
	root     string
	files    []StagedFile
	unstaged map[string]bool
	edited   map[string]bool
	blobs    map[string][]byte
	writes   map[string][]byte
	staged   map[string]string
	stages   int
	counter  int
}

func (m *mockRepo) Root() string { return m.root }

func (m *mockRepo) StagedFiles() ([]StagedFile, error) { return m.files, nil }

func (m *mockRepo) UnstagedPaths() (map[string]bool, error) { return m.unstaged, nil }

func (m *mockRepo) WorkingTreeMatches(path, _ string) (bool, error) { return !m.edited[path], nil }

func (m *mockRepo) ReadBlob(sha string) ([]byte, error) {
	content, ok := m.blobs[sha]
	if !ok {
		return nil, fmt.Errorf("unknown blob %q", sha)
	}

	return content, nil
}

func (m *mockRepo) WriteBlob(path string, content []byte) (string, error) {
	m.counter++
	sha := fmt.Sprintf("blob%d", m.counter)
	m.blobs[sha] = content
	m.writes[path] = content

	return sha, nil
}

func (m *mockRepo) Stage(entries []StagedFile) error {
	m.stages++

	for _, e := range entries {
		m.staged[e.Path] = e.SHA
	}

	return nil
}

func (m *mockRepo) Close() error { return nil }

func TestFormatStagedFullyStagedFile(t *testing.T) {
	root := t.TempDir()
	writeWorkingTree(t, root, stagedPHP)

	repo := newMockRepo(root, StagedFile{Path: phpPath, Mode: "100644", SHA: oldSHA}, stagedPHP)

	res, err := FormatStaged(repo, formatPHP, nil)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if got := string(repo.writes[phpPath]); got != formattedPHP {
		t.Errorf("index blob = %q, want formatted output", got)
	}

	if repo.staged[phpPath] == "" {
		t.Error("a.php was not re-staged")
	}

	if got := readWorkingTree(t, root); got != formattedPHP {
		t.Errorf("working tree = %q, want formatted output", got)
	}

	if len(res.Reformatted) != 1 || res.Reformatted[0] != phpPath {
		t.Errorf("Reformatted = %v, want [a.php]", res.Reformatted)
	}

	if len(res.PartiallyStaged) != 0 {
		t.Errorf("PartiallyStaged = %v, want none", res.PartiallyStaged)
	}
}

func TestFormatStagedPreservesUnstagedEdits(t *testing.T) {
	root := t.TempDir()
	wip := stagedPHP + " // work in progress"
	writeWorkingTree(t, root, wip)

	repo := newMockRepo(root, StagedFile{Path: phpPath, Mode: "100644", SHA: oldSHA}, stagedPHP)
	repo.unstaged[phpPath] = true

	res, err := FormatStaged(repo, formatPHP, nil)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if got := string(repo.writes[phpPath]); got != formattedPHP {
		t.Errorf("index blob = %q, want formatted output", got)
	}

	if got := readWorkingTree(t, root); got != wip {
		t.Errorf("working tree = %q, want the unstaged edit preserved", got)
	}

	if len(res.PartiallyStaged) != 1 || res.PartiallyStaged[0] != phpPath {
		t.Errorf("PartiallyStaged = %v, want [a.php]", res.PartiallyStaged)
	}

	if len(res.Reformatted) != 0 {
		t.Errorf("Reformatted = %v, want none", res.Reformatted)
	}
}

func TestFormatStagedFormatsWorkingTreeUnderEOLFilter(t *testing.T) {
	root := t.TempDir()
	writeWorkingTree(t, root, "<?php  echo 1;\r\n")

	repo := newMockRepo(root, StagedFile{Path: phpPath, Mode: "100644", SHA: oldSHA}, "<?php  echo 1;\n")

	res, err := FormatStaged(repo, formatPHP, nil)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if got := readWorkingTree(t, root); got != formattedPHP {
		t.Errorf("working tree = %q, want formatted output", got)
	}

	if len(res.Reformatted) != 1 || res.Reformatted[0] != phpPath {
		t.Errorf("Reformatted = %v, want [a.php]", res.Reformatted)
	}

	if len(res.PartiallyStaged) != 0 {
		t.Errorf("PartiallyStaged = %v, want none: git reports no unstaged edit", res.PartiallyStaged)
	}
}

func TestFormatStagedKeepsAnEditMadeDuringTheRun(t *testing.T) {
	const edited = "<?php  echo 1; // edited while the hook ran\n"

	root := t.TempDir()
	writeWorkingTree(t, root, edited)

	repo := newMockRepo(root, StagedFile{Path: phpPath, Mode: "100644", SHA: oldSHA}, stagedPHP)
	repo.edited[phpPath] = true

	res, err := FormatStaged(repo, formatPHP, nil)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if got := readWorkingTree(t, root); got != edited {
		t.Errorf("working tree = %q, want the edit kept", got)
	}

	if len(res.PartiallyStaged) != 1 || res.PartiallyStaged[0] != phpPath {
		t.Errorf("PartiallyStaged = %v, want [a.php]", res.PartiallyStaged)
	}

	if string(repo.blobs[repo.staged[phpPath]]) != formattedPHP {
		t.Error("the staged copy was not re-staged formatted")
	}
}

func TestFormatStagedWritesTheIndexOnce(t *testing.T) {
	const otherPath = "b.php"

	root := t.TempDir()
	writeWorkingTree(t, root, stagedPHP)

	if err := os.WriteFile(filepath.Join(root, otherPath), []byte(stagedPHP), 0o600); err != nil {
		t.Fatalf("write %s: %v", otherPath, err)
	}

	repo := newMockRepo(root, StagedFile{Path: phpPath, Mode: "100644", SHA: oldSHA}, stagedPHP)
	repo.files = append(repo.files, StagedFile{Path: otherPath, Mode: "100644", SHA: oldSHA})

	if _, err := FormatStaged(repo, formatPHP, nil); err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if repo.stages != 1 {
		t.Errorf("index written %d times, want once", repo.stages)
	}

	if repo.staged[phpPath] == "" || repo.staged[otherPath] == "" {
		t.Errorf("staged = %v, want both files", repo.staged)
	}
}

func TestFormatStagedLeavesAFailedFileUnstaged(t *testing.T) {
	repo := newMockRepo(t.TempDir(), StagedFile{Path: phpPath, Mode: "100644", SHA: oldSHA}, stagedPHP)

	res, err := FormatStaged(repo, formatPHP, nil)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if len(res.Failed) != 1 || res.Failed[0].Path != phpPath {
		t.Errorf("Failed = %v, want one entry for a.php: its working tree is missing", res.Failed)
	}

	if len(repo.staged) != 0 {
		t.Errorf("staged = %v, want a failed file left alone", repo.staged)
	}
}

func TestFormatStagedSkipsAlreadyFormatted(t *testing.T) {
	root := t.TempDir()
	writeWorkingTree(t, root, formattedPHP)

	repo := newMockRepo(root, StagedFile{Path: phpPath, Mode: "100644", SHA: oldSHA}, formattedPHP)

	format := func(_ string, src []byte) ([]byte, error) {
		return append([]byte(nil), src...), nil
	}

	res, err := FormatStaged(repo, format, nil)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if len(repo.writes) != 0 {
		t.Errorf("wrote a blob for an unchanged file: %v", repo.writes)
	}

	if len(repo.staged) != 0 {
		t.Errorf("re-staged an unchanged file: %v", repo.staged)
	}

	if len(res.Reformatted)+len(res.PartiallyStaged) != 0 {
		t.Error("reported a change for an already-formatted file")
	}
}

func TestFormatStagedSkipsNonFormattable(t *testing.T) {
	repo := newMockRepo(t.TempDir(), StagedFile{Path: "main.go", Mode: "100644", SHA: oldSHA}, "package main")

	called := false
	format := func(_ string, src []byte) ([]byte, error) {
		called = true

		return src, nil
	}

	res, err := FormatStaged(repo, format, nil)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if called {
		t.Error("formatted a non-formattable file")
	}

	if len(res.Reformatted)+len(res.PartiallyStaged)+len(res.Failed) != 0 {
		t.Error("a non-formattable file appeared in the result")
	}
}

func TestFormatStagedRecordsFormatFailure(t *testing.T) {
	repo := newMockRepo(t.TempDir(), StagedFile{Path: phpPath, Mode: "100644", SHA: oldSHA}, "<?php")

	format := func(_ string, _ []byte) ([]byte, error) {
		return nil, errors.New("parse error at line 1")
	}

	res, err := FormatStaged(repo, format, nil)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if len(res.Failed) != 1 || res.Failed[0].Path != phpPath {
		t.Errorf("Failed = %v, want one entry for a.php", res.Failed)
	}

	if len(repo.staged) != 0 {
		t.Error("re-staged a file that failed to format")
	}
}

func TestFormatStagedSkipsIgnored(t *testing.T) {
	repo := newMockRepo(t.TempDir(), StagedFile{Path: "storage/views/cache.php", Mode: "100644", SHA: oldSHA}, "<?php $x=1;")

	called := false
	format := func(_ string, src []byte) ([]byte, error) {
		called = true

		return src, nil
	}

	skip := func(p string) (bool, error) { return p == "storage/views/cache.php", nil }

	res, err := FormatStaged(repo, format, skip)
	if err != nil {
		t.Fatalf("FormatStaged: %v", err)
	}

	if called {
		t.Error("formatted an ignored staged file")
	}

	if len(res.Reformatted)+len(res.PartiallyStaged)+len(res.Failed) != 0 {
		t.Error("an ignored staged file appeared in the result")
	}
}

func newMockRepo(root string, f StagedFile, stagedContent string) *mockRepo {
	return &mockRepo{
		root:     root,
		files:    []StagedFile{f},
		unstaged: map[string]bool{},
		edited:   map[string]bool{},
		blobs:    map[string][]byte{f.SHA: []byte(stagedContent)},
		writes:   map[string][]byte{},
		staged:   map[string]string{},
	}
}

func formatPHP(_ string, _ []byte) ([]byte, error) {
	return []byte(formattedPHP), nil
}

func writeWorkingTree(t *testing.T, root, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(root, phpPath), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", phpPath, err)
	}
}

func readWorkingTree(t *testing.T, root string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, phpPath))
	if err != nil {
		t.Fatalf("read %s: %v", phpPath, err)
	}

	return string(content)
}
