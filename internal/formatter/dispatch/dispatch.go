// Package dispatch routes a source file to the language formatter implied by
// its filename extension.
//
// It lives under internal/formatter rather than at the root because it imports
// the per-language sub-packages, and those import the root for Writer.
package dispatch

import (
	"bytes"
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/language"
	markdownfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/markdown"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// formatLanguage is a variable so a test can make a formatter panic; no real
// input does.
var formatLanguage = language.Format

// File formats src with the formatter its path's extension selects, dropping
// a leading UTF-8 BOM, and returns src unchanged for an unknown extension.
func File(path string, src []byte, cfg *config.File) (out []byte, err error) {
	// A panic in one file is that file's error, not the run's: the worker pool
	// has no recovery of its own. A stack overflow is fatal, not a panic, so
	// the parsers' depth caps are what prevent one.
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("format %s: %v", path, r)
		}
	}()

	lang := formatter.DetectLanguage(path)
	if lang == formatter.LangUnknown {
		return src, nil
	}

	body := bytes.TrimPrefix(src, utf8BOM)

	if lang == formatter.LangMarkdown {
		return markdownfmt.Format(body, cfg)
	}

	return formatLanguage(lang, path, body, cfg)
}
