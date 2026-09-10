package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func TestMissingVocabSelectorPromotionSymbols(t *testing.T) {
	t.Parallel()
	check := &lint.Check{
		NeedsConfig: true,
		Vocab:       []string{"selectorPromotionSymbols"},
	}
	empty := config.Config{}
	if got := missingVocab(check, &empty); len(got) != 1 || got[0] != "selectorPromotionSymbols" {
		t.Fatalf("missingVocab(empty) = %v, want selectorPromotionSymbols", got)
	}
	configured := config.Config{SelectorPromotionSymbols: []string{"useSplitK"}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(configured) = %v, want none", got)
	}
}

func TestMissingVocabInPlaceFusionContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"inPlaceFusionContracts"}}
	empty := config.Config{}
	if got := missingVocab(check, &empty); len(got) != 1 || got[0] != "inPlaceFusionContracts" {
		t.Fatalf("missingVocab(empty) = %v, want inPlaceFusionContracts", got)
	}
	configured := config.Config{InPlaceFusionContracts: []config.InPlaceFusionContract{{Name: "swiglu"}}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(configured) = %v, want none", got)
	}
}

func TestMissingVocabReceiverStagingContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"receiverStagingContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "receiverStagingContracts" {
		t.Fatalf("missingVocab(empty) = %v, want receiverStagingContracts", got)
	}
	invalid := config.Config{ReceiverStagingContracts: []config.ReceiverStagingContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 || got[0] != "receiverStagingContracts" {
		t.Fatalf("missingVocab(invalid-only) = %v, want receiverStagingContracts", got)
	}
	valid := config.ReceiverStagingContract{
		Name:                             "inline",
		CandidateMethod:                  "example.com/project.Decoder.StepN",
		Consumer:                         "example.com/project.upload",
		ConsumerKind:                     config.ReceiverStagingCallFunction,
		LifecycleMethod:                  "example.com/project.Decoder.Release",
		MaxRetainedBytes:                 64 << 10,
		ReceiverCallsSequential:          true,
		ConsumerCompletesBeforeReturn:    true,
		ConsumerDoesNotRetainArgument:    true,
		LifecycleEndsReceiverUse:         true,
		ContentsMayPersistUntilLifecycle: true,
	}
	configured := config.Config{ReceiverStagingContracts: []config.ReceiverStagingContract{valid}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(valid) = %v, want none", got)
	}
}

func TestMissingVocabTopKOneContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"topKOneContracts"}}
	empty := config.Config{}
	if got := missingVocab(check, &empty); len(got) != 1 || got[0] != "topKOneContracts" {
		t.Fatalf("missingVocab(empty) = %v, want topKOneContracts", got)
	}
	for _, contract := range []config.TopKOneContract{
		{Name: "invalid import path", Function: "example.com/a:bad.TopKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "ambiguous without kind", Function: "example.com/project.Buffer.TopKN", KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "blank function", Function: "example.com/project._", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "blank receiver", Function: "example.com/project._.TopKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "blank method", Function: "example.com/project.Buffer._", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
	} {
		contract := contract
		t.Run(contract.Name, func(t *testing.T) {
			t.Parallel()
			invalid := config.Config{TopKOneContracts: []config.TopKOneContract{contract}}
			if got := missingVocab(check, &invalid); len(got) != 1 || got[0] != "topKOneContracts" {
				t.Fatalf("missingVocab(invalid-only) = %v, want topKOneContracts", got)
			}
		})
	}
	configured := config.Config{TopKOneContracts: []config.TopKOneContract{
		{Name: "blank sibling", Function: "example.com/project._", Kind: config.TopKOneContractFunction, KArgPosition: 2, IndicesResultPosition: 1},
		{Name: "resident", Function: "example.com/dotted.pkg.Búffer.TópKN", Kind: config.TopKOneContractMethod, KArgPosition: 2, IndicesResultPosition: 1},
	}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(mixed valid/invalid) = %v, want none", got)
	}
}

