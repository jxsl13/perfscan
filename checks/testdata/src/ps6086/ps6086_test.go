//go:build go1.22

package ps6086

import "sync"

func testFileFanout(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}
