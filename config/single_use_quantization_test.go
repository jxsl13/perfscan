package config

import (
	"strings"
	"testing"
)

func TestSingleUseQuantizationContractBoundaries(t *testing.T) {
	t.Parallel()
	base := SingleUseQuantizationContract{Quantizer: "fixture.pack", Consumer: "fixture.dot", PackedArgument: 0, RowsArgument: 1, QuantizationAndDotMeaningReviewed: true}
	if !base.Valid() {
		t.Fatal("complete contract rejected")
	}
	for _, mutate := range []func(*SingleUseQuantizationContract){
		func(c *SingleUseQuantizationContract) { c.FloatInputArgument = 1 },
		func(c *SingleUseQuantizationContract) { c.PackedArgument = -1 },
		func(c *SingleUseQuantizationContract) { c.RowsArgument = 2 },
		func(c *SingleUseQuantizationContract) { c.RowsArgument = c.PackedArgument },
		func(c *SingleUseQuantizationContract) { c.Consumer = c.Quantizer },
		func(c *SingleUseQuantizationContract) { c.BenchmarkExemptOwners = []string{"fixture.owner"} },
	} {
		c := base
		mutate(&c)
		if c.Valid() {
			t.Fatal("incomplete/ambiguous contract accepted")
		}
	}
	base.BenchmarkExemptOwners = []string{"fixture.owner"}
	base.BenchmarkExemptionReason = "reviewed campaign"
	if base.Valid() {
		t.Fatal("unproved owner-wide benchmark exemption accepted")
	}
	cloned := cloneSingleUseQuantizationContracts([]SingleUseQuantizationContract{base})
	cloned[0].BenchmarkExemptOwners[0] = "fixture.other"
	if base.BenchmarkExemptOwners[0] != "fixture.owner" {
		t.Fatal("cloned exemptions share storage")
	}
}

func TestSingleUseQuantizationPinnedPolicyReference(t *testing.T) {
	t.Parallel()
	c := SingleUseQuantizationContract{Quantizer: "fixture.pack", Consumer: "fixture.dot", ConsumerForm: "twoInputDot", PackedArgument: 0, WeightArgument: 1, RowsArgument: -1, QuantizationAndDotMeaningReviewed: true, BenchmarkExemptions: []SingleUseQuantizationExemption{{Policy: "reviewed.json", SHA256: strings.Repeat("a", 64)}}}
	if !c.Valid() {
		t.Fatal("pinned reference rejected")
	}
	cloned := cloneSingleUseQuantizationContracts([]SingleUseQuantizationContract{c})
	cloned[0].BenchmarkExemptions[0].Policy = "other.json"
	if c.BenchmarkExemptions[0].Policy != "reviewed.json" {
		t.Fatal("reference clone shares storage")
	}
	for _, edit := range []func(*SingleUseQuantizationContract){
		func(c *SingleUseQuantizationContract) { c.BenchmarkExemptions[0].Policy = "" },
		func(c *SingleUseQuantizationContract) { c.BenchmarkExemptions[0].SHA256 = "bad" },
		func(c *SingleUseQuantizationContract) { c.BenchmarkExemptions[0].SHA256 = strings.Repeat("z", 64) },
		func(c *SingleUseQuantizationContract) {
			c.BenchmarkExemptions = append(c.BenchmarkExemptions, c.BenchmarkExemptions[0])
		},
	} {
		current := cloneSingleUseQuantizationContracts([]SingleUseQuantizationContract{c})[0]
		edit(&current)
		if current.Valid() {
			t.Fatal("invalid reference accepted")
		}
	}
}

func TestSingleUseQuantizationTwoInputRoleBoundaries(t *testing.T) {
	t.Parallel()
	base := SingleUseQuantizationContract{Quantizer: "fixture.pack", Consumer: "fixture.dot", ConsumerForm: "twoInputDot", PackedArgument: 0, WeightArgument: 1, RowsArgument: -1, QuantizationAndDotMeaningReviewed: true}
	if !base.Valid() {
		t.Fatal("complete two-input roles rejected")
	}
	for _, edit := range []func(*SingleUseQuantizationContract){
		func(c *SingleUseQuantizationContract) { c.WeightArgument = c.PackedArgument },
		func(c *SingleUseQuantizationContract) { c.WeightArgument = -1 },
		func(c *SingleUseQuantizationContract) { c.WeightArgument = 2 },
		func(c *SingleUseQuantizationContract) { c.RowsArgument = 0 },
		func(c *SingleUseQuantizationContract) { c.ConsumerForm = "opaqueMatmul" },
		func(c *SingleUseQuantizationContract) { c.ConsumerForm = "selfDot" },
	} {
		c := base
		edit(&c)
		if c.Valid() {
			t.Fatal("ambiguous or unsupported consumer form accepted")
		}
	}
}

func TestSingleUseQuantizationSourceMethodRoles(t *testing.T) {
	t.Parallel()
	base := SingleUseQuantizationContract{Quantizer: "fixture.pack", Consumer: "fixture.Kernel.Dot", ConsumerForm: "sourceSummary", PackedArgument: 0, WeightArgument: -1, RowsArgument: -1, QuantizationAndDotMeaningReviewed: true}
	if !base.Valid() {
		t.Fatal("source method roles rejected")
	}
	for _, edit := range []func(*SingleUseQuantizationContract){
		func(c *SingleUseQuantizationContract) { c.PackedArgument = 1 },
		func(c *SingleUseQuantizationContract) { c.RowsArgument = 0 },
		func(c *SingleUseQuantizationContract) { c.ConsumerForm = "twoInputDot" },
		func(c *SingleUseQuantizationContract) { c.ConsumerForm = "packedByteDot" },
		func(c *SingleUseQuantizationContract) { c.QuantizationAndDotMeaningReviewed = false },
	} {
		c := base
		edit(&c)
		if c.Valid() {
			t.Fatal("unproved or ambiguous method roles accepted")
		}
	}
}
