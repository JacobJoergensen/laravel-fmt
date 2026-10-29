// Package language formats source in a language already detected. Markdown is
// left to the dispatch package, since its code blocks format through this one.
package language

import (
	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	bladefmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/blade"
	cssfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/css"
	htmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/html"
	jsonfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/json"
	livewirefmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/livewire"
	phpfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/php"
	xmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/xml"
	yamlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/yaml"
)

// Format formats src as lang and returns src unchanged for Markdown and
// LangUnknown.
func Format(lang formatter.Language, path string, src []byte, cfg *config.File) ([]byte, error) {
	switch lang {
	case formatter.LangPHP:
		return phpfmt.Format(src, cfg)
	case formatter.LangBlade:
		if livewirefmt.LooksLikeSFC(src) {
			return livewirefmt.Format(src, cfg)
		}

		return bladefmt.Format(src, cfg)
	case formatter.LangHTML:
		return htmlfmt.Format(src, cfg)
	case formatter.LangCSS:
		return cssfmt.Format(src, cfg)
	case formatter.LangJSON:
		return jsonfmt.Format(path, src, cfg)
	case formatter.LangYAML:
		return yamlfmt.Format(src, cfg)
	case formatter.LangXML:
		return xmlfmt.Format(src, cfg)
	case formatter.LangMarkdown, formatter.LangUnknown:
	}

	return src, nil
}
