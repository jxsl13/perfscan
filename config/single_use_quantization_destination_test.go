package config

import "testing"

func TestSingleUseQuantizationDestinationContract(t *testing.T) {
	t.Parallel()
	c := SingleUseQuantizationContract{Quantizer: "fixture.fill", Consumer: "fixture.dot", ProducerForm: "destination", DestinationArgument: 0, FloatInputArgument: 1, ConsumerForm: "sourceSummary", PackedArgument: 0, WeightArgument: 1, RowsArgument: -1, QuantizationAndDotMeaningReviewed: true}
	if !c.Valid() {
		t.Fatal("valid destination contract rejected")
	}
	for _, mutate := range []func(*SingleUseQuantizationContract){
		func(c *SingleUseQuantizationContract) { c.DestinationArgument = 1 },
		func(c *SingleUseQuantizationContract) { c.FloatInputArgument = 2 },
		func(c *SingleUseQuantizationContract) { c.ProducerForm = "opaque" },
		func(c *SingleUseQuantizationContract) { c.ConsumerForm = "twoInputDot" },
	} {
		bad := c
		mutate(&bad)
		if bad.Valid() {
			t.Fatalf("invalid contract accepted: %+v", bad)
		}
	}
}
