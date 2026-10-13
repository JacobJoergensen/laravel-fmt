package tailwind

import (
	"strings"
	"testing"
)

// genClassLists mirrors a real app's views, where 9% of class tokens hold a `:`
// variant and 1% a `/` modifier, so the parse fast paths run at their real
// frequency.
func genClassLists(attrs int) []string {
	pools := []string{
		"flex items-center gap-2 rounded-lg bg-white p-4 shadow border border-zinc-200",
		"text-sm font-medium text-zinc-500 truncate leading-6 tracking-tight",
		"block px-3 py-2 text-blue-600 underline-offset-4 hover:underline",
		"grid grid-cols-2 gap-4 p-6 md:grid-cols-4 lg:grid-cols-6",
		"absolute inset-0 z-50 backdrop-blur-sm bg-black/50",
		"mx-auto max-w-4xl space-y-6 px-4 py-10 dark:bg-zinc-900",
		"inline-flex h-10 items-center justify-center rounded-md px-4 text-sm font-semibold disabled:opacity-50 focus:ring-2",
		"divide-y divide-zinc-100 overflow-hidden rounded-xl w-full h-full min-w-0 flex-1 shrink-0",
	}

	out := make([]string, attrs)
	for i := range out {
		out[i] = pools[i%len(pools)]
	}

	return out
}

func BenchmarkSort(b *testing.B) {
	sizes := []struct {
		name  string
		attrs int
	}{
		{"typical", 12},
		{"large", 200},
	}

	for _, tc := range sizes {
		lists := genClassLists(tc.attrs)

		size := 0
		for _, l := range lists {
			size += len(l)
		}

		// Build the registry outside the timed loop: it is a one-time sync.Once
		// cost that would otherwise land entirely on the first iteration.
		Sort(nil, lists[0])

		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ReportAllocs()

			for b.Loop() {
				for _, l := range lists {
					if Sort(nil, l) == "" {
						b.Fatal("empty sort result")
					}
				}
			}
		})
	}
}

// Guards the frequencies genClassLists claims, so a later pool edit cannot turn
// the benchmark into a workload that over-states the `:` and `/` fast paths.
func TestGenClassListsMixMatchesComment(t *testing.T) {
	tokens := strings.Fields(strings.Join(genClassLists(8), " "))

	colons, slashes := 0, 0

	for _, tok := range tokens {
		if strings.Contains(tok, ":") {
			colons++
		}

		if strings.Contains(tok, "/") {
			slashes++
		}
	}

	if pct := colons * 100 / len(tokens); pct < 7 || pct > 13 {
		t.Errorf("%d%% of tokens carry a `:`, want near the measured 9%%", pct)
	}

	if pct := slashes * 100 / len(tokens); pct > 4 {
		t.Errorf("%d%% of tokens carry a `/`, want near the measured 1%%", pct)
	}
}
