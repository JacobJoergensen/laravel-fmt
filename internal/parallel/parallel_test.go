package parallel

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestOrderedEmitsEveryIndexInOrder(t *testing.T) {
	for _, tc := range []struct{ n, workers, window int }{
		{0, 4, 8},
		{1, 4, 8},
		{100, 1, 1},
		{100, 8, 16},
		{100, 16, 2},
	} {
		var got []int

		Ordered(tc.n, tc.workers, tc.window, func(i int) int {
			// Later indices finish first, so order comes from Ordered, not timing.
			time.Sleep(time.Duration((tc.n-i)%7) * 100 * time.Microsecond)

			return i * 2
		}, func(i, v int) {
			if v != i*2 {
				t.Fatalf("index %d got the result of another index: %d", i, v)
			}

			got = append(got, i)
		})

		if len(got) != tc.n {
			t.Fatalf("n=%d: emitted %d results", tc.n, len(got))
		}

		for i, idx := range got {
			if idx != i {
				t.Fatalf("n=%d: position %d emitted index %d", tc.n, i, idx)
			}
		}
	}
}

// Index 0 sleeps, so every later result is ready and waiting on it: the
// worst case for the window.
func TestOrderedKeepsAtMostWindowResultsAlive(t *testing.T) {
	const n, workers, window = 200, 8, 12

	var alive, peak atomic.Int64

	Ordered(n, workers, window, func(i int) int {
		if i == 0 {
			time.Sleep(20 * time.Millisecond)
		}

		now := alive.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}

		return i
	}, func(int, int) {
		alive.Add(-1)
	})

	if p := peak.Load(); p > window {
		t.Fatalf("%d results were alive at once, window is %d", p, window)
	}
}
