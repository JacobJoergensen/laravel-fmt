// Package xml formats XML documents for laravel-fmt.
//
// Normalization is minimal: element indentation, attribute layout and quote
// style. Character data (text, CDATA and anything under xml:space="preserve")
// is written byte for byte; comments, processing instructions and the
// declaration keep their text, with continuation lines re-homed.
package xml

import (
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/xml"
)

// Format returns src formatted as XML, or nil and an error joining every parse
// diagnostic. Indentation follows cfg, since XML has no tool-owned indent
// convention.
func Format(src []byte, cfg *config.File) ([]byte, error) {
	return format(src, cfg, parser.Parse)
}

// FormatFragment is Format without the one-root rule, for snippets in documentation.
func FormatFragment(src []byte, cfg *config.File) ([]byte, error) {
	return format(src, cfg, parser.ParseFragment)
}

func format(src []byte, cfg *config.File, parse func([]byte) (*parser.File, []parser.Error)) ([]byte, error) {
	src = formatter.NormalizeLoneCR(src)
	file, errs := parse(src)

	if err := formatter.JoinErrors(errs); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	w := formatter.NewWriter(cfg)

	emitFile(w, file, cfg)

	return w.Bytes(), nil
}
