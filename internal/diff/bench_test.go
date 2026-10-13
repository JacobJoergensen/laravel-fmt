package diff

import (
	"strconv"
	"strings"
	"testing"
)

// Models the real workload: a file diffed against its formatted self, long
// with a few changed lines.
func nearlyIdentical(n int, changed ...int) (string, string) {
	a := make([]string, n)
	b := make([]string, n)

	for i := range n {
		line := "line " + strconv.Itoa(i) + " of the source file"
		a[i] = line
		b[i] = line
	}

	for _, i := range changed {
		b[i] = "CHANGED " + a[i]
	}

	return strings.Join(a, "\n"), strings.Join(b, "\n")
}

func BenchmarkDiffLines(b *testing.B) {
	half := make([]int, 0, 2500)
	for i := 0; i < 5000; i += 2 {
		half = append(half, i)
	}

	cases := []struct {
		name    string
		changed []int
	}{
		{"scattered", []int{1, 2500, 4998}},
		{"clustered", []int{2500, 2501, 2502}},
		{"half", half},
	}

	for _, tc := range cases {
		old, updated := nearlyIdentical(5000, tc.changed...)
		a := strings.Split(old, "\n")
		c := strings.Split(updated, "\n")

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				diffLines(a, c)
			}
		})
	}
}