func TestMissingVocabNativeSnapshotStringCopyContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"nativeSnapshotStringCopyContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "nativeSnapshotStringCopyContracts" {
		t.Fatalf("missingVocab(empty) = %v, want nativeSnapshotStringCopyContracts", got)
	}
	invalid := config.Config{NativeSnapshotStringCopyContracts: []config.NativeSnapshotStringCopyContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 || got[0] != "nativeSnapshotStringCopyContracts" {
		t.Fatalf("missingVocab(invalid) = %v, want nativeSnapshotStringCopyContracts", got)
	}
	configured := config.Config{NativeSnapshotStringCopyContracts: []config.NativeSnapshotStringCopyContract{{
		Name:                                 "labels",
		CandidateCallable:                    "example.com/project.Recorder.Profile",
		CandidateKind:                        config.NativeSnapshotCallMethod,
		DestinationResultPosition:            1,
		DestinationStringField:               "Label",
		AcquireCallable:                      "C.snapshot",
		AcquireKind:                          config.NativeSnapshotCallCgo,
		RecordsOutArgumentPosition:           1,
		CountOutArgumentPosition:             2,
		AcquireStatusResultPosition:          1,
		NativeStringField:                    "label",
		CopyKind:                             config.NativeSnapshotCopyGoString,
		CopyPointerArgumentPosition:          1,
		CopyStringResultPosition:             1,
		LifecycleCallable:                    "example.com/project.Recorder.Free",
		LifecycleKind:                        config.NativeSnapshotCallMethod,
		SnapshotStableThroughCandidateReturn: true,
		SnapshotNotMutatedDuringExtraction:   true,
		ExtractionIsSynchronous:              true,
		CopyReturnsExactOwnedString:          true,
		ReturnedStringsOutliveLifecycle:      true,
		ExactContentCheckRequired:            true,
	}}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(configured) = %v, want none", got)
	}
}

func TestMissingVocabReusableResultLoopContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"reusableResultLoopContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "reusableResultLoopContracts" {
		t.Fatalf("missingVocab(empty) = %v, want reusableResultLoopContracts", got)
	}
	invalid := config.Config{ReusableResultLoopContracts: []config.ReusableResultLoopContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 || got[0] != "reusableResultLoopContracts" {
		t.Fatalf("missingVocab(invalid) = %v, want reusableResultLoopContracts", got)
	}
	valid := config.ReusableResultLoopContract{
		Name:                     "decode",
		Wrapper:                  "example.com/project.Decoder.Step",
		Into:                     "example.com/project.Decoder.StepInto",
		ResultPosition:           1,
		DestinationArgument:      3,
		WrapperReturnsFreshOwned: true,
		ResultLengthStableForReceiverAndListedArgs:     true,
		IntoOverwritesDestinationOnSuccess:             true,
		IntoDoesNotReadDestinationBeforeOverwrite:      true,
		IntoIgnoresDestinationIdentityAndExtraCapacity: true,
		IntoDoesNotRetainDestination:                   true,
		IntoExecutesSynchronously:                      true,
		WrapperAndIntoHaveIdenticalStateEffects:        true,
		WrapperAndIntoHaveIdenticalErrorsAndPanics:     true,
	}
	configured := config.Config{ReusableResultLoopContracts: []config.ReusableResultLoopContract{valid}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(valid) = %v, want none", got)
	}
}

func TestMissingVocabRecorderResidualAddContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"recorderResidualAddContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "recorderResidualAddContracts" {
		t.Fatalf("missingVocab(empty) = %v, want recorderResidualAddContracts", got)
	}
	invalid := config.Config{RecorderResidualAddContracts: []config.RecorderResidualAddContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 || got[0] != "recorderResidualAddContracts" {
		t.Fatalf("missingVocab(invalid) = %v, want recorderResidualAddContracts", got)
	}
	valid := config.RecorderResidualAddContract{
		Name: "residual", Projection: "example.com/p.Linear.record", Accumulate: "example.com/p.Linear.recordAdd",
		RecorderBinary: "example.com/p.Recorder.Binary", AddOperation: "example.com/p.binaryAdd", AddOperationValue: "1",
		ProjectionRecorderArgument: 1, ProjectionSourceArgument: 2, ProjectionTemporaryArgument: 3,
		BinaryDestinationArgument: 1, BinaryTemporaryArgument: 2, BinaryOutputArgument: 3, BinaryOperationArgument: 4,
		AccumulateRecorderArgument: 1, AccumulateSourceArgument: 2, AccumulateTemporaryArgument: 3, AccumulateDestinationArgument: 4,
		ProjectionOverwritesTemporary: true, TemporaryMayServeAsAccumulateScratch: true, MatchedBuffersDoNotAlias: true,
		TemporaryUnobservedOutsideMatchedCalls: true, CallsDoNotRetainArguments: true, CallsExecuteSynchronously: true,
		RecorderOrderPreserved: true, AccumulateMatchesProjectionAndResidualAdd: true, AccumulatePreservesErrorsAndPanics: true,
		AccumulatePreservesPartialOutput: true, AccumulatePreservesArithmeticPolicy: true, AccumulateSupportsConfiguredDTypesLayoutsBackends: true,
	}
	configured := config.Config{RecorderResidualAddContracts: []config.RecorderResidualAddContract{valid}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(configured) = %v, want none", got)
	}
}

func TestMissingVocabRowLocalSparseGatherContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"rowLocalSparseGatherContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "rowLocalSparseGatherContracts" {
		t.Fatalf("missingVocab(empty) = %v, want rowLocalSparseGatherContracts", got)
	}
	invalid := config.Config{RowLocalSparseGatherContracts: []config.RowLocalSparseGatherContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("missingVocab(invalid) = %v, want rowLocalSparseGatherContracts", got)
	}
	configured := config.Config{RowLocalSparseGatherContracts: []config.RowLocalSparseGatherContract{rowLocalSparseGatherContractForRunnerTest()}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(valid) = %v, want none", got)
	}
}

func TestMissingVocabTinySynchronousAcceleratorScreenContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"tinySynchronousAcceleratorScreenContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "tinySynchronousAcceleratorScreenContracts" {
		t.Fatalf("missingVocab(empty) = %v, want tinySynchronousAcceleratorScreenContracts", got)
	}
	invalid := config.Config{TinySynchronousAcceleratorScreenContracts: []config.TinySynchronousAcceleratorScreenContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("missingVocab(invalid) = %v, want tinySynchronousAcceleratorScreenContracts", got)
	}
	configured := config.Config{TinySynchronousAcceleratorScreenContracts: []config.TinySynchronousAcceleratorScreenContract{tinySynchronousAcceleratorScreenContractForRunnerTest()}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(valid) = %v, want none", got)
	}
}

func TestMissingVocabResidentWeightBenchmarkContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"residentWeightBenchmarkContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 {
		t.Fatalf("empty vocabulary reported %v", got)
	}
	invalid := config.Config{ResidentWeightBenchmarkContracts: []config.ResidentWeightBenchmarkContract{{AcceleratorCallable: "Launch", WeightArgument: 2}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("invalid vocabulary reported %v", got)
	}
	valid := config.Config{ResidentWeightBenchmarkContracts: []config.ResidentWeightBenchmarkContract{{AcceleratorCallable: "example.com/backend.Device.Launch", WeightArgument: 2}}}
	if got := missingVocab(check, &valid); len(got) != 0 {
		t.Fatalf("valid vocabulary reported %v", got)
	}
}

func TestMissingVocabForwardLossBackwardGraphContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"forwardLossBackwardGraphContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "forwardLossBackwardGraphContracts" {
		t.Fatalf("missingVocab(empty) = %v, want forwardLossBackwardGraphContracts", got)
	}
	invalid := config.Config{ForwardLossBackwardGraphContracts: []config.ForwardLossBackwardGraphContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("missingVocab(invalid) = %v, want forwardLossBackwardGraphContracts", got)
	}
	configured := config.Config{ForwardLossBackwardGraphContracts: []config.ForwardLossBackwardGraphContract{forwardLossBackwardGraphContractForRunnerTest()}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(valid) = %v, want none", got)
	}
}

