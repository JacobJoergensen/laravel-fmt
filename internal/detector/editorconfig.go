package detector

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
	"github.com/JacobJoergensen/laravel-fmt/internal/walk"
)

const editorConfigFileName = ".editorconfig"

// EditorConfig is a parsed .editorconfig; its sections keep file order, since
// the last matching section wins.
type EditorConfig struct {
	Path     string
	Root     bool
	Sections []EditorConfigSection
}

// EditorConfigSection is one [pattern] section. A zero field was absent; a
// cleared one holds editorConfigCleared or "unset", and only a cleared key
// removes what a broader section set.
type EditorConfigSection struct {
	Pattern       string
	IndentStyle   string
	IndentSize    int
	TabWidth      int
	EndOfLine     string
	MaxLineLength int
}

// Global merges the file's [*] sections.
func (ec *EditorConfig) Global() EditorConfigSection {
	merged := EditorConfigSection{Pattern: "*"}

	for _, s := range ec.Sections {
		if s.Pattern == "*" {
			merged = mergeEditorConfigSection(merged, s)
		}
	}

	return merged
}

// ResolveForFile merges the sections matching rel, a path relative to this
// file's directory.
func (ec *EditorConfig) ResolveForFile(rel string) EditorConfigSection {
	merged := EditorConfigSection{}
	rel = filepath.ToSlash(rel)

	for _, s := range ec.Sections {
		if editorConfigGlobMatches(s.Pattern, rel) {
			merged = mergeEditorConfigSection(merged, s)
		}
	}

	return merged
}

func mergeEditorConfigSection(base, overlay EditorConfigSection) EditorConfigSection {
	out := base

	if overlay.IndentStyle != "" {
		out.IndentStyle = overlay.IndentStyle
	}

	if overlay.IndentSize != 0 {
		out.IndentSize = overlay.IndentSize
	}

	if overlay.TabWidth != 0 {
		out.TabWidth = overlay.TabWidth
	}

	if overlay.EndOfLine != "" {
		out.EndOfLine = overlay.EndOfLine
	}

	if overlay.MaxLineLength != 0 {
		out.MaxLineLength = overlay.MaxLineLength
	}

	return out
}

// EditorConfigResolver resolves a file's .editorconfig chain, nearest first,
// up to root=true. It is safe for concurrent use.
type EditorConfigResolver struct {
	baseDir   string
	baseChain []*EditorConfig
	// cache is owned by the probeMemo, so the resolvers of one run share their
	// parsed files rather than each re-reading the chain above them.
	cache *sync.Map
}

// The chain above baseDir is read up front, so an unreadable config there
// fails detection; configs below it are read per file.
func newEditorConfigResolver(baseDir string, memo *probeMemo) (*EditorConfigResolver, error) {
	resolver := &EditorConfigResolver{baseDir: baseDir, cache: &memo.editorConfigs}

	base, err := resolver.chain(baseDir)
	if err != nil {
		return nil, err
	}

	resolver.baseChain = base

	return resolver, nil
}

// Global merges the [*] sections of baseDir's chain, nearest config winning.
func (r *EditorConfigResolver) Global() EditorConfigSection {
	merged := EditorConfigSection{}

	for _, v := range slices.Backward(r.baseChain) {
		merged = mergeEditorConfigSection(merged, v.Global())
	}

	return merged
}

// ResolveForFile merges the sections matching absPath across its directory
// chain, nearest config winning.
func (r *EditorConfigResolver) ResolveForFile(absPath string) (EditorConfigSection, error) {
	chain, err := r.chain(filepath.Dir(absPath))
	if err != nil {
		return EditorConfigSection{}, err
	}

	merged := EditorConfigSection{}

	for _, ec := range slices.Backward(chain) {
		rel, relErr := filepath.Rel(filepath.Dir(ec.Path), absPath)
		if relErr != nil {
			rel = absPath
		}

		merged = mergeEditorConfigSection(merged, ec.ResolveForFile(rel))
	}

	return merged, nil
}

