package cpubudget

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if runtime.GOMAXPROCS(0) != 2 || os.Getenv("GOMAXPROCS") != "2" {
		fmt.Fprintln(os.Stderr, "worker CPU budget did not reach process")
		os.Exit(1)
	}
	want := "3"
	if flag.Lookup("test.list").Value.String() != "" {
		want = "2"
	}
	if flag.Lookup("test.parallel").Value.String() != want {
		fmt.Fprintln(os.Stderr, "parallel default/explicit override not preserved")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestNestedBudget(t *testing.T) {
	t.Parallel()
	if os.Getenv("GOMAXPROCS") != "2" || runtime.GOMAXPROCS(0) != 2 {
		t.Fatal("nested budget lost")
	}
}

func TestWorkerSpawnsBoundedChild(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, executable, "-test.run=^TestNestedBudget$", "-test.parallel=3").CombinedOutput()
	if err != nil {
		t.Fatalf("nested test process: %v\n%s", err, output)
	}
}
