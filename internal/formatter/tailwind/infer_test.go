package tailwind

import (
	"slices"
	"testing"
)

// The generator ranks one sample per value, and Go answers for a value with
// the first sample sharing its data types. That holds only if samples sharing
// their data types always rank alike, which this checks on the generated table:
// a Tailwind release breaking it fails here, not in a user's sort.
func TestArbitrarySamplesSharingDataTypesRankAlike(t *testing.T) {
	ensureRegistry()

	for name, stem := range stemRegistry {
		if stem.ProbeOrders == nil {
			continue
		}

		first := make(map[dataTypes]int)

		for i, sample := range arbitrarySamples {
			set := inferDataTypes(decodeArbitrary(sample))

			j, seen := first[set]
			if !seen {
				first[set] = i

				continue
			}

			if !slices.Equal(stem.ProbeOrders[i], stem.ProbeOrders[j]) {
				t.Errorf("%s: %q ranks %v, %q with the same data types %v",
					name, sample, stem.ProbeOrders[i], arbitrarySamples[j], stem.ProbeOrders[j])
			}
		}
	}
}

func TestInferDataTypesMatchesTailwindChecks(t *testing.T) {
	cases := []struct {
		value string
		want  dataTypes
	}{
		{"var(--x)", 0},
		{"#000", typeColor | typeFamilyName},
		{"RED", typeColor | typeFamilyName},
		{"2px", typeLength | typePosition | typeBgSize | typeLineWidth},
		{"2", typeNumber | typeInteger | typeLineWidth},
		{"02", typeNumber | typeLineWidth},
		{"50%", typePercentage | typePosition | typeBgSize},
		{"1/2", typeRatio},
		{"45deg", typeAngle},
		{"1 2 3", typeLineWidth | typeVector},
		{"url(x)", typeURL | typeImage | typeFamilyName},
		{"medium", typeLineWidth | typeAbsoluteSize | typeFamilyName},
		{"0 0 2px #000", 0},
	}

	for _, tc := range cases {
		if got := inferDataTypes(tc.value); got != tc.want {
			t.Errorf("inferDataTypes(%q) = %017b, want %017b", tc.value, got, tc.want)
		}
	}
}
