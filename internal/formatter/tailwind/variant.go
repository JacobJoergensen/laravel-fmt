package tailwind

import (
	"cmp"
	"iter"
	"math"
	"slices"
	"strconv"
	"strings"
)

// variantKind mirrors the kinds of Variant in candidate.ts. variantUnknown is a
// root laravel-fmt has no registration for, such as one a JavaScript plugin
// adds: it sorts after every known variant instead of making the class
// unknown, since the engine running that plugin would know it.
type variantKind uint8

const (
	variantUnknown variantKind = iota
	variantStatic
	variantFunctional
	variantCompound
	variantArbitrary
)

type variantEntry struct {
	Kind          variantKind
	Order         int
	Compounds     compounds
	CompoundsWith compounds
}

// compounds is Compounds in variants.ts: the rules a variant generates, or the
// ones a compound variant can wrap.
type compounds uint8

const (
	compoundsAtRules compounds = 1 << iota
	compoundsStyleRules
)

type variant struct {
	Kind  variantKind
	Root  string
	Order int
	// Selector is an arbitrary variant's selector, wrapped in `&:is(…)` the way
	// candidate.ts wraps it, since that is what the engine compares.
	Selector string
	Relative bool
	// Value is a functional variant's value, decoded; HasValue is false for a
	// bare `data`, and ArbitraryValue marks a `[…]` or `(…)` one.
	Value          string
	HasValue       bool
	ArbitraryValue bool
	Modifier       string
	HasModifier    bool
	Inner          *variant
}

// breakpointGroup is one of the variant groups that compare by width: Tailwind
// registers the breakpoints (`max-*`, then `min-*` with `sm`…`2xl`) and the
// container queries (`@max-*`, then `@min-*` with `@*`) with a compare
// function over the width each variant resolves to.
type breakpointGroup struct {
	Containers bool
	Descending bool
}

// parseVariant ports parseVariant in candidate.ts. It reports false for a
// variant the engine rejects, which makes the whole class unknown.
func parseVariant(theme *Theme, text string) (variant, bool) {
	if len(text) >= 2 && text[0] == '[' && text[len(text)-1] == ']' {
		return parseArbitraryVariant(text)
	}

	base, modifier, hasModifier := text, "", false

	// Few variants carry a `/`, and the split allocates.
	if strings.IndexByte(text, '/') >= 0 {
		parts := segmentBracketAware(text, '/')
		if len(parts) > 2 {
			return variant{}, false
		}

		base = parts[0]

		if len(parts) == 2 {
			modifier, hasModifier = parts[1], true
		}
	}

	matched := false

	for root, value := range variantRoots(theme, base) {
		matched = true
		hasValue := value != ""
		entry, _ := theme.variantEntry(root)

		switch entry.Kind {
		case variantStatic:
			if hasValue || hasModifier {
				return variant{}, false
			}

			return variant{Kind: variantStatic, Root: root, Order: entry.Order}, true
		case variantFunctional:
			v, ok, next := parseFunctionalVariant(root, entry.Order, value, hasValue, modifier, hasModifier)
			if next {
				continue
			}

			// A breakpoint or container variant that resolves to no width
			// generates no rule, which leaves the class unknown.
			if group, width := breakpointGroups[entry.Order]; ok && width {
				_, ok = breakpointWidth(theme, &v, group.Containers)
			}

			return v, ok
		case variantCompound:
			return parseCompoundVariant(theme, root, entry, value, hasValue, modifier, hasModifier)
		case variantUnknown, variantArbitrary:
			// No registered root has either kind.
		}
	}

	// A default breakpoint the theme removed is known to be no variant, unlike a
	// root a plugin may add.
	if matched || theme.removedBreakpoint(base) {
		return variant{}, false
	}

	return variant{Kind: variantUnknown, Root: text, Order: theme.unknownVariantOrder()}, true
}

func parseArbitraryVariant(text string) (variant, bool) {
	if text[1] == '@' && strings.Contains(text, "&") {
		return variant{}, false
	}

	selector := decodeArbitrary(text[1 : len(text)-1])
	if strings.TrimSpace(selector) == "" {
		return variant{}, false
	}

	relative := strings.IndexByte(">+~", selector[0]) >= 0

	if !relative && selector[0] != '@' && !strings.Contains(selector, "&") {
		selector = "&:is(" + selector + ")"
	}

	return variant{Kind: variantArbitrary, Selector: selector, Relative: relative}, true
}

