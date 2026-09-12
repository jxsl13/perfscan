package config

import (
	"strings"
	"testing"
)

func dispatchCrossoverFixture() DispatchCrossoverContract {
	return DispatchCrossoverContract{DispatchFunction: "example/cpu.dispatch", ThresholdConstant: "example/cpu.boundary", SerialPolicy: "example/cpu.leaf", ParallelPolicy: "example/cpu.parallel", WorkerRunner: "example/cpu.parallel", LeafKernels: []string{"example/cpu.nativeLeaf"}, OperationInputFactory: "example/cpu.inputs", DiagnosticFunction: "example/cpu.TestDiagnostic", ProductionBenchmark: "BenchmarkAbs/n{n}", Campaign: "/retained/campaign", PlanSHA256: strings.Repeat("a", 64), RecordsSHA256: strings.Repeat("b", 64)}
}

func TestDispatchCrossoverContractsDeepCloneAndQualification(t *testing.T) {
	t.Parallel()
	c := dispatchCrossoverFixture()
	if !c.Valid() || UsableDispatchCrossoverContractCount([]DispatchCrossoverContract{c}) != 1 {
		t.Fatal("actual Tensor route must not require invented operation wrappers")
	}
	copy := cloneDispatchCrossoverContracts([]DispatchCrossoverContract{c})
	copy[0].LeafKernels[0] = "example/cpu.changed"
	if c.LeafKernels[0] != "example/cpu.nativeLeaf" || cloneDispatchCrossoverContracts(nil) != nil {
		t.Fatal("clone aliases source config or loses nil semantics")
	}
	compiled := (Config{DispatchCrossoverContracts: []DispatchCrossoverContract{c}}).Compile()
	compiled.DispatchCrossoverContracts[0].LeafKernels[0] = "example/cpu.changed"
	if c.LeafKernels[0] != "example/cpu.nativeLeaf" {
		t.Fatal("compiled config aliases mutable crossover selectors")
	}
	if UsableDispatchCrossoverContractCount([]DispatchCrossoverContract{c, c}) != 0 {
		t.Fatal("ambiguous duplicate dispatch contracts qualified")
	}
	for _, tc := range []struct {
		name string
		edit func(*DispatchCrossoverContract)
	}{
		{"missing_leaf", func(c *DispatchCrossoverContract) { c.LeafKernels = nil }},
		{"missing_factory", func(c *DispatchCrossoverContract) { c.OperationInputFactory = "" }},
		{"missing_plan_pin", func(c *DispatchCrossoverContract) { c.PlanSHA256 = "" }},
		{"nonhex_records_pin", func(c *DispatchCrossoverContract) { c.RecordsSHA256 = strings.Repeat("z", 64) }},
		{"invalid_wrapper_selector", func(c *DispatchCrossoverContract) { c.SerialOperation = "not a selector" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			changed := dispatchCrossoverFixture()
			tc.edit(&changed)
			if UsableDispatchCrossoverContractCount([]DispatchCrossoverContract{changed}) != 0 {
				t.Fatal("incomplete or ambiguous evidence configuration qualified")
			}
		})
	}
}
