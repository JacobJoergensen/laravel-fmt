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
	if cfg.PHP.RemoveTrailingCloseTag && !closeTagTerminatesPrevious(file.Src, file.Stmts) {
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

// closeTagTerminatesPrevious reports whether the file's trailing `?>` is what
// terminates the statement before it. PHP reads a close tag as an implicit
// `;`, and only a statement the builder reproduces verbatim (an
// alternative-syntax block) can still lack its own `;` in the output, so
// dropping the tag there would leave PHP that no longer parses.
func closeTagTerminatesPrevious(src []byte, stmts []parser.Stmt) bool {
	if len(stmts) < 2 {
		return false
	}

	tag, ok := stmts[len(stmts)-1].(*parser.TagStmt)
	if !ok || tag.Token.Kind != parser.KindCloseTag {
		return false
	}

	prev := stmts[len(stmts)-2]

	if !emitsVerbatim(prev) {
		return false
	}

	end := prev.Span().End.Offset
	if end <= 0 || end > len(src) {
		return false
	}

	return src[end-1] != ';'
}

// emitsVerbatim reports whether the builder reproduces a statement's source rather
// than rebuilding it: the alternative `if:`/`while:`/`for:`/`foreach:`/`switch:`
// syntaxes, whose bodies the doc model cannot express.
func emitsVerbatim(s parser.Stmt) bool {
	switch n := s.(type) {
	case *parser.IfStmt:
		return has(n.Colon)
	case *parser.WhileStmt:
		return has(n.Colon)
	case *parser.ForStmt:
		return has(n.Colon)
	case *parser.ForEachStmt:
		return has(n.Colon)
	case *parser.SwitchStmt:
		return has(n.Colon)
	}

	return false
}
