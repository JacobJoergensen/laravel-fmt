package tailwind

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/JacobJoergensen/laravel-fmt/internal/parser/css"
)

// ThemeToken is one `--<namespace>-<name>` declaration from a project's
// `@theme` block. Only the container and breakpoint namespaces read Value,
// the width their variants compare by.
type ThemeToken struct {
	Namespace string
	Name      string
	Value     string
}

// themeNamespaces are the v4 theme namespaces ExtractTheme recognizes
// (https://tailwindcss.com/docs/theme). Order matters: a namespace containing a `-`
// MUST precede any shorter prefix of it, so longest-prefix-match resolves
// `--font-weight-bold` to `font-weight` (name `bold`), not `font`.
var themeNamespaces = []string{
	"font-weight",
	"font-stretch",
	"text-shadow",
	"inset-shadow",
	"drop-shadow",
	"color",
	"font",
	"text",
	"tracking",
	"leading",
	"spacing",
	"radius",
	"shadow",
	"blur",
	"perspective",
	"aspect",
	"ease",
	"animate",
	"breakpoint",
	"container",
	"default",
}

type branch uint8

const (
	branchDefault branch = iota
	branchColor
	branchSize
)

// stemTokenKey identifies a (stem, token) pair that maps to a non-default branch,
// used only for namespaces whose name is also a stem (`--text-*` on `text`,
// `--shadow-*` on `shadow`). The stem-agnostic `--color-*` set lives in a separate
// flat map.
type stemTokenKey struct {
	Stem  string
	Token string
}

// Theme is one project's Tailwind customization: its `@theme` tokens, `@utility`
// and `@custom-variant` definitions, and prefixes. A sort reads only the Theme it
// is handed, so one parallel run can format files from several projects. A Theme
// is immutable once built, and a nil *Theme is Tailwind without customization.
type Theme struct {
	colorTokens        map[string]bool
	tokenNames         map[string]bool
	stemSpecificBranch map[stemTokenKey]branch
	breakpoints        map[string]string
	containers         map[string]string
	customUtilities    map[string]utilityEntry
	customUtilityStems map[string]utilityEntry
	customVariants     map[string]variantEntry
	customVariantStems map[string]variantEntry
	prefixes           []string
}

func newTheme(tokens []ThemeToken, utilities []CustomUtility, variants []CustomVariant, prefixes []string) *Theme {
	ensureRegistry()

	colors := make(map[string]bool, len(tokens))
	names := make(map[string]bool, len(tokens))
	stemSpecific := make(map[stemTokenKey]branch)

	var breakpoints, containers map[string]string

	for _, t := range tokens {
		names[t.Name] = true

		switch t.Namespace {
		case "color":
			colors[t.Name] = true
		case "text":
			stemSpecific[stemTokenKey{Stem: "text", Token: t.Name}] = branchSize
		case "shadow":
			stemSpecific[stemTokenKey{Stem: "shadow", Token: t.Name}] = branchSize
		case "breakpoint":
			breakpoints = withThemeValue(breakpoints, defaultBreakpoints, t)
		case "container":
			containers = withThemeValue(containers, defaultContainers, t)
		}
	}

	var customUtilities, customUtilityStems map[string]utilityEntry

	for _, u := range utilities {
		entry := buildCustomUtilityEntry(u.Properties)

		if stem, ok := functionalStem(u.Name); ok {
			if customUtilityStems == nil {
				customUtilityStems = make(map[string]utilityEntry)
			}

			customUtilityStems[stem] = entry
		} else if !strings.ContainsRune(u.Name, '*') {
			if customUtilities == nil {
				customUtilities = make(map[string]utilityEntry)
			}

			customUtilities[u.Name] = entry
		}
	}

	var customVariants, customVariantStems map[string]variantEntry
	order := customVariantOrder

	for _, v := range variants {
		if stem, ok := functionalStem(v.Name); ok {
			if _, seen := customVariantStems[stem]; seen {
				continue
			}

			if customVariantStems == nil {
				customVariantStems = make(map[string]variantEntry)
			}

			// A `name-*` variant has no one selector to read, so it may
			// compound with anything.
			customVariantStems[stem] = variantEntry{
				Kind:      variantFunctional,
				Order:     order,
				Compounds: compoundsAtRules | compoundsStyleRules,
			}
			order++
		} else if !strings.ContainsRune(v.Name, '*') {
			if _, seen := customVariants[v.Name]; seen {
				continue
			}

			if customVariants == nil {
				customVariants = make(map[string]variantEntry)
			}

			customVariants[v.Name] = variantEntry{
				Kind:      variantStatic,
				Order:     order,
				Compounds: compoundsForSelectors(v.Selectors...),
			}
			order++
		}
	}

	theme := &Theme{
		colorTokens:        colors,
		tokenNames:         names,
		stemSpecificBranch: stemSpecific,
		breakpoints:        breakpoints,
		containers:         containers,
		customUtilities:    customUtilities,
		customUtilityStems: customUtilityStems,
		customVariants:     customVariants,
		customVariantStems: customVariantStems,
		prefixes:           prefixes,
	}

	return theme
}

