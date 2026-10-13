package blade

import "testing"

func TestComponentFamilyGroupsTheTagsOneCloseTagEnds(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"x-alert", "x"},
		{"x:alert", "x"},
		{"x-admin::table.thead.tr", "x"},
		{"x-slot", "x-slot"},
		{"x-slot:title", "x-slot"},
		{"x:slot", "x-slot"},
		{"x-slotted-card", "x"},
		{"flux:icon.check", "flux"},
		{"livewire:panel", "livewire"},
		{"livewire:slot", "livewire-slot"},
		{"div", ""},
		{"my-element", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := componentFamily([]byte(tc.name)); got != tc.want {
				t.Fatalf("componentFamily(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
