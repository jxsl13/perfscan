//go:build go1.22

package ps6088initorder

import "sync"

func repeatedOnlyInLaterFile(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

var laterFileOne = repeatedOnlyInLaterFile(2, consume)
var laterFileTwo = repeatedOnlyInLaterFile(3, consume)

func consume(int) {}