// withThemeValue applies one `--breakpoint-*` or `--container-*` token over the
// defaults, copying them the first time so a project with none shares them. As
// in Tailwind's theme, `initial` removes the name, or with `*` all of them.
func withThemeValue(values, defaults map[string]string, t ThemeToken) map[string]string {
	if values == nil {
		values = maps.Clone(defaults)
	}

	value := strings.TrimSpace(t.Value)

	switch {
	case value != "initial":
		values[t.Name] = value
	case t.Name == "*":
		clear(values)
	default:
		delete(values, t.Name)
	}

	return values
}

func functionalStem(name string) (string, bool) {
	stem, ok := strings.CutSuffix(name, "-*")
	if !ok || stem == "" {
		return "", false
	}

	return stem, true
}

// buildCustomUtilityEntry builds an `@utility` class's sort key from its CSS
// properties: Order holds each known property's propertyIndex rank in
// declaration order, duplicates collapsed. A utility with no known property
// gets an empty Order and sorts at the end, not at the front like Unknown,
// since the project did declare it.
func buildCustomUtilityEntry(properties []string) utilityEntry {
	var order []int
	seen := make(map[int]bool, len(properties))

	for _, p := range properties {
		idx, ok := propertyIndex[p]
		if !ok || seen[idx] {
			continue
		}

		seen[idx] = true
		order = append(order, idx)
	}

	return utilityEntry{Order: order, Count: len(properties)}
}

func (t *Theme) customUtility(name string) (utilityEntry, bool) {
	if t == nil || t.customUtilities == nil {
		return utilityEntry{}, false
	}

	entry, ok := t.customUtilities[name]

	return entry, ok
}

// longestHyphenPrefix looks key up whole, then retries on each hyphen-delimited
// prefix from the longest down, so `text-glow-strong` can match the `text-glow`
// its stem was registered under.
func longestHyphenPrefix[V any](stems map[string]V, key string) (V, bool) {
	if value, ok := stems[key]; ok {
		return value, true
	}

	for i := len(key) - 1; i > 0; i-- {
		if key[i] != '-' {
			continue
		}

		if value, ok := stems[key[:i]]; ok {
			return value, true
		}
	}

	var zero V

	return zero, false
}

func (t *Theme) matchCustomUtilityStem(base string) (utilityEntry, bool) {
	// The nil-map check stays: without it a project that registers no @utility
	// would run the whole backwards scan for every class token.
	if t == nil || t.customUtilityStems == nil {
		return utilityEntry{}, false
	}

	return longestHyphenPrefix(t.customUtilityStems, base)
}

// classPrefixes returns the class prefixes the project's bundles use. Nil means
// no bundle sets one, which is the overwhelmingly common case and the caller's
// fast path: parsing then never looks at a prefix at all.
func (t *Theme) classPrefixes() []string {
	if t == nil {
		return nil
	}

	return t.prefixes
}

