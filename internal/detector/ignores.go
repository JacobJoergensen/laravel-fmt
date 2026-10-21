package detector

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/walk"
)

const prettierIgnoreFile = ".prettierignore"

// finderRegexModifiers are the PCRE modifiers Symfony Finder accepts after a
// regex delimiter.
const finderRegexModifiers = "imsxuADUn"

var oxfmtLanguages = []string{"*.css", "*.html", "*.md", "*.json", "*.jsonc", "*.yml", "*.yaml"}

var viteConfigNames = []string{
	"vite.config.ts", "vite.config.mts", "vite.config.cts", "vite.config.js", "vite.config.mjs", "vite.config.cjs",
}

var oxfmtConfigNames = []string{".oxfmtrc.json", ".oxfmtrc.jsonc", "oxfmt.config.ts", "oxfmt.config.mts"}

var fmtProperty = regexp.MustCompile(`(?m)^\s*fmt\s*:`)

// adoptIgnores adds Prettier's and Pint's ignore lists, so moving from those
// tools does not start formatting what they left alone. Only configs in anchor
// are read, since patterns match from there.
func adoptIgnores(cfg *config.File, ctx Context, anchor string) error {
	prettier, err := prettierIgnores(anchor)
	if err != nil {
		return err
	}

	if len(prettier) > 0 {
		cfg.AdoptedIgnore = append(cfg.AdoptedIgnore, prettier)
	}

	owns, err := oxfmtOwnsFrontend(anchor)
	if err != nil {
		return err
	}

	if owns {
		cfg.AdoptedIgnore = append(cfg.AdoptedIgnore, slices.Clone(oxfmtLanguages))
	}

	if ctx.Pint != nil && filepath.Clean(filepath.Dir(ctx.Pint.Path)) == filepath.Clean(anchor) {
		if pint := pintIgnores(ctx.Pint); len(pint) > 0 {
			cfg.AdoptedIgnore = append(cfg.AdoptedIgnore, pint)
		}
	}

	return nil
}

// A Vite+ config formats only with a `fmt` block, which `vp check` applies to
// every file Oxfmt supports; without one it only builds.
func oxfmtOwnsFrontend(dir string) (bool, error) {
	names, err := dirNames(dir)
	if err != nil {
		return false, err
	}

	if slices.ContainsFunc(oxfmtConfigNames, func(name string) bool { return names[nameKey(name)] }) {
		return true, nil
	}

	for _, name := range viteConfigNames {
		if !names[nameKey(name)] {
			continue
		}

		path := filepath.Join(dir, name)

		raw, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", path, err)
		}

		return bytes.Contains(raw, []byte("vite-plus")) && fmtProperty.Match(raw), nil
	}

	return false, nil
}

// .prettierignore is gitignore syntax, so its lines go to the walk unread.
func prettierIgnores(dir string) ([]string, error) {
	lines, err := walk.ReadIgnoreFile(filepath.Join(dir, prettierIgnoreFile))
	if err != nil {
		return nil, fmt.Errorf("prettier ignores: %w", err)
	}

	return lines, nil
}

// pintIgnores translates pint.json's Finder options: exclude names directories
// at any depth, notName globs the file name, and notPath is a path fragment.
// What gitignore cannot express (a regex, a notName with / or {}) is dropped.
func pintIgnores(p *Pint) []string {
	var patterns []string

	for _, dir := range p.Exclude {
		dir = strings.TrimRight(dir, "/")
		if dir == "" {
			continue
		}

		pattern := escapeGlob(dir) + "/"
		if strings.Contains(dir, "/") {
			pattern = "**/" + pattern
		}

		patterns = append(patterns, pattern)
	}

	for _, name := range p.NotName {
		if name == "" || strings.ContainsAny(name, "/{") || isFinderRegex(name) {
			continue
		}

		patterns = append(patterns, escapeLeading(name))
	}

	for _, fragment := range p.NotPath {
		if isFinderRegex(fragment) {
			continue
		}

		fragment = strings.Trim(fragment, "/")
		if fragment == "" {
			continue
		}

		patterns = append(patterns, "**/"+escapeGlob(fragment))
	}

	return patterns
}

func escapeGlob(literal string) string {
	var b strings.Builder

	for _, r := range literal {
		if strings.ContainsRune(`\*?[`, r) {
			b.WriteByte('\\')
		}

		b.WriteRune(r)
	}

	return escapeLeading(b.String())
}

func escapeLeading(pattern string) string {
	if pattern != "" && (pattern[0] == '#' || pattern[0] == '!') {
		return `\` + pattern
	}

	return pattern
}

// isFinderRegex ports Symfony Finder's MultiplePcreFilterIterator::isRegex.
func isFinderRegex(s string) bool {
	for n := 3; n <= len(s); n++ {
		if strings.Trim(s[n:], finderRegexModifiers) != "" {
			continue
		}

		start, end := s[0], s[n-1]

		if start == end {
			return !strings.ContainsRune(`*? \`, rune(start)) && !isAlnum(start)
		}

		for _, pair := range [...]string{"{}", "()", "[]", "<>"} {
			if start == pair[0] && end == pair[1] {
				return true
			}
		}

		return false
	}

	return false
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
