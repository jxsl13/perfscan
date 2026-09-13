package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestWorkerCPUShare(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ available, workers, want int }{{4, 4, 1}, {4, 2, 2}, {12, 2, 6}, {7, 3, 2}, {1, 4, 1}, {0, 2, 1}, {4, 0, 4}} {
		if got := workerCPUShare(tc.available, tc.workers); got != tc.want {
			t.Fatalf("share(%d,%d)=%d want=%d", tc.available, tc.workers, got, tc.want)
		}
	}
}

func TestCLIParallelDefaultsOverridesAndExplicitZero(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	executable := filepath.Join(t.TempDir(), "testparallel")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.CommandContext(ctx, "go", "build", "-o", executable, ".")
	build.Env = workerEnvironment(os.Environ(), 2, runtime.GOOS)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	for _, tc := range []struct {
		name    string
		flags   []string
		want    string
		failure bool
	}{
		{"default", nil, "worker-procs=1 parallel=1", false},
		{"explicitOverride", []string{"-parallel=3"}, "worker-procs=1 parallel=3", false},
		{"explicitZero", []string{"-parallel=0"}, "-parallel must be at least 1", true},
		{"negative", []string{"-parallel=-1"}, "-parallel must be at least 1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := []string{"-workers=2", "-timeout=1m"}
			args = append(args, tc.flags...)
			args = append(args, "./testdata/racefixture")
			command := exec.CommandContext(ctx, executable, args...)
			command.Env = workerEnvironment(os.Environ(), 2, runtime.GOOS)
			output, err := command.CombinedOutput()
			if (err != nil) != tc.failure || !strings.Contains(string(output), tc.want) {
				t.Fatalf("CLI output=%s err=%v", output, err)
			}
			if tc.failure && strings.Contains(string(output), "discovery started") {
				t.Fatal("invalid flag reached discovery")
			}
		})
	}
}

func TestWorkerEnvironmentPreservesUnrelatedKeys(t *testing.T) {
	t.Parallel()
	input := []string{"PATH=source-path", "GOMAXPROCS=99", "GOCACHE=source-cache", "gomaxprocs=98", "GoMaXpRoCs=97", "GODEBUG=source-debug", "VALUE=a=b", "GOMAXPROCS_OTHER=keep"}
	original := slices.Clone(input)
	for _, goos := range []string{"windows", "linux", "darwin"} {
		want := []string{"PATH=source-path", "GOCACHE=source-cache", "GODEBUG=source-debug", "VALUE=a=b", "GOMAXPROCS_OTHER=keep", "GOMAXPROCS=2"}
		if goos != "windows" {
			want = []string{"PATH=source-path", "GOCACHE=source-cache", "gomaxprocs=98", "GoMaXpRoCs=97", "GODEBUG=source-debug", "VALUE=a=b", "GOMAXPROCS_OTHER=keep", "GOMAXPROCS=2"}
		}
		if got := workerEnvironment(input, 2, goos); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s environment=%q want=%q", goos, got, want)
		}
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("mutated caller environment")
	}
}

func TestCPUShareReachesDiscoveryExecutionAndNestedProcess(t *testing.T) {
	t.Parallel()
	const pkg = "github.com/jxsl13/perfscan/internal/testparallel/testdata/cpubudget"
	names, err := listTestsWithCPUShare(context.Background(), pkg, false, 30*time.Second, 2)
	if err != nil || len(names) != 2 {
		t.Fatalf("budgeted discovery=%q err=%v", names, err)
	}
	job := testJob{pkg: pkg, names: names, shardCount: 1}
	// Explicit parallel=3 is deliberately different from runtime budget=2.
	// The fixture rejects lost command inheritance or overwritten overrides.
	output, err := runTestJobWithCPUShare(context.Background(), job, 3, 30*time.Second, false, "darwin", 2)
	if err != nil {
		t.Fatalf("budgeted execution/nested inheritance failed: %v\n%s", err, output)
	}
}
