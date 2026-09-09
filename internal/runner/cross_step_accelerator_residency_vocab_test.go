package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func TestMissingVocabCrossStepAcceleratorResidencyContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"crossStepAcceleratorResidencyContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "crossStepAcceleratorResidencyContracts" {
		t.Fatalf("missingVocab(empty) = %v, want crossStepAcceleratorResidencyContracts", got)
	}
	invalid := config.Config{CrossStepAcceleratorResidencyContracts: []config.CrossStepAcceleratorResidencyContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("missingVocab(invalid) = %v, want crossStepAcceleratorResidencyContracts", got)
	}
	configured := config.Config{CrossStepAcceleratorResidencyContracts: []config.CrossStepAcceleratorResidencyContract{crossStepAcceleratorResidencyContractForRunnerTest()}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(valid) = %v, want none", got)
	}
}

func crossStepAcceleratorResidencyContractForRunnerTest() config.CrossStepAcceleratorResidencyContract {
	return config.CrossStepAcceleratorResidencyContract{
		Name: "gpt-adamw", ConfiguredSite: "example.com/model.train",
		ObjectiveCallable: "example.com/accelerator.Objective.LossAndGrad", OptimizerCallable: "example.com/optim.AdamW.Step",
		ScalarObserverCallable: "example.com/metrics.Observe", ObjectiveScalarResult: 1, ObjectiveGradientResult: 2,
		ObjectiveErrorResult: 3, OptimizerGradientCallbackArgument: 1, OptimizerErrorResult: 1,
		ScalarObserverArgument: 1, ObjectiveErrorControlFlow: "return", OptimizerErrorControlFlow: "return",
		ConfiguredLoopIterations: 4, ConfiguredParameterBytes: 4096,
		ConfiguredGradientBytes: 4096, ConfiguredAvoidableBytes: 8192, TargetGOOS: "darwin", TargetGOARCH: "arm64",
		MemoryModel: "unified", ConfiguredEvidence: "owner-evidence",
		ObjectiveReceiverOwnsParameters: true, OptimizerReceiverOwnsParameters: true,
		ReceiverParameterSetsIdentical: true, GradientCallbackMapsReceiverParameter: true,
		DenseGradientPerParameter: true, StableParameterGradientOrder: true,
		ParameterUploadEveryStep: true, GradientHostMaterializedEveryStep: true, HostOptimizerEveryStep: true,
		ParametersReuploadedNextStep: true, OnlyScalarObservedBetweenSteps: true, ScalarResultIsHostMetric: true,
		ScalarObservationDoesNotSynchronizeState: true, NoIntermediateCheckpointRequired: true,
		ObjectiveAndOptimizerSynchronous: true, ObjectiveDoesNotRetainArguments: true,
		OptimizerDoesNotRetainArguments: true, NoCustomHooks: true, NoAliasesOrEscapes: true,
		NoConcurrentSessionAccess: true, DeviceStorageAndTransfersKnown: true, ResidentSessionOpportunityConfirmed: true,
		ResidentSessionConstructionSerialized: true,
		ExactDTypeCoverage:                    true, ExactLayoutCoverage: true,
		ExactOptimizerCoverage: true, LifetimeParity: true, NumericalParity: true, CheckpointParity: true,
		ErrorAndPanicParity: true, MutationParity: true, OwnershipParity: true, AutogradParity: true,
		BackendSelectionParity: true, ExplicitSyncRequired: true, ExplicitCheckpointRequired: true,
		PairedEndToEndValidationRequired: true,
	}
}
