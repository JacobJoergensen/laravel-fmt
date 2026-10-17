package markdown

import (
	"bytes"
	"errors"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	bladefmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/blade"
	cssfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/css"
	htmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/html"
	jsonfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/json"
	livewirefmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/livewire"
	phpfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/php"
	xmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/xml"
	yamlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/yaml"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/markdown"
)

// Markdown is left out: a nested document's fences would format it recursively.
var codeFormatters = map[string]func(src []byte, cfg *config.File) ([]byte, error){
	"php":   formatPHPSnippet,
	"blade": formatBlade,
	"html":  htmlfmt.Format,
	"css":   cssfmt.Format,
	"json":  formatJSON("code.json"),
	"jsonc": formatJSON("code.jsonc"),
	"yaml":  yamlfmt.Format,
	"yml":   yamlfmt.Format,
	"xml":   xmlfmt.Format,
}

var phpSnippetShapes = []func(src []byte, cfg *config.File) ([]byte, error){
	formatPHPStatements,
	formatPHPMembers,
	formatPHPExpression,
}

const phpSnippetOpenTag = "<?php\n\n"

// The brace may print on either line; the body opens after the first `{` line
// and closes on the last.
const (
	snippetClassOpen  = "class Snippet\n{\n"
	snippetClassClose = "\n}\n"
)

var (
	errOpenTagRewritten   = errors.New("the snippet's open tag did not format back as written")
	errMembersRewritten   = errors.New("the snippet's members did not format back as written")
	errSemicolonRewritten = errors.New("the `;` ending the snippet did not format back as its last byte")
)

func formatCode(b *parser.FencedCodeBlock, cfg *config.File) ([]byte, bool) {
	format, ok := codeFormatters[codeLanguage(b.Info)]
	if !ok || len(bytes.TrimSpace(b.Content)) == 0 {
		return nil, false
	}

	// The fence emitter writes the file's line endings itself, so the code
	// formats with LF and its last terminator is a lone `\n` to drop.
	codeCfg := *cfg
	codeCfg.EndOfLine = config.EndOfLineLF

	out, err := format(b.Content, &codeCfg)
	// Documentation is full of fragments that do not parse; those stay as written.
	if err != nil {
		return nil, false
	}

	return bytes.TrimSuffix(out, []byte("\n")), true
}

func codeLanguage(info []byte) string {
	if end := bytes.IndexAny(info, " \t"); end >= 0 {
		info = info[:end]
	}

	return strings.ToLower(string(info))
}

func formatPHPSnippet(src []byte, cfg *config.File) ([]byte, error) {
	if bytes.HasPrefix(bytes.TrimLeft(src, " \t\r\n"), []byte("<?")) {
		return phpfmt.Format(src, cfg)
	}

	var errs []error

	for _, format := range phpSnippetShapes {
		out, err := format(src, cfg)
		if err == nil {
			return out, nil
		}

		errs = append(errs, err)
	}

	return nil, errors.Join(errs...)
}

func formatPHPStatements(src []byte, cfg *config.File) ([]byte, error) {
	out, err := phpfmt.Format(slices.Concat([]byte(phpSnippetOpenTag), src), cfg)
	if err != nil {
		return nil, err
	}

	code, ok := bytes.CutPrefix(out, []byte(phpSnippetOpenTag))
	if !ok {
		return nil, errOpenTagRewritten
	}

	return code, nil
}

func formatPHPMembers(src []byte, cfg *config.File) ([]byte, error) {
	class, err := formatPHPStatements(wrapInSnippetClass(src), cfg)
	if err != nil {
		return nil, err
	}

	_, body, ok := bytes.Cut(class, []byte("{\n"))
	if !ok {
		return nil, errMembersRewritten
	}

	members := outdent(bytes.TrimSuffix(body, []byte("}\n")), cfg)

	// An outdent cannot tell a member's indent from the same spaces opening a
	// line of a string literal; the members format back to the same class only
	// when it took indents alone.
	again, err := formatPHPStatements(wrapInSnippetClass(members), cfg)
	if err != nil || !bytes.Equal(again, class) {
		return nil, errMembersRewritten
	}

	return members, nil
}

func wrapInSnippetClass(members []byte) []byte {
	return slices.Concat([]byte(snippetClassOpen), members, []byte(snippetClassClose))
}

func outdent(src []byte, cfg *config.File) []byte {
	level := []byte(strings.Repeat(" ", cfg.TabWidth))
	if cfg.UseTabs() {
		level = []byte("\t")
	}

	out := make([]byte, 0, len(src))

	for line := range bytes.Lines(src) {
		out = append(out, bytes.TrimPrefix(line, level)...)
	}

	return out
}

func formatPHPExpression(src []byte, cfg *config.File) ([]byte, error) {
	stmt := slices.Concat(bytes.TrimRight(src, " \t\r\n"), []byte(";"))

	out, err := formatPHPStatements(stmt, cfg)
	if err != nil {
		return nil, err
	}

	expr, ok := bytes.CutSuffix(out, []byte(";\n"))
	if !ok {
		return nil, errSemicolonRewritten
	}

	return append(expr, '\n'), nil
}

func formatBlade(src []byte, cfg *config.File) ([]byte, error) {
	if livewirefmt.LooksLikeSFC(src) {
		return livewirefmt.Format(src, cfg)
	}

	return bladefmt.Format(src, cfg)
}

func formatJSON(filename string) func(src []byte, cfg *config.File) ([]byte, error) {
	return func(src []byte, cfg *config.File) ([]byte, error) {
		return jsonfmt.Format(filename, src, cfg)
	}
}
