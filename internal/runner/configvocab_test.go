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
