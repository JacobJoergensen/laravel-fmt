package tailwind

import "testing"

// The tables are generated, so a regeneration that breaks one of these fails
// here instead of misordering classes at sort time.
func TestRegistryInvariants(t *testing.T) {
	ensureRegistry()

	if arbitraryVariantSlot >= variantMaskSlots {
		t.Errorf("arbitraryVariantSlot %d does not fit a %d-slot variantMask; increase variantMaskWords",
			arbitraryVariantSlot, variantMaskSlots)
	}

	for name := range defaultBreakpointWidths {
		if _, ok := variantRegistry[name]; !ok {
			t.Errorf("default breakpoint %q is missing from variantRegistry", name)
		}
	}

	if span := breakpointBlockEnd - breakpointBlockStart + 1; span != len(defaultBreakpointWidths) {
		t.Errorf("default breakpoints span %d variant priorities, want %d contiguous ones", span, len(defaultBreakpointWidths))
	}
}
