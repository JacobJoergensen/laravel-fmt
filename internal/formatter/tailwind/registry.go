package tailwind

// Regenerate registry_generated.go after a Tailwind release: `npm ci` in
// scripts/generate-tailwind-registry, then `go generate ./...`. The generator
// introspects a live Tailwind v4 design system rather than reading source, so the
// output is stamped against the installed tailwindcss version (its `// Source:`
// header).
//
//go:generate node ../../../scripts/generate-tailwind-registry/generate.mjs

import (
	"slices"
	"strings"
	"sync"
)

type utilityEntry struct {
	Order []int
	Count int
}

// stemEntry is the per-stem fallback for a utility root. Order is the earliest
// rank among the stem's most common shape of class (the default branch).
// ColorOrder and SizeOrder are the earliest ranks of its color- and size-valued
// classes, so on an ambiguous stem (`text`, `border`, `shadow`, `ring`, …) an
// arbitrary value shaped like a color (`border-[#fff]`) or a length
// (`border-[2px]`) picks the matching branch. Each is nil when the stem has no
// class of that shape.
type stemEntry struct {
	Order      []int
	Count      int
	ColorOrder []int
	ColorCount int
	SizeOrder  []int
	SizeCount  int
}

type variantEntry struct {
	Priority int
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
// sorts a class list builds none of the ~24k-entry tables.
// arbitraryVariantSlot is one past the highest generated variant priority,
// the slot an arbitrary or unrecognized variant starts from.
var (
	registryOnce         sync.Once
	propertyIndex        map[string]int
	utilityRegistry      map[string]utilityEntry
	stemRegistry         map[string]stemEntry
	variantRegistry      map[string]variantEntry
	arbitraryVariantSlot int
	breakpointBlockStart int
	breakpointBlockEnd   int
)

// ensureRegistry builds the tables on the first call and is a no-op after.
// Every function that reads them calls it first.
//
// The variantMask-width check runs here because the tables are not built at
// init; any sort trips it, and newTheme checks each project's additions.
func ensureRegistry() {
	registryOnce.Do(func() {
		utilityRegistry = buildUtilityRegistry()
		stemRegistry = buildStemRegistry()
		variantRegistry = buildVariantRegistry()

		propertyIndex = buildPropertyRanks()

		highest := 0
		for _, entry := range variantRegistry {
			highest = max(highest, entry.Priority)
		}
		arbitraryVariantSlot = highest + 1

		breakpointBlockStart, breakpointBlockEnd = computeBreakpointBlock()

		if arbitraryVariantSlot >= variantMaskSlots {
			panic("tailwind: variant priority slot exceeds variantMask width; increase variantMaskWords")
		}
	})
}

// computeBreakpointBlock returns the lowest and highest variant priority the default
// breakpoints occupy. The block must be contiguous: variantPriority renumbers it in
// place for custom `--breakpoint-*` sizes, shifting higher variants up to make room,
// and that stays order-preserving only if no non-breakpoint variant sits inside the
// block. A regenerated registry that breaks contiguity panics here rather than
// misordering breakpoints at sort time.
func computeBreakpointBlock() (int, int) {
	lowest := -1
	highest := -1

	for name := range defaultBreakpointWidths {
		entry, ok := variantRegistry[name]
		if !ok {
			panic("tailwind: default breakpoint missing from variantRegistry: " + name)
		}

		if lowest == -1 || entry.Priority < lowest {
			lowest = entry.Priority
		}

		highest = max(highest, entry.Priority)
	}

	if highest-lowest != len(defaultBreakpointWidths)-1 {
		panic("tailwind: default breakpoint priorities are not contiguous; the breakpoint renumbering would misorder them")
	}

	return lowest, highest
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
			order, count := selectStemBranch(theme, c.Root, &entry, c.Value)
			return classification{Order: order, Count: count}
		}
	}

	return classification{Unknown: true}
}