func forwardLossBackwardGraphContractForRunnerTest() config.ForwardLossBackwardGraphContract {
	return config.ForwardLossBackwardGraphContract{
		Name: "vit", ObjectiveSite: "example.com/vision.ViT.LossAndGrad",
		RecorderFactoryCallable: "example.com/autograd.NewTape", RecorderBindingCallable: "example.com/backend.Context.WithRecorder",
		ForwardCallable: "example.com/vision.ViT.Forward", LossCallable: "example.com/nn.CrossEntropy",
		BackwardCallable: "example.com/autograd.Tape.Backward", ParameterOrderCallable: "example.com/vision.ViT.Params",
		GradientCallable: "example.com/autograd.Tape.Grad", RecorderFactoryBackendArgument: 1,
		RecorderBindingArgument: 1, ForwardRecorderArgument: 1,
		LossRecorderArgument: 1, LossForwardArgument: 2, BackwardLossArgument: 1, GradientParameterArgument: 1,
		ConfiguredGeometry: "B=8,S=65,D=128", ConfiguredGradientCount: 56,
		ConfiguredCurrentSubmissionCount: 3, ConfiguredCandidateSubmissions: 1, MaxCacheEntries: 4,
		ConfiguredEvidence:   "paired-owner-campaign",
		RecreatesForwardWork: true, StableGeometry: true, ExactScalarLossReduction: true,
		StableCompleteGradientOrder: true, PrivateRecorderOwnership: true, CustomHooksExcluded: true,
		MutationExcluded: true, DTypeLayoutBackendConstrained: true, CacheKeyCoversGeometry: true,
		CacheKeyCoversDTypeLayoutObjective: true, BoundedCache: true, PortableFallbackPreserved: true,
		ForwardParity: true, ScalarLossParity: true, AllGradientParity: true, InputAndParameterImmutability: true,
		ErrorAndPanicParity: true, RecorderIsolation: true, PerOperationAndLayerRoutesExcluded: true,
		PrivateTapePreservesBackendRoutes: true, BackendSelectionParity: true, PairedNumericalValidationRequired: true,
		PairedEndToEndValidationRequired: true,
	}
}

func TestMissingVocabFragmentedAcceleratorObjectiveContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"fragmentedAcceleratorObjectiveContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "fragmentedAcceleratorObjectiveContracts" {
		t.Fatalf("missingVocab(empty) = %v, want fragmentedAcceleratorObjectiveContracts", got)
	}
	invalid := config.Config{FragmentedAcceleratorObjectiveContracts: []config.FragmentedAcceleratorObjectiveContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("missingVocab(invalid) = %v, want fragmentedAcceleratorObjectiveContracts", got)
	}
	configured := config.Config{FragmentedAcceleratorObjectiveContracts: []config.FragmentedAcceleratorObjectiveContract{fragmentedAcceleratorObjectiveContractForRunnerTest()}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(valid) = %v, want none", got)
	}
	for _, test := range []struct {
		name string
		edit func(*config.FragmentedAcceleratorObjectiveContract)
	}{
		{"per-operation route preservation", func(c *config.FragmentedAcceleratorObjectiveContract) {
			c.PerOperationBackendRoutePreservationRequired = false
		}},
		{"true causal mask", func(c *config.FragmentedAcceleratorObjectiveContract) { c.TrueCausalMaskSemanticsRequired = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			contract := fragmentedAcceleratorObjectiveContractForRunnerTest()
			test.edit(&contract)
			candidate := config.Config{FragmentedAcceleratorObjectiveContracts: []config.FragmentedAcceleratorObjectiveContract{contract}}
			if got := missingVocab(check, &candidate); len(got) != 1 || got[0] != "fragmentedAcceleratorObjectiveContracts" {
				t.Fatalf("missingVocab(incomplete gate) = %v, want fragmentedAcceleratorObjectiveContracts", got)
			}
		})
	}
}

func fragmentedAcceleratorObjectiveContractForRunnerTest() config.FragmentedAcceleratorObjectiveContract {
	return config.FragmentedAcceleratorObjectiveContract{
		Name: "gpt-objective", ObjectiveSite: "example.com/model.lossAndGrad",
		Calls: []config.FragmentedAcceleratorObjectiveCall{
			{ConfiguredSite: "example.com/model.lossAndGrad", AcceleratorCallable: "example.com/backend.Execute", OperationArgument: 1, OperationConstant: "example.com/backend.OpForward", OperationConstantValue: "1", ExpectedOccurrences: 3},
			{ConfiguredSite: "example.com/model.lossAndGrad", AcceleratorCallable: "example.com/backend.Execute", OperationArgument: 1, OperationConstant: "example.com/backend.OpBackward", OperationConstantValue: "2", ExpectedOccurrences: 2},
		},
		ConfiguredAcceleratorCallCount: 5, ConfiguredSynchronousBoundaryCount: 5, CandidateSubmissionCount: 1,
		ScalarObjectiveResult: 1, ParameterGradientsResult: 2, ErrorResult: 3,
		GeometryCacheKey: "batch,sequence,width,depth,dtype,layout", ConfiguredEvidence: "paired-owner-campaign",
		CompleteForwardLossReverseMode: true, StableObjectiveGeometry: true, GeometryCacheKeyComplete: true,
		CacheReusedAcrossObjectives: true, EachAcceleratorCallSubmits: true, EachAcceleratorCallSynchronizesBeforeReturn: true,
		CandidateReturnsScalarAndAllParameterGradients: true, NoOtherMaterializedResults: true,
		NoDynamicDispatch: true, NoCustomHooks: true, InputsAndParametersImmutable: true, OutputsDoNotAliasInputs: true,
		NoPreexistingDeviceResidency: true, NoPostObjectiveDeviceResidency: true, ExactDTypeLayoutAttributesReduction: true,
		FloatingPointParityRequired: true, ErrorPanicParityRequired: true, RecorderAutogradBackendParityRequired: true,
		PerOperationBackendRoutePreservationRequired: true, TrueCausalMaskSemanticsRequired: true,
		PairedApplicationBenchmarkRequired: true, NumericalValidationRequired: true,
		ShapeAwareEmbeddingGradientValidationRequired: true, RepeatedIndexGradientParityRequired: true, ScatterNDNotAssumedFaster: true,
	}
}

