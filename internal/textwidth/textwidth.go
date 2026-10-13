// Package textwidth answers how many terminal columns a rune occupies.
package textwidth

import (
	"sort"
	"unicode"
)

// Rune returns the number of columns r paints: zero for a mark that composes
// onto the character before it and for a format control such as a zero-width
// joiner, two for East Asian Wide and Fullwidth, one otherwise.
//
// Tab and carriage return are the caller's business: a tab's width depends on
// the configured stop, and a carriage return is trimmed off a line end before
// anything is written.
func Rune(r rune) int {
	if r < 0x00AD {
		return 1
	}

	// U+00AD SOFT HYPHEN is a format control: it paints nothing unless a line
	// happens to break there. It is the one character the ASCII fast path below
	// would otherwise measure wrong, since it sits under the combining-mark cutoff.
	if r == 0x00AD {
		return 0
	}

	if r < 0x0300 {
		return 1
	}

	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
		return 0
	}

	// The generated table is ascending, so the first range that could hold r is
	// the first one ending at or after it.
	i := sort.Search(len(wideRanges), func(i int) bool { return wideRanges[i][1] >= r })
	if i < len(wideRanges) && r >= wideRanges[i][0] {
		return 2
	}

	return 1
}
