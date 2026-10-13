// Package markdown formats Markdown source for laravel-fmt.
//
// Inline content has no tree of its own: the formatter only moves the
// whitespace between words and adds the escapes a line needs to stay
// paragraph text.
package markdown

import (
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/markdown"
)

// Format returns src formatted as Markdown, or nil and an error joining every
// parse diagnostic.
func Format(src []byte, cfg *config.File) ([]byte, error) {
	file, errs := parser.Parse(src)

	if err := formatter.JoinErrors(errs); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	w := formatter.NewWriter(cfg)

	emitFile(w, file, cfg)

	out := w.Bytes()

	if leadingFenceNeedsEscape(file, out) {
		return append([]byte{'\\'}, out...), nil
	}

	return out, nil
}
