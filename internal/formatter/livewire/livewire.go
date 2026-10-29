// Package livewire formats Livewire 4 single-file components for laravel-fmt.
// An SFC is a `.blade.php` whose first section is a `<?php ... ?>` frontmatter
// block followed by a Blade template.
//
// The frontmatter's closing `?>` is preserved regardless of the config's
// RemoveTrailingCloseTag, because the tag is the essential separator between the
// PHP and Blade halves.
package livewire

import (
	"bytes"
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	bladefmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/blade"
	htmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/html"
	phpfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/php"
	bladeparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/livewire"
	phpparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// LooksLikeSFC reports whether src opens with `<?php` after optional
// whitespace, matched case-insensitively as PHP's own lexer does.
func LooksLikeSFC(src []byte) bool {
	trimmed := bytes.TrimLeft(src, " \t\r\n")

	if len(trimmed) < len("<?php") {
		return false
	}

	return bytes.EqualFold(trimmed[:len("<?php")], []byte("<?php"))
}

// Format returns src formatted as a Livewire single-file component, or nil and
// an error joining every parse diagnostic.
func Format(src []byte, cfg *config.File) ([]byte, error) {
	file, errs := parser.Parse(src, bladefmt.Directives(cfg))

	// A plain view may open with `<?php` too (`<?php echo '<?xml …'; ?>`), and
	// its block renders: reshaping it would change the page. This runs before
	// the error check because half a structure (`<?php if ($a): ?>`) cannot
	// parse alone, while a broken component still recovers its class.
	if !holdsComponent(file.PHP) {
		return bladefmt.Format(src, cfg)
	}

	if err := formatter.JoinErrors(errs); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	hasPHP := file.PHP != nil && len(file.PHP.Stmts) > 0
	hasTemplate := file.Template != nil && hasContent(file.Template.Nodes)

	var out []byte

	if hasPHP {
		out = phpfmt.BuildFile(file.PHP, cfg)
	}

	if hasTemplate {
		w := formatter.NewWriter(cfg)
		bladefmt.EmitLivewireView(w, src, file.Template, cfg)

		// One blank line separates the frontmatter's `?>` from the template; the
		// frontmatter already ended with a single terminator, so add one more.
		if hasPHP {
			out = append(out, cfg.EOL()...)
		}

		out = append(out, w.Bytes()...)
	}

	return out, nil
}

func holdsComponent(file *phpparser.File) bool {
	if file == nil {
		return false
	}

	for _, stmt := range file.Stmts {
		var expr phpparser.Expr

		switch s := stmt.(type) {
		case *phpparser.ExpressionStmt:
			expr = s.Expr
		case *phpparser.ReturnStmt:
			expr = s.Expr
		}

		if _, ok := expr.(*phpparser.AnonClassExpr); ok {
			return true
		}
	}

	return false
}

// hasContent reports whether nodes has any non-whitespace node. A
// whitespace-only template counts as empty for the blank-line-separator
// decision, since the emit path drops those nodes anyway.
func hasContent(nodes []bladeparser.Node) bool {
	for _, n := range nodes {
		if !htmlfmt.IsWhitespaceText(n) {
			return true
		}
	}

	return false
}
