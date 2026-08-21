package ps6021

import (
	"slices"
	"testing"
)

type FusedGEMMGate struct {
	ExactOutputRequired         bool
	ExpandedOperandStorageExact bool
	FiniteOutput                bool
	SegmentNRMECeiling          float64
	TrainedSemanticParity       bool
}

func TestMPSF16FusedWideGEMMExactOutput(t *testing.T) {
	// Three MPS f16 projections are concatenated into one wider result shape.
	separateOutput := []float32{1, 2}
	wideOutput := []float32{1, 2}
	if !slices.Equal(separateOutput, wideOutput) { // want "fused floating-point vendor GEMM shape rewrite makes bit-exact output a mandatory promotion gate; missing replacement evidence: exact operand/expanded-storage bytes, finite bounded numerical error, end-to-end semantic/quality gate"
		t.Fatal("MPS fused GEMM output must be bit exact")
	}
}

func TestMetalF16GroupedProjectionManifest(t *testing.T) {
	// Exact expanded operand storage, finite segment NRMSE, and trained semantic
	// parity are present, but exact output is still incorrectly mandatory for
	// the wider vendor projection shape.
	gate := FusedGEMMGate{
		ExactOutputRequired:         true, // want "fused floating-point vendor GEMM shape rewrite makes bit-exact output a mandatory promotion gate; exact output is not a generic shape-fusion invariant"
		ExpandedOperandStorageExact: true,
		FiniteOutput:                true,
		SegmentNRMECeiling:          1e-3,
		TrainedSemanticParity:       true,
	}
	_ = gate
	_ = t
}

func TestMetalF16GroupedProjectionBoundedGate(t *testing.T) {
	// Wider combined MPS GEMM shape with the valid separated evidence model.
	gate := FusedGEMMGate{
		ExactOutputRequired:         false,
		ExpandedOperandStorageExact: true,
		FiniteOutput:                true,
		SegmentNRMECeiling:          1e-3,
		TrainedSemanticParity:       true,
	}
	_ = gate
	_ = t
}

func TestMetalF16GroupedProjectionStorageExact(t *testing.T) {
	// Exact expanded weight storage is valid and is not an exact OUTPUT gate.
	expandedWeights := []uint16{1, 2}
	wantWeights := []uint16{1, 2}
	if !slices.Equal(expandedWeights, wantWeights) {
		t.Fatal("expanded f16 storage differs")
	}
}

func TestStandaloneF16ProjectionExactOutput(t *testing.T) {
	// No fused/grouped result-shape rewrite: ordinary exact contract.
	gotOutput, wantOutput := []float32{1}, []float32{1}
	if !slices.Equal(gotOutput, wantOutput) {
		t.Fatal("output differs")
	}
}

func TestCustomF16FusedWideGEMMExactOutput(t *testing.T) {
	// custom fixed reduction order: shape-independent and documented.
	gotOutput, wantOutput := []float32{1}, []float32{1}
	if !slices.Equal(gotOutput, wantOutput) {
		t.Fatal("output differs")
	}
}
