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

const prettierIgnoreFileName = ".prettierignore"

// finderRegexModifiers are the PCRE modifiers Symfony Finder accepts after a
// regex delimiter.
const finderRegexModifiers = "imsxuADUn"

var oxfmtLanguages = []string{"*.css", "*.html", "*.md", "*.json", "*.jsonc", "*.yml", "*.yaml"}

var viteConfigNames = []string{
	"vite.config.ts", "vite.config.mts", "vite.config.cts", "vite.config.js", "vite.config.mjs", "vite.config.cjs",
}

var oxfmtConfigNames = []string{".oxfmtrc.json", ".oxfmtrc.jsonc", "oxfmt.config.ts", "oxfmt.config.mts"}

var oxfmtCandidateNames = slices.Concat(oxfmtConfigNames, viteConfigNames)

var fmtProperty = regexp.MustCompile(`(?m)^\s*fmt\s*:`)

// adoptIgnores adds Prettier's and Pint's ignore lists, so moving from those
// tools does not start formatting what they left alone. Only configs in anchor
// are read, since patterns match from there.
func adoptIgnores(cfg *config.File, ctx Context, anchor string) error {
	prettier, err := prettierIgnores(anchor)
	if err != nil {
		return err
	}

	// Prettier's matcher ignores case on every file system; Symfony Finder,
	// which applies Pint's excludes, never does.
	if len(prettier) > 0 {
		cfg.AdoptedIgnore = append(cfg.AdoptedIgnore, config.IgnoreLayer{Patterns: prettier, IgnoreCase: true})
	}

	owns, err := oxfmtOwnsFrontend(anchor)
	if err != nil {
		return err
	}

	if owns {
		cfg.AdoptedIgnore = append(cfg.AdoptedIgnore, config.IgnoreLayer{Patterns: slices.Clone(oxfmtLanguages)})
	}

	if ctx.Pint != nil && filepath.Clean(filepath.Dir(ctx.Pint.Path)) == filepath.Clean(anchor) {
		if pint := pintIgnores(ctx.Pint); len(pint) > 0 {
			cfg.AdoptedIgnore = append(cfg.AdoptedIgnore, config.IgnoreLayer{Patterns: pint})
		}
	}

	return nil
}

// A Vite+ config formats only with a `fmt` block, which `vp check` applies to
// every file Oxfmt supports; without one it only builds.
func oxfmtOwnsFrontend(dir string) (bool, error) {
	names, err := dirNames(dir, oxfmtCandidateNames)
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
	lines, err := walk.ReadIgnoreFile(filepath.Join(dir, prettierIgnoreFileName))
	if err != nil {
		return nil, fmt.Errorf("prettier ignores: %w", err)
	}

	return lines, nil
}

// pintIgnores translates pint.json's Finder options: exclude names directories
// at any depth, notName globs the file name, and notPath is found anywhere in
// the path.
func pintIgnores(p *Pint) []string {
	var patterns []string

	// Finder filters names without pruning, so a directory a notName matches
	// is still walked. notName goes first so exclude and notPath still win
	// over that negation.
	for _, name := range p.NotName {
		for _, glob := range notNameGlobs(name) {
			patterns = append(patterns, glob, "!"+glob+"/")
		}
	}

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

	for _, fragment := range p.NotPath {
		if isFinderRegex(fragment) {
			literal, ok := regexLiteral(fragment)
			if !ok {
				continue
			}

			fragment = literal
		}

		if strings.Trim(fragment, "/") == "" || strings.Contains(fragment, "//") {
			continue
		}

		patterns = append(patterns, substringPattern(fragment))
	}

	return patterns
}

// Symfony's glob expands {a,b}, and a name never holds a slash, so a notName
// with one matches nothing.
func notNameGlobs(name string) []string {
	if isFinderRegex(name) {
		literal, ok := regexLiteral(name)
		if !ok || strings.Contains(literal, "/") {
			return nil
		}

		return []string{"*" + escapeGlob(literal) + "*"}
	}

	// gitignore and Symfony both read a backslash as an escape, so expanding
	// around an escaped brace would misread it.
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, `\`) && strings.Contains(name, "{") {
		return nil
	}

	globs := expandBraces(name)
	for i, glob := range globs {
		globs[i] = escapeLeading(glob)
	}

	return globs
}

// An unclosed brace is an invalid regex to Symfony, which then matches nothing.
func expandBraces(glob string) []string {
	open := strings.IndexByte(glob, '{')
	if open < 0 {
		return []string{glob}
	}

	depth, start := 0, open+1

	var alternatives []string

	for i := open; i < len(glob); i++ {
		switch glob[i] {
		case '{':
			depth++
		case ',':
			if depth == 1 {
				alternatives = append(alternatives, glob[start:i])
				start = i + 1
			}
		case '}':
			depth--
			if depth > 0 {
				continue
			}

			alternatives = append(alternatives, glob[start:i])

			var out []string

			for _, alternative := range alternatives {
				for _, rest := range expandBraces(alternative + glob[i+1:]) {
					out = append(out, glob[:open]+rest)
				}
			}

			return out
		}
	}

	return nil
}

// A Finder regex with no regex syntax or modifiers finds its text anywhere,
// like a substring.
func regexLiteral(regex string) (string, bool) {
	closing := regex[0]

	for _, pair := range [...]string{"{}", "()", "[]", "<>"} {
		if closing == pair[0] {
			closing = pair[1]
		}
	}

	body := regex[1 : len(regex)-1]

	if regex[len(regex)-1] != closing || body == "" || strings.ContainsAny(body, `\^$.|?*+()[]{}`+string(regex[0])) {
		return "", false
	}

	return body, true
}

// The fragment's first and last parts may be the ends of longer names; a
// slash at either end pins a name boundary there.
func substringPattern(fragment string) string {
	parts := strings.Split(fragment, "/")
	for i, part := range parts {
		parts[i] = escapeGlob(part)
	}

	if len(parts) == 1 {
		return "*" + parts[0] + "*"
	}

	last := len(parts) - 1

	if parts[0] == "" {
		parts[0] = "*/**"
	} else {
		parts[0] = "**/*" + parts[0]
	}

	if parts[last] != "" {
		parts[last] += "*"
	}

	return strings.Join(parts, "/")
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