// requiresPrefix reports whether every class must lead with a prefix, which holds
// only when no bundle accepts unprefixed classes.
func requiresPrefix(prefixes []string) bool {
	return len(prefixes) > 0 && !slices.Contains(prefixes, "")
}

func (t *Theme) breakpoint(name string) (string, bool) {
	values := defaultBreakpoints
	if t != nil && t.breakpoints != nil {
		values = t.breakpoints
	}

	value, ok := values[name]

	return value, ok
}

func (t *Theme) container(name string) (string, bool) {
	values := defaultContainers
	if t != nil && t.containers != nil {
		values = t.containers
	}

	value, ok := values[name]

	return value, ok
}

func (t *Theme) removedBreakpoint(name string) bool {
	_, isDefault := defaultBreakpoints[name]
	_, kept := t.breakpoint(name)

	return isDefault && !kept
}

func (t *Theme) hasVariant(root string) bool {
	_, ok := t.variantEntry(root)

	return ok
}

// variantEntry returns how Tailwind registers root. Breakpoints come from the
// theme, one static variant in the `min-*` group per `--breakpoint-*`, so `md`
// is no variant once a project resets the namespace. Any other built-in wins
// over a project's own, so `group-*` cannot take `group-hover`, and a project's
// `@custom-variant`s follow every built-in in the order the stylesheet
// declares them.
func (t *Theme) variantEntry(root string) (variantEntry, bool) {
	if _, breakpoint := defaultBreakpoints[root]; !breakpoint {
		if entry, ok := variantRegistry[root]; ok {
			return entry, true
		}
	}

	if _, ok := t.breakpoint(root); ok {
		group := variantRegistry["min"]

		return variantEntry{Kind: variantStatic, Order: group.Order, Compounds: group.Compounds}, true
	}

	if t == nil {
		return variantEntry{}, false
	}

	if entry, ok := t.customVariants[root]; ok {
		return entry, true
	}

	if entry, ok := t.customVariantStems[root]; ok {
		return entry, true
	}

	return variantEntry{}, false
}

// unknownVariantOrder places a variant no registration covers after the
// project's own.
func (t *Theme) unknownVariantOrder() int {
	if t == nil {
		return customVariantOrder
	}

	return customVariantOrder + len(t.customVariants) + len(t.customVariantStems)
}

// LoadTheme reads each CSS entry point and builds one Theme from the tokens,
// utilities, custom variants, and prefixes across all of them. It returns an
// error rather than logging so the caller decides what a broken stylesheet means.
//
// The entry points merge in the order given, later definitions winning as they do
// in CSS. Each expands against its own `seen` set, so it reads exactly as it would
// alone: sharing one set across them would drop a partial that two entry points
// import in different orders, silently handing the conflict to the wrong file.
//
// Loading the same unchanged stylesheets again returns the cached Theme, since
// the language server resolves a project on every request.
func LoadTheme(paths ...string) (*Theme, error) {
	key := strings.Join(paths, "\x00")

	themeCache.Lock()
	cached := themeCache.entries[key]
	themeCache.Unlock()

	if cached.stillCurrent() {
		return cached.theme, nil
	}

	var src []byte

	stamps := make(map[string]fileStamp)

	for _, path := range paths {
		expanded, err := readWithImports(path, map[string]bool{}, stamps)
		if err != nil {
			return nil, err
		}

		src = append(append(src, expanded...), '\n')
	}

	theme := newTheme(ExtractTheme(src), ExtractUtilities(src), ExtractCustomVariants(src), ExtractPrefixes(src))

	themeCache.Lock()
	themeCache.entries[key] = &cachedTheme{theme: theme, stamps: stamps}
	themeCache.Unlock()

	return theme, nil
}

