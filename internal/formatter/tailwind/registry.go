package tailwind

// Regenerate registry_generated.go after a Tailwind release: `npm ci` in
// scripts/generate-tailwind-registry, then `go generate ./...`. The generator
// introspects a live Tailwind v4 design system rather than reading source, so the
// output is stamped against the installed tailwindcss version (its `// Source:`
// header).
//
//go:generate node ../../../scripts/generate-tailwind-registry/generate.mjs

import (
	"sync"
)

type utilityEntry struct {
	Order []int
	Count int
}

// stemEntry is the per-stem fallback for a utility root. Order is the earliest
// rank among the stem's most common shape of class (the default branch).
// ColorOrder and SizeOrder are the earliest ranks of its color- and size-valued
// classes, which a project `@theme` token picks between (`text-brand`). Each is
// nil when the stem has no class of that shape. BracketOrder and ParenOrder
// rank an arbitrary `[…]` or `(…)` value; ProbeOrders, set only when the rank
// depends on the value, holds one per arbitraryProbeIndex, nil where the engine
// rejects the value.
type stemEntry struct {
	Order        []int
	Count        int
	ColorOrder   []int
	ColorCount   int
	SizeOrder    []int
	SizeCount    int
	BracketOrder []int
	ParenOrder   []int
	ProbeOrders  [][]int
}

// classification is the resolved sort key for a candidate. Unknown marks a candidate
// that matched no registered utility; it floats to the front of the sort, mirroring
// prettier-plugin-tailwindcss's null-order handling for project-specific classes.
type classification struct {
	Unknown bool
	Order   []int
	Count   int
}

// The registry is built on the first sort, not at init, so a run that never
// sorts a class list builds none of the ~24k-entry tables. customVariantOrder
// is the first order past the built-in variants, where Tailwind registers a
// project's own; breakpointGroups maps each width-compared group's order to how
// it compares.
var (
	registryOnce       sync.Once
	propertyIndex      map[string]int
	utilityRegistry    map[string]utilityEntry
	stemRegistry       map[string]stemEntry
	variantRegistry    map[string]variantEntry
	customVariantOrder int
	breakpointGroups   map[int]breakpointGroup
)

// ensureRegistry builds the tables on the first call and is a no-op after.
// Every function that reads them calls it first.
func ensureRegistry() {
	registryOnce.Do(func() {
		utilityRegistry = buildUtilityRegistry()
		stemRegistry = buildStemRegistry()
		variantRegistry = buildVariantRegistry()

		propertyIndex = buildPropertyRanks()

		highest := 0
		for _, entry := range variantRegistry {
			highest = max(highest, entry.Order)
		}
		customVariantOrder = highest + 1

		breakpointGroups = map[int]breakpointGroup{
			variantRegistry["max"].Order:  {Descending: true},
			variantRegistry["min"].Order:  {},
			variantRegistry["@max"].Order: {Containers: true, Descending: true},
			variantRegistry["@min"].Order: {Containers: true},
		}
	})
}

