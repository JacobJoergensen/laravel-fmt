// Package css formats CSS stylesheets for laravel-fmt.
//
// Selectors, at-rule preludes and declaration values keep their tokens: the
// formatter changes only whitespace, string quote delimiters and layout, and
// with cfg.Tailwind sorts the classes in an @apply prelude.
package css

import (
	"bytes"
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/css"
)

// Format formats src as CSS, failing on any parse diagnostic.
func Format(src []byte, cfg *config.File) ([]byte, error) {
	file, errs := parser.Parse(normalizeNewlines(src))

	if err := formatter.JoinErrors(errs); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	w := formatter.NewWriter(cfg)

	emitFile(w, file, cfg)

	return w.Bytes(), nil
}

// EmitStyleBody formats a <style> body into w one level under the tag, or
// emits nothing and returns false when the body does not parse or holds a
// non-CSS at-rule such as a Blade @if.
func EmitStyleBody(w *formatter.Writer, src []byte, cfg *config.File) bool {
	file, errs := parser.Parse(src)
	if len(errs) > 0 {
		return false
	}

	if hasForeignAtRule(file.Nodes) {
		return false
	}

	if len(file.Nodes) == 0 {
		return true
	}

	w.Indent()
	w.Newline()
	emitFile(w, file, cfg)
	w.Dedent()

	return true
}

// normalizeNewlines reads CRLF and a lone CR as LF before parsing, as CSS
// Syntax 3 section 3.3 does, so a `\` before a CRLF cannot split the pair.
func normalizeNewlines(src []byte) []byte {
	if bytes.IndexByte(src, '\r') < 0 {
		return src
	}

	return bytes.ReplaceAll(formatter.NormalizeLoneCR(src), []byte("\r\n"), []byte("\n"))
}
