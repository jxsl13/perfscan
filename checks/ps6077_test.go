package checks

import (
	"go/build/constraint"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6077(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6077.Analyzer, "ps6077")
}

func TestPS6077PartitionSatisfiability(t *testing.T) {
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
