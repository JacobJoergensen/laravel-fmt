package tailwind

import (
	"slices"
	"strings"
)

// candidate is a narrow port of tailwindcss/src/candidate.ts's Candidate,
// holding only what the sort comparator needs. Parsing drops a trailing `!`, a
// leading `-` and a `/modifier`, since the comparator reorders the original
// class strings and never rebuilds them.
type candidate struct {
	Variants []variant
	Base     string
	Root     string
	Value    string
	// ArbitraryProperty is the CSS property of a `[prop:value]` candidate, empty for
	// every other shape (including arbitrary VALUES like `bg-[#fff]`). When set,
	// classify looks it up in propertyOrder instead of the stem registry, so
	// `[-webkit-text-fill-color:transparent]` sorts at its property's position rather
	// than floating to the front as an unknown class.
	ArbitraryProperty string
}

// variant is one entry in a class's variant chain. An arbitrary variant
// (`[&>a:hover]:`) sets Arbitrary and keeps its selector in Root, which the
// comparator orders by, since every arbitrary variant shares one priority
// slot. A container-query variant (`@md`, `@2xl/sidebar`) sets Root to `@` and
// its size in Container, which the comparator orders by width, since every
// `@<size>` shares one slot too.
type variant struct {
	Root      string
	Arbitrary bool
	Container string
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

	for _, v := range variantParts {
		c.Variants = append(c.Variants, parseVariant(theme, v))
	}

	if len(base) > 0 && base[len(base)-1] == '!' {
		base = base[:len(base)-1]
	}

	if len(base) > 0 && base[0] == '-' {
		base = base[1:]
	}

	base, _ = splitModifier(base)
	c.Base = base

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

func parseVariant(theme *Theme, s string) variant {
	v := variant{}

	if len(s) > 0 && s[0] == '[' {
		v.Arbitrary = true
		// generatedVariantPriority checks Arbitrary before it reads Root, so
		// the selector can sit there.
		v.Root = s
		return v
	}

	// Container query `@<size>` / `@<size>/<name>`, where size is a known breakpoint,
	// the default `--container-*` scale, or a project `@theme` size. The bracketed
	// `@min-[…]` / `@max-[…]` forms and any unknown `@foo` carry no known size and
	// fall through to the branches below, landing in the arbitrary slot.
	if len(s) > 1 && s[0] == '@' && strings.IndexByte(s, '[') < 0 {
		size := s[1:]
		if slash := strings.IndexByte(size, '/'); slash >= 0 {
			size = size[:slash]
		}

		if _, ok := theme.containerRank(size); ok {
			v.Root = "@"
			v.Container = size
			return v
		}
	}

	if i := strings.IndexByte(s, '['); i > 0 && strings.HasSuffix(s, "]") {
		v.Root = strings.TrimSuffix(s[:i], "-")
		return v
	}

	v.Root = s
	return v
}

// segmentBracketAware splits s at sep outside `[]`, `()` and string literals.
//
// The escape skip is a body-side `i++`, so the loop has to stay C-style: a
// range loop reassigns i and would silently drop the skip.
func segmentBracketAware(s string, sep byte) []string {
	var parts []string
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
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}

	parts = append(parts, s[start:])
	return parts
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
