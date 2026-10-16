package ci

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
)

// OnGitHubActions reports whether the process runs in a GitHub Actions job.
func OnGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// GitHubError writes an error annotation to w; a line of 0 or less annotates
// the whole file.
func GitHubError(w io.Writer, path string, line, col int, msg string) {
	ew := &errWriter{w: w}

	if line > 0 {
		ew.printf(
			"::error file=%s,line=%d,col=%d,title=laravel-fmt::%s\n",
			escapeProperty(path), line, col, escapeData(msg),
		)

		return
	}

	ew.printf("::error file=%s,title=laravel-fmt::%s\n", escapeProperty(path), escapeData(msg))
}

// GitHubSummaryPath returns the job summary file path, or "" outside GitHub
// Actions.
func GitHubSummaryPath() string {
	return os.Getenv("GITHUB_STEP_SUMMARY")
}

func (f Failure) location() string {
	if f.Line <= 0 {
		return ""
	}

	return fmt.Sprintf("%d:%d", f.Line, f.Column)
}

// WriteGitHubSummary renders s as Markdown to w, listing every file: GitHub
// shows at most 10 error annotations per step.
func WriteGitHubSummary(w io.Writer, s Summary) {
	ew := &errWriter{w: w}
	ew.printf("### laravel-fmt\n\n")

	if len(s.Failures) > 0 {
		ew.printf("#### %s Failed to format (%d)\n\n", terminal.CrossMark, len(s.Failures))
		ew.printf("| File | Location | Message |\n| --- | --- | --- |\n")

		for _, f := range s.Failures {
			ew.printf("| `%s` | %s | %s |\n", escapeCell(f.Path), f.location(), escapeCell(f.Message))
		}

		ew.printf("\n")
	}

	if len(s.Changed) > 0 {
		writeChangedSection(ew, s)
	}

	if len(s.Failures) == 0 && len(s.Changed) == 0 {
		ew.printf("%s All files already formatted\n", terminal.CheckMark)
	}
}

func writeChangedSection(ew *errWriter, s Summary) {
	heading := fmt.Sprintf("%s Formatted (%d)", terminal.CheckMark, len(s.Changed))

	if s.Check {
		heading = fmt.Sprintf("%s Would be reformatted (%d)", terminal.WarningSign, len(s.Changed))
	}

	ew.printf("#### %s · %d already formatted\n\n", heading, s.Unchanged)
	ew.printf("| File |\n| --- |\n")

	for _, c := range s.Changed {
		ew.printf("| `%s` |\n", escapeCell(c.Path))
	}

	ew.printf("\n")
}

// errWriter stops at the first failed write and drops the error: annotations
// and the job summary are best-effort and never change how a run ends.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) printf(format string, args ...any) {
	if ew.err != nil {
		return
	}

	_, ew.err = fmt.Fprintf(ew.w, format, args...)
}

var (
	cellEscaper = strings.NewReplacer(
		"|", "\\|",
		"\r\n", " ",
		"\n", " ",
		"\r", " ",
	)

	// Escaped as GitHub's own toolkit does it:
	// https://github.com/actions/toolkit/blob/main/packages/core/src/command.ts
	dataEscaper = strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	)

	propertyEscaper = strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
		",", "%2C",
		":", "%3A",
	)
)

func escapeCell(s string) string { return cellEscaper.Replace(s) }

func escapeData(s string) string { return dataEscaper.Replace(s) }

func escapeProperty(s string) string { return propertyEscaper.Replace(s) }