func (r *EditorConfigResolver) chain(startDir string) ([]*EditorConfig, error) {
	var chain []*EditorConfig

	err := upward.Walk(startDir, func(dir string) (bool, error) {
		ec, err := r.load(dir)
		if err != nil {
			return false, err
		}

		if ec != nil {
			chain = append(chain, ec)

			if ec.Root {
				return true, nil
			}
		}

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return chain, nil
}

func (r *EditorConfigResolver) load(dir string) (*EditorConfig, error) {
	if cached, ok := r.cache.Load(dir); ok {
		ec, _ := cached.(*EditorConfig)

		return ec, nil
	}

	ec, _, err := readEditorConfig(dir)
	if err != nil {
		return nil, err
	}

	// LoadOrStore, not Store: concurrent misses on the same dir converge on a
	// single cached *EditorConfig rather than racing to overwrite it.
	actual, _ := r.cache.LoadOrStore(dir, ec)
	shared, _ := actual.(*EditorConfig)

	return shared, nil
}

func readEditorConfig(dir string) (*EditorConfig, bool, error) {
	path := filepath.Join(dir, editorConfigFileName)

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}

		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	ec := &EditorConfig{Path: path}

	if err := parseEditorConfig(raw, ec); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}

	return ec, true, nil
}

func parseEditorConfig(raw []byte, ec *EditorConfig) error {
	raw = bytes.TrimPrefix(raw, []byte(walk.UTF8BOM))
	scanner := bufio.NewScanner(bytes.NewReader(raw))

	var current *EditorConfigSection

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			pattern := line[1 : len(line)-1]
			ec.Sections = append(ec.Sections, EditorConfigSection{Pattern: pattern})
			current = &ec.Sections[len(ec.Sections)-1]

			continue
		}

		rawKey, rawValue, ok := strings.Cut(line, "=")

		if !ok {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(rawKey))
		value := strings.TrimSpace(strings.ToLower(rawValue))

		if current == nil {
			if key == "root" && value == "true" {
				ec.Root = true
			}

			continue
		}

		applyEditorConfigKey(current, key, value)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	return nil
}

// The spec lets any property be cleared with `unset`, which drops back to
// whatever the tool would do on its own rather than to the broader section's
// value. Values arrive lowercased, so `UNSET` lands here as `unset`.
const editorConfigUnset = "unset"

// editorConfigCleared marks a numeric property the file cleared. The zero value
// cannot serve: it already means the key was never mentioned, which inherits.
const editorConfigCleared = -1

func applyEditorConfigKey(section *EditorConfigSection, key, value string) {
	switch key {
	case "indent_style":
		section.IndentStyle = value
	case "indent_size":
		// `tab` defers the width to tab_width, and clearing the size is exactly
		// how applyEditorConfigSection already falls through to it.
		section.IndentSize = editorConfigSize(value, value == "tab")
	case "tab_width":
		section.TabWidth = editorConfigSize(value, false)
	case "end_of_line":
		section.EndOfLine = value
	case "max_line_length":
		// `off` asks for no limit. laravel-fmt has no unlimited width, and an
		// effectively infinite one would collapse every array and chain onto
		// one line, so it falls back to printWidth the way an absent key does.
		section.MaxLineLength = editorConfigSize(value, value == "off")
	}
}

func editorConfigSize(value string, alsoClears bool) int {
	if alsoClears || value == editorConfigUnset {
		return editorConfigCleared
	}

	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}

	return n
}

// The one part of the grammar left out is the POSIX character class,
// [[:alpha:]]. The spec does not define it; the reference implementations
// accept it only because they hand the class body to PCRE and to minimatch.
func editorConfigGlobMatches(pattern, path string) bool {
	switch {
	case pattern == "":
		return path == ""
	case !strings.ContainsRune(pattern, '/'):
		pattern = "**/" + pattern
	case pattern[0] == '/':
		pattern = pattern[1:]
	}

	return matchEditorConfigGlob(pattern, path)
}

