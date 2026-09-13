package config

import "testing"

func TestSingleUseQuantizationCompileClonesEvidence(t *testing.T) {
	t.Parallel()
	c := Config{SingleUseQuantizationContracts: []SingleUseQuantizationContract{{Quantizer: "fixture.pack", Consumer: "fixture.dot", BenchmarkExemptions: []SingleUseQuantizationExemption{{Policy: "source.json"}}}}}
	compiled := c.Compile()
	c.SingleUseQuantizationContracts[0].Quantizer = "changed"
	c.SingleUseQuantizationContracts[0].BenchmarkExemptions[0].Policy = "changed"
	if len(compiled.SingleUseQuantizationContracts) != 1 || compiled.SingleUseQuantizationContracts[0].Quantizer != "fixture.pack" || compiled.SingleUseQuantizationContracts[0].BenchmarkExemptions[0].Policy != "source.json" {
		t.Fatalf("compiled source/evidence aliases mutable config: %+v", compiled.SingleUseQuantizationContracts)
	}
}