// fileStamp is how a stylesheet is recognized as unchanged. Size alone misses an
// edit that keeps the length; the modification time alone misses a rewrite
// within the filesystem's timestamp resolution.
type fileStamp struct {
	modified time.Time
	size     int64
}

// cachedTheme is a built Theme with every file its expansion read. A file added
// to the graph needs an `@import` written into a file already in it, and that
// file's stamp changes, so the recorded set is enough to notice one appearing.
type cachedTheme struct {
	theme  *Theme
	stamps map[string]fileStamp
}

// themeCache holds one entry per stylesheet set, so a language server moving
// between projects keeps each one's Theme instead of rebuilding on every switch.
// It grows by one small entry per distinct set the process loads.
var themeCache = struct {
	sync.Mutex

	entries map[string]*cachedTheme
}{entries: make(map[string]*cachedTheme)}

func (c *cachedTheme) stillCurrent() bool {
	if c == nil {
		return false
	}

	for path, want := range c.stamps {
		info, err := os.Stat(path)
		if err != nil || info.Size() != want.size || !info.ModTime().Equal(want.modified) {
			return false
		}
	}

	return true
}

// readStamped reads a stylesheet and records what it looked like, taking the
// stamp from the open handle so recognizing it later costs no extra lookup now.
func readStamped(path string) ([]byte, fileStamp, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fileStamp{}, fmt.Errorf("read tailwind entry: %w", err)
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, fileStamp{}, fmt.Errorf("read tailwind entry: %w", err)
	}

	buf := bytes.NewBuffer(make([]byte, 0, info.Size()))

	if _, err := buf.ReadFrom(file); err != nil {
		return nil, fileStamp{}, fmt.Errorf("read tailwind entry: %w", err)
	}

	return buf.Bytes(), fileStamp{modified: info.ModTime(), size: info.Size()}, nil
}

// maxImportedFiles bounds one entry point's graph so a generated tree cannot
// make the formatter read the disk without end. It is generous because
// vendored UI kits are large: Filament's panel theme reaches 129 files.
const maxImportedFiles = 1024

// ImportsTailwind reports whether the stylesheet at path pulls in Tailwind,
// directly or through the files it imports, which is what makes it an entry
// point rather than a partial.
//
// The test has to follow imports because a generated theme need not name
// Tailwind itself. Filament writes resources/css/filament/<panel>/theme.css as a
// single `@import` of its vendored stylesheet, and that file is where
// `@import "tailwindcss"` lives. A missing or unreadable file is not an entry
// point rather than an error: the caller is surveying a directory, not acting on
// a path someone named.
func ImportsTailwind(path string) bool {
	return importsTailwind(path, map[string]bool{})
}

// The check is a substring on the quoted prefix (no closing quote) so sub-path
// imports like "tailwindcss/preflight" and qualifiers like `source(none)` still
// match.
func looksLikeTailwindV4Import(content []byte) bool {
	return bytes.Contains(content, []byte(`@import "tailwindcss`)) ||
		bytes.Contains(content, []byte(`@import 'tailwindcss`))
}

func importsTailwind(path string, seen map[string]bool) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	if seen[abs] || len(seen) >= maxImportedFiles {
		return false
	}

	seen[abs] = true

	src, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	if looksLikeTailwindV4Import(src) {
		return true
	}

	dir := filepath.Dir(abs)

	for _, ref := range scanImports(src) {
		target := filepath.Join(dir, filepath.FromSlash(ref.target))

		info, statErr := os.Stat(target)
		if statErr != nil || info.IsDir() {
			continue
		}

		if importsTailwind(target, seen) {
			return true
		}
	}

	return false
}

