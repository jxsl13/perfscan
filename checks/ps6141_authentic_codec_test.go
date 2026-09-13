package checks

import (
	"go/types"
	"os"
	"strings"
	"testing"
)

// This source is an authentic model-weight producer, not an observed activation
// boundary. Keep the coverage gap explicit until byte-block summaries land.
func TestPS6141AuthenticQ8CodecCoverageGap(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/ps6141_authentic_q8_0.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "7954b0e8f28b029d895b12a3891b3858842826bc") {
		t.Fatal("missing source provenance")
	}
	pass, _ := ps6141TypedFixture(t, source)
	declarations := ps6099LocalFunctionDeclarations(pass)
	var producer *types.Func
	for fn := range declarations {
		if fn.Name() == "quantizeQ8_0" {
			producer = fn
		}
	}
	if producer == nil {
		t.Fatal("authentic typed producer prerequisite missing")
	}
	layout := ps6141ByteBlocks(pass, declarations[producer])
	if !layout.established || !layout.freshOutput || !layout.readOnlyViews || layout.floatExtent != 32 || layout.byteStride != 34 || layout.metadataWidth != 2 {
		t.Fatalf("authentic layout prerequisite: %+v", layout)
	}
	if layout.allocationArithmeticKnown || layout.scalarEffectsKnown || layout.completionKnown {
		t.Fatal("unresolved authentic obligations became known")
	}
	sig := producer.Type().(*types.Signature)
	if sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		t.Fatal("unexpected authentic signature")
	}
	index := &ps6141SummaryIndex{pass: pass, declarations: declarations, memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
	if index.function(producer).valid {
		t.Fatal("coverage baseline changed: replace gap assertion with positive byte-block source proof and negative controls")
	}
}
