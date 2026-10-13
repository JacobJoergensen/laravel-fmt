// Package parallel runs indexed work concurrently and hands the results back in
// index order.
package parallel

import "sync"

// Ordered calls work for every index in [0, n) on up to workers goroutines,
// and emit for each result in index order on the calling goroutine.
//
// CONTRACT: at most window results exist at once, so memory is bounded by the
// window, not by n. The window is raised to at least workers, so no worker
// idles for want of room.
func Ordered[T any](n, workers, window int, work func(i int) T, emit func(i int, v T)) {
	if n <= 0 {
		return
	}

	workers = max(1, min(workers, n))
	window = max(window, workers)

	// A result lives in the slot of its index modulo the window. The token count
	// guarantees index i+window is not started before index i is emitted, so a
	// slot is empty whenever it is written.
	slots := make([]chan T, window)
	for i := range slots {
		slots[i] = make(chan T, 1)
	}

	tokens := make(chan struct{}, window)
	jobs := make(chan int)

	go func() {
		for i := range n {
			tokens <- struct{}{}
			jobs <- i
		}

		close(jobs)
	}()

	var wg sync.WaitGroup

	for range workers {
		wg.Go(func() {
			for i := range jobs {
				slots[i%window] <- work(i)
			}
		})
	}

	for i := range n {
		emit(i, <-slots[i%window])
		<-tokens
	}

	wg.Wait()
}