// readWithImports reads the stylesheet at path with every local `@import`
// replaced by the file it names, expanded the same way, since a project may
// split its `@theme`, `@utility` and `@custom-variant` across files. The
// content is spliced in where the at-rule stood, because a later definition
// wins, as in CSS. seen guards one entry point against cycles; stamps records
// every file read across the whole registration.
func readWithImports(path string, seen map[string]bool, stamps map[string]fileStamp) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	// A file already read is a cycle, which is legal enough to survive quietly.
	// Running out of budget is not: dropping the rest of the graph loses whatever
	// it defined, and the sorter would go on as if those classes did not exist.
	if seen[abs] {
		return nil, nil
	}

	if len(seen) >= maxImportedFiles {
		return nil, fmt.Errorf("tailwind stylesheet imports more than %d files, stopping at %s", maxImportedFiles, path)
	}

	seen[abs] = true

	src, stamp, err := readStamped(path)
	if err != nil {
		return nil, err
	}

	stamps[abs] = stamp

	var out []byte

	dir := filepath.Dir(abs)
	last := 0

	for _, ref := range scanImports(src) {
		target := filepath.Join(dir, filepath.FromSlash(ref.target))

		// A target we cannot find is a package specifier (`tailwindcss`) or a URL,
		// which the bundler resolves and we have no business guessing at. The
		// built-in registry already covers Tailwind's own tokens.
		info, statErr := os.Stat(target)
		if statErr != nil || info.IsDir() {
			continue
		}

		nested, err := readWithImports(target, seen, stamps)
		if err != nil {
			return nil, err
		}

		out = append(out, src[last:ref.start]...)
		out = append(out, nested...)
		out = append(out, '\n')
		last = ref.end
	}

	if out == nil {
		return src, nil
	}

	return append(out, src[last:]...), nil
}

// importRef is one `@import` at-rule: the path it names and the byte range of
// the whole rule, so the caller can splice the file in where it stood.
type importRef struct {
	target     string
	start, end int
}

// scanImports finds the `@import` at-rules in src, skipping comments so a rule
// written inside one is not followed. `url(...)` and a bare quoted target are
// both accepted; the at-rule runs to its `;`.
func scanImports(src []byte) []importRef {
	var refs []importRef

	for i := 0; i < len(src); {
		if bytes.HasPrefix(src[i:], []byte("/*")) {
			end := bytes.Index(src[i+2:], []byte("*/"))
			if end < 0 {
				break
			}

			i += 2 + end + 2

			continue
		}

		if src[i] != '@' || !hasPrefixFold(src[i:], "@import") {
			i++

			continue
		}

		ref, next, ok := parseImport(src, i)
		if !ok {
			i++

			continue
		}

		if ref.target != "" {
			refs = append(refs, ref)
		}

		i = next
	}

	return refs
}

// parseImport reads the at-rule starting at i, returning it and the offset just
// past its `;`.
func parseImport(src []byte, i int) (importRef, int, bool) {
	j := i + len("@import")

	for j < len(src) && css.IsSpaceByte(src[j]) {
		j++
	}

	if bytes.HasPrefix(src[j:], []byte("url(")) {
		j += len("url(")

		for j < len(src) && css.IsSpaceByte(src[j]) {
			j++
		}
	}

	if j >= len(src) || (src[j] != '"' && src[j] != '\'') {
		return importRef{}, 0, false
	}

	quote := src[j]
	j++
	start := j

	for j < len(src) && src[j] != quote && src[j] != '\n' {
		j++
	}

	if j >= len(src) || src[j] != quote {
		return importRef{}, 0, false
	}

	target := string(src[start:j])

	// The rule may carry a layer or media query after the target; it ends at `;`.
	for j < len(src) && src[j] != ';' {
		j++
	}

	if j >= len(src) {
		return importRef{}, 0, false
	}

	return importRef{target: target, start: i, end: j + 1}, j + 1, true
}

// hasPrefixFold matches an at-rule name, which CSS compares case-insensitively.
func hasPrefixFold(src []byte, prefix string) bool {
	if len(src) < len(prefix) {
		return false
	}

	return strings.EqualFold(string(src[:len(prefix)]), prefix)
}