// parseFunctionalVariant returns next for a value candidate.ts skips to try a
// shorter root, such as `foo-[#bar]` whose value does not open with `[`.
func parseFunctionalVariant(root string, order int, value string, hasValue bool, modifier string, hasModifier bool) (v variant, ok, next bool) {
	v = variant{Kind: variantFunctional, Root: root, Order: order}

	if hasModifier {
		parsed, valid := parseVariantModifier(modifier)
		if !valid {
			return variant{}, false, false
		}

		v.Modifier, v.HasModifier = parsed, true
	}

	if !hasValue {
		return v, true, false
	}

	last := value[len(value)-1]

	switch {
	case last == ']' || last == ')':
		open := byte('[')
		if last == ')' {
			open = '('
		}

		if value[0] != open {
			return variant{}, false, true
		}

		decoded := decodeArbitrary(value[1 : len(value)-1])
		if strings.TrimSpace(decoded) == "" {
			return variant{}, false, false
		}

		if open == '(' {
			if !strings.HasPrefix(decoded, "--") {
				return variant{}, false, false
			}

			decoded = "var(" + decoded + ")"
		}

		v.Value, v.HasValue, v.ArbitraryValue = decoded, true, true
	case isNamedValue(value):
		v.Value, v.HasValue = value, true
	default:
		return variant{}, false, true
	}

	return v, true, false
}

func parseCompoundVariant(theme *Theme, root string, entry variantEntry, value string, hasValue bool, modifier string, hasModifier bool) (variant, bool) {
	if !hasValue {
		return variant{}, false
	}

	// candidate.ts forwards these roots' modifier to the variant inside, so
	// `not-group-hover/name` names the group.
	if hasModifier && slices.Contains(modifierForwardingRoots, root) {
		value += "/" + modifier
		modifier, hasModifier = "", false
	}

	inner, ok := parseVariant(theme, value)
	if !ok {
		return variant{}, false
	}

	// These roots' handlers in variants.ts give up on a relative selector.
	if inner.Relative && slices.Contains(relativeRejectingRoots, root) {
		return variant{}, false
	}

	// compoundsWith in variants.ts: the parent must wrap some kind of rule the
	// inner variant generates, so `group-md`, an at-rule, is no variant.
	if entry.CompoundsWith&compoundsOf(theme, &inner) == 0 {
		return variant{}, false
	}

	v := variant{Kind: variantCompound, Root: root, Order: entry.Order, Inner: &inner}

	if hasModifier {
		parsed, valid := parseVariantModifier(modifier)
		if !valid {
			return variant{}, false
		}

		v.Modifier, v.HasModifier = parsed, true
	}

	return v, true
}

const notVariant = "not"

var (
	modifierForwardingRoots = []string{notVariant, "has", "in"}
	relativeRejectingRoots  = []string{"group", "peer", notVariant}
)

// compoundsOf is the rules v generates. An arbitrary variant's come from its
// selector, as compoundsForSelectors in variants.ts reads it, and a root
// laravel-fmt has no registration for is let through.
func compoundsOf(theme *Theme, v *variant) compounds {
	switch v.Kind {
	case variantArbitrary:
		return compoundsForSelectors(v.Selector)
	case variantUnknown:
		return compoundsAtRules | compoundsStyleRules
	case variantStatic, variantFunctional, variantCompound:
	}

	entry, _ := theme.variantEntry(v.Root)

	return entry.Compounds
}

// compoundsForSelectors ports the function of that name in variants.ts.
func compoundsForSelectors(selectors ...string) compounds {
	var rules compounds

	for _, selector := range selectors {
		switch {
		case strings.HasPrefix(selector, "@"):
			if !hasAnyPrefix(selector, "@media", "@supports", "@container") {
				return 0
			}

			rules |= compoundsAtRules
		case strings.Contains(selector, "::"):
			return 0
		default:
			rules |= compoundsStyleRules
		}
	}

	return rules
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	return slices.ContainsFunc(prefixes, func(prefix string) bool {
		return strings.HasPrefix(s, prefix)
	})
}

// parseVariantModifier ports parseModifier in candidate.ts.
func parseVariantModifier(modifier string) (string, bool) {
	if len(modifier) >= 2 && modifier[0] == '[' && modifier[len(modifier)-1] == ']' {
		decoded := decodeArbitrary(modifier[1 : len(modifier)-1])

		return decoded, strings.TrimSpace(decoded) != ""
	}

	if len(modifier) >= 2 && modifier[0] == '(' && modifier[len(modifier)-1] == ')' {
		decoded := decodeArbitrary(modifier[1 : len(modifier)-1])

		return "var(" + decoded + ")", strings.HasPrefix(decoded, "--")
	}

	return modifier, isNamedValue(modifier)
}

