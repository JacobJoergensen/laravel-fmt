// Package formatter holds the shared types and helpers every language
// formatter emits through. Dispatch to a language sub-package is the
// internal/formatter/dispatch package's job, so nothing here imports
// internal/formatter/<lang> and the import graph stays acyclic.
package formatter

import (
	"errors"
	"path/filepath"
	"strings"
)

// JoinErrors collapses errs into a single error via errors.Join, or nil when
// errs is empty.
func JoinErrors[E error](errs []E) error {
	if len(errs) == 0 {
		return nil
	}

	wrapped := make([]error, len(errs))

	for i, e := range errs {
		wrapped[i] = e
	}

	return errors.Join(wrapped...)
}

// Language identifies one of the languages laravel-fmt knows how to
// format. The zero value LangUnknown means "no formatter for this path".
type Language uint8

const (
	LangUnknown Language = iota
	LangPHP
	LangBlade
	LangHTML
	LangCSS
	LangMarkdown
	LangJSON
	LangYAML
	LangXML
)

// DetectLanguage returns the Language implied by path's extension
// (case-insensitive). Compound extensions (.blade.php) are tested before plain
// ones (.php) so Blade templates do not fall through to the PHP formatter. A
// .blade.php that renders no HTML (see isNonHTMLBlade) is LangUnknown.
func DetectLanguage(path string) Language {
	lower := strings.ToLower(filepath.Base(path))

	// A trailing .dist marks a distributable config template
	// (phpunit.xml.dist), so the type underneath it decides.
	lower = strings.TrimSuffix(lower, ".dist")

	switch {
	case strings.HasSuffix(lower, ".blade.php") && isNonHTMLBlade(path, lower):
		return LangUnknown
	case strings.HasSuffix(lower, ".blade.php"):
		return LangBlade
	case strings.HasSuffix(lower, ".php"):
		return LangPHP
	case strings.HasSuffix(lower, ".html"):
		return LangHTML
	case strings.HasSuffix(lower, ".css"):
		return LangCSS
	case strings.HasSuffix(lower, ".md"):
		return LangMarkdown
	case strings.HasSuffix(lower, ".yml"), strings.HasSuffix(lower, ".yaml"):
		return LangYAML
	case strings.HasSuffix(lower, ".xml"):
		return LangXML
	case strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".jsonc"):
		return LangJSON
	}

	return LangUnknown
}

// isNonHTMLBlade reports whether a .blade.php file is a document Blade renders
// into something other than HTML, so no HTML layout applies and any would
// damage it: an Envoy task file is shell, and a Laravel Boost guideline or
// skill (a project's .ai/ directory, a package's resources/boost/) is
// Markdown.
func isNonHTMLBlade(path, lowerBase string) bool {
	if lowerBase == "envoy.blade.php" {
		return true
	}

	slashed := "/" + filepath.ToSlash(path)

	return strings.Contains(slashed, "/.ai/") || strings.Contains(slashed, "/resources/boost/")
}
