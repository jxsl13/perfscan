package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6100(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6100.Analyzer, "ps6100")
}

func TestPS6100CandidateBounds(t *testing.T) {
	t.Parallel()
	if ps6100MaxScansPerIteration < 2 || ps6100MaxMutationsPerIteration < 2 || ps6100MaxPredicateBits < 2 || ps6100MaxCallableWork < ps6100MaxHelperDepth {
		t.Fatalf("invalid finite-state candidate bounds: scans=%d mutations=%d bits=%d callable-work=%d helper-depth=%d", ps6100MaxScansPerIteration, ps6100MaxMutationsPerIteration, ps6100MaxPredicateBits, ps6100MaxCallableWork, ps6100MaxHelperDepth)
	}
}