func matchEditorConfigGlob(pattern, path string) bool {
	if pattern == "" {
		return path == ""
	}

	switch pattern[0] {
	case '*':
		return matchEditorConfigStar(pattern, path)

	case '?':
		if path == "" || path[0] == '/' {
			return false
		}

		return matchEditorConfigGlob(pattern[1:], path[1:])

	case '[':
		if body, negated, rest, ok := parseEditorConfigClass(pattern); ok {
			if path == "" || path[0] == '/' || matchEditorConfigClass(body, path[0]) == negated {
				return false
			}

			return matchEditorConfigGlob(rest, path[1:])
		}

	case '{':
		if alts, rest, ok := splitEditorConfigAlternatives(pattern); ok {
			return slices.ContainsFunc(alts, func(alt string) bool {
				return matchEditorConfigGlob(alt+rest, path)
			})
		}

		if lo, hi, rest, ok := parseEditorConfigNumericRange(pattern); ok {
			return matchEditorConfigNumber(lo, hi, rest, path)
		}

	case '\\':
		if len(pattern) > 1 {
			if path == "" || path[0] != pattern[1] {
				return false
			}

			return matchEditorConfigGlob(pattern[2:], path[1:])
		}
	}

	if path == "" || pattern[0] != path[0] {
		return false
	}

	return matchEditorConfigGlob(pattern[1:], path[1:])
}

// ** crosses separators, and **/ also matches zero directories, so
// app/**/*.php covers app/User.php.
func matchEditorConfigStar(pattern, path string) bool {
	if strings.HasPrefix(pattern, "**/") {
		rest := pattern[3:]

		if matchEditorConfigGlob(rest, path) {
			return true
		}

		for i := range len(path) {
			if path[i] == '/' && matchEditorConfigGlob(rest, path[i+1:]) {
				return true
			}
		}

		return false
	}

	if strings.HasPrefix(pattern, "**") {
		rest := pattern[2:]

		for i := range len(path) + 1 {
			if matchEditorConfigGlob(rest, path[i:]) {
				return true
			}
		}

		return false
	}

	rest := pattern[1:]

	for i := range len(path) + 1 {
		if matchEditorConfigGlob(rest, path[i:]) {
			return true
		}

		if i < len(path) && path[i] == '/' {
			return false
		}
	}

	return false
}

// Per the spec, a class that never closes or holds a / is literal text, not a
// class, and ok is false.
func parseEditorConfigClass(pattern string) (body string, negated bool, rest string, ok bool) {
	i := 1

	// The spec writes negation as !, but both reference implementations also
	// take the ^ that regex and shell habits produce, because they hand the
	// sequence to PCRE and to minimatch. Reading ^ as an ordinary member would
	// silently match the wrong files.
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		negated = true
		i++
	}

	start := i

	// A ] straight after the opening bracket is a member, not the terminator.
	if i < len(pattern) && pattern[i] == ']' {
		i++
	}

	for i < len(pattern) {
		switch pattern[i] {
		case '\\':
			i += 2
		case '/':
			return "", false, "", false
		case ']':
			return pattern[start:i], negated, pattern[i+1:], true
		default:
			i++
		}
	}

	return "", false, "", false
}

func matchEditorConfigClass(body string, c byte) bool {
	for i := 0; i < len(body); {
		lo, next := editorConfigClassByte(body, i)
		i = next

		// A dash between two members makes a range; one at either end of the
		// sequence is an ordinary member, which is why [-ab] and [a-] work.
		if i < len(body)-1 && body[i] == '-' {
			hi, after := editorConfigClassByte(body, i+1)
			i = after

			if lo <= c && c <= hi {
				return true
			}

			continue
		}

		if c == lo {
			return true
		}
	}

	return false
}

func editorConfigClassByte(body string, i int) (byte, int) {
	if body[i] == '\\' && i+1 < len(body) {
		return body[i+1], i + 2
	}

	return body[i], i + 1
}