// branchFor reports the branch hint the project's `@theme` gives a (stem, value)
// pair, or branchDefault when none matches. A stem-specific entry wins over the
// stem-agnostic color set, mirroring the engine's most-specific-first lookup.
func (t *Theme) branchFor(stemName, value string) branch {
	if t == nil {
		return branchDefault
	}

	if hint, ok := t.stemSpecificBranch[stemTokenKey{Stem: stemName, Token: value}]; ok {
		return hint
	}

	if t.colorTokens[value] {
		return branchColor
	}

	return branchDefault
}

// namedValueExists reports whether a stem's value can produce CSS. The registry
// already holds every default-theme name, so past it Tailwind only accepts a
// project token, a bare number (`p-13`) or a `(--var)` shorthand; anything else
// (`text-coral-500` with no `--color-coral-500`) is a class the engine does not
// generate, which the plugin sorts as unknown. Without a project theme its
// tokens are not known, so every value is taken to exist.
func (t *Theme) namedValueExists(value string) bool {
	if t == nil || value == "" {
		return true
	}

	switch first := value[0]; {
	case first == '[' || first == '(':
		return true
	case first >= '0' && first <= '9':
		return true
	}

	return t.tokenNames[value]
}

// ExtractTheme returns every recognized token in any top-level `@theme` block of src,
// split into (namespace, name) by longest-prefix match against themeNamespaces. Parse
// errors are non-fatal: it walks whatever CST the CSS parser recovered, returning an
// empty result for fully unparseable input rather than panicking.
func ExtractTheme(src []byte) []ThemeToken {
	file, _ := css.Parse(src)

	var out []ThemeToken

	for _, n := range file.Nodes {
		atRule, ok := n.(*css.AtRule)

		if !ok || !atRule.Block || !bytes.EqualFold(atRule.Name, []byte("theme")) {
			continue
		}

		for _, child := range atRule.Children {
			decl, ok := child.(*css.Declaration)

			if !ok {
				continue
			}

			property := bytes.TrimSpace(decl.Property)

			if !bytes.HasPrefix(property, []byte("--")) {
				continue
			}

			namespace, name := splitThemeNamespace(string(property[2:]))

			if namespace == "" || name == "" {
				continue
			}

			out = append(out, ThemeToken{
				Namespace: namespace,
				Name:      name,
				Value:     string(bytes.TrimSpace(decl.Value)),
			})
		}
	}

	return out
}

// CustomUtility is one `@utility <name> { ... }` block. Properties holds each direct
// declaration's CSS property in source order, NOT deduplicated; the declaration
// count feeds the sort key.
type CustomUtility struct {
	Name       string
	Properties []string
}

// ExtractUtilities returns every top-level `@utility` class in src, static and
// functional alike (the functional `tab-*` keeps its `*`). Only direct declarations
// are read; ones nested in a rule inside the block (`&:hover { ... }`) are not. Parse
// errors are non-fatal: it walks whatever CST the parser recovered.
func ExtractUtilities(src []byte) []CustomUtility {
	file, _ := css.Parse(src)

	var out []CustomUtility

	for _, n := range file.Nodes {
		atRule, ok := n.(*css.AtRule)

		if !ok || !atRule.Block || !bytes.EqualFold(atRule.Name, []byte("utility")) {
			continue
		}

		name := string(bytes.TrimSpace(atRule.Prelude))

		if name == "" || strings.ContainsAny(name, " \t\r\n") {
			continue
		}

		var properties []string

		for _, child := range atRule.Children {
			decl, ok := child.(*css.Declaration)

			if !ok {
				continue
			}

			property := string(bytes.TrimSpace(decl.Property))

			if property == "" {
				continue
			}

			properties = append(properties, property)
		}

		out = append(out, CustomUtility{Name: name, Properties: properties})
	}

	return out
}

// CustomVariant is one `@custom-variant`: its name and the selectors Tailwind
// reads its compounds from.
type CustomVariant struct {
	Name      string
	Selectors []string
}

