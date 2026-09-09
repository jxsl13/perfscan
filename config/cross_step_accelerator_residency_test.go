package config

import (
	"reflect"
	"testing"
)

func TestCrossStepAcceleratorResidencyContractValid(t *testing.T) {
	t.Parallel()
	valid := crossStepAcceleratorResidencyContractForTest()
	if !valid.Valid() {
		t.Fatal("complete cross-step accelerator residency contract is invalid")
	}

	tests := []struct {
		name   string
		mutate func(*CrossStepAcceleratorResidencyContract)
	}{
		{"blank name", func(c *CrossStepAcceleratorResidencyContract) { c.Name = "" }},
		{"padded name", func(c *CrossStepAcceleratorResidencyContract) { c.Name = " padded" }},
		{"bad site", func(c *CrossStepAcceleratorResidencyContract) { c.ConfiguredSite = "train" }},
		{"same callables", func(c *CrossStepAcceleratorResidencyContract) { c.OptimizerCallable = c.ObjectiveCallable }},
		{"colliding objective results", func(c *CrossStepAcceleratorResidencyContract) { c.ObjectiveGradientResult = c.ObjectiveScalarResult }},
		{"colliding error result", func(c *CrossStepAcceleratorResidencyContract) { c.ObjectiveErrorResult = c.ObjectiveScalarResult }},
		{"out-of-range objective result", func(c *CrossStepAcceleratorResidencyContract) { c.ObjectiveErrorResult = 4 }},
		{"bad callback role", func(c *CrossStepAcceleratorResidencyContract) { c.OptimizerGradientCallbackArgument = 2 }},
		{"bad optimizer error role", func(c *CrossStepAcceleratorResidencyContract) { c.OptimizerErrorResult = 2 }},
		{"bad objective error flow", func(c *CrossStepAcceleratorResidencyContract) { c.ObjectiveErrorControlFlow = "panic" }},
		{"bad optimizer error flow", func(c *CrossStepAcceleratorResidencyContract) { c.OptimizerErrorControlFlow = "ignore" }},
		{"non-first scalar observer arg", func(c *CrossStepAcceleratorResidencyContract) { c.ScalarObserverArgument = 2 }},
		{"one iteration", func(c *CrossStepAcceleratorResidencyContract) { c.ConfiguredLoopIterations = 1 }},
		{"zero parameters", func(c *CrossStepAcceleratorResidencyContract) { c.ConfiguredParameterBytes = 0 }},
		{"bad sum", func(c *CrossStepAcceleratorResidencyContract) { c.ConfiguredAvoidableBytes++ }},
		{"missing evidence", func(c *CrossStepAcceleratorResidencyContract) { c.ConfiguredEvidence = "" }},
		{"total overflow", func(c *CrossStepAcceleratorResidencyContract) { c.ConfiguredLoopIterations = 1 << 62 }},
		{"bad target", func(c *CrossStepAcceleratorResidencyContract) { c.TargetGOOS = "Darwin" }},
		{"bad memory", func(c *CrossStepAcceleratorResidencyContract) { c.MemoryModel = "unknown" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			contract := valid
			test.mutate(&contract)
			if contract.Valid() {
				t.Fatal("incomplete cross-step accelerator residency contract accepted")
			}
		})
	}
}

func TestCrossStepAcceleratorResidencyContractRequiresEverySemanticFact(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf(CrossStepAcceleratorResidencyContract{})
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if field.Type.Kind() != reflect.Bool || field.Name == "ExistingResidentSession" || field.Name == "IntentionalHostOptimizer" {
			continue
		}
		index := index
		name := field.Name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contract := crossStepAcceleratorResidencyContractForTest()
			reflect.ValueOf(&contract).Elem().Field(index).SetBool(false)
			if contract.Valid() {
				t.Fatalf("contract accepted with %s=false", name)
			}
		})
	}
}

func TestCompileClonesCrossStepAcceleratorResidencyContracts(t *testing.T) {
	t.Parallel()
	contract := crossStepAcceleratorResidencyContractForTest()
	configuration := Config{CrossStepAcceleratorResidencyContracts: []CrossStepAcceleratorResidencyContract{contract}}
	sets := configuration.Compile()
	if len(sets.CrossStepAcceleratorResidencyContracts) != 1 || sets.CrossStepAcceleratorResidencyContracts[0].Name != contract.Name {
		t.Fatalf("Compile lost cross-step accelerator residency contract: %+v", sets.CrossStepAcceleratorResidencyContracts)
	}
	configuration.CrossStepAcceleratorResidencyContracts[0].Name = "mutated"
	if sets.CrossStepAcceleratorResidencyContracts[0].Name != contract.Name {
		t.Fatal("Compile must clone cross-step accelerator residency contracts")
	}
}

func crossStepAcceleratorResidencyContractForTest() CrossStepAcceleratorResidencyContract {
	return CrossStepAcceleratorResidencyContract{
		Name: "gpt-adamw", ConfiguredSite: "example.com/model.train",
		ObjectiveCallable: "example.com/accelerator.Objective.LossAndGrad", OptimizerCallable: "example.com/optim.AdamW.Step",
		ScalarObserverCallable: "example.com/metrics.Observe", ObjectiveScalarResult: 1, ObjectiveGradientResult: 2,
		ObjectiveErrorResult: 3, OptimizerGradientCallbackArgument: 1, OptimizerErrorResult: 1, ScalarObserverArgument: 1,
		ObjectiveErrorControlFlow: "return", OptimizerErrorControlFlow: "return",
		ConfiguredLoopIterations: 20, ConfiguredParameterBytes: 4096, ConfiguredGradientBytes: 4096,
		ConfiguredAvoidableBytes: 8192, TargetGOOS: "darwin", TargetGOARCH: "arm64", MemoryModel: "unified",
		ConfiguredEvidence:              "goai-pr1202-b86d9a755090df93ed64a30583774f30eefa0c3a",
		ObjectiveReceiverOwnsParameters: true, OptimizerReceiverOwnsParameters: true,
		ReceiverParameterSetsIdentical: true, GradientCallbackMapsReceiverParameter: true,
		DenseGradientPerParameter: true, StableParameterGradientOrder: true, ParameterUploadEveryStep: true,
		GradientHostMaterializedEveryStep: true, HostOptimizerEveryStep: true, ParametersReuploadedNextStep: true,
		OnlyScalarObservedBetweenSteps: true, ScalarResultIsHostMetric: true, ScalarObservationDoesNotSynchronizeState: true,
		NoIntermediateCheckpointRequired: true, ObjectiveAndOptimizerSynchronous: true,
		ObjectiveDoesNotRetainArguments: true, OptimizerDoesNotRetainArguments: true, NoCustomHooks: true,
		NoAliasesOrEscapes: true, NoConcurrentSessionAccess: true, DeviceStorageAndTransfersKnown: true,
		ResidentSessionConstructionSerialized: true,
		ResidentSessionOpportunityConfirmed:   true, ExactDTypeCoverage: true, ExactLayoutCoverage: true,
		ExactOptimizerCoverage: true, LifetimeParity: true, NumericalParity: true, CheckpointParity: true,
		ErrorAndPanicParity: true, MutationParity: true, OwnershipParity: true, AutogradParity: true,
		BackendSelectionParity: true, ExplicitSyncRequired: true, ExplicitCheckpointRequired: true,
		PairedEndToEndValidationRequired: true,
	}
}
