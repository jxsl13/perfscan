package checks

import (
	"math"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6065(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6065.Analyzer, "ps6065")
}

func TestPS6065ClampResidueAmplificationContract(t *testing.T) {
	floor := math.Float32frombits(0x00800000) // smallest positive normal float32
	input := float32(-100)
	upstream := float32(1e37)
	exactTail := float32(math.Exp(float64(input)))
	approximate := upstream * floor
	exact := upstream * exactTail
	if !(approximate > 0.1 && exact < 0.000_001 && approximate/exact > 100_000) {
		t.Fatalf("planted residue was not materially amplified: approximate=%g exact=%g ratio=%g", approximate, exact, approximate/exact)
	}
	if !math.IsNaN(float64(float32(math.Inf(1)) * float32(0))) {
		t.Fatal("zero-masked exact underflow must retain Inf*0 = NaN semantics")
	}
	if !math.IsInf(float64(float32(math.Inf(1))*floor), 1) {
		t.Fatal("nonzero approximation floor should demonstrate the incorrect Inf result")
	}
}
