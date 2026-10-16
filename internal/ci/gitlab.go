package ci

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

const (
	gitLabCheckError       = "laravel-fmt/error"
	gitLabCheckUnformatted = "laravel-fmt/unformatted"
	gitLabSeverityError    = "critical"
	gitLabSeverityStyle    = "minor"
)

const fingerprintBytes = 16

// GitLabProjectDir returns the checkout a GitLab CI job runs in, or "" outside
// GitLab CI.
func GitLabProjectDir() string {
	return os.Getenv("CI_PROJECT_DIR")
}

type gitLabIssue struct {
	Description string         `json:"description"`
	CheckName   string         `json:"check_name"`
	Fingerprint string         `json:"fingerprint"`
	Severity    string         `json:"severity"`
	Location    gitLabLocation `json:"location"`
}

type gitLabLocation struct {
	Path  string      `json:"path"`
	Lines gitLabLines `json:"lines"`
}

type gitLabLines struct {
	Begin int `json:"begin"`
}

// WriteGitLabReport writes s to w as a GitLab Code Quality report. The paths in
// s are relative to workDir; GitLab reads them from the repository root, so
// they are rewritten relative to projectDir unless it is "".
func WriteGitLabReport(w io.Writer, s Summary, workDir, projectDir string) error {
	issues := make([]gitLabIssue, 0, len(s.Failures)+len(s.Changed))
	occurrences := map[string]int{}

	add := func(check, severity, path string, line int, message string) {
		path = gitLabPath(path, workDir, projectDir)
		key := check + "\x00" + path + "\x00" + message

		issues = append(issues, gitLabIssue{
			// GitLab does not show the check name, so the description names
			// the tool.
			Description: "laravel-fmt: " + message,
			CheckName:   check,
			Fingerprint: fingerprint(key, occurrences[key]),
			Severity:    severity,
			Location:    gitLabLocation{Path: path, Lines: gitLabLines{Begin: max(line, 1)}},
		})

		occurrences[key]++
	}

	for _, f := range s.Failures {
		add(gitLabCheckError, gitLabSeverityError, f.Path, f.Line, f.Message)
	}

	for _, c := range s.Changed {
		add(gitLabCheckUnformatted, gitLabSeverityStyle, c.Path, c.Line, UnformattedMessage)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(issues); err != nil {
		return fmt.Errorf("write GitLab report: %w", err)
	}

	return nil
}

func gitLabPath(path, workDir, projectDir string) string {
	if projectDir != "" {
		abs := path

		if !filepath.IsAbs(abs) {
			abs = filepath.Join(workDir, path)
		}

		if rel, err := filepath.Rel(projectDir, abs); err == nil {
			path = rel
		}
	}

	return filepath.ToSlash(path)
}

// The line is left out of the fingerprint so an issue keeps it when code above
// moves, and GitLab can tell it apart from a new one. The occurrence count
// separates identical issues in one file, which GitLab would merge into one.
func fingerprint(key string, occurrence int) string {
	sum := sha256.Sum256([]byte(key + "\x00" + strconv.Itoa(occurrence)))

	return hex.EncodeToString(sum[:fingerprintBytes])
}
