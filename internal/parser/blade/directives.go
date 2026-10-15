package blade

import (
	"maps"
	"slices"
	"strings"
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
