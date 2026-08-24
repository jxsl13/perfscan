//go:build go1.22

package ps6088initswitch

import "sync"

func repeatedOnlyAfterSwitchCase(n int, work func(int)) int {
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

var switchInitializer = func() int {
	switch 0 {
	case func() int { select {} }():
	case 0:
		return repeatedOnlyAfterSwitchCase(2, consume) + repeatedOnlyAfterSwitchCase(3, consume)
	}
	return 0
}()

func consume(int) {}
