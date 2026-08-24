//go:build go1.22

package ps6088initcomma

import "sync"

func repeatedCommaOKInitializer(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedCommaOKInitializer creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

var commaValue, commaOK = any(repeatedCommaOKInitializer(2, consume)).(string)
var commaLater = repeatedCommaOKInitializer(3, consume)

func consume(int) {}
