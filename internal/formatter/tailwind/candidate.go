package tailwind

import (
	"iter"
	"slices"
	"strings"
)

// candidate is a narrow port of tailwindcss/src/candidate.ts's Candidate,
// holding only what the sort comparator needs. Parsing drops a trailing `!`
// and a `/modifier`, since the comparator reorders the original class strings
// and never rebuilds them. A leading `-` stays: `-mt` is a root of its own,
// ranked apart from `mt`. Fraction keeps the modifier for a class the engine
// lists whole, such as `translate-x-2/3`.
type candidate struct {
	Variants []classVariant
	Base     string
	Fraction string
	Root     string
	Value    string
	// ArbitraryProperty is the CSS property of a `[prop:value]` candidate, empty for
	// every other shape (including arbitrary VALUES like `bg-[#fff]`). When set,
	// classify looks it up in propertyOrder instead of the stem registry, so
	// `[-webkit-text-fill-color:transparent]` sorts at its property's position rather
	// than floating to the front as an unknown class.
	ArbitraryProperty string
}

// classVariant is a variant as written, the text variantRanking tells variants
// apart by, with its parse.
type classVariant struct {
	Text    string
	Variant variant
}

// parseCandidate turns a Tailwind class string into a candidate. It always returns a
// candidate, even for nonsense input: a downstream registry miss classifies it as
// unknown and floats it to the front, matching prettier-plugin-tailwindcss for any
// class getClassOrder returns null for.
func parseCandidate(theme *Theme, s string) candidate {
	c := candidate{}

	base := s

	var variantParts []string

	// Nine class tokens in ten carry no `:` variant, and the split allocates its
	// result slice.
	if strings.IndexByte(s, ':') >= 0 {
		parts := segmentBracketAware(s, ':')
		if len(parts) == 0 {
			return c
		}

		// A v4 prefix is the first `:`-segment (`tw:flex`). With several
		// bundles merged, a class may carry any bundle's prefix, or none when
		// some bundle sets none; anything else is not a Tailwind class and
		// stays Unknown, as in the engine.
		if prefixes := theme.classPrefixes(); len(prefixes) > 0 {
			if len(parts) > 1 && parts[0] != "" && slices.Contains(prefixes, parts[0]) {
				parts = parts[1:]
			} else if requiresPrefix(prefixes) {
				return c
			}
		}

		base = parts[len(parts)-1]
		variantParts = parts[:len(parts)-1]
	} else if requiresPrefix(theme.classPrefixes()) {
		// The prefix is a `:`-segment of its own, so a class with no `:` at all
		// cannot carry it.
		return c
	}

	for _, text := range variantParts {
		// A variant the engine rejects leaves the whole class unknown.
		v, ok := parseVariant(theme, text)
		if !ok {
			return candidate{}
		}

		c.Variants = append(c.Variants, classVariant{Text: text, Variant: v})
	}

	if len(base) > 0 && base[len(base)-1] == '!' {
		base = base[:len(base)-1]
	}

	whole := base
	base, modifier := splitModifier(base)
	c.Base = base

	if modifier != "" {
		c.Fraction = whole
	}

	if len(base) > 0 && base[0] == '[' {
		c.Root = base
		c.ArbitraryProperty = arbitraryPropertyName(base)
		return c
	}

	c.Root, c.Value = splitRoot(base)
	return c
}

func arbitraryPropertyName(base string) string {
	if len(base) < 2 || base[0] != '[' || base[len(base)-1] != ']' {
		return ""
	}

	inner := base[1 : len(base)-1]
	depth := 0

	for i := range len(inner) {
		switch inner[i] {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				return inner[:i]
			}
		}
	}

	return ""
}

// segmentBracketAware splits s at sep outside `[]`, `()` and string literals.
func segmentBracketAware(s string, sep byte) []string {
	var parts []string

	for part := range segments(s, sep) {
		parts = append(parts, part)
	}

	return parts
}

// segments yields what segmentBracketAware returns without building a slice.
//
// The escape skip is a body-side `i++`, so the loop has to stay C-style: a
// range loop reassigns i and would silently drop the skip.
func segments(s string, sep byte) iter.Seq[string] {
	return func(yield func(string) bool) {
		depth := 0
		inStr := byte(0)
		start := 0

		for i := 0; i < len(s); i++ {
			c := s[i]

			if inStr != 0 {
				if c == '\\' && i+1 < len(s) {
					i++
					continue
				}
				if c == inStr {
					inStr = 0
				}
				continue
			}

			switch c {
			case '"', '\'':
				inStr = c
			case '[', '(':
				depth++
			case ']', ')':
				if depth > 0 {
					depth--
				}
			case sep:
				if depth == 0 {
					if !yield(s[start:i]) {
						return
					}

					start = i + 1
				}
			}
		}

		yield(s[start:])
	}
}

func splitModifier(base string) (string, string) {
	// Only about one class token in a hundred carries a `/` modifier, and the
	// split allocates its result slice, so the miss is worth testing for first.
	if strings.IndexByte(base, '/') < 0 {
		return base, ""
	}

	parts := segmentBracketAware(base, '/')
	if len(parts) <= 1 {
		return base, ""
	}

	last := parts[len(parts)-1]
	head := strings.Join(parts[:len(parts)-1], "/")
	return head, last
}

// splitRoot derives Root and Value from a non-arbitrary base by taking the longest
// `-`-delimited prefix that is a registered utility root (the upstream
// design.utilities.has scan in candidate.ts). Most-specific wins, so a compound root
// like `bg-gradient-to-r` resolves whole rather than to `bg`. No match returns the
// whole base as Root with empty Value, which downstream classifies as unknown.
func splitRoot(base string) (string, string) {
	if _, ok := utilityRegistry[base]; ok {
		return base, ""
	}

	if _, ok := stemRegistry[base]; ok {
		return base, ""
	}

	for i := len(base) - 1; i > 0; i-- {
		if base[i] != '-' {
			continue
		}

		root := base[:i]
		if _, ok := utilityRegistry[root]; ok {
			return root, base[i+1:]
		}

		if _, ok := stemRegistry[root]; ok {
			return root, base[i+1:]
		}
	}

	return base, ""
}