// isNamedValue is IS_VALID_NAMED_VALUE in candidate.ts.
func isNamedValue(value string) bool {
	if value == "" {
		return false
	}

	for i := range len(value) {
		c := value[i]

		named := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '.' || c == '%' || c == '-'

		if !named {
			return false
		}
	}

	return true
}

// variantRoots ports findRoots in candidate.ts: the whole input when it is a
// root, then each prefix ending before a dash, longest first, then `@` with the
// rest as its value. findRoots never yields an empty value, so "" means none.
func variantRoots(theme *Theme, input string) iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		if theme.hasVariant(input) && !yield(input, "") {
			return
		}

		for i := strings.LastIndexByte(input, '-'); i > 0; i = strings.LastIndexByte(input[:i], '-') {
			root := input[:i]

			if !theme.hasVariant(root) {
				continue
			}

			if i+1 == len(input) {
				break
			}

			if root == "@" && input[i] == '-' {
				break
			}

			if !yield(root, input[i+1:]) {
				return
			}
		}

		if input != "" && input[0] == '@' && theme.hasVariant("@") {
			yield("@", input[1:])
		}
	}
}

// compareVariants ports Variants.compare in variants.ts.
func compareVariants(theme *Theme, a, b *variant) int {
	switch {
	case a.Kind == variantArbitrary && b.Kind == variantArbitrary:
		return cmp.Compare(a.Selector, b.Selector)
	case a.Kind == variantArbitrary:
		return 1
	case b.Kind == variantArbitrary:
		return -1
	}

	if c := cmp.Compare(a.Order, b.Order); c != 0 {
		return c
	}

	if a.Kind == variantCompound && b.Kind == variantCompound {
		if c := compareVariants(theme, a.Inner, b.Inner); c != 0 {
			return c
		}

		return compareModifiers(a, b)
	}

	if group, ok := breakpointGroups[a.Order]; ok {
		return compareBreakpointVariants(theme, a, b, group)
	}

	if a.Root != b.Root {
		return cmp.Compare(a.Root, b.Root)
	}

	switch {
	case !a.HasValue || !b.HasValue:
		return cmp.Compare(boolRank(a.HasValue), boolRank(b.HasValue))
	case a.ArbitraryValue != b.ArbitraryValue:
		return cmp.Compare(boolRank(a.ArbitraryValue), boolRank(b.ArbitraryValue))
	}

	return cmp.Compare(a.Value, b.Value)
}

// A variant without a modifier sorts before one with.
func compareModifiers(a, b *variant) int {
	if a.HasModifier && b.HasModifier {
		return cmp.Compare(a.Modifier, b.Modifier)
	}

	return cmp.Compare(boolRank(a.HasModifier), boolRank(b.HasModifier))
}

func boolRank(b bool) int {
	if b {
		return 1
	}

	return 0
}

// compareBreakpointVariants ports the function of that name in variants.ts.
// parseVariant rejects a variant with no width, so both resolve here.
func compareBreakpointVariants(theme *Theme, a, b *variant, group breakpointGroup) int {
	aWidth, _ := breakpointWidth(theme, a, group.Containers)
	bWidth, _ := breakpointWidth(theme, b, group.Containers)

	return compareBreakpoints(aWidth, bWidth, group.Descending)
}

func directed(order int, descending bool) int {
	if descending {
		return -order
	}

	return order
}

// breakpointWidth resolves a variant to the width it compares by, as
// resolvedBreakpoints and resolvedWidths do in variants.ts.
func breakpointWidth(theme *Theme, v *variant, containers bool) (string, bool) {
	var width string

	switch {
	case v.Kind == variantStatic && !containers:
		resolved, ok := theme.breakpoint(v.Root)
		if !ok {
			return "", false
		}

		width = resolved
	case v.Kind != variantFunctional || !v.HasValue || (v.HasModifier && !containers):
		return "", false
	case v.ArbitraryValue:
		width = v.Value
	case containers:
		resolved, ok := theme.container(v.Value)
		if !ok {
			return "", false
		}

		width = resolved
	default:
		resolved, ok := theme.breakpoint(v.Value)
		if !ok {
			return "", false
		}

		width = resolved
	}

	if strings.Contains(width, "var(") {
		return "", false
	}

	return width, true
}

// compareBreakpoints ports utils/compare-breakpoints.ts: widths group by unit,
// or by function name, before they compare by number, so every `px` width
// sorts before every `rem` one.
func compareBreakpoints(a, b string, descending bool) int {
	if a == b {
		return 0
	}

	if c := compareBuckets(a, b); c != 0 {
		return c
	}

	aNumber, aOK := parseLeadingFloat(a)
	bNumber, bOK := parseLeadingFloat(b)

	if !aOK || !bOK {
		if a < b {
			return -1
		}

		return 1
	}

	return directed(cmp.Compare(aNumber, bNumber), descending)
}

