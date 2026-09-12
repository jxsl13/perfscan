package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// discoverTests retains package-index order independently of completion order.
// No results are published until every worker has stopped successfully. Each
// callback still owns its existing per-package timeout and retry semantics.
func discoverTests(ctx context.Context, packages []string, workers int, race bool, timeout time.Duration, list func(context.Context, string, bool, time.Duration) ([]string, error)) ([][]string, error) {
	if workers < 1 {
		return nil, errors.New("testparallel: discovery workers must be at least 1")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([][]string, len(packages))
	failures := make([]error, len(packages))
	queue := make(chan int)
	var wg sync.WaitGroup
	for range min(workers, len(packages)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range queue {
				if ctx.Err() != nil {
					return
				}
				names, err := list(ctx, packages[index], race, timeout)
				if err != nil {
					failures[index] = fmt.Errorf("discovery %s: %w", packages[index], err)
					cancel()
					return
				}
				results[index] = names
			}
		}()
	}
dispatch:
	for index := range packages {
		select {
		case queue <- index:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(queue)
	wg.Wait()
	var observed []error
	for _, failure := range failures {
		if failure != nil {
			observed = append(observed, failure)
		}
	}
	if err := ctx.Err(); err != nil {
		observed = append(observed, err)
	}
	if err := errors.Join(observed...); err != nil {
		return nil, err
	}
	return results, nil
}
