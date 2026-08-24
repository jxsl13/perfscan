//go:build go1.22

package ps6088initselect

import "sync"

func repeatedOnlyAfterSelectOperand(n int, work func(int)) int {
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

var selectInitializer = func() int {
	select {
	case (chan int)(nil) <- func() int { select {} }():
	default:
		return repeatedOnlyAfterSelectOperand(2, consume) + repeatedOnlyAfterSelectOperand(3, consume)
	}
	return 0
}()

func consume(int) {}
