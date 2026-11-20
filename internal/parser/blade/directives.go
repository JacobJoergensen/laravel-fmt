package blade

import (
	"bytes"
	"maps"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

// BlockDirective declares a project's own block directive, the way Blade::if or
// a Blade::directive pair defines one: its opener, its closer, and the branches
// that may sit between them.
type BlockDirective struct {
	Open     string
	Close    string
	Branches []string
}

// Directives is the set of block directives a parse recognizes: Blade's own,
// plus any a project declares for directives of its own (Pennant's @feature).
type Directives struct {
	closers     map[string][]string
	branches    map[string][]string
	closerNames map[string]bool
	branchNames map[string]bool
}

var builtinDirectives = &Directives{
	closers:     blockDirectiveClosers,
	branches:    blockDirectiveBranches,
	closerNames: derivedNameSet(blockDirectiveClosers),
	branchNames: derivedNameSet(blockDirectiveBranches),
}

// NewDirectives returns Blade's built-in block directives plus declared.
func NewDirectives(declared []BlockDirective) *Directives {
	if len(declared) == 0 {
		return builtinDirectives
	}

	closers := maps.Clone(blockDirectiveClosers)
	branches := maps.Clone(blockDirectiveBranches)

	for _, d := range declared {
		if IsBuiltinBlock(d.Open, d.Close, d.Branches) {
			continue
		}

		closers[d.Open] = []string{d.Close}

		if len(d.Branches) > 0 {
			branches[d.Open] = d.Branches
		}
	}

	return &Directives{
		closers:     closers,
		branches:    branches,
		closerNames: derivedNameSet(closers),
		branchNames: derivedNameSet(branches),
	}
}

// IsBuiltinOpenerOrCloser reports whether name already opens or closes one of
// Blade's own block directives, which a project cannot declare again.
func IsBuiltinOpenerOrCloser(name string) bool {
	return builtinDirectives.IsOpener(name) || builtinDirectives.IsCloser(name)
}

// IsBuiltinBlock reports whether opener is one of Blade's own block directives,
// closer one of its closers and every branch one of its branches, so declaring
// the block adds nothing.
func IsBuiltinBlock(opener, closer string, branches []string) bool {
	known := blockDirectiveBranches[opener]

	return slices.Contains(blockDirectiveClosers[opener], closer) &&
		!slices.ContainsFunc(branches, func(b string) bool { return !slices.Contains(known, b) })
}

// IsDirectiveName reports whether name is the \w+ Blade's compiler accepts as a
// directive name.
func IsDirectiveName(name string) bool {
	return name != "" && directiveNameLen([]byte(name)) == len(name)
}

func directiveNameLen(s []byte) int {
	n := 0
	for n < len(s) && (html.IsNameStart(s[n]) || html.IsDigit(s[n])) {
		n++
	}

	return n
}

// HasBuiltinBlockLines reports whether a line of src opens one of Blade's own
// block directives and a later line closes it. Alpine's `@error="…"` and
// Angular's `@if (x) {` never meet an `@end…`, so neither counts.
func HasBuiltinBlockLines(src []byte) bool {
	var open []string

	for line := range bytes.Lines(src) {
		name, ok := lineDirective(line)
		if !ok {
			continue
		}

		if slices.ContainsFunc(open, func(o string) bool { return IsBuiltinBlock(o, name, nil) }) {
			return true
		}

		if builtinDirectives.IsOpener(name) {
			open = append(open, name)
		}
	}

	return false
}

func lineDirective(line []byte) (string, bool) {
	line = bytes.TrimLeft(line, " \t")
	if len(line) == 0 || line[0] != '@' {
		return "", false
	}

	end := directiveNameLen(line[1:])

	return string(line[1 : 1+end]), end > 0
}

// BuiltinOpeners returns the names of Blade's own block directives, sorted.
func BuiltinOpeners() []string {
	return slices.Sorted(maps.Keys(blockDirectiveClosers))
}

// IsOpener reports whether name opens a block directive (if, foreach, …).
func (d *Directives) IsOpener(name string) bool {
	_, ok := d.closers[name]

	return ok
}

// IsCloser reports whether name closes a block directive (`endif`, `show`, …).
func (d *Directives) IsCloser(name string) bool {
	return d.closerNames[name]
}

// IsBranch reports whether name is a mid-block branch (`else`, `case`, …).
func (d *Directives) IsBranch(name string) bool {
	return d.branchNames[name]
}

func derivedNameSet(m map[string][]string) map[string]bool {
	set := make(map[string]bool)

	for _, names := range m {
		for _, name := range names {
			set[name] = true
		}
	}

	return set
}

// laravelArgDirectives are the directives illuminate/view compiles from an
// argument (one `compileX($arg)` method each, as of laravel/framework 13.34),
// keyed lowercase because PHP resolves `compile.ucfirst($name)` case
// insensitively, so `@hassection` reaches compileHasSection. Their argument
// is PHP, so one that does not parse is an error.
var laravelArgDirectives = nameSet(`
	auth aware bool break can canany cannot case checked choice class component
	componentfirst context continue dd disabled dump each elseauth elseguest
	elsepush elsepushif elsecan elsecanany elsecannot elseif empty endcontext
	enderror endsession env error extends extendsfirst fonts for foreach forelse
	fragment guest hassection hasstack if include includefirst includeif
	includeisolated includeunless includewhen inject isset js json lang method once
	php prepend prependonce props push pushif pushonce readonly required section
	sectionmissing selected session slot stack style switch unless unset use vite
	while yield`)

func nameSet(names string) map[string]bool {
	set := make(map[string]bool)

	for name := range strings.FieldsSeq(names) {
		set[name] = true
	}

	return set
}

func isLaravelArgDirective(name string) bool {
	return laravelArgDirectives[strings.ToLower(name)]
}
