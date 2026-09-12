package crossover

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/allocationcampaign"
	"github.com/jxsl13/perfscan/internal/closureenv"
)

func TestCampaignRawPreflightBeforeSourceFactory(t *testing.T) {
	t.Parallel()
	for _, corruption := range []string{"stdout", "stderr", "exit", "failed-status", "order", "valid"} {
		t.Run(corruption, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			p := preflightPlan(t)
			records := preflightRecords(t, directory, p.Invocations)
			last := len(records) - 1 // every invocation must be checked, not only sample zero
			name := "sample-" + strconv.Itoa(last)
			switch corruption {
			case "stdout", "stderr", "exit":
				preflightWrite(t, directory, name+"."+corruption, []byte("changed\n"))
			case "failed-status":
				records[last].Raw.Exit = 54
				preflightWrite(t, directory, name+".exit", []byte("54\n"))
			case "order":
				records[last].Invocation = records[0].Invocation
			}
			repository := t.TempDir()
			if corruption == "valid" {
				p.Snapshot = preflightSnapshot(t, repository, filepath.Join(directory, "snapshot"))
			} else if err := os.Mkdir(filepath.Join(directory, "snapshot"), 0700); err != nil {
				t.Fatal(err)
			}
			preflightWrite(t, directory, "evidence.test", nil)
			planData, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			recordsData, err := json.Marshal(records)
			if err != nil {
				t.Fatal(err)
			}
			preflightWrite(t, directory, "plan.json", planData)
			preflightWrite(t, directory, "records.json", recordsData)
			called := false
			stop := errors.New("source factory reached; build verification remains required")
			factory := func(*BuildSelection, *config.DispatchCrossoverContract) (*HarnessModel, error) {
				called = true
				return nil, stop
			}
			_, err = VerifyCampaign(context.Background(), repository, directory, "not-a-compiler", digest(planData), digest(recordsData), factory)
			if corruption == "valid" {
				if !called || !errors.Is(err, stop) {
					t.Fatalf("valid raw evidence bypassed source/build verification: called=%v err=%v", called, err)
				}
			} else {
				if called || err == nil {
					t.Fatalf("corrupt raw evidence reached source factory: called=%v err=%v", called, err)
				}
				want := "raw invocation"
				if corruption == "order" {
					want = "sample order"
				}
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("rejected at wrong stage: %v", err)
				}
			}
		})
	}
}

func TestCampaignRawPreflightDoesNotCacheStreams(t *testing.T) {
	t.Parallel()
	for _, stream := range []string{"stdout", "stderr", "exit"} {
		t.Run(stream, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			expected := []Invocation{{Scope: "policy"}}
			records := preflightRecords(t, directory, expected)
			if err := preflightInvocations(directory, expected, records); err != nil {
				t.Fatal(err)
			}
			// Simulate a mutation during the intervening source/build reproduction.
			preflightWrite(t, directory, "sample-0."+stream, []byte("changed after preflight\n"))
			if _, err := allocationcampaign.ReadInvocation(directory, "sample-0", records[0].Raw); err == nil {
				t.Fatal("post-build raw re-read accepted a changed artifact")
			}
		})
	}
}

func preflightPlan(t *testing.T) Plan {
	t.Helper()
	invocations, err := Schedule(2, []int{1, 2}, []int{2}, 2)
	if err != nil {
		t.Fatal(err)
	}
	return Plan{Schema: 1, Boundary: 2, Sizes: []int{1, 2}, Procs: []int{2}, Pairs: 2, N: 2, TimeoutNanos: int64(time.Minute), Invocations: invocations,
		Build:    closureenv.BinaryBuild{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, SourceSHA256: map[string]string{}},
		Runtime:  map[string]string{"GODEBUG": "", "GOGC": "", "GOMEMLIMIT": "", "GOTRACEBACK": ""},
		Contract: config.DispatchCrossoverContract{DispatchFunction: "fixture.dispatch", ThresholdConstant: "fixture.threshold", SerialPolicy: "fixture.serial", ParallelPolicy: "fixture.parallel", WorkerRunner: "fixture.worker", OperationInputFactory: "fixture.inputs", DiagnosticFunction: "fixture.TestDiagnostic", ProductionBenchmark: "BenchmarkFixture/n{n}", LeafKernels: []string{"fixture.leaf"}, Campaign: "fixture"}}
}

func preflightRecords(t *testing.T, directory string, expected []Invocation) []Record {
	t.Helper()
	records := make([]Record, len(expected))
	for i, invocation := range expected {
		out := []byte("synthetic retained stdout\n")
		records[i] = Record{Invocation: invocation, Raw: allocationcampaign.RawInvocation{StdoutSHA256: digest(out), StderrSHA256: digest(nil)}}
		name := "sample-" + strconv.Itoa(i)
		preflightWrite(t, directory, name+".stdout", out)
		preflightWrite(t, directory, name+".stderr", nil)
		preflightWrite(t, directory, name+".exit", []byte("0\n"))
	}
	return records
}

func preflightWrite(t *testing.T, directory, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func preflightSnapshot(t *testing.T, repository, output string) allocationcampaign.Snapshot {
	t.Helper()
	preflightWrite(t, repository, "go.mod", []byte("module fixture\ngo 1.25\n"))
	env := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "git", args...)
		command.Dir, command.Env = repository, env
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet")
	git("add", "go.mod")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "core.hooksPath=", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture")
	pin, err := allocationcampaign.RetainSnapshot(repository, git("rev-parse", "HEAD"), output)
	if err != nil {
		t.Fatal(err)
	}
	return pin
}