// ExtractCustomVariants returns every top-level `@custom-variant` in src, in
// source order. Parse errors are non-fatal: it walks whatever CST the parser
// recovered.
func ExtractCustomVariants(src []byte) []CustomVariant {
	file, _ := css.Parse(src)

	var out []CustomVariant

	for _, n := range file.Nodes {
		atRule, ok := n.(*css.AtRule)

		if !ok || !bytes.EqualFold(atRule.Name, []byte("custom-variant")) {
			continue
		}

		name := leadingVariantName(atRule.Prelude)

		if name == "" {
			continue
		}

		out = append(out, CustomVariant{Name: name, Selectors: customVariantSelectors(atRule, name)})
	}

	return out
}

// customVariantSelectors collects them as index.ts does: the shorthand's list,
// `(&:hover, &:focus)`, or the body's through bodySelectors.
func customVariantSelectors(atRule *css.AtRule, name string) []string {
	if atRule.Block {
		return bodySelectors(atRule.Children, nil)
	}

	list := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(atRule.Prelude), []byte(name)))

	if len(list) < 2 || list[0] != '(' || list[len(list)-1] != ')' {
		return nil
	}

	var selectors []string

	for selector := range segments(string(list[1:len(list)-1]), ',') {
		selectors = append(selectors, strings.TrimSpace(selector))
	}

	return selectors
}

// bodySelectors walks a variant body as fromAst in variants.ts does.
func bodySelectors(nodes []css.Node, selectors []string) []string {
	for _, n := range nodes {
		switch node := n.(type) {
		case *css.Rule:
			selectors = append(selectors, string(bytes.TrimSpace(node.Selector)))
			selectors = bodySelectors(node.Children, selectors)
		case *css.AtRule:
			name := string(node.Name)

			if name != "slot" && name != "variant" {
				selectors = append(selectors, "@"+name+" "+string(bytes.TrimSpace(node.Prelude)))
			}

			selectors = bodySelectors(node.Children, selectors)
		}
	}

	return selectors
}

func leadingVariantName(prelude []byte) string {
	prelude = bytes.TrimSpace(prelude)

	end := bytes.IndexFunc(prelude, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '('
	})

	if end < 0 {
		return string(prelude)
	}

	return string(prelude[:end])
}

// ExtractPrefixes returns the class prefix each `@import "tailwindcss"` sets, in
// source order, with "" for an import that sets none. Parse errors are non-fatal:
// it walks whatever CST the parser recovered.
//
// It is plural because entry points merge, and a prefix is the one thing a merged
// registry cannot represent by name: with a single prefix recorded, every class in
// a bundle that uses a different one parses as unprefixed and resolves to Unknown,
// so that bundle's views sort as if Tailwind were not installed at all.
func ExtractPrefixes(src []byte) []string {
	file, _ := css.Parse(src)

	var out []string

	for _, n := range file.Nodes {
		atRule, ok := n.(*css.AtRule)

		if !ok || !bytes.EqualFold(atRule.Name, []byte("import")) {
			continue
		}

		// Only Tailwind's own import carries the prefix, and an import without one
		// says the bundle takes unprefixed classes, which is itself worth recording.
		if !bytes.Contains(atRule.Prelude, []byte("tailwindcss")) {
			continue
		}

		prefix := importPrefix(atRule.Prelude)

		if !slices.Contains(out, prefix) {
			out = append(out, prefix)
		}
	}

	return out
}

func importPrefix(prelude []byte) string {
	_, after, ok0 := bytes.Cut(prelude, []byte("prefix("))

	if !ok0 {
		return ""
	}

	before, _, ok := bytes.Cut(after, []byte{')'})

	if !ok {
		return ""
	}

	return string(bytes.TrimSpace(before))
}

func splitThemeNamespace(rest string) (string, string) {
	for _, ns := range themeNamespaces {
		if len(rest) > len(ns) && rest[len(ns)] == '-' && rest[:len(ns)] == ns {
			return ns, rest[len(ns)+1:]
		}
	}

	return "", ""
}