// classify resolves a candidate in two passes: an EXACT match on c.Base against
// utilityRegistry, then a STEM fallback on c.Root against stemRegistry. The exact
// pass is what disambiguates ambiguous stems, each known class is registered with
// its own tuple (`text-xs` → font-size, `text-red-500` → color), while the stem
// pass catches values the registry never saw as a name (theme tokens, arbitrary
// values, plugin classes).
func classify(theme *Theme, c *candidate) classification {
	if c.Base == "" {
		return classification{Unknown: true}
	}

	if entry, ok := utilityRegistry[c.Fraction]; ok && c.Fraction != "" {
		return classification{Order: entry.Order, Count: entry.Count}
	}

	if entry, ok := utilityRegistry[c.Base]; ok {
		return classification{Order: entry.Order, Count: entry.Count}
	}

	// A known arbitrary property gets its propertyIndex rank as a one-element
	// tuple; an unknown one gets an empty tuple, which the comparator's +Inf
	// rule sorts to the end rather than to the front like Unknown.
	if c.ArbitraryProperty != "" {
		count := 1
		if idx, ok := propertyIndex[c.ArbitraryProperty]; ok {
			return classification{Order: []int{idx}, Count: count}
		}
		return classification{Order: nil, Count: count}
	}

	// Custom `@utility` classes are consulted before the stem fallback so a custom
	// utility whose name shares a generated stem (`text-fancy`) sorts at its own
	// declared property, not through the stem's dominant tuple.
	if entry, ok := theme.customUtility(c.Base); ok {
		return classification{Order: entry.Order, Count: entry.Count}
	}

	// A functional `@utility name-*` (e.g. `tab-4` via stem `tab`) is likewise
	// consulted before the generated stem fallback, so a functionally redefined
	// namespace sorts at its own declared property.
	if entry, ok := theme.matchCustomUtilityStem(c.Base); ok {
		return classification{Order: entry.Order, Count: entry.Count}
	}

	if c.Root != "" {
		if entry, ok := stemRegistry[c.Root]; ok && theme.namedValueExists(c.Value) {
			if isArbitraryValue(c.Value) {
				return classifyArbitrary(&entry, c.Value)
			}

			order, count := selectStemBranch(theme, c.Root, &entry, c.Value)
			return classification{Order: order, Count: count}
		}
	}

	return classification{Unknown: true}
}

func isArbitraryValue(value string) bool {
	if len(value) < 2 {
		return false
	}

	last := value[len(value)-1]

	return (value[0] == '[' && last == ']') || (value[0] == '(' && last == ')')
}

// classifyArbitrary ranks a `[…]` or `(…)` value where the engine ranked a
// value with the same data types. A value the engine rejects is Unknown, as
// prettier-plugin-tailwindcss leaves it; one no sample covers takes the rank
// of its form.
func classifyArbitrary(stem *stemEntry, value string) classification {
	fallback := stem.BracketOrder
	if value[0] == '(' {
		fallback = stem.ParenOrder
	}

	if fallback == nil {
		fallback = stem.Order
	}

	if stem.ProbeOrders == nil {
		return classification{Order: fallback, Count: 1}
	}

	index, found, invalid := arbitraryProbeIndex(value)

	switch {
	case invalid:
		return classification{Unknown: true}
	case !found:
		return classification{Order: fallback, Count: 1}
	}

	order := stem.ProbeOrders[index]
	if order == nil {
		return classification{Unknown: true}
	}

	return classification{Order: order, Count: 1}
}

// selectStemBranch picks the property tuple for a stem given a named value: the
// project's `@theme` tokens decide through Theme.branchFor. A branch that is nil
// falls through to default, so a theme hint registered for an unambiguous stem
// is a no-op.
func selectStemBranch(theme *Theme, stemName string, stem *stemEntry, value string) ([]int, int) {
	switch theme.branchFor(stemName, value) {
	case branchSize:
		if stem.SizeOrder != nil {
			return stem.SizeOrder, stem.SizeCount
		}
	case branchColor:
		if stem.ColorOrder != nil {
			return stem.ColorOrder, stem.ColorCount
		}
	case branchDefault:
	}

	return stem.Order, stem.Count
}

// variantMaskWords is the number of 64-bit words in a variantMask. Its bits
// number the distinct variants of one class list, so two words hold 128 of
// them; variantRanking.bits gives any past that the last bit.
const variantMaskWords = 2

const variantMaskSlots = variantMaskWords * 64

// variantMask is a fixed-width bitset over the variants of a class list, the
// bigint compileCandidates builds, so the sort allocates nothing per token.
// Comparing words from the most significant down matches a numeric comparison
// of the whole set.
type variantMask [variantMaskWords]uint64

func (m variantMask) with(bit int) variantMask {
	m[bit/64] |= uint64(1) << (bit % 64)

	return m
}

func (m variantMask) cmp(other variantMask) int {
	for i := variantMaskWords - 1; i >= 0; i-- {
		if m[i] != other[i] {
			if m[i] < other[i] {
				return -1
			}

			return 1
		}
	}

	return 0
}
