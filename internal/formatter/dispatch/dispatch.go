// Package dispatch routes a source file to the language formatter implied by
// its filename extension.
//
// It lives under internal/formatter rather than at the root because it imports
// every per-language sub-package, and those import the root for Writer.
package dispatch

import (
	"bytes"
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	bladefmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/blade"
	cssfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/css"
	htmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/html"
	jsonfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/json"
	livewirefmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/livewire"
	markdownfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/markdown"
	phpfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/php"
	xmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/xml"
	yamlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/yaml"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// formatPHP is a variable so a test can make a formatter panic; no real input
// does.
var formatPHP = phpfmt.Format

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

	body := bytes.TrimPrefix(src, utf8BOM)

	switch formatter.DetectLanguage(path) {
	case formatter.LangPHP:
		return formatPHP(body, cfg)
	case formatter.LangBlade:
		if livewirefmt.LooksLikeSFC(body) {
			return livewirefmt.Format(body, cfg)
		}

		return bladefmt.Format(body, cfg)
	case formatter.LangHTML:
		return htmlfmt.Format(body, cfg)
	case formatter.LangCSS:
		return cssfmt.Format(body, cfg)
	case formatter.LangMarkdown:
		return markdownfmt.Format(body, cfg)
	case formatter.LangJSON:
		return jsonfmt.Format(path, body, cfg)
	case formatter.LangYAML:
		return yamlfmt.Format(body, cfg)
	case formatter.LangXML:
		return xmlfmt.Format(body, cfg)
	case formatter.LangUnknown:
		return src, nil
	}

	return src, nil
}
