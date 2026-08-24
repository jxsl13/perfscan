//go:build go1.23

package ps6088iter

import "sync"

type iterator func(func(int) bool)

func repeatedOnlyInNilIteratorRange(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nilIteratorRangeOne(n int) {
	for value := range iterator(nil) {
		_ = value
		repeatedOnlyInNilIteratorRange(n, consume)
	}
}

func nilIteratorRangeTwo(n int) {
	for value := range iterator(nil) {
		_ = value
		repeatedOnlyInNilIteratorRange(n, consume)
	}
}

func localNilIteratorRangeOne(n int) {
	var values iterator
	for value := range values {
		_ = value
		repeatedOnlyInNilIteratorRange(n, consume)
	}
}

func localNilIteratorRangeTwo(n int) {
	values := iterator(nil)
	for value := range values {
		_ = value
		repeatedOnlyInNilIteratorRange(n, consume)
	}
}

func localEmptySliceRangeOne(n int) {
	values := []int{}
	for range values {
		repeatedOnlyInNilIteratorRange(n, consume)
	}
}

func localEmptySliceRangeTwo(n int) {
	values := []int{}
	for range values {
		repeatedOnlyInNilIteratorRange(n, consume)
	}
}

func localNilMapRangeOne(n int) {
	var values map[int]int
	for range values {
		repeatedOnlyInNilIteratorRange(n, consume)
	}
}

func localNilMapRangeTwo(n int) {
	var values map[int]int
	for range values {
		repeatedOnlyInNilIteratorRange(n, consume)
	}
}

func repeatedInLiveIteratorRange(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInLiveIteratorRange creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func liveIterator(yield func(int) bool) {
	yield(1)
}

func liveIteratorRangeOne(n int) {
	for value := range liveIterator {
		_ = value
		repeatedInLiveIteratorRange(n, consume)
	}
}

func liveIteratorRangeTwo(n int) {
	for value := range liveIterator {
		_ = value
		repeatedInLiveIteratorRange(n, consume)
	}
}

func consume(int) {}
