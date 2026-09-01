package checks

import (
	"go/build/constraint"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6077(t *testing.T) {
	t.Parallel()
	const child = "PERFSCAN_PS6077_ARM64_TEST"
	if os.Getenv(child) == "1" {
		analysistest.Run(t, analysistest.TestData(), PS6077.Analyzer, "ps6077")
		return
	}
	// The diagnostic is anchored in the arm64 scalar partition. Pin the
	// analysistest package load in an isolated process so this test can still
	// run in parallel without mutating the parent process environment.
	command := exec.Command(os.Args[0], "-test.run=^TestPS6077$", "-test.parallel=1")
	command.Env = append(os.Environ(), "GOARCH=arm64", child+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("arm64 analysistest child failed: %v\n%s", err, output)
	}
}

func TestPS6077PartitionSatisfiability(t *testing.T) {
	t.Parallel()
	parse := func(text string) constraint.Expr {
		expression, err := constraint.Parse("//go:build " + text)
		if err != nil {
			t.Fatal(err)
		}
		return expression
	}
	simd := ps6077Source{constraint: parse("arm64 && goexperiment.simd")}
	scalar := ps6077Source{constraint: parse("arm64 && !goexperiment.simd")}
	if !ps6077MutuallyExclusive(simd, scalar) {
		t.Fatal("opposite experiment partitions must be satisfiable and exclusive")
	}
	overlap := ps6077Source{constraint: parse("arm64")}
	if ps6077MutuallyExclusive(simd, overlap) {
		t.Fatal("a broader arm64 partition overlaps the SIMD feature partition")
	}
	arches := ps6077SatisfiableArchitectures(simd)
	if len(arches) != 1 || !arches["arm64"] {
		t.Fatalf("unexpected satisfiable architecture set: %v", arches)
	}
}