// Braces alternate only around a comma list; {single} and {} are literal.
func splitEditorConfigAlternatives(pattern string) ([]string, string, bool) {
	closeIdx := editorConfigMatchingBrace(pattern)
	if closeIdx < 0 {
		return nil, "", false
	}

	alts := splitTopLevelCommas(pattern[1:closeIdx])
	if len(alts) < 2 {
		return nil, "", false
	}

	return alts, pattern[closeIdx+1:], true
}

// A range is numeric or not a range: {a..c} is literal text, as the spec's
// own test suite asserts.
func parseEditorConfigNumericRange(pattern string) (string, string, string, bool) {
	closeIdx := editorConfigMatchingBrace(pattern)
	if closeIdx < 0 {
		return "", "", "", false
	}

	lo, hi, found := strings.Cut(pattern[1:closeIdx], "..")
	if !found || !isEditorConfigInteger(lo) || !isEditorConfigInteger(hi) {
		return "", "", "", false
	}

	return lo, hi, pattern[closeIdx+1:], true
}

// Digit runs are tried shortest first, so {1..3}4 matches 24 by reading 2.
func matchEditorConfigNumber(loText, hiText, rest, path string) bool {
	lo, loErr := strconv.Atoi(loText)
	hi, hiErr := strconv.Atoi(hiText)

	if loErr != nil || hiErr != nil {
		return false
	}

	if lo > hi {
		lo, hi = hi, lo
	}

	// Writing the bounds zero-padded asks for zero-padded names: {01..09}
	// takes 07 and not 7, while plain {3..120} takes 60 and not 060.
	width := 0
	if len(loText) > 1 && loText[0] == '0' {
		width = len(loText)
	}

	start := 0
	if strings.HasPrefix(path, "-") {
		start = 1
	}

	end := start
	for end < len(path) && path[end] >= '0' && path[end] <= '9' {
		end++
	}

	for stop := start + 1; stop <= end; stop++ {
		text := path[:stop]

		n, err := strconv.Atoi(text)
		if err != nil || n < lo || n > hi || text != formatEditorConfigNumber(n, width) {
			continue
		}

		if matchEditorConfigGlob(rest, path[stop:]) {
			return true
		}
	}

	return false
}

func formatEditorConfigNumber(n, width int) string {
	if width > 0 {
		return fmt.Sprintf("%0*d", width, n)
	}

	return strconv.Itoa(n)
}

func isEditorConfigInteger(s string) bool {
	s = strings.TrimPrefix(s, "-")

	if s == "" {
		return false
	}

	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}

	return true
}

func editorConfigMatchingBrace(pattern string) int {
	depth := 0

	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--

			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

func splitTopLevelCommas(s string) []string {
	var parts []string

	depth := 0
	start := 0

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}

	return append(parts, s[start:])
}

func applyEditorConfig(cfg *config.File, resolver *EditorConfigResolver) {
	applyEditorConfigSection(cfg, resolver.Global(), nil)
}

func applyEditorConfigSection(cfg *config.File, section EditorConfigSection, userKeys map[string]bool) {
	if !userKeys["indentStyle"] {
		switch section.IndentStyle {
		case "tab":
			cfg.IndentStyle = config.IndentStyleTab
		case "space":
			cfg.IndentStyle = config.IndentStyleSpace
		}
	}

	if !userKeys["indentWidth"] {
		switch {
		case section.IndentSize > 0:
			cfg.TabWidth = section.IndentSize
			cfg.IndentExplicit = true
		case section.TabWidth > 0:
			cfg.TabWidth = section.TabWidth
			cfg.IndentExplicit = true
		}
	}

	if !userKeys["endOfLine"] {
		switch section.EndOfLine {
		case "lf":
			cfg.EndOfLine = config.EndOfLineLF
		case "crlf":
			cfg.EndOfLine = config.EndOfLineCRLF
		}
	}

	if !userKeys["printWidth"] {
		if section.MaxLineLength > 0 {
			cfg.PrintWidth = section.MaxLineLength
		}
	}
}
