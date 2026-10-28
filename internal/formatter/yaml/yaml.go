// Package yaml formats YAML documents for laravel-fmt.
//
// Block mappings, sequences and scalars are re-indented to a block layout of
// two spaces or the configured width. Block scalars, flow collections and
// anchored, aliased or tagged values keep their text, with continuation lines
// re-homed; nothing is re-quoted, reordered or converted between styles.
package yaml

import (
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/yaml"
)

// indentWidth is the fallback YAML block indent, used when no indent was set
// explicitly. YAML is data and forbids tab indentation, so the default follows the
// format's own two-space convention rather than the project tab width; an explicit
// EditorConfig/config indent overrides the width, never the spaces.
const indentWidth = 2

// Format returns src formatted as YAML, or nil and an error joining every
// parse diagnostic. It always indents with spaces, since YAML forbids tabs.
func Format(src []byte, cfg *config.File) ([]byte, error) {
	// Lone CRs are normalized at intake so the parser, the freeze decision and
	// the emitter read the same bytes; normalizing only in the emitter removes
	// the bail that froze the file, and the next pass formats it differently.
	src = formatter.NormalizeLoneCR(src)

	file, errs := parser.Parse(src)

	if err := formatter.JoinErrors(errs); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	yamlCfg := *cfg
	yamlCfg.IndentStyle = config.IndentStyleSpace

	if !cfg.IndentExplicit {
		yamlCfg.TabWidth = indentWidth
	}

	w := formatter.NewWriter(&yamlCfg)

	emitFile(w, file)

	return w.Bytes(), nil
}
