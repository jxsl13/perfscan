package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6062(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6062.Analyzer, "ps6062")
}

func TestPS6062EffectiveFraction(t *testing.T) {
	leaf := 1.20
	observed := ps6062EffectiveFraction(leaf, 1.02)
	required := ps6062EffectiveFraction(leaf, 1.03)
	if !(observed > 0 && required > observed && required < 1) {
		t.Fatalf("unexpected Amdahl fractions: observed=%g required=%g", observed, required)
	}
}
