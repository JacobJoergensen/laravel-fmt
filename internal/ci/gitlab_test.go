package ci

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func decodeGitLabReport(t *testing.T, s Summary, workDir, projectDir string) []gitLabIssue {
	t.Helper()

	var buf bytes.Buffer
	if err := WriteGitLabReport(&buf, s, workDir, projectDir); err != nil {
		t.Fatalf("WriteGitLabReport: %v", err)
	}

	var issues []gitLabIssue
	if err := json.Unmarshal(buf.Bytes(), &issues); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, buf.String())
	}

	return issues
}

func TestWriteGitLabReportDescribesEachFinding(t *testing.T) {
	s := Summary{
		Check:   true,
		Changed: []Change{{Path: "app/Models/User.php", Line: 7}},
		Failures: []Failure{
			{Path: "app/Broken.php", Line: 3, Column: 5, Message: "unexpected '<'"},
			{Path: "app/Unreadable.php", Message: "read: denied"},
		},
	}

	issues := decodeGitLabReport(t, s, "", "")

	want := []struct {
		check, severity, path, description string
		line                               int
	}{
		{gitLabCheckError, gitLabSeverityError, "app/Broken.php", "laravel-fmt: unexpected '<'", 3},
		{gitLabCheckError, gitLabSeverityError, "app/Unreadable.php", "laravel-fmt: read: denied", 1},
		{gitLabCheckUnformatted, gitLabSeverityStyle, "app/Models/User.php", "laravel-fmt: " + UnformattedMessage, 7},
	}

	if len(issues) != len(want) {
		t.Fatalf("got %d issues, want %d: %+v", len(issues), len(want), issues)
	}

	for i, w := range want {
		got := issues[i]
		if got.CheckName != w.check || got.Severity != w.severity || got.Location.Path != w.path ||
			got.Description != w.description || got.Location.Lines.Begin != w.line {
			t.Errorf("issue %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestWriteGitLabReportWritesAnEmptyListWhenAllIsFormatted(t *testing.T) {
	var buf strings.Builder
	if err := WriteGitLabReport(&buf, Summary{Check: true, Unchanged: 3}, "", ""); err != nil {
		t.Fatalf("WriteGitLabReport: %v", err)
	}

	if got := buf.String(); got != "[]\n" {
		t.Fatalf("report = %q, want an empty JSON list", got)
	}
}

func TestWriteGitLabReportPathsAreRelativeToTheProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	workDir := filepath.Join(projectDir, "packages", "site")
	s := Summary{Changed: []Change{{Path: filepath.Join("app", "User.php"), Line: 1}}}

	issues := decodeGitLabReport(t, s, workDir, projectDir)

	if got := issues[0].Location.Path; got != "packages/site/app/User.php" {
		t.Fatalf("path = %q, want packages/site/app/User.php", got)
	}
}

func TestWriteGitLabReportFingerprintSurvivesALineMove(t *testing.T) {
	failure := Failure{Path: "app/Broken.php", Line: 3, Message: "unexpected token"}
	moved := failure
	moved.Line = 9

	before := decodeGitLabReport(t, Summary{Failures: []Failure{failure}}, "", "")
	after := decodeGitLabReport(t, Summary{Failures: []Failure{moved}}, "", "")

	if before[0].Fingerprint != after[0].Fingerprint {
		t.Fatalf("fingerprint changed when the error moved to another line: %s, then %s", before[0].Fingerprint, after[0].Fingerprint)
	}
}

func TestWriteGitLabReportFingerprintSeparatesIdenticalErrors(t *testing.T) {
	failure := Failure{Path: "app/Broken.php", Line: 3, Message: "unexpected token"}

	issues := decodeGitLabReport(t, Summary{Failures: []Failure{failure, failure}}, "", "")

	if issues[0].Fingerprint == issues[1].Fingerprint {
		t.Fatalf("two identical errors in one file share fingerprint %s; GitLab would merge them", issues[0].Fingerprint)
	}
}
