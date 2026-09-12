package main

import (
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCrossoverIntegerLists(t *testing.T) {
	t.Parallel()
	values, err := ints("2048,2097152")
	if err != nil || len(values) != 2 || values[0] != 2048 || values[1] != 2097152 {
		t.Fatalf("valid matrix list: %v %v", values, err)
	}
	for _, value := range []string{"", "2048,", "n2048", "1.5", "2 3"} {
		if _, err := ints(value); err == nil {
			t.Fatalf("accepted ambiguous list %q", value)
		}
	}
}

func TestCrossoverCLIRejectsUnpinnedVerification(t *testing.T) {
	t.Parallel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrossoverCLIChild$")
	cmd.Env = append(os.Environ(), "PERFSCAN_CROSSOVER_CLI_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "external") {
		t.Fatalf("CLI did not fail closed before loading unpinned evidence: %v %s", err, out)
	}
}

func TestCrossoverCLIChild(t *testing.T) {
	if os.Getenv("PERFSCAN_CROSSOVER_CLI_CHILD") != "1" {
		return
	}
	// Only this isolated process changes the command's flag state.
	flag.CommandLine = flag.NewFlagSet("crossover", flag.ContinueOnError)
	os.Args = []string{"crossover", "-verify", "/untrusted/campaign"}
	main()
}
