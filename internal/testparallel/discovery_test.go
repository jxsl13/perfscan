package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type discoveryResult struct {
	names [][]string
	err   error
}

func TestDiscoveryBoundedConcurrentOrderedAndExhaustive(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		packages := []string{"a", "b", "c", "d"}
		gates := make(map[string]chan struct{}, len(packages))
		for _, pkg := range packages {
			gates[pkg] = make(chan struct{})
		}
		var mu sync.Mutex
		active, maximum := 0, 0
		calls := make(map[string]int, len(packages))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan discoveryResult, 1)
		go func() {
			names, err := discoverTests(ctx, packages, 2, true, 17*time.Second, func(ctx context.Context, pkg string, race bool, timeout time.Duration) ([]string, error) {
				if !race || timeout != 17*time.Second {
					t.Errorf("discovery changed race/timeout: %v/%s", race, timeout)
				}
				mu.Lock()
				calls[pkg]++
				active++
				maximum = max(maximum, active)
				mu.Unlock()
				select {
				case <-gates[pkg]:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				mu.Lock()
				active--
				mu.Unlock()
				return []string{"Test" + strings.ToUpper(pkg), "Example" + strings.ToUpper(pkg)}, nil
			})
			done <- discoveryResult{names, err}
		}()
		synctest.Wait()
		mu.Lock()
		initial := reflect.DeepEqual(calls, map[string]int{"a": 1, "b": 1}) && active == 2 && maximum == 2
		mu.Unlock()
		if !initial {
			t.Fatal("discovery did not start exactly two blocked package callbacks")
		}
		// Deliberately finish b/c/d before a: results must retain input order,
		// and no partial successful result may cross the barrier.
		for _, pkg := range []string{"b", "c", "d"} {
			close(gates[pkg])
			synctest.Wait()
			select {
			case result := <-done:
				t.Fatalf("discovery published incomplete results: %+v", result)
			default:
			}
		}
		close(gates["a"])
		result := <-done
		want := [][]string{{"TestA", "ExampleA"}, {"TestB", "ExampleB"}, {"TestC", "ExampleC"}, {"TestD", "ExampleD"}}
		if result.err != nil || !reflect.DeepEqual(result.names, want) {
			t.Fatalf("discovery = %+v, want %v", result, want)
		}
		mu.Lock()
		defer mu.Unlock()
		if maximum != 2 || active != 0 || !reflect.DeepEqual(calls, map[string]int{"a": 1, "b": 1, "c": 1, "d": 1}) {
			t.Fatalf("callback census calls=%v active=%d max=%d", calls, active, maximum)
		}
		for external := range 3 {
			for index, pkg := range packages {
				if got, wantJobs := makeTestJobs(pkg, result.names[index], 2, external, 3), makeTestJobs(pkg, want[index], 2, external, 3); !reflect.DeepEqual(got, wantJobs) {
					t.Fatalf("discovery changed exact external/inner job selection: %v vs %v", got, wantJobs)
				}
			}
		}
	})
}

func TestDiscoveryErrorCancelsAndDrainsWithoutPartialResults(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		firstError, secondError := errors.New("first failure"), errors.New("second failure")
		fail := make(chan struct{})
		finish := make(chan struct{})
		started := make(chan string, 3)
		done := make(chan discoveryResult, 1)
		go func() {
			names, err := discoverTests(context.Background(), []string{"a", "b", "never"}, 2, false, time.Minute, func(ctx context.Context, pkg string, _ bool, _ time.Duration) ([]string, error) {
				started <- pkg
				if pkg == "a" {
					<-fail
					return nil, firstError
				}
				<-ctx.Done()
				<-finish
				return nil, secondError
			})
			done <- discoveryResult{names, err}
		}()
		synctest.Wait()
		close(fail)
		synctest.Wait()
		select {
		case result := <-done:
			t.Fatalf("discovery returned without draining an active callback: %+v", result)
		default:
		}
		close(finish)
		result := <-done
		if result.names != nil || !errors.Is(result.err, firstError) || !errors.Is(result.err, secondError) {
			t.Fatalf("partial results or lost errors: %+v", result)
		}
		if len(started) != 2 || strings.Index(result.err.Error(), "discovery a:") >= strings.Index(result.err.Error(), "discovery b:") {
			t.Fatalf("new callback after failure or nondeterministic error order: %v", result.err)
		}
	})
}

func TestDiscoveryCancellationDrainsAndFailsClosed(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		finish := make(chan struct{})
		started := make(chan string, 3)
		done := make(chan discoveryResult, 1)
		go func() {
			names, err := discoverTests(ctx, []string{"a", "b", "never"}, 2, false, time.Minute, func(ctx context.Context, pkg string, _ bool, _ time.Duration) ([]string, error) {
				started <- pkg
				<-ctx.Done()
				<-finish
				// Even a callback that ignores cancellation on return must not
				// make the overall discovery barrier successful.
				return []string{"TestA"}, nil
			})
			done <- discoveryResult{names, err}
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case result := <-done:
			t.Fatalf("discovery returned before active callbacks stopped: %+v", result)
		default:
		}
		close(finish)
		result := <-done
		if result.names != nil || !errors.Is(result.err, context.Canceled) || len(started) != 2 {
			t.Fatalf("cancellation did not fail closed: %+v, started=%d", result, len(started))
		}
	})
}

func TestDiscoveryEmptyAndInvalidBoundaries(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	list := func(context.Context, string, bool, time.Duration) ([]string, error) {
		t.Error("unexpected callback")
		return nil, nil
	}
	for _, packages := range [][]string{nil, {"a"}} {
		if names, err := discoverTests(ctx, packages, 2, false, time.Minute, list); names != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-canceled discovery = %v, %v", names, err)
		}
	}
	if names, err := discoverTests(context.Background(), nil, 2, false, time.Minute, list); err != nil || len(names) != 0 {
		t.Fatalf("empty discovery = %v, %v", names, err)
	}
	if names, err := discoverTests(context.Background(), []string{"a"}, 0, false, time.Minute, list); err == nil || names != nil {
		t.Fatalf("invalid workers = %v, %v", names, err)
	}
}

func TestDiscoveryDropsCompletedInventoryOnLaterFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("later listing failed")
	var called []string
	names, err := discoverTests(context.Background(), []string{"a", "b", "never"}, 1, false, time.Minute, func(_ context.Context, pkg string, race bool, timeout time.Duration) ([]string, error) {
		called = append(called, pkg)
		if race || timeout != time.Minute {
			t.Errorf("discovery changed ordinary build/timeout: %v/%s", race, timeout)
		}
		if pkg == "b" {
			return nil, failure
		}
		return []string{"TestA", "FuzzInput", "Example"}, nil
	})
	if names != nil || !errors.Is(err, failure) || !reflect.DeepEqual(called, []string{"a", "b"}) {
		t.Fatalf("completed prefix was published or dispatch continued: names=%v err=%v called=%v", names, err, called)
	}
}
