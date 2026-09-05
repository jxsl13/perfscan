// Command testparallel runs ordinary Go tests in isolated parallel shards.
// Benchmarks are deliberately excluded and remain in their dedicated CI step.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type testJob struct {
	pkg        string
	shard      int
	shardCount int
	names      []string
}

func main() {
	workers := flag.Int("workers", runtime.GOMAXPROCS(0), "maximum concurrent go test processes")
	// Shards already provide cross-test concurrency. Keeping the per-process
	// test semaphore at one avoids multiplying GOMAXPROCS-heavy analyzer loads.
	parallel := flag.Int("parallel", 1, "maximum tests run in parallel within each shard")
	timeout := flag.Duration("timeout", 20*time.Minute, "timeout for each test shard")
	race := flag.Bool("race", false, "run each shard with the race detector")
	shardIndex := flag.Int("shard-index", 0, "zero-based external shard assigned to this process")
	shardCount := flag.Int("shard-count", 1, "number of external shards covering the complete test set")
	flag.Parse()
	if *workers < 1 {
		_, _ = io.WriteString(os.Stderr, "testparallel: -workers must be at least 1\n")
		os.Exit(2)
	}
	if *parallel < 1 {
		_, _ = io.WriteString(os.Stderr, "testparallel: -parallel must be at least 1\n")
		os.Exit(2)
	}
	if *timeout <= 0 {
		_, _ = io.WriteString(os.Stderr, "testparallel: -timeout must be greater than zero\n")
		os.Exit(2)
	}
	if err := validateExternalShard(*shardIndex, *shardCount); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	packages, err := listPackages(ctx, patterns, *race)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	jobs := make([]testJob, 0, len(packages)*(*workers))
	for _, pkg := range packages {
		names, err := listTests(ctx, pkg, *race, *timeout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		jobs = append(jobs, makeTestJobs(pkg, names, *workers, *shardIndex, *shardCount)...)
	}
	if err := runJobs(ctx, jobs, *workers, *parallel, *timeout, *race); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func validateExternalShard(index, count int) error {
	if count < 1 {
		return errors.New("testparallel: -shard-count must be at least 1")
	}
	if index < 0 || index >= count {
		return fmt.Errorf("testparallel: -shard-index must be between 0 and %d", count-1)
	}
	return nil
}

func makeTestJobs(pkg string, names []string, workers, externalIndex, externalCount int) []testJob {
	selected := selectExternalShard(pkg, names, externalIndex, externalCount)
	groups := partition(selected, workers)
	jobs := make([]testJob, 0, len(groups))
	for shard, group := range groups {
		jobs = append(jobs, testJob{pkg: pkg, shard: shard, shardCount: len(groups), names: group})
	}
	return jobs
}

// selectExternalShard assigns each test by a stable hash of its package and
// name. Package qualification avoids concentrating common names such as
// TestBasic on one external runner. Names shared by independently discovered
// lists retain the same assignment instead of shifting with list position.
func selectExternalShard(pkg string, names []string, index, count int) []string {
	if count == 1 {
		return names
	}
	selected := make([]string, 0, len(names)/count+1)
	for _, name := range names {
		if externalShardForName(pkg, name, count) == index {
			selected = append(selected, name)
		}
	}
	return selected
}

func externalShardForName(pkg, name string, count int) int {
	const (
		fnvOffset64 = uint64(14695981039346656037)
		fnvPrime64  = uint64(1099511628211)
	)
	hash := fnvOffset64
	for _, value := range []string{pkg, "\x00", name} {
		for i := range len(value) {
			hash ^= uint64(value[i])
			hash *= fnvPrime64
		}
	}
	return int(hash % uint64(count))
}

func listPackages(ctx context.Context, patterns []string, race bool) ([]string, error) {
	args := []string{"list"}
	if race {
		args = append(args, "-race")
	}
	args = append(args, "-f", "{{.ImportPath}}")
	args = append(args, patterns...)
	cmd := exec.CommandContext(ctx, "go", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("testparallel: go list failed: %w\n%s%s", err, out, stderr.Bytes())
	}
	packages := strings.Fields(string(out))
	slices.Sort(packages)
	return packages, nil
}

func listTests(ctx context.Context, pkg string, race bool, timeout time.Duration) ([]string, error) {
	out, err := runTestAttempts(ctx, runtime.GOOS, timeout, func(attemptCtx context.Context, remaining time.Duration) (string, error) {
		args := []string{"test"}
		if race {
			args = append(args, "-race")
		}
		args = append(args, "-timeout", remaining.String(), "-list", ".", pkg)
		cmd := exec.CommandContext(attemptCtx, "go", args...)
		output, err := cmd.CombinedOutput()
		return string(output), err
	})
	if err != nil {
		return nil, fmt.Errorf("testparallel: listing %s failed: %w\n%s", pkg, err, out)
	}
	return parseTestNames(out), nil
}

func parseTestNames(output string) []string {
	var names []string
	for line := range strings.SplitSeq(output, "\n") {
		name := strings.TrimSpace(line)
		if token.IsIdentifier(name) && (isTestName(name, "Test") || isTestName(name, "Fuzz") || isTestName(name, "Example")) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// isTestName mirrors cmd/go's test-name rule: the bare prefix is valid, and
// a suffix must not begin with a lower-case Unicode letter.
func isTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}

func partition(names []string, workers int) [][]string {
	if len(names) == 0 {
		return nil
	}
	count := min(workers, len(names))
	groups := make([][]string, count)
	for i, name := range names {
		groups[i%count] = append(groups[i%count], name)
	}
	return groups
}

func runJobs(ctx context.Context, jobs []testJob, workers, parallel int, timeout time.Duration, race bool) error {
	queue := make(chan testJob)
	errs := make(chan error, len(jobs))
	var outputMu sync.Mutex
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range queue {
				output, err := runTestJob(ctx, job, parallel, timeout, race, runtime.GOOS)
				outputMu.Lock()
				fmt.Printf("=== %s shard %d/%d ===\n%s", job.pkg, job.shard+1, job.shardCount, output)
				outputMu.Unlock()
				if err != nil {
					errs <- fmt.Errorf("%s shard %d/%d: %w", job.pkg, job.shard+1, job.shardCount, err)
				}
			}
		}()
	}
	go func() {
		defer close(queue)
		for _, job := range jobs {
			select {
			case queue <- job:
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	failures := make([]string, 0, len(jobs)+1)
	for err := range errs {
		failures = append(failures, err.Error())
	}
	if ctx.Err() != nil {
		failures = append(failures, ctx.Err().Error())
	}
	if len(failures) > 0 {
		slices.Sort(failures)
		return fmt.Errorf("testparallel: %d shard(s) failed:\n%s", len(failures), strings.Join(failures, "\n"))
	}
	return nil
}

func runTestJob(ctx context.Context, job testJob, parallel int, timeout time.Duration, race bool, goos string) (string, error) {
	return runTestAttempts(ctx, goos, timeout, func(attemptCtx context.Context, remaining time.Duration) (string, error) {
		args := testArgs(job, parallel, remaining, race)
		cmd := exec.CommandContext(attemptCtx, "go", args...)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		return output.String(), err
	})
}

const windowsRuntimeCrashAttempts = 3

func runTestAttempts(ctx context.Context, goos string, timeout time.Duration, run func(context.Context, time.Duration) (string, error)) (string, error) {
	attemptCtx := ctx
	cancel := func() {}
	var budgetDeadline time.Time
	if timeout > 0 {
		grace := testTimeoutGrace(timeout)
		budgetDeadline = time.Now().Add(timeout)
		attemptCtx, cancel = context.WithTimeout(ctx, timeout+grace)
		if parentDeadline, ok := ctx.Deadline(); ok {
			parentBudgetDeadline := parentDeadline.Add(-grace)
			if parentBudgetDeadline.Before(budgetDeadline) {
				budgetDeadline = parentBudgetDeadline
			}
		}
	}
	defer cancel()

	var combined strings.Builder
	combined.Grow(256)
	for attempt := 1; attempt <= windowsRuntimeCrashAttempts; attempt++ {
		remaining := timeout
		if timeout > 0 {
			remaining = time.Until(budgetDeadline)
			if remaining <= 0 {
				return combined.String(), context.DeadlineExceeded
			}
		}
		output, err := run(attemptCtx, remaining)
		combined.WriteString(output)
		if !retryableWindowsRuntimeCrash(attemptCtx, goos, output, err) || attempt == windowsRuntimeCrashAttempts {
			return combined.String(), err
		}
		if timeout > 0 && time.Until(budgetDeadline) <= 0 {
			return combined.String(), context.DeadlineExceeded
		}
		fmt.Fprintf(&combined, "testparallel: Windows Go runtime crashed; retrying this complete shard (attempt %d/%d)\n", attempt+1, windowsRuntimeCrashAttempts)
	}
	panic("unreachable")
}

func testTimeoutGrace(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 0
	}
	grace := timeout / 10
	if grace < time.Millisecond {
		return time.Millisecond
	}
	return min(grace, 5*time.Second)
}

func retryableWindowsRuntimeCrash(ctx context.Context, goos, output string, err error) bool {
	if err == nil || ctx.Err() != nil || goos != "windows" {
		return false
	}
	accessViolation := false
	waitingListCorruption := false
	runtimeThrow := false
	runtimePanicSource := false
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(strings.ToLower(line))
		switch {
		case strings.HasPrefix(line, "--- fail:"),
			line == "warning: data race",
			strings.Contains(line, "race detected"),
			strings.HasPrefix(line, "panic:"),
			strings.Contains(line, "[build failed]"),
			strings.HasPrefix(line, "found ") && strings.HasSuffix(line, " data race(s)"):
			return false
		case line == "exit status 0xc0000005":
			accessViolation = true
		case line == "fatal error: g waiting list is corrupted":
			waitingListCorruption = true
		case strings.HasPrefix(line, "fatal error:"):
			return false
		case strings.HasPrefix(line, "runtime.throw("):
			runtimeThrow = true
		case strings.Contains(line, "/src/runtime/panic.go:") || strings.Contains(line, `\src\runtime\panic.go:`):
			runtimePanicSource = true
		}
	}
	return accessViolation || waitingListCorruption && runtimeThrow && runtimePanicSource
}

func testArgs(job testJob, parallel int, timeout time.Duration, race bool) []string {
	args := []string{
		"test",
		"-count=1",
		fmt.Sprintf("-timeout=%s", timeout),
		fmt.Sprintf("-parallel=%d", parallel),
		"-run",
		testPattern(job.names),
	}
	if race {
		args = append(args, "-race")
	}
	return append(args, job.pkg)
}

func testPattern(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}
