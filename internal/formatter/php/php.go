// Package php formats PHP source for laravel-fmt. Source is parsed and
// BuildFile reprints it through the Doc model, width-driven and idempotent,
// keeping only the author breaks the layout rules preserve. Blade and Livewire
// embed PHP via BuildExpr and BuildStmts.
package php

import (
	"bytes"
	"fmt"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// Format returns src formatted as PHP, or nil and an error joining every parse
// diagnostic.
func Format(src []byte, cfg *config.File) ([]byte, error) {
	file, errs := parser.Parse(src)

	if err := formatter.JoinErrors(errs); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	file.Stmts = stripTrailingBlankHTML(file.Stmts)

	// A file drops its terminal `?>` per config. Livewire keeps its frontmatter
	// `?>` (the template separator) by calling BuildFile directly, so the strip
	// lives here rather than in BuildFile.
	if cfg.PHP.RemoveTrailingCloseTag && containsOnlyPHP(file.Tokens) {
		kept := len(file.Stmts)
		file.Stmts = stripTrailingCloseTag(file.Stmts)

		if len(file.Stmts) < kept {
			moveCloseTagTrivia(file.Tokens)
		}
	}

	out := BuildFile(file, cfg)

	// Appended after the printer, never through it: these bytes are the archive a
	// PHAR stub carries, so they take no indent, no line-ending rewrite and no
	// final newline. The formatted source ends where `__halt_compiler();` does,
	// which is exactly where PHP stops compiling.
	if payload := haltPayload(file.Stmts); len(payload) > 0 {
		out = append(bytes.TrimRight(out, "\r\n"), payload...)
	}

	return out, nil
}

// moveCloseTagTrivia hands the stripped final `?>`'s leading trivia to the
// end-of-file token, which prints trailing comments, so the comments before
// the tag survive the cut.
func moveCloseTagTrivia(tokens []parser.Token) {
	last := len(tokens) - 1

	for i := last - 1; i >= 0; i-- {
		if tokens[i].Kind != parser.KindCloseTag {
			continue
		}

		tokens[last].Leading = append(tokens[i].Leading, tokens[last].Leading...)
		tokens[i].Leading = nil

		return
	}
}

// haltPayload returns the bytes after a top-level `__halt_compiler();`, or nil.
func haltPayload(stmts []parser.Stmt) []byte {
	for _, s := range stmts {
		halt, ok := s.(*parser.HaltCompilerStmt)
		if ok && halt.Payload.Kind == parser.KindHaltPayload {
			return halt.Payload.Text
		}
	}

	return nil
}

// containsOnlyPHP reports whether tokens are a single PHP block, after at most
// a shebang line. PSR-12 omits the closing tag only from a file containing only
// PHP, as PHP-CS-Fixer's no_closing_tag does. Whitespace after the final tag
// does not count: stripTrailingBlankHTML has already dropped it.
func containsOnlyPHP(tokens []parser.Token) bool {
	opens := 0
	last := len(tokens) - 1

	for i, tok := range tokens {
		switch tok.Kind {
		case parser.KindOpenTag, parser.KindOpenTagEcho:
			opens++
		case parser.KindInlineHTML:
			shebang := i == 0 && bytes.HasPrefix(tok.Text, []byte("#!"))
			blankTail := i == last-1 && tokens[last].Kind == parser.KindEOF && len(bytes.TrimSpace(tok.Text)) == 0

			if !shebang && !blankTail {
				return false
			}
		}
	}

	return opens == 1
}

// stripTrailingCloseTag drops a trailing `?>` tag. It is deliberately narrow:
// only the final element is checked (so a file ending in inline HTML after `?>`
// keeps its tag), and it refuses to cut a `?>` that closes a short-echo
// construct (walking back past an ExpressionStmt run to a ShortEchoStmt).
func stripTrailingCloseTag(stmts []parser.Stmt) []parser.Stmt {
	if len(stmts) == 0 {
		return stmts
	}

	tag, ok := stmts[len(stmts)-1].(*parser.TagStmt)

	if !ok || tag.Token.Kind != parser.KindCloseTag {
		return stmts
	}

	for i := len(stmts) - 2; i >= 0; i-- {
		if _, isExpr := stmts[i].(*parser.ExpressionStmt); isExpr {
			continue
		}

		if _, isShort := stmts[i].(*parser.ShortEchoStmt); isShort {
			return stmts
		}

		break
	}

	return stmts[:len(stmts)-1]
}
