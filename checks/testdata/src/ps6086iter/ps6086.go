//go:build go1.23

package ps6086iter

import "sync"

func iteratorFanout(iterator func(func(int) bool), work func(int)) {
	var wg sync.WaitGroup
	for item := range iterator {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(item)
		}()
	}
	wg.Wait()
}

func genericNestedIterator[T ~func(func(int) bool)](n int, iterator T, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		for range iterator {
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func concreteNestedIterator(n int, iterator func(func(int) bool), work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		for range iterator {
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}
