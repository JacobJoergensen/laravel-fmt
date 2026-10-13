// Package livewire parses Livewire 4 single-file components, splitting the
// <?php ... ?> frontmatter (to php.ParseFrontmatter) from the Blade template
// (to blade).
package livewire

import (
	"slices"

	"github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// Parse parses a Livewire SFC with the template's spans absolute to src;
// dirs is the set of Blade block directives, nil meaning Blade's own.
func Parse(src []byte, dirs *blade.Directives) (*File, []Error) {
	phpFile, templateStart, hasTemplate, phpErrs := php.ParseFrontmatter(src)

	if !hasTemplate {
		return &File{Src: src, PHP: phpFile}, phpErrs
	}

	bladeFile, bladeErrs := blade.ParseAt(src, templateStart, dirs)

	errs := slices.Concat(phpErrs, bladeErrs)

	return &File{Src: src, PHP: phpFile, Template: bladeFile}, errs
}
