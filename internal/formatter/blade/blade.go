// Package blade formats Blade templates for laravel-fmt.
//
// Echoes, directive arguments, bound (`:name="expr"`) attributes and `@php`
// bodies go through the PHP formatter. wire:* values are a Livewire DSL and
// stay verbatim, while Alpine's JavaScript values only have their token
// spacing normalized (internal/formatter/alpine).
//
// Element and attribute emission is Blade's own rather than the HTML
// formatter's, so an attribute value can reach the Attr.Parts decomposition
// that `class="btn {{ $variant }}"` needs.
package blade

import (
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
)

// Format parses src as a Blade template and returns the formatted bytes. It
// returns a non-nil error wrapping every parser diagnostic when src does not
// parse cleanly; the caller should keep the original bytes rather than trust a
// partially-recovered emission.
func Format(src []byte, cfg *config.File) ([]byte, error) {
	file, errs := parser.Parse(src, Directives(cfg))

	if err := formatter.JoinErrors(errs); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	w := formatter.NewWriter(cfg)

	e := &emitter{w: w, src: src, cfg: cfg}
	e.emitFile(file)

	return w.Bytes(), nil
}

// Directives returns the block directives a template is parsed with: Blade's own
// plus the ones the project declares in blade.blockDirectives.
func Directives(cfg *config.File) *parser.Directives {
	declared := make([]parser.BlockDirective, 0, len(cfg.Blade.BlockDirectives))

	for _, d := range cfg.Blade.BlockDirectives {
		declared = append(declared, parser.BlockDirective{Open: d.Open, Close: d.Closer(), Branches: d.Branches})
	}

	return parser.NewDirectives(declared)
}

// EmitLivewireView formats the template of a Livewire single-file component into
// w. src is the buffer the file was parsed from, still needed because the PHP
// expression fallback for unhandled kinds reads source spans. A <script> or
// <style> keeps the column Livewire reads it from.
func EmitLivewireView(w *formatter.Writer, src []byte, file *parser.File, cfg *config.File) {
	e := &emitter{w: w, src: src, cfg: cfg, livewireView: true}
	e.emitFile(file)
}
