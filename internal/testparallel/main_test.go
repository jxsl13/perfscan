package main

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestListTestsUsesRequestedRaceBuild(t *testing.T) {
	t.Parallel()
	const fixture = "./testdata/racefixture"
	ordinary, err := listTests(context.Background(), fixture, false)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(ordinary, "TestRaceBuildOnly") {
		t.Fatalf("ordinary discovery unexpectedly included race-tagged test: %v", ordinary)
	}
	traced, err := listTests(context.Background(), fixture, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(traced, "TestRaceBuildOnly") {
		t.Fatalf("race discovery omitted race-tagged test: %v", traced)
	}
}

func TestListPackagesUsesRequestedRaceBuild(t *testing.T) {
	t.Parallel()
	packages, err := listPackages(context.Background(), []string{"./testdata/racepackage"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || !strings.HasSuffix(packages[0], "/internal/testparallel/testdata/racepackage") {
		t.Fatalf("race package discovery = %v, want the race-only fixture package", packages)
	}
}

func TestParseTestNamesExcludesBenchmarksAndNoise(t *testing.T) {
	t.Parallel()
	got := parseTestNames("TestBeta\nBenchmarkHot\nExampleThing\nFuzzInput\nok pkg 0.1s\nTestAlpha\n")
	want := []string{"ExampleThing", "FuzzInput", "TestAlpha", "TestBeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseTestNames() = %v, want %v", got, want)
	}
}

func TestPartitionIsStableAndBalanced(t *testing.T) {
	t.Parallel()
	got := partition([]string{"A", "B", "C", "D", "E"}, 3)
	want := [][]string{{"A", "D"}, {"B", "E"}, {"C"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("partition() = %v, want %v", got, want)
	}
}

func TestPatternIsAnchoredAndEscaped(t *testing.T) {
	t.Parallel()
	if got, want := testPattern([]string{"TestA/B", "TestC+"}), `^(TestA/B|TestC\+)$`; got != want {
		t.Fatalf("testPattern() = %q, want %q", got, want)
	}
}

func TestTestArgsBoundNestedParallelismAndTimeout(t *testing.T) {
	t.Parallel()
	job := testJob{pkg: "example.com/p", names: []string{"TestA"}}
	got := testArgs(job, 1, 20*time.Minute, true)
	want := []string{
		"test",
		"-count=1",
		"-timeout=20m0s",
		"-parallel=1",
		"-run",
		"^(TestA)$",
		"-race",
		"example.com/p",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("testArgs() = %v, want %v", got, want)
	}
}

func TestRetryableWindowsAccessViolation(t *testing.T) {
	t.Parallel()
	err := context.DeadlineExceeded
	if !retryableWindowsAccessViolation(context.Background(), "windows", "exit status 0xC0000005\nFAIL", err) {
		t.Fatal("Windows access violation was not retryable")
	}
	for _, test := range []struct {
		name   string
		goos   string
		output string
		err    error
	}{
		{name: "success", goos: "windows", output: "ok", err: nil},
		{name: "ordinary failure", goos: "windows", output: "--- FAIL: TestBroken", err: err},
		{name: "test failure plus status text", goos: "windows", output: "--- FAIL: TestBroken\nexit status 0xc0000005", err: err},
		{name: "embedded status text", goos: "windows", output: "diagnostic: exit status 0xc0000005", err: err},
		{name: "other platform", goos: "linux", output: "exit status 0xc0000005", err: err},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if retryableWindowsAccessViolation(context.Background(), test.goos, test.output, test.err) {
				t.Fatal("unexpected retry")
			}
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if retryableWindowsAccessViolation(canceled, "windows", "exit status 0xc0000005", err) {
		t.Fatal("canceled job was retryable")
	}
}

func TestRunTestAttemptsRetriesTwoConsecutiveAccessViolations(t *testing.T) {
	t.Parallel()
	attempts := 0
	output, err := runTestAttempts(context.Background(), "windows", time.Minute, func(context.Context, time.Duration) (string, error) {
		attempts++
		if attempts <= 2 {
			return "exit status 0xc0000005\nFAIL\n", context.DeadlineExceeded
		}
		return "ok\n", nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("err = %v, attempts = %d; want nil, 3", err, attempts)
	}
	if strings.Count(output, "retrying this complete shard") != 2 || !strings.Contains(output, "attempt 3/3") || !strings.HasSuffix(output, "ok\n") {
		t.Fatalf("combined output did not preserve retry evidence and success: %q", output)
	}
}

func TestRunTestAttemptsKeepsPersistentCrashFailed(t *testing.T) {
	t.Parallel()
	attempts := 0
	_, err := runTestAttempts(context.Background(), "windows", time.Minute, func(context.Context, time.Duration) (string, error) {
		attempts++
		return "exit status 0xc0000005\nFAIL\n", context.DeadlineExceeded
	})
	if err == nil || attempts != windowsAccessViolationAttempts {
		t.Fatalf("err = %v, attempts = %d; want failure after %d attempts", err, attempts, windowsAccessViolationAttempts)
	}
}

func TestRunTestAttemptsDoesNotRetryOrdinaryFailure(t *testing.T) {
	t.Parallel()
	attempts := 0
	_, err := runTestAttempts(context.Background(), "windows", time.Minute, func(context.Context, time.Duration) (string, error) {
		attempts++
		return "--- FAIL: TestBroken\n", context.DeadlineExceeded
	})
	if err == nil || attempts != 1 {
		t.Fatalf("err = %v, attempts = %d; want failure after 1 attempt", err, attempts)
	}
}

func TestRunTestAttemptsSharesTimeoutAcrossRetries(t *testing.T) {
	t.Parallel()
	const timeout = 30 * time.Millisecond
	attempts := 0
	started := time.Now()
	output, err := runTestAttempts(context.Background(), "windows", timeout, func(attemptCtx context.Context, remaining time.Duration) (string, error) {
		attempts++
		if remaining <= 0 || remaining > timeout {
			return "", context.Canceled
		}
		if attempts == 1 {
			return "exit status 0xc0000005\nFAIL\n", context.DeadlineExceeded
		}
		<-attemptCtx.Done()
		return "", attemptCtx.Err()
	})
	if err == nil || attempts != 2 {
		t.Fatalf("err = %v, attempts = %d; want timeout failure after one retry", err, attempts)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("shared timeout took %s; retry appears to have received a fresh budget", elapsed)
	}
	if !strings.Contains(output, "retrying this complete shard") {
		t.Fatalf("combined output omitted retry evidence: %q", output)
	}
}

func TestRunTestAttemptsLeavesTimeoutDiagnosticGrace(t *testing.T) {
	t.Parallel()
	const timeout = 10 * time.Second
	var diagnosticGrace time.Duration
	_, err := runTestAttempts(context.Background(), "linux", timeout, func(attemptCtx context.Context, remaining time.Duration) (string, error) {
		outerDeadline, ok := attemptCtx.Deadline()
		if !ok {
			return "", context.Canceled
		}
		diagnosticGrace = time.Until(outerDeadline) - remaining
		return "ok\n", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnosticGrace < 900*time.Millisecond || diagnosticGrace > 1100*time.Millisecond {
		t.Fatalf("diagnostic grace = %s, want about 1s", diagnosticGrace)
	}
}