// selectStemBranch picks the property tuple for a stem given the candidate's value.
// An arbitrary value (`[…]`) is shape-inferred: color literal to the color branch,
// length to the size branch, else default. A named value consults the project's
// `@theme` tokens via Theme.branchFor. A branch that is nil falls through to default,
// so a theme hint registered for an unambiguous stem is a no-op.
func selectStemBranch(theme *Theme, stemName string, stem *stemEntry, value string) ([]int, int) {
	if len(value) > 0 && value[0] == '[' {
		if stem.ColorOrder != nil && bracketLooksLikeColor(value) {
			return stem.ColorOrder, stem.ColorCount
		}
		if stem.SizeOrder != nil && bracketLooksLikeSize(value) {
			return stem.SizeOrder, stem.SizeCount
		}
		return stem.Order, stem.Count
	}

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

// bracketLooksLikeColor reports whether a bracketed arbitrary value holds a CSS color
// (a `#`-hex, a color function, or `var(--color…)`). It deliberately ignores a bare
// `var(--token)` without the `color` prefix: that could be a width or a color, and
// guessing wrong is worse than falling through to the default branch.
func bracketLooksLikeColor(v string) bool {
	inner := unwrapBracket(v)
	if inner == "" {
		return false
	}
	if inner[0] == '#' {
		return true
	}
	for _, prefix := range colorFunctionPrefixes {
		if strings.HasPrefix(inner, prefix) {
			return true
		}
	}
	return strings.HasPrefix(inner, "var(--color")
}

var colorFunctionPrefixes = []string{
	"rgb(", "rgba(",
	"hsl(", "hsla(",
	"oklch(", "oklab(",
	"lab(", "lch(",
	"hwb(",
	"color(",
}

// bracketLooksLikeSize reports whether a bracketed arbitrary value is a number plus a
// CSS length/time unit. The unit is required: a bare `[2]` is not unambiguously a
// length and falls through to the default branch.
func bracketLooksLikeSize(v string) bool {
	inner := unwrapBracket(v)
	if inner == "" {
		return false
	}

	i := 0
	if inner[i] == '-' {
		i++
	}

	digitStart := i
	for i < len(inner) && inner[i] >= '0' && inner[i] <= '9' {
		i++
	}
	if i < len(inner) && inner[i] == '.' {
		i++
		for i < len(inner) && inner[i] >= '0' && inner[i] <= '9' {
			i++
		}
	}
	if i == digitStart {
		return false
	}

	unit := inner[i:]
	return slices.Contains(sizeUnits, unit)
}

var sizeUnits = []string{
	"px", "rem", "em",
	"vw", "vh", "vmin", "vmax",
	"%", "ch", "ex",
	"pt", "pc", "in", "cm", "mm",
	"fr",
	"deg", "rad", "turn",
	"s", "ms",
}

func unwrapBracket(v string) string {
	if len(v) >= 2 && v[0] == '[' && v[len(v)-1] == ']' {
		return v[1 : len(v)-1]
	}
	return v
}

// variantMaskWords is the number of 64-bit words in a variantMask; two span priority
// slots 0..127. Bump it if a future Tailwind release pushes arbitraryVariantSlot past
// 127; ensureRegistry asserts the bound, so the overflow fails loudly instead of a
// single uint64 silently aliasing high slots.
const variantMaskWords = 2

const variantMaskSlots = variantMaskWords * 64

// variantMask is a fixed-width bitset over variant priority slots, so the sort
// allocates nothing per token. Comparing words from the most significant down
// matches a numeric comparison of the whole set.
type variantMask [variantMaskWords]uint64

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

// containerSizeRank orders the built-in container-query breakpoints by width. Every
// `@<size>` variant shares the single `@` priority slot, so the bitmask cannot tell
// `@sm` from `@xl`; the comparator breaks that tie with this rank. A project `@theme`
// that declares `--container-*` sizes replaces it at registration with a merged
// default+custom ranking, read through Theme.containerRank.
var containerSizeRank = map[string]int{
	"3xs": 0,
	"2xs": 1,
	"xs":  2,
	"sm":  3,
	"md":  4,
	"lg":  5,
	"xl":  6,
	"2xl": 7,
	"3xl": 8,
	"4xl": 9,
	"5xl": 10,
	"6xl": 11,
	"7xl": 12,
}

func variantBitmask(theme *Theme, variants []variant) variantMask {
	var mask variantMask

	for _, v := range variants {
		p := variantPriority(theme, v)
		mask[p/64] |= uint64(1) << (p % 64)
	}

	return mask
}

// variantPriority returns a variant's sort priority. Custom `--breakpoint-*` sizes
// renumber the breakpoint block by width and shift every higher variant up: keeping
// breakpoints on distinct width-ordered bits (not one shared slot) is what preserves
// the engine's ordering in mixed chains, where a wider breakpoint's higher bit must
// outweigh a narrower one combined with a pseudo-class. A `@custom-variant` takes a
// slot past the generated maximum, and the arbitrary slot moves above the whole
// custom-variant block, so named custom variants sort after built-ins but before
// arbitrary ones. With no custom breakpoints or variants, output matches the
// generated priorities exactly.
func variantPriority(theme *Theme, v variant) int {
	if rank, ok := theme.breakpointRank(v.Root); ok {
		return breakpointBlockStart + rank
	}

	if idx, ok := theme.customVariantIndex(v.Root); ok {
		return arbitraryVariantSlot + theme.breakpointShift() + idx
	}

	base := generatedVariantPriority(v)

	if base == arbitraryVariantSlot {
		// A functional `@custom-variant name-*` (e.g. `theme-midnight` via stem
		// `theme`) is matched only here, after the generated registry reports the root
		// unknown, so it can never capture a built-in like `group-hover`.
		if idx, ok := theme.matchCustomVariantStem(v.Root); ok {
			return arbitraryVariantSlot + theme.breakpointShift() + idx
		}

		return arbitraryVariantSlot + theme.breakpointShift() + theme.customVariantCount()
	}

	if base > breakpointBlockEnd {
		return base + theme.breakpointShift()
	}

	return base
}

func generatedVariantPriority(v variant) int {
	if v.Arbitrary {
		return arbitraryVariantSlot
	}

	if entry, ok := variantRegistry[v.Root]; ok {
		return entry.Priority
	}

	if i := strings.IndexByte(v.Root, '-'); i > 0 {
		if entry, ok := variantRegistry[v.Root[:i]]; ok {
			return entry.Priority
		}
	}

	return arbitraryVariantSlot
}
