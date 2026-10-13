// Package tailwind sorts Tailwind CSS class lists into the canonical order.
package tailwind

import (
	"slices"
	"strings"
)

// Sort returns classes reordered into the canonical Tailwind order for theme.
func Sort(theme *Theme, classes string) string {
	return SortPinned(theme, classes, false, false)
}

// IsClassSeparator reports whether c is HTML's ASCII whitespace, which a
// DOMTokenList splits a class list on; a vertical tab is not, so `a\vb` is one
// class.
func IsClassSeparator(c byte) bool {
	return classSeparators[c]
}

// ClassSeparators is the cutset form of IsClassSeparator.
const ClassSeparators = " \t\n\r\f"

var classSeparators = func() [256]bool {
	var set [256]bool

	for i := range len(ClassSeparators) {
		set[ClassSeparators[i]] = true
	}

	return set
}()

// classTokens splits classes at IsClassSeparator. It scans bytes: every
// separator is ASCII, and strings.FieldsFunc made Sort 6% slower.
func classTokens(classes string) []string {
	n := 0

	for i := range len(classes) {
		if !IsClassSeparator(classes[i]) && (i == 0 || IsClassSeparator(classes[i-1])) {
			n++
		}
	}

	tokens := make([]string, 0, n)
	begin := -1

	for i := range len(classes) {
		switch {
		case !IsClassSeparator(classes[i]) && begin < 0:
			begin = i
		case IsClassSeparator(classes[i]) && begin >= 0:
			tokens = append(tokens, classes[begin:i])
			begin = -1
		}
	}

	if begin >= 0 {
		tokens = append(tokens, classes[begin:])
	}

	return tokens
}

// IsKnownUtility reports whether class is a utility the registry holds under
// exactly that name. The stem fallback classify applies to values (`p-7`,
// `text-[#fff]`) is deliberately not consulted: a caller asking whether a token
// is a Tailwind class at all, rather than how to order it, needs a registered
// name and not a plausible shape.
func IsKnownUtility(class string) bool {
	ensureRegistry()

	_, ok := utilityRegistry[class]

	return ok
}

// SortPinned sorts classes, holding the first and/or last token in place.
//
// A list that abuts a hole (a Blade echo or a PHP concatenation) is a fragment: in
// `'... bg-'.$color.'-500'` the trailing `bg-` only becomes a class once the hole
// is substituted, so moving it changes the rendered class string. A list of one or
// zero tokens keeps its original whitespace, and a trailing `!important` stays last.
func SortPinned(theme *Theme, classes string, pinFirst, pinLast bool) string {
	tokens := classTokens(classes)

	if len(tokens) <= 1 {
		return classes
	}

	trailingImportant := false

	if tokens[len(tokens)-1] == "!important" {
		trailingImportant = true
		tokens = tokens[:len(tokens)-1]
	}

	low := 0
	if pinFirst {
		low = 1
	}

	high := len(tokens)
	if pinLast && high > low {
		high--
	}

	if high-low > 1 {
		sortCanonical(theme, tokens[low:high])
	}

	if trailingImportant {
		tokens = append(tokens, "!important")
	}

	return strings.Join(tokens, " ")
}

// sortEntry pairs a token with its key so the sort permutes one slice, not three.
//
// The key stays behind a pointer on purpose. sortKey is 96 bytes, and a stable
// sort moves and copies elements far more often than it builds them; inlining
// the key into the element measured 12% slower than this shape even though it
// allocates one slice fewer.
type sortEntry struct {
	key   *sortKey
	token string
}

func sortCanonical(theme *Theme, tokens []string) {
	ensureRegistry()

	keys := make([]sortKey, len(tokens))
	entries := make([]sortEntry, len(tokens))

	for i, t := range tokens {
		keys[i] = buildSortKey(theme, t)
		entries[i] = sortEntry{key: &keys[i], token: t}
	}

	slices.SortStableFunc(entries, func(a, b sortEntry) int {
		return compareSortKeys(*a.key, *b.key)
	})

	for i, e := range entries {
		tokens[i] = e.token
	}
}
