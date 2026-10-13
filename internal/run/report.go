package run

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JacobJoergensen/laravel-fmt/internal/ci"
	"github.com/JacobJoergensen/laravel-fmt/internal/detector"
	"github.com/JacobJoergensen/laravel-fmt/internal/diff"
	"github.com/JacobJoergensen/laravel-fmt/internal/git"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
)

// Failures take precedence over drift: a run with both exits 2, not 1.
func (r *runner) finish(failed, changed, unchanged int, elapsed time.Duration) error {
	if failed > 0 {
		r.errf("\n")
		r.renderSummary(terminal.Red, terminal.CrossMark, pluralFiles(failed, "failed to format"), elapsed)

		return ErrSilentFailure
	}

	if r.opts.Check || r.opts.Diff {
		if changed == 0 {
			r.renderSummary(terminal.Green, terminal.CheckMark, "All files already formatted", elapsed)

			return nil
		}

		r.errf("\n")
		drift := pluralFiles(changed, "would be reformatted") + " · " + strconv.Itoa(unchanged) + " already formatted"
		r.renderSummary(terminal.Yellow, terminal.WarningSign, drift, elapsed)

		return ErrDrift
	}

	if changed == 0 {
		r.renderSummary(terminal.Green, terminal.CheckMark, pluralFiles(unchanged, "already formatted"), elapsed)

		return nil
	}

	r.errf("\n")
	formatted := "Formatted " + filesWord(changed) + " · " + strconv.Itoa(unchanged) + " unchanged"
	r.renderSummary(terminal.Green, terminal.CheckMark, formatted, elapsed)

	return nil
}

// Only a file that failed to format fails a --staged run; a partially staged
// one is reported and re-staged all the same.
func (r *runner) finishStaged(res git.Result, root string, elapsed time.Duration) error {
	for _, fe := range res.Failed {
		path := repoPath(root, fe.Path)
		r.renderFileError(path, displayPath(r.workDir, path), nil, fe.Err)
	}

	for _, relPath := range res.Reformatted {
		path := repoPath(root, relPath)
		r.renderCheckPath(displayPath(r.workDir, path))
	}

	for _, relPath := range res.PartiallyStaged {
		path := repoPath(root, relPath)

		r.errf(
			"%s%s%s %s%s%s %s(re-staged; working tree kept, unstaged changes)%s\n",
			terminal.Yellow, terminal.Tilde, terminal.Reset,
			terminal.Bold, displayPath(r.workDir, path), terminal.Reset,
			terminal.Dim, terminal.Reset,
		)
	}

	if len(res.Failed) > 0 {
		r.errf("\n")
		r.renderSummary(terminal.Red, terminal.CrossMark, pluralFiles(len(res.Failed), "failed to format"), elapsed)

		return ErrSilentFailure
	}

	restaged := len(res.Reformatted) + len(res.PartiallyStaged)

	if restaged == 0 {
		r.renderSummary(terminal.Green, terminal.CheckMark, "All staged files already formatted", elapsed)

		return nil
	}

	r.errf("\n")
	r.renderSummary(terminal.Green, terminal.CheckMark, pluralFiles(restaged, "formatted and re-staged"), elapsed)

	return nil
}

// A nested laravel-fmt.json is never read; saying so spares its author a hunt
// for a setting that never applied. A run told which config to use says
// nothing.
func (r *runner) warnNestedConfig(ctx detector.Context) {
	if ctx.NestedConfigPath == "" || r.opts.Config != "" || r.opts.NoConfig {
		return
	}

	r.errf(
		"%s%s%s %s%s%s: not applied, laravel-fmt.json is read from the project root\n",
		terminal.Yellow, terminal.WarningSign, terminal.Reset,
		terminal.Bold, displayPath(r.workDir, ctx.NestedConfigPath), terminal.Reset,
	)
}

// renderFileError prints shown for a person and annotates path for GitHub,
// whose runner maps a path onto the checkout only when it is absolute and
// reads a relative one from the workspace root, not from where the run began.
func (r *runner) renderFileError(path, shown string, src []byte, err error) {
	r.errf("%s", pathLine(terminal.Red, terminal.CrossMark, shown))

	parseErrors := php.CollectErrors(err)

	if len(parseErrors) == 0 {
		r.errf("  %s%v%s\n", terminal.Dim, err, terminal.Reset)

		if ci.OnGitHubActions() {
			ci.GitHubError(r.streams.Err, path, 0, 0, err.Error())
		}

		return
	}

	for _, pe := range parseErrors {
		r.errf("  %s%d:%d: %s%s\n", terminal.Dim, pe.Pos.Line, pe.Pos.Column, pe.Msg, terminal.Reset)

		for _, line := range snippetLines(src, pe.Pos.Line, pe.Pos.Column) {
			r.errf("    %s\n", line)
		}

		if ci.OnGitHubActions() {
			ci.GitHubError(r.streams.Err, path, pe.Pos.Line, pe.Pos.Column, pe.Msg)
		}
	}
}

func (r *runner) renderCheckPath(path string) {
	r.errf("%s", pathLine(terminal.Yellow, terminal.Tilde, path))
}

// Timing is left off when stderr is piped, so CI output stays byte-stable.
func (r *runner) renderSummary(color, glyph, phrase string, elapsed time.Duration) {
	timing := ""

	if r.streams.ErrIsTTY {
		timing = fmt.Sprintf(" %s· %s%s", terminal.Dim, formatDuration(elapsed), terminal.Reset)
	}

	r.errf("%s%s %s%s%s\n", color, glyph, phrase, terminal.Reset, timing)
}

// errf writes status output to Err. Its write error is dropped on purpose:
// stderr is where a failure would be reported, so its own has nowhere to go.
func (r *runner) errf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.streams.Err, format, args...)
}

func pathLine(color, glyph, path string) string {
	return fmt.Sprintf("%s%s%s %s%s%s\n", color, glyph, terminal.Reset, terminal.Bold, path, terminal.Reset)
}

// displayPath returns path relative to workDir, the form a person there reads
// and types back. A path with no relative form, such as one on another Windows
// drive, stays absolute.
func displayPath(workDir, path string) string {
	rel, err := filepath.Rel(workDir, path)
	if err != nil {
		return path
	}

	return rel
}

func collectFailures(path string, err error) []ci.Failure {
	parseErrors := php.CollectErrors(err)

	if len(parseErrors) == 0 {
		return []ci.Failure{{Path: path, Message: err.Error()}}
	}

	failures := make([]ci.Failure, 0, len(parseErrors))

	for _, pe := range parseErrors {
		failures = append(failures, ci.Failure{
			Path:     path,
			Location: fmt.Sprintf("%d:%d", pe.Pos.Line, pe.Pos.Column),
			Message:  pe.Msg,
		})
	}

	return failures
}

// Best-effort: the job summary is an extra CI artifact and must never change
// how a run ends.
func writeJobSummary(s ci.Summary) {
	path := ci.GitHubSummaryPath()
	if path == "" {
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}

	defer func() { _ = f.Close() }()

	ci.WriteGitHubSummary(f, s)
}

func snippetLines(src []byte, line, col int) []string {
	snippet := diff.Snippet(src, line, col)
	if snippet == "" {
		return nil
	}

	return strings.Split(strings.TrimRight(snippet, "\n"), "\n")
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

func filesWord(n int) string {
	if n == 1 {
		return "1 file"
	}

	return strconv.Itoa(n) + " files"
}

func pluralFiles(n int, suffix string) string {
	return filesWord(n) + " " + suffix
}
