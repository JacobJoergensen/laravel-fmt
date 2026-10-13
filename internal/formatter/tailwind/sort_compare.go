package tailwind

import (
	"cmp"
	"math"
	"strings"
)

type sortKey struct {
	Unknown       bool
	Variants      variantMask
	ContainerRank int
	Arbitrary     string
	Order         []int
	Count         int
	Raw           string
}

func buildSortKey(theme *Theme, token string) sortKey {
	c := parseCandidate(theme, token)
	cls := classify(theme, &c)

	return sortKey{
		Unknown:       cls.Unknown,
		Variants:      variantBitmask(theme, c.Variants),
		ContainerRank: containerRankOf(theme, c.Variants),
		Arbitrary:     arbitraryVariantKey(c.Variants),
		Order:         cls.Order,
		Count:         cls.Count,
		Raw:           token,
	}
}

// arbitraryVariantKey joins a candidate's arbitrary-variant selectors. They all
// share one priority slot, so the bitmask can't order them; this tie-breaks by
// selector, grouping same-selector classes, matching prettier-plugin-tailwindcss.
func arbitraryVariantKey(variants []variant) string {
	var b strings.Builder

	for _, v := range variants {
		if v.Arbitrary {
			b.WriteString(v.Root)
			b.WriteByte(0)
		}
	}

	return b.String()
}

func containerRankOf(theme *Theme, variants []variant) int {
	rank := -1

	for _, v := range variants {
		if v.Container == "" {
			continue
		}

		if r, ok := theme.containerRank(v.Container); ok && r > rank {
			rank = r
		}
	}

	return rank
}

// compareSortKeys ports the compileCandidates comparator in
// tailwindcss/src/compile.ts, comparing in order: unknown first (as
// prettier-plugin-tailwindcss does), variant bitset, container-query width,
// arbitrary-variant selector, property-order tuple, declaration count
// descending (so `truncate` sorts above a utility setting a subset of its
// properties), then the raw string.
func compareSortKeys(a, b sortKey) int {
	if a.Unknown != b.Unknown {
		if a.Unknown {
			return -1
		}
		return 1
	}

	// Both unknown: return 0 so the stable sort preserves input order, matching
	// prettier-plugin-tailwindcss (null === null → 0). Falling through to the raw-string
	// tie-break would reorder equally-unsortable classes, e.g. flip
	// `border-[#fff] border-[2px]` because `#` < `2` in ASCII.
	if a.Unknown {
		return 0
	}

	if c := a.Variants.cmp(b.Variants); c != 0 {
		return c
	}

	if c := cmp.Compare(a.ContainerRank, b.ContainerRank); c != 0 {
		return c
	}

	if c := cmp.Compare(a.Arbitrary, b.Arbitrary); c != 0 {
		return c
	}

	if c := compareOrderTuples(a.Order, b.Order); c != 0 {
		return c
	}

	if c := cmp.Compare(b.Count, a.Count); c != 0 {
		return c
	}

	return cmp.Compare(a.Raw, b.Raw)
}

// compareOrderTuples lex-compares two property-order tuples with the engine's +Inf
// rule: when one tuple runs out, that side compares as larger at that position, so a
// class with fewer properties sorts later once the earlier positions match (the
// `order[offset] ?? Infinity` comparison in compileCandidates).
func compareOrderTuples(a, b []int) int {
	n := min(len(b), len(a))

	for i := range n {
		if a[i] != b[i] {
			return cmp.Compare(a[i], b[i])
		}
	}

	aTail := math.MaxInt
	bTail := math.MaxInt
	if len(a) > n {
		aTail = a[n]
	}
	if len(b) > n {
		bTail = b[n]
	}

	return cmp.Compare(aTail, bTail)
}
