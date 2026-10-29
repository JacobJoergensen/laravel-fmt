package markdown

import (
	"bytes"
	"errors"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/language"
	phpfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/php"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/markdown"
)

var tagExtensions = map[string]string{"blade": "blade.php"}

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
	path := codePath(b.Info)
	lang := formatter.DetectLanguage(path)

	// Markdown is skipped, or a block's own fences would format recursively.
	if lang == formatter.LangUnknown || lang == formatter.LangMarkdown || len(bytes.TrimSpace(b.Content)) == 0 {
		return nil, false
	}

	// The fence emitter writes the file's line endings itself, so the code
	// formats with LF and its last terminator is a lone `\n` to drop.
	codeCfg := *cfg
	codeCfg.EndOfLine = config.EndOfLineLF

	// A lone CR ends a line in Markdown, which the fence emitter writes as one,
	// so the code has to be formatted with that line break, not as a CR byte.
	src := formatter.NormalizeLoneCR(b.Content)

	var (
		out []byte
		err error
	)

	if lang == formatter.LangPHP {
		out, err = formatPHPSnippet(src, &codeCfg)
	} else {
		out, err = language.Format(lang, path, src, &codeCfg)
	}

	// Documentation is full of fragments that do not parse; those stay as written.
	if err != nil {
		return nil, false
	}

	return bytes.TrimSuffix(out, []byte("\n")), true
}

func codePath(info []byte) string {
	if end := bytes.IndexAny(info, " \t"); end >= 0 {
		info = info[:end]
	}

	tag := strings.ToLower(string(info))
	if ext, ok := tagExtensions[tag]; ok {
		tag = ext
	}

	return "code." + tag
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
