//go:build go1.21

package ps6086old

import "sync"

// Go 1.21 reuses the range variable across iterations. The goroutine captures
// that shared variable, so PS6086 must not treat the launch as a safely
// splittable per-iteration worker.
func oldVersionRangeCapture(chunks []int, work func(int)) {
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(chunk)
		}()
	}
	wg.Wait()
}

// Passing the old-semantics variable by value still makes a safe snapshot.
func oldVersionExplicitSnapshot(chunks []int, work func(int)) {
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Add(1)
		go func(chunk int) { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(chunk)
		}(chunk)
	}
	wg.Wait()
}
