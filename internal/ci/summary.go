// Package ci writes CI reports for GitHub Actions and GitLab.
package ci

// UnformattedMessage reports a file that formatting would change.
const UnformattedMessage = "File is not formatted; run laravel-fmt to fix"

// Summary is one run's result for a CI report.
type Summary struct {
	Check     bool
	Changed   []Change
	Unchanged int
	Failures  []Failure
}

// Change is a file that formatting changes, from Line on.
type Change struct {
	Path string
	Line int
}

// Failure is one error in a file; a Line of 0 places it on no line.
type Failure struct {
	Path    string
	Line    int
	Column  int
	Message string
}