func tinySynchronousAcceleratorScreenContractForRunnerTest() config.TinySynchronousAcceleratorScreenContract {
	return config.TinySynchronousAcceleratorScreenContract{
		Name: "tiny-loss",
		Calls: []config.TinySynchronousAcceleratorScreenCall{{
			ConfiguredSite: "example.com/model.loss", AcceleratorCallable: "example.com/backend.Metal.CrossEntropy",
			HostAlternativeCallable: "example.com/backend.hostCrossEntropy", RowsArgument: 1, ColumnsArgument: 2,
		}},
		ConfiguredRows: 8, ConfiguredColumns: 10, ConfiguredWorkingSetBytes: 640,
		MaxElements: 80, MaxWorkingSetBytes: 640, ConfiguredSubmissionCount: 1,
		TargetGOOS: "darwin", TargetGOARCH: "arm64", MemoryModel: "unified",
		BlockingCompletion: true, HostAccessibleInputs: true, HostAccessibleOutput: true, NoTransferRequired: true,
		NoPreexistingDeviceResidency: true, NoPostDeviceResidency: true, NoGraphContext: true, NoRecorderContext: true,
		NoCommandBufferContext: true, ExactDTypeCoverage: true, ExactLayoutCoverage: true, ExactAttributesCoverage: true,
		ExactReductionCoverage: true, ForwardParity: true, GradientParity: true, FloatingPointParity: true, ErrorParity: true,
		PanicParity: true, MutationParity: true, AliasParity: true, OwnershipParity: true, RecorderParity: true,
		AutogradParity: true, BackendSelectionParity: true, PairedEndToEndValidationRequired: true,
	}
}

func rowLocalSparseGatherContractForRunnerTest() config.RowLocalSparseGatherContract {
	return config.RowLocalSparseGatherContract{
		Name: "vit", ConfiguredSite: "example.com/vision.ViT.Forward", Transform: "example.com/nn.LayerNorm.Forward",
		Gather: "example.com/vision.gather", Concat: "example.com/backend.Execute",
		SliceOperation: "example.com/backend.OpSlice", SliceOperationValue: "1", ConcatOperation: "example.com/backend.OpConcat", ConcatOperationValue: "2",
		SliceAttrsType: "example.com/backend.SliceAttrs", SliceAxisField: "example.com/backend.SliceAttrs.Axis",
		SliceStartField: "example.com/backend.SliceAttrs.Start", SliceEndField: "example.com/backend.SliceAttrs.End",
		ConcatAttrsType: "example.com/backend.ConcatAttrs", ConcatAxisField: "example.com/backend.ConcatAttrs.Axis",
		TransformInputArgument: 2, GatherOperationArgument: 2, GatherAttrsArgument: 3, GatherInputArgument: 4,
		ConcatOperationArgument: 2, ConcatCollectionArgument: 3, ConcatAttrsArgument: 4,
		PackedRowsEqualBatchTimesStride: true, BatchPositive: true, StrideGreaterThanOne: true, NativeIntArithmeticNoOverflow: true,
		TransformRowsIndependent: true, TransformPreservesRowOrderAndWidth: true, TransformParametersImmutable: true,
		TransformDoesNotMutateInput: true, TransformInputOutputDoNotAlias: true, TransformDoesNotRetainArguments: true,
		CallsExecuteSynchronously: true, DiscardedRowsHaveNoEffectsStateOrRNG: true, GatherDeterministicAndValueIndependent: true,
		GatherAndConcatDoNotMutateOrRetain: true, SelectedFirstEquivalent: true, FusedForwardParity: true,
		FloatingPointPolicyPreserved: true, ErrorAndPanicParity: true, PartialOutputParity: true, RecorderOrderParity: true,
		VJPAllInputGradientsParity: true, SupportedDTypesLayoutsBackends: true, EquivalentFallbackUnlessForwardAndBackwardAvailable: true,
	}
}

func TestMissingVocabSchedulerTileGrainContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"schedulerTileGrainContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 || got[0] != "schedulerTileGrainContracts" {
		t.Fatalf("missingVocab(empty) = %v, want schedulerTileGrainContracts", got)
	}
	invalid := config.Config{SchedulerTileGrainContracts: []config.SchedulerTileGrainContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 || got[0] != "schedulerTileGrainContracts" {
		t.Fatalf("missingVocab(invalid) = %v, want schedulerTileGrainContracts", got)
	}
	configured := config.Config{SchedulerTileGrainContracts: []config.SchedulerTileGrainContract{schedulerTileGrainContractForTest()}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(configured) = %v, want none", got)
	}
}

func TestMissingVocabScopedBackendRoutingContracts(t *testing.T) {
	t.Parallel()
	check := &lint.Check{NeedsConfig: true, Vocab: []string{"scopedBackendRoutingContracts"}}
	if got := missingVocab(check, &config.Config{}); len(got) != 1 {
		t.Fatalf("missingVocab(empty)=%v", got)
	}
	invalid := config.Config{ScopedBackendRoutingContracts: []config.ScopedBackendRoutingContract{{Name: "incomplete"}}}
	if got := missingVocab(check, &invalid); len(got) != 1 {
		t.Fatalf("missingVocab(invalid)=%v", got)
	}
	contract := config.ScopedBackendRoutingContract{
		Name: "cpu", ConfiguredSite: "example.com/p.TestCPU", BackendGet: "example.com/b.Get",
		ContextWithBackend: "example.com/b.Context.WithBackend", Preference: "example.com/b.Preference",
		SetPreference: "example.com/b.SetPreference", SelectedBackend: "example.com/b.CPU",
		ConstructionCallables: []string{"example.com/m.Load"}, GlobalRouters: []string{"example.com/b.Default"},
		ProcessGlobalWriterIsolation: true,
	}
	configured := config.Config{ScopedBackendRoutingContracts: []config.ScopedBackendRoutingContract{contract}}
	if got := missingVocab(check, &configured); len(got) != 0 {
		t.Fatalf("missingVocab(configured)=%v", got)
	}
	configured.ScopedBackendRoutingContracts = append(configured.ScopedBackendRoutingContracts, contract)
	if got := missingVocab(check, &configured); len(got) != 1 {
		t.Fatalf("missingVocab(ambiguous)=%v", got)
	}
}

func schedulerTileGrainContractForTest() config.SchedulerTileGrainContract {
	return config.SchedulerTileGrainContract{
		Name:               "attention-bands",
		Scheduler:          "example.com/project.schedule",
		GrainConstant:      "example.com/project.bandRows",
		Band:               "example.com/project.band",
		BandRowsArgument:   2,
		KernelEntry:        "example.com/project.kernelRows",
		KernelRowsArgument: 2,
		RepeatedFullTasks:  true,
		Variants: []config.SchedulerTileGrainVariant{
			{Name: "amd64", GOOS: "linux", GOARCH: "amd64", TileHeight: 6, TileRouter: "example.com/project.kernelRows", TileRouterRowsArgument: 2, TiledKernel: "example.com/project.tile6", ScalarFallback: "example.com/project.scalar", ScalarFallbackRowsArgument: 2, KernelEntryRoutesFullTiles: true, ScalarFallbackHandlesTileRemainder: true},
			{Name: "arm64", GOOS: "darwin", GOARCH: "arm64", BuildTags: []string{"goexperiment.simd"}, TileHeight: 4, TileRouter: "example.com/project.kernelRows", TileRouterRowsArgument: 2, TiledKernel: "example.com/project.tile4", ScalarFallback: "example.com/project.scalar", ScalarFallbackRowsArgument: 2, KernelEntryRoutesFullTiles: true, ScalarFallbackHandlesTileRemainder: true},
		},
	}
}
