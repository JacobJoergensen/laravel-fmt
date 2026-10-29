// Package json formats JSON documents for laravel-fmt.
//
// Layout follows the source's line shape, capped by width: an empty container
// stays inline unless it holds a comment; a container whose delimiters sat on
// one source line emits flat when it fits PrintWidth (else wraps); one spread
// across lines always wraps. An authored multi-line container is never
// collapsed back to flat, so a hand-compact shape and a tool-expanded shape
// both survive without a config knob; the source shape is the signal.
//
// String contents and numeric literals are preserved byte-for-byte: no escape
// decoding, no number canonicalization, so `1.0`, `1e5`, and a string's exact
// Unicode escapes round-trip unchanged.
package json

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/json"
)

// Format returns src formatted as JSON, or nil and an error joining every parse
// diagnostic. It always indents with spaces, by cfg.TabWidth when the indent
// was set explicitly and by the file's convention otherwise.
func Format(filename string, src []byte, cfg *config.File) ([]byte, error) {
	parse := parser.Parse
	if isJSONCFilename(filename) {
		parse = parser.ParseJSONC
	}

	file, errs := parse(formatter.NormalizeLoneCR(src))

	if err := formatter.JoinErrors(errs); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	jsonCfg := *cfg
	jsonCfg.IndentStyle = config.IndentStyleSpace

	if !cfg.IndentExplicit {
		jsonCfg.TabWidth = indentWidth(filename)
	}

	w := formatter.NewWriter(&jsonCfg)

	emitFile(w, file)

	return w.Bytes(), nil
}

// EmitScriptBody formats a JSON <script> body into w one level under the tag,
// or emits nothing and returns false when the body is not strict JSON.
func EmitScriptBody(w *formatter.Writer, src []byte) bool {
	file, errs := parser.Parse(src)
	if len(errs) > 0 || file.Root == nil {
		return false
	}

	w.Indent()
	w.Newline()
	emitFile(w, file)
	w.Dedent()

	return true
}

// isJSONCFilename reports whether filename is a JSONC document: a `.jsonc` file,
// or a `tsconfig.*`/`jsconfig.*` config (which allow comments and trailing commas
// by convention). Every other `.json` is parsed as strict RFC 8259 JSON.
func isJSONCFilename(filename string) bool {
	base := strings.ToLower(filepath.Base(filename))

	if strings.HasSuffix(base, ".jsonc") {
		return true
	}

	if !strings.HasSuffix(base, ".json") {
		return false
	}

	return strings.HasPrefix(base, "tsconfig.") || strings.HasPrefix(base, "jsconfig.")
}

// indentWidth follows the tool that owns the file, Composer's 4 for
// composer.json and npm's 2 otherwise, since that tool rewrites the file with
// its own indent and the two would ping-pong.
func indentWidth(filename string) int {
	if strings.EqualFold(filepath.Base(filename), "composer.json") {
		return 4
	}

	return 2
}
