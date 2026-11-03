package tailwind

import (
	"cmp"
	"math"
)

// sortKey is what compileCandidates in compile.ts sorts a class by. Variants
// holds a bit per variant, numbered by variantRanking.bits across the class list.
type sortKey struct {
	Unknown  bool
	Variants variantMask
	Order    []int
	Count    int
	Raw      string
}

func newSortKey(theme *Theme, c *candidate, token string) sortKey {
	cls := classify(theme, c)

	return sortKey{
		Unknown: cls.Unknown,
		Order:   cls.Order,
		Count:   cls.Count,
		Raw:     token,
	}
}

// compareSortKeys ports the compileCandidates comparator in
// tailwindcss/src/compile.ts, comparing in order: unknown first (as
// prettier-plugin-tailwindcss does), variant bitset, property-order tuple,
// declaration count descending (so `truncate` sorts above a utility setting a
// subset of its properties), then the class itself, numbers by value.
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

	if c := compareOrderTuples(a.Order, b.Order); c != 0 {
		return c
	}

	if c := cmp.Compare(b.Count, a.Count); c != 0 {
		return c
	}

	return naturalCompare(a.Raw, b.Raw)
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
