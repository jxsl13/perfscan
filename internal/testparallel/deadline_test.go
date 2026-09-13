package main

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"testing/synctest"
	"time"
)

func TestRunTestAttemptsPreservesOuterDeadlineAndProcessError(t *testing.T) {
	t.Parallel()
	for _, processError := range []error{&exec.ExitError{}, errors.New("process failed"), nil} {
		t.Run(errorKind(processError), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				const budget = 20 * time.Minute
				const progress = "=== RUN   TestStillRunning\n"
				started := time.Now()
				attempts := 0
				output, err := runTestAttempts(context.Background(), "windows", budget, func(ctx context.Context, remaining time.Duration) (string, error) {
					attempts++
					if remaining != budget {
						t.Fatalf("remaining = %s, want %s", remaining, budget)
					}
					<-ctx.Done()
					return progress, processError
				})
				if attempts != 1 || output != progress || !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("attempts=%d output=%q err=%v; want one attempt, preserved progress and deadline", attempts, output, err)
				}
				if processError != nil && !errors.Is(err, processError) {
					t.Fatalf("process error was discarded: %v", err)
				}
				if elapsed := time.Since(started); elapsed != budget+5*time.Second {
					t.Fatalf("timeout changed: elapsed=%s want=%s", elapsed, budget+5*time.Second)
				}
			})
		})
	}
}

func TestRunTestAttemptsPreservesParentCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	processError := errors.New("process terminated")
	const progress = "exit status 0xc0000005\n"
	attempts := 0
	output, err := runTestAttempts(ctx, "windows", time.Minute, func(context.Context, time.Duration) (string, error) {
		attempts++
		cancel()
		return progress, processError
	})
	if attempts != 1 || output != progress || !errors.Is(err, context.Canceled) || !errors.Is(err, processError) {
		t.Fatalf("attempts=%d output=%q err=%v; want cancellation and process error without retry", attempts, output, err)
	}
}

func TestRunTestAttemptsDoesNotInventDeadline(t *testing.T) {
	t.Parallel()
	processError := errors.New("ordinary failure")
	attempts := 0
	output, err := runTestAttempts(context.Background(), "windows", time.Minute, func(context.Context, time.Duration) (string, error) {
		attempts++
		return "--- FAIL: TestBroken\n", processError
	})
	if attempts != 1 || output != "--- FAIL: TestBroken\n" || err != processError || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("attempts=%d output=%q err=%v; ordinary failure identity must remain unchanged", attempts, output, err)
	}
}

func errorKind(err error) string {
	if err == nil {
		return "nil-process-error"
	}
	if _, ok := err.(*exec.ExitError); ok {
		return "exit-error"
	}
	return "ordinary-error"
}
