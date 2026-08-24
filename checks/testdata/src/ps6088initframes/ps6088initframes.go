//go:build go1.22

package ps6088initframes

import "sync"

func repeatedInitializerArgument(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInitializerArgument creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

var initializerFrameResult = func(int, int) int {
	panic("after arguments")
}(repeatedInitializerArgument(2, consume), repeatedInitializerArgument(3, consume))

func consume(int) {}
