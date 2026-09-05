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
	ordinary, err := listTests(context.Background(), fixture, false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(ordinary, "TestRaceBuildOnly") {
		t.Fatalf("ordinary discovery unexpectedly included race-tagged test: %v", ordinary)
	}
	if !slices.Contains(ordinary, "Test") {
		t.Fatalf("ordinary discovery omitted valid bare Test function: %v", ordinary)
	}
	traced, err := listTests(context.Background(), fixture, true, time.Minute)
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
	got := parseTestNames("TestBeta\nBenchmarkHot\nExampleThing\nFuzzInput\nok pkg 0.1s\nTestAlpha\nTestAlpha\nTestlower\nTest Fake\nTest\nFuzz\nExample\nFuzzlower\nExamplelower\n")
	want := []string{"Example", "ExampleThing", "Fuzz", "FuzzInput", "Test", "TestAlpha", "TestBeta"}
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

func TestValidateExternalShard(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		index, count int
		wantError    bool
	}{
		{name: "single shard", index: 0, count: 1},
		{name: "last shard", index: 3, count: 4},
		{name: "zero count", index: 0, count: 0, wantError: true},
		{name: "negative count", index: 0, count: -1, wantError: true},
		{name: "negative index", index: -1, count: 2, wantError: true},
		{name: "index at count", index: 2, count: 2, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validateExternalShard(test.index, test.count); (err != nil) != test.wantError {
				t.Fatalf("validateExternalShard(%d, %d) error = %v, wantError = %v", test.index, test.count, err, test.wantError)
			}
		})
	}
}

func TestExternalShardsAreStableDisjointAndComplete(t *testing.T) {
	t.Parallel()
	names := []string{"A", "B", "C", "D", "E", "F", "G"}
	for _, count := range []int{1, 2, 3, 8} {
		seen := make(map[string]int, len(names))
		for index := range count {
			selected := selectExternalShard("example.com/p", names, index, count)
			if !slices.IsSorted(selected) {
				t.Fatalf("count %d index %d: unstable order %v", count, index, selected)
			}
			for _, name := range selected {
				seen[name]++
			}
		}
		for _, name := range names {
			if seen[name] != 1 {
				t.Fatalf("count %d: %s appeared %d times across external shards, want once", count, name, seen[name])
			}
		}
	}
	maxInt := int(^uint(0) >> 1)
	for _, name := range names {
		index := externalShardForName("example.com/p", name, maxInt)
		if got, want := selectExternalShard("example.com/p", names, index, maxInt), []string{name}; !reflect.DeepEqual(got, want) {
			t.Fatalf("selectExternalShard() with maximum shard count = %v, want %v", got, want)
		}
	}
}

func TestCommonNamesKeepAssignmentAcrossDiscoveryLists(t *testing.T) {
	t.Parallel()
	const count = 2
	workerNames := [][]string{
		{"TestA", "TestC"},
		{"TestA", "TestB", "TestC"},
	}
	for _, name := range []string{"TestA", "TestC"} {
		seen := 0
		for index, names := range workerNames {
			if slices.Contains(selectExternalShard("example.com/p", names, index, count), name) {
				seen++
			}
		}
		if seen != 1 {
			t.Fatalf("%s appeared %d times across workers with different discovery lists, want once", name, seen)
		}
	}
}

func TestExternalAssignmentIncludesPackageIdentity(t *testing.T) {
	t.Parallel()
	const count = 17
	assignments := make(map[int]bool)
	for _, pkg := range []string{"example.com/a", "example.com/b", "example.com/c", "example.com/d"} {
		assignments[externalShardForName(pkg, "TestBasic", count)] = true
	}
	if len(assignments) == 1 {
		t.Fatalf("same-named tests in distinct packages all mapped to one of %d shards", count)
	}
}

func TestExternalAndWorkerShardsCoverEveryTestOnce(t *testing.T) {
	t.Parallel()
	names := []string{"TestA", "TestB", "TestC", "TestD", "TestE", "TestF", "TestG"}
	seen := make(map[string]int, len(names))
	for external := range 2 {
		for _, job := range makeTestJobs("example.com/p", names, 3, external, 2) {
			if job.pkg != "example.com/p" || job.shardCount < 1 || job.shard >= job.shardCount {
				t.Fatalf("invalid job metadata: %+v", job)
			}
			for _, name := range job.names {
				seen[name]++
			}
		}
	}
	for _, name := range names {
		if seen[name] != 1 {
			t.Fatalf("%s appeared %d times after both sharding layers, want once", name, seen[name])
		}
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
