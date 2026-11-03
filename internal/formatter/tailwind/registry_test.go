package tailwind

import "testing"

// The tables are generated, so a regeneration that breaks what compareVariants
// assumes of them fails here instead of misordering classes at sort time.
func TestRegistryInvariants(t *testing.T) {
	ensureRegistry()

	minOrder := variantRegistry["min"].Order

	for name := range defaultBreakpoints {
		entry := variantRegistry[name]

		if entry.Kind != variantStatic || entry.Order != minOrder {
			t.Errorf("breakpoint %q is %+v, want a static variant in the min group (order %d)", name, entry, minOrder)
		}
	}

	if variantRegistry["@"].Order != variantRegistry["@min"].Order {
		t.Errorf("@ and @min sit in groups %d and %d, want one", variantRegistry["@"].Order, variantRegistry["@min"].Order)
	}
}