// compareBuckets compares the buckets compareBreakpoints groups widths by: the
// function name before a `(`, or else the width without its digits and dots,
// so `40rem` and `48.5rem` share `rem`. It reads both in place, since this runs
// for every pair of breakpoint variants a sort compares.
func compareBuckets(a, b string) int {
	aEnd, aStrip := bucketEnd(a)
	bEnd, bStrip := bucketEnd(b)

	for i, j := 0, 0; ; i, j = i+1, j+1 {
		i = skipNumber(a, i, aEnd, aStrip)
		j = skipNumber(b, j, bEnd, bStrip)

		switch {
		case i == aEnd || j == bEnd:
			return cmp.Compare(aEnd-i, bEnd-j)
		case a[i] != b[j]:
			return cmp.Compare(a[i], b[j])
		}
	}
}

func bucketEnd(width string) (int, bool) {
	if paren := strings.IndexByte(width, '('); paren >= 0 {
		return paren, false
	}

	return len(width), true
}

func skipNumber(s string, i, end int, strip bool) int {
	for strip && i < end && (s[i] == '.' || isDigit(s[i])) {
		i++
	}

	return i
}

// parseLeadingFloat is JavaScript's parseFloat: the longest number that opens
// the string, ignoring leading whitespace.
func parseLeadingFloat(s string) (float64, bool) {
	s = strings.TrimLeft(s, regexpWhitespace)
	sign := 1.0

	if rest, ok := strings.CutPrefix(s, "-"); ok {
		s, sign = rest, -1
	} else {
		s = strings.TrimPrefix(s, "+")
	}

	if strings.HasPrefix(s, "Infinity") {
		return sign * math.Inf(1), true
	}

	end := numberEnd(s)
	if end == 0 {
		return 0, false
	}

	n, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return 0, false
	}

	return sign * n, true
}

type variantRanking struct {
	distinct []*classVariant
	uses     []variantUse
}

type variantUse struct {
	token   int
	variant int
}

const rankingCapacity = 8

// A class list holds a handful of distinct variants, so a scan beats a map, and
// room for eight saves most lists a regrow.
func (r *variantRanking) add(token int, cv *classVariant) {
	if r.uses == nil {
		r.distinct = make([]*classVariant, 0, rankingCapacity)
		r.uses = make([]variantUse, 0, rankingCapacity)
	}

	i := slices.IndexFunc(r.distinct, func(d *classVariant) bool { return d.Text == cv.Text })
	if i < 0 {
		i = len(r.distinct)
		r.distinct = append(r.distinct, cv)
	}

	r.uses = append(r.uses, variantUse{token: token, variant: i})
}

// bits ports getVariantOrder in design-system.ts over one class list: it sorts
// the distinct variants and numbers them, giving variants that compare equal
// one bit. The engine numbers every variant it has parsed, but only the
// relative order reaches a sort, so the list's own variants are enough.
func (r *variantRanking) bits(theme *Theme) []int {
	n := len(r.distinct)
	indexes := make([]int, 2*n)
	order, bits := indexes[:n], indexes[n:]

	for i := range order {
		order[i] = i
	}

	compare := func(a, b int) int {
		return compareVariants(theme, &r.distinct[a].Variant, &r.distinct[b].Variant)
	}

	slices.SortStableFunc(order, compare)

	bit := 0

	for k, i := range order {
		if k > 0 && compare(order[k-1], i) != 0 {
			bit++
		}

		bits[i] = min(bit, variantMaskSlots-1)
	}

	return bits
}

// naturalCompare ports utils/compare.ts, the engine's last tie-break: runs of
// digits compare as numbers, so `w-[9px]` sorts before `w-[10px]`.
func naturalCompare(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if isDigit(a[i]) && isDigit(b[i]) {
			aEnd, bEnd := digitsEnd(a, i), digitsEnd(b, i)

			if c := compareDigitRuns(a[i:aEnd], b[i:bEnd]); c != 0 {
				return c
			}

			continue
		}

		if a[i] != b[i] {
			return cmp.Compare(a[i], b[i])
		}
	}

	return cmp.Compare(len(a), len(b))
}

// compareDigitRuns compares two runs of digits by value, then, for `0123`
// against `123`, as strings.
func compareDigitRuns(a, b string) int {
	aTrimmed, bTrimmed := strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")

	if c := cmp.Compare(len(aTrimmed), len(bTrimmed)); c != 0 {
		return c
	}

	if c := cmp.Compare(aTrimmed, bTrimmed); c != 0 {
		return c
	}

	return cmp.Compare(a, b)
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
