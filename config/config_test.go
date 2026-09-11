package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestToSetAndCompile(t *testing.T) {
	t.Parallel()
	// An empty slice compiles to a nil set (not an empty map) so membership
	// tests are a plain, allocation-free nil-map read.
	if got := toSet(nil); got != nil {
		t.Errorf("toSet(nil) = %v, want nil", got)
	}
	if got := toSet([]string{}); got != nil {
		t.Errorf("toSet(empty) = %v, want nil", got)
	}
	s := toSet([]string{"A", "B", "A"})
	if !s["A"] || !s["B"] || s["C"] {
		t.Errorf("toSet membership wrong: %v", s)
	}

	c := Config{
		CacheLineBytes:           128,
		ElementAccessors:         []string{"AtF64", "SetF64"},
		AllocatorFuncs:           []string{"Zeros"},
		SelectorPromotionSymbols: []string{"selectKernel"},
		InPlaceFusionContracts: []InPlaceFusionContract{{
			Name: "swiglu",
		}},
		BoundedScratchFlowContracts: []BoundedScratchFlowContract{{
			Name:      "bounded-gelu",
			Observers: []BoundedScratchObserverContract{{Function: "example.com/p.Device.Use"}},
		}},
		ReusableOneShotWrapperContracts: []ReusableOneShotWrapperContract{{
			Name:                   "recorder-shell",
			MutableStateFields:     []string{"example.com/p.Recorder.encoder"},
			AllowedSynchronousUses: []ReusableOneShotMethod{{Static: "example.com/p.Recorder.Encode"}},
		}},
		ReusableResultLoopContracts: []ReusableResultLoopContract{{
			Name:                   "decode-logits",
			ShapeArgumentPositions: []int{2},
		}},
		RecorderResidualAddContracts: []RecorderResidualAddContract{{
			Name:                      "projection-residual",
			ProjectionExtentArguments: []int{4},
			AccumulateExtentArguments: []int{5},
			Implementations:           []RecorderResidualAddImplementation{{Projection: "example.com/p.Linear.record"}},
		}},
		RowLocalSparseGatherContracts: []RowLocalSparseGatherContract{{Name: "vit-classifier"}},
		TinySynchronousAcceleratorScreenContracts: []TinySynchronousAcceleratorScreenContract{{
			Name: "tiny-loss", Calls: []TinySynchronousAcceleratorScreenCall{{ConfiguredSite: "example.com/p.loss"}},
		}},
		ForwardLossBackwardGraphContracts: []ForwardLossBackwardGraphContract{{Name: "vit-objective"}},
		FragmentedAcceleratorObjectiveContracts: []FragmentedAcceleratorObjectiveContract{{
			Name: "gpt-objective", Calls: []FragmentedAcceleratorObjectiveCall{{ConfiguredSite: "example.com/p.lossAndGrad"}},
			PerOperationBackendRoutePreservationRequired: true, TrueCausalMaskSemanticsRequired: true,
		}},
		TopKOneContracts: []TopKOneContract{{
			Name: "resident-topk-one",
		}},
		ReceiverStagingContracts: []ReceiverStagingContract{{
			Name: "prefill-staging",
		}},
		NativeSnapshotStringCopyContracts: []NativeSnapshotStringCopyContract{{
			Name: "profile-labels",
		}},
		SchedulerTileGrainContracts: []SchedulerTileGrainContract{{
			Name:                                  "attention-bands",
			SynchronousRunner:                     "example.com/project.parallelWork",
			SynchronousRunnerWorkArgument:         3,
			SynchronousRunnerExecutesBeforeReturn: true,
			Variants:                              []SchedulerTileGrainVariant{{Name: "arm64", BuildTags: []string{"goexperiment.simd"}}},
		}},
		ElementCountMethods: nil, // stays nil after compile
	}
	sets := c.Compile()
	if sets.CacheLineBytes != 128 {
		t.Errorf("Compile lost cache line size: %d", sets.CacheLineBytes)
	}
	if !sets.ElementAccessors["AtF64"] || !sets.ElementAccessors["SetF64"] {
		t.Errorf("Compile lost element accessors: %v", sets.ElementAccessors)
	}
	if !sets.AllocatorFuncs["Zeros"] {
		t.Errorf("Compile lost allocator funcs: %v", sets.AllocatorFuncs)
	}
	if !sets.SelectorPromotionSymbols["selectKernel"] {
		t.Errorf("Compile lost selector promotion symbols: %v", sets.SelectorPromotionSymbols)
	}
	if len(sets.InPlaceFusionContracts) != 1 || sets.InPlaceFusionContracts[0].Name != "swiglu" {
		t.Errorf("Compile lost in-place fusion contracts: %+v", sets.InPlaceFusionContracts)
	}
	if len(sets.BoundedScratchFlowContracts) != 1 || sets.BoundedScratchFlowContracts[0].Name != "bounded-gelu" ||
		len(sets.BoundedScratchFlowContracts[0].Observers) != 1 {
		t.Errorf("Compile lost bounded-scratch contracts: %+v", sets.BoundedScratchFlowContracts)
	}
	if len(sets.ReusableOneShotWrapperContracts) != 1 || sets.ReusableOneShotWrapperContracts[0].Name != "recorder-shell" {
		t.Errorf("Compile lost reusable one-shot wrapper contracts: %+v", sets.ReusableOneShotWrapperContracts)
	}
	if len(sets.ReusableResultLoopContracts) != 1 || sets.ReusableResultLoopContracts[0].Name != "decode-logits" {
		t.Errorf("Compile lost reusable-result loop contracts: %+v", sets.ReusableResultLoopContracts)
	}
	if len(sets.RecorderResidualAddContracts) != 1 || sets.RecorderResidualAddContracts[0].Name != "projection-residual" {
		t.Errorf("Compile lost recorder residual-add contracts: %+v", sets.RecorderResidualAddContracts)
	}
	if len(sets.RowLocalSparseGatherContracts) != 1 || sets.RowLocalSparseGatherContracts[0].Name != "vit-classifier" {
		t.Errorf("Compile lost row-local sparse-gather contracts: %+v", sets.RowLocalSparseGatherContracts)
	}
	if len(sets.TinySynchronousAcceleratorScreenContracts) != 1 || sets.TinySynchronousAcceleratorScreenContracts[0].Name != "tiny-loss" ||
		len(sets.TinySynchronousAcceleratorScreenContracts[0].Calls) != 1 {
		t.Errorf("Compile lost tiny synchronous accelerator-screen contracts: %+v", sets.TinySynchronousAcceleratorScreenContracts)
	}
	if len(sets.ForwardLossBackwardGraphContracts) != 1 || sets.ForwardLossBackwardGraphContracts[0].Name != "vit-objective" {
		t.Errorf("Compile lost forward-loss-backward graph contracts: %+v", sets.ForwardLossBackwardGraphContracts)
	}
	if len(sets.FragmentedAcceleratorObjectiveContracts) != 1 || sets.FragmentedAcceleratorObjectiveContracts[0].Name != "gpt-objective" ||
		len(sets.FragmentedAcceleratorObjectiveContracts[0].Calls) != 1 ||
		!sets.FragmentedAcceleratorObjectiveContracts[0].PerOperationBackendRoutePreservationRequired ||
		!sets.FragmentedAcceleratorObjectiveContracts[0].TrueCausalMaskSemanticsRequired {
		t.Errorf("Compile lost fragmented accelerator-objective contracts: %+v", sets.FragmentedAcceleratorObjectiveContracts)
	}
	if len(sets.TopKOneContracts) != 1 || sets.TopKOneContracts[0].Name != "resident-topk-one" {
		t.Errorf("Compile lost Top-K(k=1) contracts: %+v", sets.TopKOneContracts)
	}
	if len(sets.ReceiverStagingContracts) != 1 || sets.ReceiverStagingContracts[0].Name != "prefill-staging" {
		t.Errorf("Compile lost receiver staging contracts: %+v", sets.ReceiverStagingContracts)
	}
	if len(sets.NativeSnapshotStringCopyContracts) != 1 || sets.NativeSnapshotStringCopyContracts[0].Name != "profile-labels" {
		t.Errorf("Compile lost native snapshot string-copy contracts: %+v", sets.NativeSnapshotStringCopyContracts)
	}
	if len(sets.SchedulerTileGrainContracts) != 1 || sets.SchedulerTileGrainContracts[0].Name != "attention-bands" ||
		sets.SchedulerTileGrainContracts[0].SynchronousRunnerWorkArgument != 3 ||
		!sets.SchedulerTileGrainContracts[0].SynchronousRunnerExecutesBeforeReturn ||
		len(sets.SchedulerTileGrainContracts[0].Variants) != 1 {
		t.Errorf("Compile lost scheduler tile-grain contracts: %+v", sets.SchedulerTileGrainContracts)
	}
	c.TopKOneContracts[0].Name = "mutated"
	if sets.TopKOneContracts[0].Name != "resident-topk-one" {
		t.Error("Compile must clone Top-K(k=1) contracts")
	}
	c.InPlaceFusionContracts[0].Name = "mutated"
	c.SchedulerTileGrainContracts[0].Variants[0].BuildTags[0] = "mutated"
	if sets.SchedulerTileGrainContracts[0].Variants[0].BuildTags[0] != "goexperiment.simd" {
		t.Error("Compile must deeply clone scheduler tile-grain contracts")
	}
	if sets.InPlaceFusionContracts[0].Name != "swiglu" {
		t.Error("Compile must clone in-place fusion contracts")
	}
	c.BoundedScratchFlowContracts[0].Name = "mutated"
	c.BoundedScratchFlowContracts[0].Observers[0].Function = "mutated"
	if sets.BoundedScratchFlowContracts[0].Name != "bounded-gelu" ||
		sets.BoundedScratchFlowContracts[0].Observers[0].Function != "example.com/p.Device.Use" {
		t.Error("Compile must deeply clone bounded-scratch contracts")
	}
	c.ReceiverStagingContracts[0].Name = "mutated"
	if sets.ReceiverStagingContracts[0].Name != "prefill-staging" {
		t.Error("Compile must clone receiver staging contracts")
	}
	c.NativeSnapshotStringCopyContracts[0].Name = "mutated"
	if sets.NativeSnapshotStringCopyContracts[0].Name != "profile-labels" {
		t.Error("Compile must clone native snapshot string-copy contracts")
	}
	c.ForwardLossBackwardGraphContracts[0].Name = "mutated"
	if sets.ForwardLossBackwardGraphContracts[0].Name != "vit-objective" {
		t.Error("Compile must clone forward-loss-backward graph contracts")
	}
	c.ReusableOneShotWrapperContracts[0].Name = "mutated"
	c.ReusableOneShotWrapperContracts[0].MutableStateFields[0] = "mutated"
	c.ReusableOneShotWrapperContracts[0].AllowedSynchronousUses[0].Static = "mutated"
	if sets.ReusableOneShotWrapperContracts[0].Name != "recorder-shell" ||
		sets.ReusableOneShotWrapperContracts[0].MutableStateFields[0] != "example.com/p.Recorder.encoder" ||
		sets.ReusableOneShotWrapperContracts[0].AllowedSynchronousUses[0].Static != "example.com/p.Recorder.Encode" {
		t.Error("Compile must deeply clone reusable one-shot wrapper contracts")
	}
	c.ReusableResultLoopContracts[0].Name = "mutated"
	c.ReusableResultLoopContracts[0].ShapeArgumentPositions[0] = 9
	if sets.ReusableResultLoopContracts[0].Name != "decode-logits" || sets.ReusableResultLoopContracts[0].ShapeArgumentPositions[0] != 2 {
		t.Error("Compile must deeply clone reusable-result loop contracts")
	}
	c.RecorderResidualAddContracts[0].Name = "mutated"
	c.RecorderResidualAddContracts[0].ProjectionExtentArguments[0] = 9
	c.RecorderResidualAddContracts[0].Implementations[0].Projection = "mutated"
	if sets.RecorderResidualAddContracts[0].Name != "projection-residual" ||
		sets.RecorderResidualAddContracts[0].ProjectionExtentArguments[0] != 4 ||
		sets.RecorderResidualAddContracts[0].Implementations[0].Projection != "example.com/p.Linear.record" {
		t.Error("Compile must deeply clone recorder residual-add contracts")
	}
	c.RowLocalSparseGatherContracts[0].Name = "mutated"
	if sets.RowLocalSparseGatherContracts[0].Name != "vit-classifier" {
		t.Error("Compile must clone row-local sparse-gather contracts")
	}
	c.TinySynchronousAcceleratorScreenContracts[0].Name = "mutated"
	c.TinySynchronousAcceleratorScreenContracts[0].Calls[0].ConfiguredSite = "mutated"
	if sets.TinySynchronousAcceleratorScreenContracts[0].Name != "tiny-loss" ||
		sets.TinySynchronousAcceleratorScreenContracts[0].Calls[0].ConfiguredSite != "example.com/p.loss" {
		t.Error("Compile must deeply clone tiny synchronous accelerator-screen contracts")
	}
	c.FragmentedAcceleratorObjectiveContracts[0].Name = "mutated"
	c.FragmentedAcceleratorObjectiveContracts[0].Calls[0].ConfiguredSite = "mutated"
	c.FragmentedAcceleratorObjectiveContracts[0].PerOperationBackendRoutePreservationRequired = false
	c.FragmentedAcceleratorObjectiveContracts[0].TrueCausalMaskSemanticsRequired = false
	if sets.FragmentedAcceleratorObjectiveContracts[0].Name != "gpt-objective" ||
		sets.FragmentedAcceleratorObjectiveContracts[0].Calls[0].ConfiguredSite != "example.com/p.lossAndGrad" ||
		!sets.FragmentedAcceleratorObjectiveContracts[0].PerOperationBackendRoutePreservationRequired ||
		!sets.FragmentedAcceleratorObjectiveContracts[0].TrueCausalMaskSemanticsRequired {
		t.Error("Compile must deeply clone fragmented accelerator-objective contracts")
	}
	if sets.ElementCountMethods != nil {
		t.Errorf("empty field must compile to a nil set, got %v", sets.ElementCountMethods)
	}
}

func TestTinySynchronousAcceleratorScreenContractValid(t *testing.T) {
	t.Parallel()
	valid := tinySynchronousAcceleratorScreenContractForTest()
	if !valid.Valid() {
		t.Fatal("complete tiny synchronous accelerator-screen contract is invalid")
	}
	tests := []struct {
		name   string
		mutate func(*TinySynchronousAcceleratorScreenContract)
	}{
		{"missing name", func(c *TinySynchronousAcceleratorScreenContract) { c.Name = "" }},
		{"no calls", func(c *TinySynchronousAcceleratorScreenContract) { c.Calls = nil }},
		{"too many calls", func(c *TinySynchronousAcceleratorScreenContract) { c.Calls = append(c.Calls, c.Calls[0]) }},
		{"duplicate call", func(c *TinySynchronousAcceleratorScreenContract) { c.Calls[1] = c.Calls[0] }},
		{"bad site", func(c *TinySynchronousAcceleratorScreenContract) { c.Calls[0].ConfiguredSite = "loss" }},
		{"bad accelerator", func(c *TinySynchronousAcceleratorScreenContract) { c.Calls[0].AcceleratorCallable = "run" }},
		{"bad host", func(c *TinySynchronousAcceleratorScreenContract) { c.Calls[0].HostAlternativeCallable = "host" }},
		{"same host", func(c *TinySynchronousAcceleratorScreenContract) {
			c.Calls[0].HostAlternativeCallable = c.Calls[0].AcceleratorCallable
		}},
		{"duplicate geometry roles", func(c *TinySynchronousAcceleratorScreenContract) {
			c.Calls[0].ColumnsArgument = c.Calls[0].RowsArgument
		}},
		{"partial operation", func(c *TinySynchronousAcceleratorScreenContract) { c.Calls[0].OperationConstantValue = "" }},
		{"operation role collision", func(c *TinySynchronousAcceleratorScreenContract) {
			c.Calls[0].OperationArgument = c.Calls[0].RowsArgument
		}},
		{"zero rows", func(c *TinySynchronousAcceleratorScreenContract) { c.ConfiguredRows = 0 }},
		{"overflow elements", func(c *TinySynchronousAcceleratorScreenContract) { c.ConfiguredRows = 1 << 62; c.ConfiguredColumns = 8 }},
		{"zero bytes", func(c *TinySynchronousAcceleratorScreenContract) { c.ConfiguredWorkingSetBytes = 0 }},
		{"no limits", func(c *TinySynchronousAcceleratorScreenContract) { c.MaxElements = 0; c.MaxWorkingSetBytes = 0 }},
		{"element limit", func(c *TinySynchronousAcceleratorScreenContract) { c.MaxElements = 79 }},
		{"byte limit", func(c *TinySynchronousAcceleratorScreenContract) { c.MaxWorkingSetBytes = 639 }},
		{"submission mismatch", func(c *TinySynchronousAcceleratorScreenContract) { c.ConfiguredSubmissionCount = 1 }},
		{"bad target", func(c *TinySynchronousAcceleratorScreenContract) { c.TargetGOOS = "Darwin" }},
		{"unsupported GOOS", func(c *TinySynchronousAcceleratorScreenContract) { c.TargetGOOS = "beos" }},
		{"unsupported GOARCH", func(c *TinySynchronousAcceleratorScreenContract) { c.TargetGOARCH = "alpha" }},
		{"bad memory", func(c *TinySynchronousAcceleratorScreenContract) { c.MemoryModel = "discrete" }},
		{"blocking gap", func(c *TinySynchronousAcceleratorScreenContract) { c.BlockingCompletion = false }},
		{"input host-access gap", func(c *TinySynchronousAcceleratorScreenContract) { c.HostAccessibleInputs = false }},
		{"output host-access gap", func(c *TinySynchronousAcceleratorScreenContract) { c.HostAccessibleOutput = false }},
		{"transfer gap", func(c *TinySynchronousAcceleratorScreenContract) { c.NoTransferRequired = false }},
		{"pre-residency gap", func(c *TinySynchronousAcceleratorScreenContract) { c.NoPreexistingDeviceResidency = false }},
		{"post-residency gap", func(c *TinySynchronousAcceleratorScreenContract) { c.NoPostDeviceResidency = false }},
		{"graph gap", func(c *TinySynchronousAcceleratorScreenContract) { c.NoGraphContext = false }},
		{"recorder gap", func(c *TinySynchronousAcceleratorScreenContract) { c.NoRecorderContext = false }},
		{"command-buffer gap", func(c *TinySynchronousAcceleratorScreenContract) { c.NoCommandBufferContext = false }},
		{"dtype gap", func(c *TinySynchronousAcceleratorScreenContract) { c.ExactDTypeCoverage = false }},
		{"layout gap", func(c *TinySynchronousAcceleratorScreenContract) { c.ExactLayoutCoverage = false }},
		{"attrs gap", func(c *TinySynchronousAcceleratorScreenContract) { c.ExactAttributesCoverage = false }},
		{"reduction gap", func(c *TinySynchronousAcceleratorScreenContract) { c.ExactReductionCoverage = false }},
		{"forward gap", func(c *TinySynchronousAcceleratorScreenContract) { c.ForwardParity = false }},
		{"gradient gap", func(c *TinySynchronousAcceleratorScreenContract) { c.GradientParity = false }},
		{"floating-point gap", func(c *TinySynchronousAcceleratorScreenContract) { c.FloatingPointParity = false }},
		{"error gap", func(c *TinySynchronousAcceleratorScreenContract) { c.ErrorParity = false }},
		{"panic gap", func(c *TinySynchronousAcceleratorScreenContract) { c.PanicParity = false }},
		{"mutation gap", func(c *TinySynchronousAcceleratorScreenContract) { c.MutationParity = false }},
		{"alias gap", func(c *TinySynchronousAcceleratorScreenContract) { c.AliasParity = false }},
		{"ownership gap", func(c *TinySynchronousAcceleratorScreenContract) { c.OwnershipParity = false }},
		{"recorder parity gap", func(c *TinySynchronousAcceleratorScreenContract) { c.RecorderParity = false }},
		{"autograd gap", func(c *TinySynchronousAcceleratorScreenContract) { c.AutogradParity = false }},
		{"backend-selection gap", func(c *TinySynchronousAcceleratorScreenContract) { c.BackendSelectionParity = false }},
		{"paired validation gap", func(c *TinySynchronousAcceleratorScreenContract) { c.PairedEndToEndValidationRequired = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			candidate.Calls = slices.Clone(valid.Calls)
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Error("incomplete contract is valid")
			}
		})
	}
	cgo := valid
	cgo.Calls = slices.Clone(valid.Calls[:1])
	cgo.Calls[0].AcceleratorCallable = "C.cross_entropy"
	cgo.ConfiguredSubmissionCount = 1
	if !cgo.Valid() {
		t.Error("real-cgo callable identity must be accepted by configuration validation")
	}
	elementsOnly := valid
	elementsOnly.MaxWorkingSetBytes = 0
	if !elementsOnly.Valid() {
		t.Error("an element-only inclusive limit must be valid")
	}
	bytesOnly := valid
	bytesOnly.MaxElements = 0
	bytesOnly.MemoryModel = "shared"
	if !bytesOnly.Valid() {
		t.Error("a byte-only inclusive limit on shared memory must be valid")
	}
}

func tinySynchronousAcceleratorScreenContractForTest() TinySynchronousAcceleratorScreenContract {
	return TinySynchronousAcceleratorScreenContract{
		Name: "tiny-cross-entropy",
		Calls: []TinySynchronousAcceleratorScreenCall{
			{ConfiguredSite: "example.com/model.forward", AcceleratorCallable: "example.com/backend.Metal.Forward", HostAlternativeCallable: "example.com/backend.hostForward", RowsArgument: 2, ColumnsArgument: 3, OperationArgument: 1, OperationConstant: "example.com/backend.crossEntropy", OperationConstantValue: "7"},
			{ConfiguredSite: "example.com/model.backward", AcceleratorCallable: "example.com/backend.Metal.Backward", HostAlternativeCallable: "example.com/backend.hostBackward", RowsArgument: 2, ColumnsArgument: 3},
		},
		ConfiguredRows: 8, ConfiguredColumns: 10, ConfiguredWorkingSetBytes: 640,
		MaxElements: 80, MaxWorkingSetBytes: 640, ConfiguredSubmissionCount: 2,
		TargetGOOS: "darwin", TargetGOARCH: "arm64", MemoryModel: "unified",
		BlockingCompletion: true, HostAccessibleInputs: true, HostAccessibleOutput: true, NoTransferRequired: true,
		NoPreexistingDeviceResidency: true, NoPostDeviceResidency: true, NoGraphContext: true, NoRecorderContext: true,
		NoCommandBufferContext: true, ExactDTypeCoverage: true, ExactLayoutCoverage: true, ExactAttributesCoverage: true,
		ExactReductionCoverage: true, ForwardParity: true, GradientParity: true, FloatingPointParity: true, ErrorParity: true,
		PanicParity: true, MutationParity: true, AliasParity: true, OwnershipParity: true, RecorderParity: true,
		AutogradParity: true, BackendSelectionParity: true, PairedEndToEndValidationRequired: true,
	}
}

func TestForwardLossBackwardGraphContractValid(t *testing.T) {
	t.Parallel()
	valid := forwardLossBackwardGraphContractForTest()
	if !valid.Valid() {
		t.Fatal("complete forward-loss-backward graph contract is invalid")
	}
	tests := []struct {
		name   string
		mutate func(*ForwardLossBackwardGraphContract)
	}{
		{"missing name", func(c *ForwardLossBackwardGraphContract) { c.Name = "" }},
		{"bad site", func(c *ForwardLossBackwardGraphContract) { c.ObjectiveSite = "objective" }},
		{"duplicate callable", func(c *ForwardLossBackwardGraphContract) { c.LossCallable = c.ForwardCallable }},
		{"bad factory backend role", func(c *ForwardLossBackwardGraphContract) { c.RecorderFactoryBackendArgument = 0 }},
		{"bad recorder role", func(c *ForwardLossBackwardGraphContract) { c.RecorderBindingArgument = 0 }},
		{"partial reduction", func(c *ForwardLossBackwardGraphContract) { c.LossReductionConstantValue = "" }},
		{"missing geometry", func(c *ForwardLossBackwardGraphContract) { c.ConfiguredGeometry = "" }},
		{"missing evidence", func(c *ForwardLossBackwardGraphContract) { c.ConfiguredEvidence = "" }},
		{"missing gradients", func(c *ForwardLossBackwardGraphContract) { c.ConfiguredGradientCount = 0 }},
		{"one current submission", func(c *ForwardLossBackwardGraphContract) { c.ConfiguredCurrentSubmissionCount = 1 }},
		{"two candidate submissions", func(c *ForwardLossBackwardGraphContract) { c.ConfiguredCandidateSubmissions = 2 }},
		{"unbounded cache", func(c *ForwardLossBackwardGraphContract) { c.MaxCacheEntries = 0 }},
		{"no recreation", func(c *ForwardLossBackwardGraphContract) { c.RecreatesForwardWork = false }},
		{"unstable geometry", func(c *ForwardLossBackwardGraphContract) { c.StableGeometry = false }},
		{"loss gap", func(c *ForwardLossBackwardGraphContract) { c.ExactScalarLossReduction = false }},
		{"gradient order gap", func(c *ForwardLossBackwardGraphContract) { c.StableCompleteGradientOrder = false }},
		{"recorder ownership gap", func(c *ForwardLossBackwardGraphContract) { c.PrivateRecorderOwnership = false }},
		{"hook gap", func(c *ForwardLossBackwardGraphContract) { c.CustomHooksExcluded = false }},
		{"mutation gap", func(c *ForwardLossBackwardGraphContract) { c.MutationExcluded = false }},
		{"eligibility gap", func(c *ForwardLossBackwardGraphContract) { c.DTypeLayoutBackendConstrained = false }},
		{"geometry key gap", func(c *ForwardLossBackwardGraphContract) { c.CacheKeyCoversGeometry = false }},
		{"semantic key gap", func(c *ForwardLossBackwardGraphContract) { c.CacheKeyCoversDTypeLayoutObjective = false }},
		{"cache bound gap", func(c *ForwardLossBackwardGraphContract) { c.BoundedCache = false }},
		{"fallback gap", func(c *ForwardLossBackwardGraphContract) { c.PortableFallbackPreserved = false }},
		{"forward parity gap", func(c *ForwardLossBackwardGraphContract) { c.ForwardParity = false }},
		{"loss parity gap", func(c *ForwardLossBackwardGraphContract) { c.ScalarLossParity = false }},
		{"gradient parity gap", func(c *ForwardLossBackwardGraphContract) { c.AllGradientParity = false }},
		{"immutability gap", func(c *ForwardLossBackwardGraphContract) { c.InputAndParameterImmutability = false }},
		{"failure parity gap", func(c *ForwardLossBackwardGraphContract) { c.ErrorAndPanicParity = false }},
		{"recorder isolation gap", func(c *ForwardLossBackwardGraphContract) { c.RecorderIsolation = false }},
		{"per-operation route gap", func(c *ForwardLossBackwardGraphContract) { c.PerOperationAndLayerRoutesExcluded = false }},
		{"private-tape route gap", func(c *ForwardLossBackwardGraphContract) { c.PrivateTapePreservesBackendRoutes = false }},
		{"backend-selection gap", func(c *ForwardLossBackwardGraphContract) { c.BackendSelectionParity = false }},
		{"numerical gate gap", func(c *ForwardLossBackwardGraphContract) { c.PairedNumericalValidationRequired = false }},
		{"end-to-end gate gap", func(c *ForwardLossBackwardGraphContract) { c.PairedEndToEndValidationRequired = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Error("incomplete contract is valid")
			}
		})
	}
	implicitMean := valid
	implicitMean.LossReductionArgument = 0
	implicitMean.LossReductionConstant = ""
	implicitMean.LossReductionConstantValue = ""
	if !implicitMean.Valid() {
		t.Error("an exact contract for an implicitly mean loss must be valid")
	}
}

func forwardLossBackwardGraphContractForTest() ForwardLossBackwardGraphContract {
	return ForwardLossBackwardGraphContract{
		Name: "vit", ObjectiveSite: "example.com/vision.ViT.LossAndGrad",
		RecorderFactoryCallable: "example.com/autograd.NewTape", RecorderBindingCallable: "example.com/backend.Context.WithRecorder",
		ForwardCallable: "example.com/vision.ViT.Forward", LossCallable: "example.com/nn.CrossEntropy",
		BackwardCallable: "example.com/autograd.Tape.Backward", ParameterOrderCallable: "example.com/vision.ViT.Params",
		GradientCallable: "example.com/autograd.Tape.Grad", RecorderFactoryBackendArgument: 1,
		RecorderBindingArgument: 1, ForwardRecorderArgument: 1,
		LossRecorderArgument: 1, LossForwardArgument: 2, BackwardLossArgument: 1, GradientParameterArgument: 1,
		LossReductionArgument: 4, LossReductionConstant: "example.com/nn.Mean", LossReductionConstantValue: "1",
		ConfiguredGeometry: "B=8,S=65,D=128,H=4,FFN=512,depth=4,C=10", ConfiguredGradientCount: 56,
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

func TestRowLocalSparseGatherContractValid(t *testing.T) {
	t.Parallel()
	valid := rowLocalSparseGatherContractForTest()
	if !valid.Valid() {
		t.Fatal("complete row-local sparse-gather contract is invalid")
	}
	tests := []struct {
		name   string
		mutate func(*RowLocalSparseGatherContract)
	}{
		{"missing site", func(c *RowLocalSparseGatherContract) { c.ConfiguredSite = "" }},
		{"interface-shaped transform id", func(c *RowLocalSparseGatherContract) { c.Transform = "Forward" }},
		{"same gather and concat", func(c *RowLocalSparseGatherContract) { c.Concat = c.Gather }},
		{"same operation constants", func(c *RowLocalSparseGatherContract) { c.ConcatOperation = c.SliceOperation }},
		{"duplicate slice fields", func(c *RowLocalSparseGatherContract) { c.SliceEndField = c.SliceStartField }},
		{"duplicate gather roles", func(c *RowLocalSparseGatherContract) { c.GatherInputArgument = c.GatherAttrsArgument }},
		{"duplicate concat roles", func(c *RowLocalSparseGatherContract) { c.ConcatAttrsArgument = c.ConcatCollectionArgument }},
		{"missing packed geometry", func(c *RowLocalSparseGatherContract) { c.PackedRowsEqualBatchTimesStride = false }},
		{"missing positive batch", func(c *RowLocalSparseGatherContract) { c.BatchPositive = false }},
		{"missing nontrivial stride", func(c *RowLocalSparseGatherContract) { c.StrideGreaterThanOne = false }},
		{"missing overflow proof", func(c *RowLocalSparseGatherContract) { c.NativeIntArithmeticNoOverflow = false }},
		{"missing row locality", func(c *RowLocalSparseGatherContract) { c.TransformRowsIndependent = false }},
		{"missing row order and width", func(c *RowLocalSparseGatherContract) { c.TransformPreservesRowOrderAndWidth = false }},
		{"missing immutable parameters", func(c *RowLocalSparseGatherContract) { c.TransformParametersImmutable = false }},
		{"missing input ownership", func(c *RowLocalSparseGatherContract) { c.TransformDoesNotMutateInput = false }},
		{"missing input output nonalias", func(c *RowLocalSparseGatherContract) { c.TransformInputOutputDoNotAlias = false }},
		{"missing transform retention", func(c *RowLocalSparseGatherContract) { c.TransformDoesNotRetainArguments = false }},
		{"missing synchronous calls", func(c *RowLocalSparseGatherContract) { c.CallsExecuteSynchronously = false }},
		{"missing discarded row effects", func(c *RowLocalSparseGatherContract) { c.DiscardedRowsHaveNoEffectsStateOrRNG = false }},
		{"missing deterministic gather", func(c *RowLocalSparseGatherContract) { c.GatherDeterministicAndValueIndependent = false }},
		{"missing gather concat ownership", func(c *RowLocalSparseGatherContract) { c.GatherAndConcatDoNotMutateOrRetain = false }},
		{"missing selected-first equivalence", func(c *RowLocalSparseGatherContract) { c.SelectedFirstEquivalent = false }},
		{"missing fused forward parity", func(c *RowLocalSparseGatherContract) { c.FusedForwardParity = false }},
		{"missing float parity", func(c *RowLocalSparseGatherContract) { c.FloatingPointPolicyPreserved = false }},
		{"missing error panic parity", func(c *RowLocalSparseGatherContract) { c.ErrorAndPanicParity = false }},
		{"missing partial output parity", func(c *RowLocalSparseGatherContract) { c.PartialOutputParity = false }},
		{"missing recorder order", func(c *RowLocalSparseGatherContract) { c.RecorderOrderParity = false }},
		{"missing vjp parity", func(c *RowLocalSparseGatherContract) { c.VJPAllInputGradientsParity = false }},
		{"missing backend coverage", func(c *RowLocalSparseGatherContract) { c.SupportedDTypesLayoutsBackends = false }},
		{"missing fallback", func(c *RowLocalSparseGatherContract) { c.EquivalentFallbackUnlessForwardAndBackwardAvailable = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Errorf("mutated contract unexpectedly valid: %+v", candidate)
			}
		})
	}
}

func rowLocalSparseGatherContractForTest() RowLocalSparseGatherContract {
	return RowLocalSparseGatherContract{
		Name: "vit-classifier", ConfiguredSite: "example.com/vision.ViT.Forward",
		Transform: "example.com/nn.LayerNorm.Forward", Gather: "example.com/vision.visExec1", Concat: "example.com/backend.Execute",
		SliceOperation: "example.com/backend.OpSlice", SliceOperationValue: "1", ConcatOperation: "example.com/backend.OpConcat", ConcatOperationValue: "2",
		SliceAttrsType: "example.com/backend.SliceAttrs", SliceAxisField: "example.com/backend.SliceAttrs.Axis",
		SliceStartField: "example.com/backend.SliceAttrs.Start", SliceEndField: "example.com/backend.SliceAttrs.End",
		ConcatAttrsType: "example.com/backend.ConcatAttrs", ConcatAxisField: "example.com/backend.ConcatAttrs.Axis",
		TransformInputArgument: 2, GatherOperationArgument: 2, GatherAttrsArgument: 3, GatherInputArgument: 4,
		ConcatOperationArgument: 2, ConcatCollectionArgument: 3, ConcatAttrsArgument: 4,
		PackedRowsEqualBatchTimesStride: true, BatchPositive: true, StrideGreaterThanOne: true, NativeIntArithmeticNoOverflow: true,
		TransformRowsIndependent: true, TransformPreservesRowOrderAndWidth: true, TransformParametersImmutable: true,
		TransformDoesNotMutateInput: true, TransformInputOutputDoNotAlias: true, TransformDoesNotRetainArguments: true,
		CallsExecuteSynchronously: true, DiscardedRowsHaveNoEffectsStateOrRNG: true,
		GatherDeterministicAndValueIndependent: true, GatherAndConcatDoNotMutateOrRetain: true,
		SelectedFirstEquivalent: true, FusedForwardParity: true, FloatingPointPolicyPreserved: true,
		ErrorAndPanicParity: true, PartialOutputParity: true, RecorderOrderParity: true, VJPAllInputGradientsParity: true,
		SupportedDTypesLayoutsBackends: true, EquivalentFallbackUnlessForwardAndBackwardAvailable: true,
	}
}

func TestRecorderResidualAddContractValid(t *testing.T) {
	t.Parallel()
	valid := recorderResidualAddContractForTest()
	if !valid.Valid() {
		t.Fatal("complete recorder residual-add contract is invalid")
	}
	tests := []struct {
		name   string
		mutate func(*RecorderResidualAddContract)
	}{
		{"blank name", func(c *RecorderResidualAddContract) { c.Name = "" }},
		{"unqualified projection", func(c *RecorderResidualAddContract) { c.Projection = "record" }},
		{"same sibling", func(c *RecorderResidualAddContract) { c.Accumulate = c.Projection }},
		{"unqualified binary", func(c *RecorderResidualAddContract) { c.RecorderBinary = "Binary" }},
		{"missing add value", func(c *RecorderResidualAddContract) { c.AddOperationValue = "" }},
		{"invalid eager helper", func(c *RecorderResidualAddContract) { c.EagerSequence = "firstErr" }},
		{"missing first-error promise", func(c *RecorderResidualAddContract) { c.EagerSequenceReturnsFirstError = false }},
		{"duplicate projection role", func(c *RecorderResidualAddContract) { c.ProjectionSourceArgument = c.ProjectionRecorderArgument }},
		{"projection extent overlaps role", func(c *RecorderResidualAddContract) { c.ProjectionExtentArguments[0] = c.ProjectionSourceArgument }},
		{"extent count mismatch", func(c *RecorderResidualAddContract) { c.AccumulateExtentArguments = nil }},
		{"accumulate length overlaps role", func(c *RecorderResidualAddContract) {
			c.AccumulateDestinationLengthArgument = c.AccumulateDestinationArgument
		}},
		{"negative destination length role", func(c *RecorderResidualAddContract) { c.AccumulateDestinationLengthArgument = -1 }},
		{"missing overwrite", func(c *RecorderResidualAddContract) { c.ProjectionOverwritesTemporary = false }},
		{"missing runtime nonalias", func(c *RecorderResidualAddContract) { c.MatchedBuffersDoNotAlias = false }},
		{"missing recorder order", func(c *RecorderResidualAddContract) { c.RecorderOrderPreserved = false }},
		{"missing numerical policy", func(c *RecorderResidualAddContract) { c.AccumulatePreservesArithmeticPolicy = false }},
		{"interface list without site", func(c *RecorderResidualAddContract) { c.ConfiguredSite = "" }},
		{"interface list without coverage", func(c *RecorderResidualAddContract) { c.AllDynamicProjectionTypesCovered = false }},
		{"duplicate implementation", func(c *RecorderResidualAddContract) {
			c.Implementations = append(c.Implementations, c.Implementations[0])
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneRecorderResidualAddContracts([]RecorderResidualAddContract{valid})[0]
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Errorf("mutated contract unexpectedly valid: %+v", candidate)
			}
		})
	}
}

func recorderResidualAddContractForTest() RecorderResidualAddContract {
	return RecorderResidualAddContract{
		Name:                                "projection-residual",
		Projection:                          "example.com/model.Linear.record",
		Accumulate:                          "example.com/model.Linear.recordAdd",
		RecorderBinary:                      "example.com/model.Recorder.Binary",
		EagerSequence:                       "example.com/model.firstErr",
		AddOperation:                        "example.com/model.binaryAdd",
		AddOperationValue:                   "1",
		ConfiguredSite:                      "example.com/model.Decoder.Step",
		ProjectionRecorderArgument:          1,
		ProjectionSourceArgument:            2,
		ProjectionTemporaryArgument:         3,
		ProjectionExtentArguments:           []int{4},
		BinaryDestinationArgument:           1,
		BinaryTemporaryArgument:             2,
		BinaryOutputArgument:                3,
		BinaryOperationArgument:             4,
		AccumulateRecorderArgument:          1,
		AccumulateSourceArgument:            2,
		AccumulateTemporaryArgument:         3,
		AccumulateDestinationArgument:       4,
		AccumulateExtentArguments:           []int{5},
		AccumulateDestinationLengthArgument: 6,
		Implementations: []RecorderResidualAddImplementation{{
			Projection: "example.com/model.F32Linear.record",
			Accumulate: "example.com/model.F32Linear.recordAdd",
		}},
		ProjectionOverwritesTemporary:                     true,
		TemporaryMayServeAsAccumulateScratch:              true,
		MatchedBuffersDoNotAlias:                          true,
		TemporaryUnobservedOutsideMatchedCalls:            true,
		CallsDoNotRetainArguments:                         true,
		CallsExecuteSynchronously:                         true,
		RecorderOrderPreserved:                            true,
		AccumulateMatchesProjectionAndResidualAdd:         true,
		AccumulatePreservesErrorsAndPanics:                true,
		AccumulatePreservesPartialOutput:                  true,
		AccumulatePreservesArithmeticPolicy:               true,
		AccumulateSupportsConfiguredDTypesLayoutsBackends: true,
		AllDynamicProjectionTypesCovered:                  true,
		EagerSequenceReturnsFirstError:                    true,
	}
}

func TestReusableResultLoopContractValid(t *testing.T) {
	t.Parallel()
	valid := ReusableResultLoopContract{
		Name:                     "decode-logits",
		Wrapper:                  "example.com/model.Decoder.Step",
		Into:                     "example.com/model.Decoder.StepInto",
		ResultPosition:           1,
		DestinationArgument:      3,
		ShapeArgumentPositions:   []int{2},
		ConfiguredResultElements: 50257,
		ConfiguredLoopIterations: 8,
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
	if !valid.Valid() {
		t.Fatal("complete reusable-result loop contract is invalid")
	}
	tests := []struct {
		name   string
		mutate func(*ReusableResultLoopContract)
	}{
		{"blank name", func(c *ReusableResultLoopContract) { c.Name = "" }},
		{"padded name", func(c *ReusableResultLoopContract) { c.Name = " decode" }},
		{"invalid wrapper", func(c *ReusableResultLoopContract) { c.Wrapper = "Step" }},
		{"invalid into", func(c *ReusableResultLoopContract) { c.Into = "StepInto" }},
		{"same methods", func(c *ReusableResultLoopContract) { c.Into = c.Wrapper }},
		{"zero result role", func(c *ReusableResultLoopContract) { c.ResultPosition = 0 }},
		{"zero destination role", func(c *ReusableResultLoopContract) { c.DestinationArgument = 0 }},
		{"duplicate shape role", func(c *ReusableResultLoopContract) { c.ShapeArgumentPositions = []int{2, 2} }},
		{"negative shape role", func(c *ReusableResultLoopContract) { c.ShapeArgumentPositions = []int{-1} }},
		{"partial configured model", func(c *ReusableResultLoopContract) { c.ConfiguredLoopIterations = 0 }},
		{"missing fresh ownership", func(c *ReusableResultLoopContract) { c.WrapperReturnsFreshOwned = false }},
		{"unstable result shape", func(c *ReusableResultLoopContract) { c.ResultLengthStableForReceiverAndListedArgs = false }},
		{"partial overwrite", func(c *ReusableResultLoopContract) { c.IntoOverwritesDestinationOnSuccess = false }},
		{"destination read", func(c *ReusableResultLoopContract) { c.IntoDoesNotReadDestinationBeforeOverwrite = false }},
		{"identity-sensitive destination", func(c *ReusableResultLoopContract) { c.IntoIgnoresDestinationIdentityAndExtraCapacity = false }},
		{"retained destination", func(c *ReusableResultLoopContract) { c.IntoDoesNotRetainDestination = false }},
		{"asynchronous into", func(c *ReusableResultLoopContract) { c.IntoExecutesSynchronously = false }},
		{"state mismatch", func(c *ReusableResultLoopContract) { c.WrapperAndIntoHaveIdenticalStateEffects = false }},
		{"error mismatch", func(c *ReusableResultLoopContract) { c.WrapperAndIntoHaveIdenticalErrorsAndPanics = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			candidate.ShapeArgumentPositions = slices.Clone(valid.ShapeArgumentPositions)
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Errorf("mutated contract unexpectedly valid: %+v", candidate)
			}
		})
	}
}

func TestBoundedScratchFlowContractValid(t *testing.T) {
	t.Parallel()
	valid := BoundedScratchFlowContract{
		Name:                                  "bounded-gelu",
		Allocator:                             "example.com/project.Device.Alloc",
		AllocatorCapacityArg:                  1,
		Producer:                              "example.com/project.Device.Produce",
		ProducerBufferArg:                     1,
		ProducerActiveArg:                     2,
		Consumer:                              "example.com/project.Device.Activate",
		ConsumerBufferArg:                     1,
		Observers:                             []BoundedScratchObserverContract{{Function: "example.com/project.Device.Observe", BufferArg: 1, ActiveArg: 2}},
		AllocatorReturnsFreshOwned:            true,
		ProducerWritesOnlyActivePrefix:        true,
		ConsumerTraversesFullCapacity:         true,
		ObserversReadOnlyActivePrefix:         true,
		CallsDoNotRetainBuffer:                true,
		CallsExecuteSynchronously:             true,
		InactiveTailNotRequired:               true,
		Replacement:                           "example.com/project.Device.BiasGELU",
		ReplacementMatchesComposition:         true,
		ReplacementPreservesActivePrefixBits:  true,
		ReplacementLeavesInactiveTail:         true,
		ReplacementPreservesErrorsSideEffects: true,
		ReplacementPreservesProviderFallback:  true,
		ReplacementRejectsUnsupported:         true,
		ReplacementFailureUnmodified:          true,
	}
	if !valid.Valid() {
		t.Fatal("complete bounded-scratch contract is invalid")
	}
	cases := map[string]func(*BoundedScratchFlowContract){
		"same buffer and extent role": func(contract *BoundedScratchFlowContract) { contract.ProducerActiveArg = contract.ProducerBufferArg },
		"missing observer":            func(contract *BoundedScratchFlowContract) { contract.Observers = nil },
		"partial configured values":   func(contract *BoundedScratchFlowContract) { contract.ConfiguredCapacityElements = 1024 },
		"bad configured ratio": func(contract *BoundedScratchFlowContract) {
			contract.ConfiguredCapacityElements, contract.ConfiguredActiveElements = 16, 16
		},
		"partial replacement promise": func(contract *BoundedScratchFlowContract) { contract.Replacement = "" },
		"missing base guarantee":      func(contract *BoundedScratchFlowContract) { contract.CallsExecuteSynchronously = false },
	}
	for name, mutate := range cases {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			candidate.Observers = slices.Clone(valid.Observers)
			mutate(&candidate)
			if candidate.Valid() {
				t.Errorf("mutated contract unexpectedly valid: %+v", candidate)
			}
		})
	}
}

func TestSchedulerTileGrainContractValid(t *testing.T) {
	t.Parallel()
	valid := SchedulerTileGrainContract{
		Name:               "attention-bands",
		Scheduler:          "example.com/project.schedule",
		GrainConstant:      "example.com/project.bandRows",
		Band:               "example.com/project.band",
		BandRowsArgument:   2,
		KernelEntry:        "example.com/project.kernelRows",
		KernelRowsArgument: 3,
		RepeatedFullTasks:  true,
		Variants: []SchedulerTileGrainVariant{
			{Name: "amd64", GOOS: "linux", GOARCH: "amd64", TileHeight: 6, TileRouter: "example.com/project.kernelRows", TileRouterRowsArgument: 3, TiledKernel: "example.com/project.tile6", ScalarFallback: "example.com/project.scalar", ScalarFallbackRowsArgument: 3, KernelEntryRoutesFullTiles: true, ScalarFallbackHandlesTileRemainder: true},
			{Name: "arm64", GOOS: "darwin", GOARCH: "arm64", BuildTags: []string{"goexperiment.simd"}, TileHeight: 4, TileRouter: "example.com/project.kernelRows", TileRouterRowsArgument: 3, TiledKernel: "example.com/project.tile4", ScalarFallback: "example.com/project.scalar", ScalarFallbackRowsArgument: 3, KernelEntryRoutesFullTiles: true, ScalarFallbackHandlesTileRemainder: true},
		},
	}
	if !valid.Valid() {
		t.Fatal("complete scheduler tile-grain contract is invalid")
	}
	synchronous := valid
	synchronous.SynchronousRunner = "example.com/project.parallelWork"
	synchronous.SynchronousRunnerWorkArgument = 3
	synchronous.SynchronousRunnerExecutesBeforeReturn = true
	if !synchronous.Valid() {
		t.Fatal("complete synchronous-runner scheduler tile-grain contract is invalid")
	}
	tests := map[string]func(*SchedulerTileGrainContract){
		"single architecture": func(contract *SchedulerTileGrainContract) { contract.Variants = contract.Variants[:1] },
		"same architecture":   func(contract *SchedulerTileGrainContract) { contract.Variants[1].GOARCH = "amd64" },
		"missing os":          func(contract *SchedulerTileGrainContract) { contract.Variants[1].GOOS = "" },
		"invalid build tag":   func(contract *SchedulerTileGrainContract) { contract.Variants[1].BuildTags = []string{"bad tag"} },
		"duplicate build tag": func(contract *SchedulerTileGrainContract) { contract.Variants[1].BuildTags = []string{"simd", "simd"} },
		"one-row tile":        func(contract *SchedulerTileGrainContract) { contract.Variants[1].TileHeight = 1 },
		"missing scalar promise": func(contract *SchedulerTileGrainContract) {
			contract.Variants[1].ScalarFallbackHandlesTileRemainder = false
		},
		"missing scalar row role": func(contract *SchedulerTileGrainContract) {
			contract.Variants[1].ScalarFallbackRowsArgument = 0
		},
		"not repeated": func(contract *SchedulerTileGrainContract) { contract.RepeatedFullTasks = false },
		"different package": func(contract *SchedulerTileGrainContract) {
			contract.Variants[1].TiledKernel = "other.example/project.tile4"
		},
		"method id": func(contract *SchedulerTileGrainContract) { contract.Scheduler = "example.com/project.Device.schedule" },
		"runner without role": func(contract *SchedulerTileGrainContract) {
			contract.SynchronousRunner = "example.com/project.parallelWork"
			contract.SynchronousRunnerExecutesBeforeReturn = true
		},
		"runner without synchronous promise": func(contract *SchedulerTileGrainContract) {
			contract.SynchronousRunner = "example.com/project.parallelWork"
			contract.SynchronousRunnerWorkArgument = 3
		},
		"runner in different package": func(contract *SchedulerTileGrainContract) {
			contract.SynchronousRunner = "other.example/project.parallelWork"
			contract.SynchronousRunnerWorkArgument = 3
			contract.SynchronousRunnerExecutesBeforeReturn = true
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			candidate.Variants = cloneSchedulerTileGrainContracts([]SchedulerTileGrainContract{valid})[0].Variants
			mutate(&candidate)
			if candidate.Valid() {
				t.Fatal("incomplete scheduler tile-grain contract is valid")
			}
		})
	}
}

func TestReceiverStagingContractValid(t *testing.T) {
	t.Parallel()
	valid := ReceiverStagingContract{
		Name:                                 "prefill",
		CandidateMethod:                      "example.com/project.Decoder.StepN",
		Overwrite:                            "example.com/project.Decoder.GatherInto",
		OverwriteKind:                        ReceiverStagingCallMethod,
		OverwriteArg:                         0,
		Consumer:                             "example.com/project.Device.Upload",
		ConsumerKind:                         ReceiverStagingCallMethod,
		ConsumerArg:                          0,
		LifecycleMethod:                      "example.com/project.Decoder.Release",
		MaxRetainedBytes:                     64 << 10,
		ReceiverCallsSequential:              true,
		OverwriteWritesAllBeforeRead:         true,
		OverwriteCompletesBeforeReturn:       true,
		OverwriteDoesNotRetainArgument:       true,
		OverwriteAccessesOnlyArgumentLength:  true,
		OverwriteIgnoresCapacityAndIdentity:  true,
		OverwritePreservesExtentInputs:       true,
		ExtentIsNonNegativeAndNonOverflowing: true,
		ConsumerCompletesBeforeReturn:        true,
		ConsumerDoesNotRetainArgument:        true,
		LifecycleEndsReceiverUse:             true,
		ContentsMayPersistUntilLifecycle:     true,
	}
	if !valid.Valid() {
		t.Fatal("complete receiver staging contract is invalid")
	}

	tests := []struct {
		name   string
		mutate func(*ReceiverStagingContract)
	}{
		{"blank name", func(c *ReceiverStagingContract) { c.Name = "" }},
		{"invalid candidate", func(c *ReceiverStagingContract) { c.CandidateMethod = "StepN" }},
		{"invalid overwrite", func(c *ReceiverStagingContract) { c.Overwrite = "GatherInto" }},
		{"missing overwrite kind", func(c *ReceiverStagingContract) { c.OverwriteKind = "" }},
		{"negative overwrite arg", func(c *ReceiverStagingContract) { c.OverwriteArg = -1 }},
		{"invalid consumer", func(c *ReceiverStagingContract) { c.Consumer = "Upload" }},
		{"missing consumer kind", func(c *ReceiverStagingContract) { c.ConsumerKind = "" }},
		{"negative consumer arg", func(c *ReceiverStagingContract) { c.ConsumerArg = -1 }},
		{"invalid lifecycle", func(c *ReceiverStagingContract) { c.LifecycleMethod = "Release" }},
		{"missing cap", func(c *ReceiverStagingContract) { c.MaxRetainedBytes = 0 }},
		{"negative cap", func(c *ReceiverStagingContract) { c.MaxRetainedBytes = -1 }},
		{"unreasonable cap", func(c *ReceiverStagingContract) { c.MaxRetainedBytes = MaxReceiverStagingBytes + 1 }},
		{"concurrent receiver", func(c *ReceiverStagingContract) { c.ReceiverCallsSequential = false }},
		{"overwriter reads first", func(c *ReceiverStagingContract) { c.OverwriteWritesAllBeforeRead = false }},
		{"overwriter incomplete on return", func(c *ReceiverStagingContract) { c.OverwriteCompletesBeforeReturn = false }},
		{"overwriter retains", func(c *ReceiverStagingContract) { c.OverwriteDoesNotRetainArgument = false }},
		{"overwriter accesses capacity", func(c *ReceiverStagingContract) { c.OverwriteAccessesOnlyArgumentLength = false }},
		{"overwriter observes identity", func(c *ReceiverStagingContract) { c.OverwriteIgnoresCapacityAndIdentity = false }},
		{"overwriter mutates extent inputs", func(c *ReceiverStagingContract) { c.OverwritePreservesExtentInputs = false }},
		{"unchecked extent", func(c *ReceiverStagingContract) { c.ExtentIsNonNegativeAndNonOverflowing = false }},
		{"asynchronous consumer", func(c *ReceiverStagingContract) { c.ConsumerCompletesBeforeReturn = false }},
		{"retaining consumer", func(c *ReceiverStagingContract) { c.ConsumerDoesNotRetainArgument = false }},
		{"missing lifecycle ownership", func(c *ReceiverStagingContract) { c.LifecycleEndsReceiverUse = false }},
		{"sensitive eager-clear contents", func(c *ReceiverStagingContract) { c.ContentsMayPersistUntilLifecycle = false }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Error("incomplete/unsafe receiver staging contract is valid")
			}
		})
	}

	direct := valid
	direct.Overwrite = ""
	direct.OverwriteKind = ""
	direct.OverwriteArg = 0
	direct.OverwriteWritesAllBeforeRead = false
	direct.OverwriteCompletesBeforeReturn = false
	direct.OverwriteDoesNotRetainArgument = false
	direct.OverwriteAccessesOnlyArgumentLength = false
	direct.OverwriteIgnoresCapacityAndIdentity = false
	direct.OverwritePreservesExtentInputs = false
	direct.ExtentIsNonNegativeAndNonOverflowing = false
	if !direct.Valid() {
		t.Error("inline source-proven overwrite contract is invalid")
	}
}

func TestNativeSnapshotStringCopyContractValid(t *testing.T) {
	t.Parallel()
	valid := NativeSnapshotStringCopyContract{
		Name:                                 "labels",
		CandidateCallable:                    "example.com/project.Recorder.Profile",
		CandidateKind:                        NativeSnapshotCallMethod,
		DestinationResultPosition:            1,
		DestinationSliceField:                "Events",
		DestinationStringField:               "Label",
		AcquireCallable:                      "C.profile_snapshot",
		AcquireKind:                          NativeSnapshotCallCgo,
		RecordsOutArgumentPosition:           2,
		CountOutArgumentPosition:             3,
		AcquireStatusResultPosition:          1,
		NativeStringField:                    "label",
		CopyKind:                             NativeSnapshotCopyGoString,
		CopyPointerArgumentPosition:          1,
		CopyStringResultPosition:             1,
		LifecycleCallable:                    "example.com/project.Recorder.Free",
		LifecycleKind:                        NativeSnapshotCallMethod,
		SnapshotStableThroughCandidateReturn: true,
		SnapshotNotMutatedDuringExtraction:   true,
		ExtractionIsSynchronous:              true,
		CopyReturnsExactOwnedString:          true,
		ReturnedStringsOutliveLifecycle:      true,
		ExactContentCheckRequired:            true,
	}
	if !valid.Valid() {
		t.Fatal("complete native snapshot string-copy contract is invalid")
	}
	tests := []struct {
		name   string
		mutate func(*NativeSnapshotStringCopyContract)
	}{
		{"blank name", func(c *NativeSnapshotStringCopyContract) { c.Name = "" }},
		{"invalid candidate", func(c *NativeSnapshotStringCopyContract) { c.CandidateCallable = "Profile" }},
		{"cgo candidate", func(c *NativeSnapshotStringCopyContract) { c.CandidateKind = NativeSnapshotCallCgo }},
		{"zero destination result", func(c *NativeSnapshotStringCopyContract) { c.DestinationResultPosition = 0 }},
		{"dotted destination field", func(c *NativeSnapshotStringCopyContract) { c.DestinationSliceField = "Profile.Events" }},
		{"same acquisition args", func(c *NativeSnapshotStringCopyContract) { c.CountOutArgumentPosition = c.RecordsOutArgumentPosition }},
		{"fake cgo acquisition", func(c *NativeSnapshotStringCopyContract) { c.AcquireCallable = "profile_snapshot" }},
		{"zero status result", func(c *NativeSnapshotStringCopyContract) { c.AcquireStatusResultPosition = 0 }},
		{"GoString length", func(c *NativeSnapshotStringCopyContract) { c.CopyLengthArgumentPosition = 2 }},
		{"negative copy length", func(c *NativeSnapshotStringCopyContract) {
			c.CopyKind = NativeSnapshotCallFunction
			c.CopyCallable = "example.com/project.ownedString"
			c.NativeLengthField = "length"
			c.CopyLengthArgumentPosition = -1
		}},
		{"GoStringN without length field", func(c *NativeSnapshotStringCopyContract) {
			c.CopyKind = NativeSnapshotCopyGoStringN
			c.CopyLengthArgumentPosition = 2
		}},
		{"overlapping pointer and length", func(c *NativeSnapshotStringCopyContract) {
			c.CopyKind = NativeSnapshotCopyGoStringN
			c.NativeLengthField = "length"
			c.CopyLengthArgumentPosition = c.CopyPointerArgumentPosition
		}},
		{"copy bytes kind", func(c *NativeSnapshotStringCopyContract) { c.CopyKind = NativeSnapshotCallKind("c-go-bytes") }},
		{"missing lifecycle", func(c *NativeSnapshotStringCopyContract) { c.LifecycleCallable = "" }},
		{"unstable snapshot", func(c *NativeSnapshotStringCopyContract) { c.SnapshotStableThroughCandidateReturn = false }},
		{"mutable extraction", func(c *NativeSnapshotStringCopyContract) { c.SnapshotNotMutatedDuringExtraction = false }},
		{"asynchronous extraction", func(c *NativeSnapshotStringCopyContract) { c.ExtractionIsSynchronous = false }},
		{"borrowed copy", func(c *NativeSnapshotStringCopyContract) { c.CopyReturnsExactOwnedString = false }},
		{"lifecycle ownership missing", func(c *NativeSnapshotStringCopyContract) { c.ReturnedStringsOutliveLifecycle = false }},
		{"collision check missing", func(c *NativeSnapshotStringCopyContract) { c.ExactContentCheckRequired = false }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Errorf("mutated contract is valid: %+v", candidate)
			}
		})
	}
	wrapper := valid
	wrapper.CopyKind = NativeSnapshotCallFunction
	wrapper.CopyCallable = "example.com/project.ownedStringN"
	wrapper.NativeLengthField = "length"
	wrapper.CopyLengthArgumentPosition = 2
	if !wrapper.Valid() {
		t.Fatal("complete typed GoStringN wrapper contract is invalid")
	}
}

func TestReusableOneShotWrapperContractValid(t *testing.T) {
	t.Parallel()
	valid := ReusableOneShotWrapperContract{
		Name:                              "recorder-shell",
		WrapperType:                       "example.com/backend.Recorder",
		ProviderType:                      "example.com/backend.Device",
		Acquisition:                       "example.com/model.CommandFactory.NewRecorder",
		ConcreteAcquisition:               "example.com/backend.Device.NewRecorder",
		WrapperConstructor:                "example.com/backend.NewRecorder",
		Terminal:                          ReusableOneShotMethod{Static: "example.com/model.CommandRecorder.Free", Concrete: "example.com/backend.Recorder.Free"},
		Reset:                             ReusableOneShotMethod{Static: "example.com/model.CommandRecorder.Reset", Concrete: "example.com/backend.Recorder.Reset"},
		FreshNativeHandleFactory:          "example.com/backend.Device.NewCommandBuffer",
		NativeHandleField:                 "example.com/backend.Recorder.commandBuffer",
		MutableStateFields:                []string{"example.com/backend.Recorder.encoder", "example.com/backend.Recorder.committed"},
		AllowedSynchronousUses:            []ReusableOneShotMethod{{Static: "example.com/model.CommandRecorder.Encode", Concrete: "example.com/backend.Recorder.Encode"}},
		AcquisitionWrapperResult:          1,
		AcquisitionStatusResult:           2,
		ConstructorWrapperResult:          1,
		ResetStatusResult:                 2,
		AcquisitionFailureMode:            ReusableOneShotAcquisitionNilError,
		ResetFailureState:                 ReusableOneShotResetErrorEmptySafe,
		SlotBound:                         1,
		AcquisitionCreatesFreshGoWrapper:  true,
		AcquisitionHasExactDynamicWrapper: true,
		FailedAcquisitionHasNoGeneration:  true,
		NativeHandleIsOneShot:             true,
		ResetAlwaysCreatesFreshHandle:     true,
		TerminalSynchronouslyReleases:     true,
		TerminalClearsHandle:              true,
		TerminalIsIdempotent:              true,
		ResetClearsMutableState:           true,
		UsesExecuteSynchronously:          true,
		UsesDoNotRetainGeneration:         true,
		NoStaleGenerationReferences:       true,
		OwnerAccessIsNonConcurrent:        true,
		ProviderFallbackIsPreserved:       true,
		FailuresAndPanicsArePreserved:     true,
	}
	if !valid.Valid() {
		t.Fatal("complete reusable one-shot wrapper contract is invalid")
	}
	tests := []struct {
		name   string
		mutate func(*ReusableOneShotWrapperContract)
	}{
		{"blank name", func(c *ReusableOneShotWrapperContract) { c.Name = "" }},
		{"padded name", func(c *ReusableOneShotWrapperContract) { c.Name = " recorder-shell" }},
		{"bad wrapper type", func(c *ReusableOneShotWrapperContract) { c.WrapperType = "Recorder" }},
		{"bad provider type", func(c *ReusableOneShotWrapperContract) { c.ProviderType = "Device" }},
		{"bad acquisition", func(c *ReusableOneShotWrapperContract) { c.Acquisition = "NewRecorder" }},
		{"bad concrete acquisition", func(c *ReusableOneShotWrapperContract) { c.ConcreteAcquisition = "NewRecorder" }},
		{"bad constructor", func(c *ReusableOneShotWrapperContract) { c.WrapperConstructor = "NewRecorder" }},
		{"bad native factory", func(c *ReusableOneShotWrapperContract) { c.FreshNativeHandleFactory = "NewHandle" }},
		{"bad native field", func(c *ReusableOneShotWrapperContract) { c.NativeHandleField = "handle" }},
		{"same result roles", func(c *ReusableOneShotWrapperContract) { c.AcquisitionStatusResult = 1 }},
		{"missing mutable state", func(c *ReusableOneShotWrapperContract) { c.MutableStateFields = nil }},
		{"duplicate mutable state", func(c *ReusableOneShotWrapperContract) { c.MutableStateFields[1] = c.MutableStateFields[0] }},
		{"handle repeated as mutable state", func(c *ReusableOneShotWrapperContract) { c.MutableStateFields[0] = c.NativeHandleField }},
		{"missing use", func(c *ReusableOneShotWrapperContract) { c.AllowedSynchronousUses = nil }},
		{"terminal reused as use", func(c *ReusableOneShotWrapperContract) { c.AllowedSynchronousUses[0] = c.Terminal }},
		{"reset equals terminal", func(c *ReusableOneShotWrapperContract) { c.Reset = c.Terminal }},
		{"acquisition reused as terminal", func(c *ReusableOneShotWrapperContract) { c.Terminal.Static = c.Acquisition }},
		{"factory reused as reset", func(c *ReusableOneShotWrapperContract) { c.Reset.Concrete = c.FreshNativeHandleFactory }},
		{"use roles overlap", func(c *ReusableOneShotWrapperContract) {
			c.AllowedSynchronousUses = append(c.AllowedSynchronousUses, c.AllowedSynchronousUses[0])
		}},
		{"bad slot bound", func(c *ReusableOneShotWrapperContract) { c.SlotBound = 3 }},
		{"fallible two-slot", func(c *ReusableOneShotWrapperContract) { c.SlotBound = 2 }},
		{"unknown acquisition mode", func(c *ReusableOneShotWrapperContract) { c.AcquisitionFailureMode = "boolean" }},
		{"unknown reset state", func(c *ReusableOneShotWrapperContract) { c.ResetFailureState = "live-on-error" }},
		{"missing fresh wrapper promise", func(c *ReusableOneShotWrapperContract) { c.AcquisitionCreatesFreshGoWrapper = false }},
		{"missing exact dynamic type", func(c *ReusableOneShotWrapperContract) { c.AcquisitionHasExactDynamicWrapper = false }},
		{"failed acquisition retains generation", func(c *ReusableOneShotWrapperContract) { c.FailedAcquisitionHasNoGeneration = false }},
		{"native handle reusable", func(c *ReusableOneShotWrapperContract) { c.NativeHandleIsOneShot = false }},
		{"reset reuses native handle", func(c *ReusableOneShotWrapperContract) { c.ResetAlwaysCreatesFreshHandle = false }},
		{"terminal asynchronous", func(c *ReusableOneShotWrapperContract) { c.TerminalSynchronouslyReleases = false }},
		{"terminal retains handle", func(c *ReusableOneShotWrapperContract) { c.TerminalClearsHandle = false }},
		{"terminal non-idempotent", func(c *ReusableOneShotWrapperContract) { c.TerminalIsIdempotent = false }},
		{"reset retains mutable state", func(c *ReusableOneShotWrapperContract) { c.ResetClearsMutableState = false }},
		{"asynchronous use", func(c *ReusableOneShotWrapperContract) { c.UsesExecuteSynchronously = false }},
		{"use retains generation", func(c *ReusableOneShotWrapperContract) { c.UsesDoNotRetainGeneration = false }},
		{"stale references", func(c *ReusableOneShotWrapperContract) { c.NoStaleGenerationReferences = false }},
		{"concurrent owner", func(c *ReusableOneShotWrapperContract) { c.OwnerAccessIsNonConcurrent = false }},
		{"fallback not preserved", func(c *ReusableOneShotWrapperContract) { c.ProviderFallbackIsPreserved = false }},
		{"panic behavior changed", func(c *ReusableOneShotWrapperContract) { c.FailuresAndPanicsArePreserved = false }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			candidate.MutableStateFields = slices.Clone(valid.MutableStateFields)
			candidate.AllowedSynchronousUses = slices.Clone(valid.AllowedSynchronousUses)
			test.mutate(&candidate)
			if candidate.Valid() {
				t.Error("incomplete reusable one-shot wrapper contract is valid")
			}
		})
	}

	infallible := valid
	infallible.AcquisitionFailureMode = ReusableOneShotAcquisitionInfallible
	infallible.AcquisitionStatusResult = 0
	infallible.ResetFailureState = ReusableOneShotResetInfallibleEmpty
	infallible.ResetStatusResult = 0
	infallible.SlotBound = 2
	if !infallible.Valid() {
		t.Error("complete infallible two-slot contract is invalid")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	// Valid YAML round-trips into the typed config.
	good := filepath.Join(dir, "perfscan.yaml")
	os.WriteFile(good, []byte("cacheLineBytes: 128\nelementAccessors: [AtF64, SetF64]\nallocatorFuncs: [Zeros]\n"), 0o644)
	c, err := Load(good)
	if err != nil {
		t.Fatalf("Load(valid) error: %v", err)
	}
	if c.CacheLineBytes != 128 || len(c.ElementAccessors) != 2 || c.ElementAccessors[0] != "AtF64" || c.AllocatorFuncs[0] != "Zeros" {
		t.Errorf("Load parsed wrong config: %+v", c)
	}

	// A missing file is an error.
	if _, err := Load(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Error("Load(missing) = nil error, want error")
	}

	// Malformed YAML is an error naming the file.
	bad := filepath.Join(dir, "bad.yaml")
	os.WriteFile(bad, []byte("elementAccessors: [unterminated\n"), 0o644)
	_, err = Load(bad)
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("Load(malformed) error = %v, want it to name bad.yaml", err)
	}
}

func TestDiscover(t *testing.T) {
	// Layout:
	//   root/               (has go.mod — the module root)
	//     perfscan.yaml     (the config)
	//     a/b/              (a deep package dir)
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// No config anywhere yet -> zero config, empty path (the walk stops at the
	// go.mod-bearing module root).
	if c, p := Discover(deep); p != "" || len(c.ElementAccessors) != 0 {
		t.Errorf("Discover(no config) = %+v,%q want zero,\"\"", c, p)
	}

	// A config in the module root IS found from a deep package dir: the name
	// check runs before the go.mod stop in the same directory.
	cfg := filepath.Join(root, "perfscan.yaml")
	if err := os.WriteFile(cfg, []byte("allocatorFuncs: [Zeros]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, p := Discover(deep)
	if p != cfg {
		t.Fatalf("Discover(with root config) path = %q, want %q", p, cfg)
	}
	if !c.Compile().AllocatorFuncs["Zeros"] {
		t.Errorf("Discover returned a config missing its vocabulary: %+v", c)
	}

	// The go.mod stop prevents escaping the module: a config ABOVE the module
	// root is not discovered. Self-contained layout: base/perfscan.yaml,
	// base/mod/go.mod, base/mod/pkg — discovering from pkg must stop at
	// mod/go.mod and never reach base/perfscan.yaml.
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "perfscan.yaml"), []byte("allocatorFuncs: [Escaped]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(base, "mod", "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "mod", "go.mod"), []byte("module y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, p := Discover(pkg); p != "" || len(c.AllocatorFuncs) != 0 {
		t.Errorf("Discover must stop at mod/go.mod, not escape to base/perfscan.yaml; got %+v,%q", c, p)
	}
}

func TestSetForTesting(t *testing.T) {
	restore := SetForTesting(Config{CacheLineBytes: 128, FanOutHelpers: []string{"parallelFor"}})
	if Current().CacheLineBytes != 128 {
		t.Error("SetForTesting did not install numeric tuning")
	}
	if !Current().FanOutHelpers["parallelFor"] {
		t.Error("SetForTesting did not install the vocabulary")
	}
	restore()
	if Current().FanOutHelpers["parallelFor"] {
		t.Error("restore() did not revert the vocabulary")
	}
}

func TestResidentWeightBenchmarkContractsCompileClone(t *testing.T) {
	t.Parallel()
	configuration := Config{ResidentWeightBenchmarkContracts: []ResidentWeightBenchmarkContract{{AcceleratorCallable: "example.com/backend.Device.Launch", WeightArgument: 2}}}
	compiled := configuration.Compile()
	configuration.ResidentWeightBenchmarkContracts[0].WeightArgument = 3
	if got := compiled.ResidentWeightBenchmarkContracts[0].WeightArgument; got != 2 {
		t.Fatalf("Compile did not clone resident-weight contracts: got %d", got)
	}
}

func TestUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "perfscan.yaml")

	// elementAccesors (missing an 's') and bogus are unknown; the two correctly
	// spelled keys are not. Result is sorted.
	os.WriteFile(p, []byte("elementAccessors: [A]\nelementAccesors: [B]\nbogus: 1\nallocatorFuncs: [Z]\n"), 0o644)
	got := UnknownKeys(p)
	if len(got) != 2 || got[0] != "bogus" || got[1] != "elementAccesors" {
		t.Errorf("UnknownKeys = %v, want [bogus elementAccesors]", got)
	}

	// All-known config -> no unknowns.
	os.WriteFile(p, []byte("_comment: metadata\nelementAccessors: [A]\nfanOutHelpers: [F]\n"), 0o644)
	if got := UnknownKeys(p); len(got) != 0 {
		t.Errorf("UnknownKeys(all known) = %v, want none", got)
	}

	// Missing file and a non-mapping YAML document are best-effort nil.
	if got := UnknownKeys(filepath.Join(dir, "nope.yaml")); got != nil {
		t.Errorf("UnknownKeys(missing) = %v, want nil", got)
	}
	seq := filepath.Join(dir, "seq.yaml")
	os.WriteFile(seq, []byte("- a\n- b\n"), 0o644)
	if got := UnknownKeys(seq); got != nil {
		t.Errorf("UnknownKeys(non-mapping) = %v, want nil", got)
	}
}

func TestGoAICurrentVocabularyCompatibility(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "perfscan.json")
	data := []byte(`{
  "_comment": "schema compatibility fixture",
	"cacheLineBytes": 128,
  "pureComputeFuncs": ["forward"],
  "layoutOpConstants": ["OpSlice", "OpConcat"],
  "pointerTypeNames": ["Tensor", "Storage"],
  "variadicDispatchWrappers": ["exec", "visExecN"],
  "topKSelectorFuncs": ["topKIndices"],
  "topKOneContracts": [{"name":"resident", "function":"example.com/goai.DeviceBuffer.TopKN", "kind":"method", "kArgPosition":2, "indicesResultPosition":1}],
  "nativeSnapshotStringCopyContracts": [{"name":"labels", "candidateCallable":"example.com/goai.Recorder.Profile", "candidateKind":"method", "destinationResultPosition":1, "destinationStringField":"Label", "acquireCallable":"C.snapshot", "acquireKind":"cgo", "recordsOutArgumentPosition":1, "countOutArgumentPosition":2, "acquireStatusResultPosition":1, "nativeStringField":"label", "copyKind":"c-go-string", "copyPointerArgumentPosition":1, "copyStringResultPosition":1, "lifecycleCallable":"example.com/goai.Recorder.Free", "lifecycleKind":"method", "snapshotStableThroughCandidateReturn":true, "snapshotNotMutatedDuringExtraction":true, "extractionIsSynchronous":true, "copyReturnsExactOwnedString":true, "returnedStringsOutliveLifecycle":true, "exactContentCheckRequired":true}],
  "inputViewFuncs": ["f64Data", "f32Data"],
  "outputViewFuncs": ["outF64", "outF32"],
  "referenceBackendPkg": "ref",
  "optimizedBackendPkgs": ["cpu"],
  "kernelRegisterFuncs": ["add"]
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if unknown := UnknownKeys(path); len(unknown) != 0 {
		t.Fatalf("current GoAI vocabulary has unknown keys: %v", unknown)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(current GoAI vocabulary): %v", err)
	}
	if cfg.Comment == "" || cfg.CacheLineBytes != 128 || cfg.ReferenceBackendPkg != "ref" ||
		len(cfg.PureComputeFuncs) != 1 || len(cfg.LayoutOpConstants) != 2 ||
		len(cfg.PointerTypeNames) != 2 || len(cfg.VariadicDispatchWrappers) != 2 ||
		len(cfg.TopKSelectorFuncs) != 1 || len(cfg.TopKOneContracts) != 1 ||
		len(cfg.NativeSnapshotStringCopyContracts) != 1 ||
		cfg.TopKOneContracts[0].Kind != TopKOneContractMethod || len(cfg.InputViewFuncs) != 2 ||
		len(cfg.OutputViewFuncs) != 2 || len(cfg.OptimizedBackendPkgs) != 1 ||
		len(cfg.KernelRegisterFuncs) != 1 {
		t.Fatalf("current GoAI vocabulary did not round-trip: %+v", cfg)
	}
	sets := cfg.Compile()
	if sets.CacheLineBytes != 128 || !sets.PureComputeFuncs["forward"] || !sets.LayoutOpConstants["OpSlice"] ||
		!sets.PointerTypeNames["Tensor"] || !sets.VariadicDispatchWrappers["exec"] ||
		!sets.TopKSelectorFuncs["topKIndices"] || len(sets.TopKOneContracts) != 1 || !sets.InputViewFuncs["f64Data"] ||
		len(sets.NativeSnapshotStringCopyContracts) != 1 ||
		!sets.OutputViewFuncs["outF64"] || sets.ReferenceBackendPkg != "ref" ||
		!sets.OptimizedBackendPkgs["cpu"] || !sets.KernelRegisterFuncs["add"] {
		t.Fatalf("Compile lost current GoAI vocabulary: %+v", sets)
	}
}

func TestRecursiveMetadataIgnoreContractValidation(t *testing.T) {
	t.Parallel()
	valid := RecursiveMetadataIgnoreContract{Name: "records", Builder: "example.com/tool.newConfig", Matcher: "example.com/tool.config.ignoredBy", RootField: "root", IgnoreField: "ignoreParts", DirectoryName: ".records", AnyDepthIntent: true, NonEmbeddedMetadataOnly: true}
	if !valid.Valid() {
		t.Fatal("complete recursive metadata contract rejected")
	}
	tests := map[string]func(*RecursiveMetadataIgnoreContract){
		"no recursive intent":   func(c *RecursiveMetadataIgnoreContract) { c.AnyDepthIntent = false },
		"contents not reviewed": func(c *RecursiveMetadataIgnoreContract) { c.NonEmbeddedMetadataOnly = false },
		"nested directory":      func(c *RecursiveMetadataIgnoreContract) { c.DirectoryName = "a/.records" },
		"missing builder":       func(c *RecursiveMetadataIgnoreContract) { c.Builder = "" },
		"missing matcher":       func(c *RecursiveMetadataIgnoreContract) { c.Matcher = "" },
		"missing root field":    func(c *RecursiveMetadataIgnoreContract) { c.RootField = "" },
		"missing ignore field":  func(c *RecursiveMetadataIgnoreContract) { c.IgnoreField = "" },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contract := valid
			mutate(&contract)
			if contract.Valid() {
				t.Fatal("incomplete recursive metadata contract accepted")
			}
		})
	}
}

// TestExampleConfigIsValidAndGeneric pins the shipped template: it must parse,
// contain NO unknown keys (so it never drifts back to stale/renamed fields), and
// populate every documented field so it stays a complete, working reference.
func TestExampleConfigIsValidAndGeneric(t *testing.T) {
	t.Parallel()
	const path = "../docs/perfscan.example.yaml"
	if unk := UnknownKeys(path); len(unk) != 0 {
		t.Errorf("example config has unknown keys %v — every key must map to a Config field", unk)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load(example): %v", err)
	}
	fields := map[string]int{
		"ElementAccessors": len(c.ElementAccessors), "FastPathHelpers": len(c.FastPathHelpers),
		"SelectorPromotionSymbols": len(c.SelectorPromotionSymbols),
		"ElementCountMethods":      len(c.ElementCountMethods), "ShapeMethods": len(c.ShapeMethods),
		"IndexDecomposeFuncs": len(c.IndexDecomposeFuncs), "AllocatorFuncs": len(c.AllocatorFuncs),
		"PerElementVisitors": len(c.PerElementVisitors), "BulkCopyHelpers": len(c.BulkCopyHelpers),
		"VectorizedSiblingFuncs": len(c.VectorizedSiblingFuncs), "FanOutHelpers": len(c.FanOutHelpers),
		"DtypeMethods": len(c.DtypeMethods), "OutputBufferElemTypes": len(c.OutputBufferElemTypes),
		"CompiledResourceFuncs": len(c.CompiledResourceFuncs), "GPUReductionKernels": len(c.GPUReductionKernels),
		"PureComputeFuncs": len(c.PureComputeFuncs), "LayoutOpConstants": len(c.LayoutOpConstants),
		"PointerTypeNames": len(c.PointerTypeNames), "VariadicDispatchWrappers": len(c.VariadicDispatchWrappers),
		"TopKSelectorFuncs": len(c.TopKSelectorFuncs), "InputViewFuncs": len(c.InputViewFuncs),
		"TopKOneContracts":                  len(c.TopKOneContracts),
		"NativeSnapshotStringCopyContracts": len(c.NativeSnapshotStringCopyContracts),
		"SchedulerTileGrainContracts":       len(c.SchedulerTileGrainContracts),
		"OutputViewFuncs":                   len(c.OutputViewFuncs), "OptimizedBackendPkgs": len(c.OptimizedBackendPkgs),
		"KernelRegisterFuncs":             len(c.KernelRegisterFuncs),
		"InPlaceFusionContracts":          len(c.InPlaceFusionContracts),
		"BoundedScratchFlowContracts":     len(c.BoundedScratchFlowContracts),
		"ReceiverStagingContracts":        len(c.ReceiverStagingContracts),
		"ReusableOneShotWrapperContracts": len(c.ReusableOneShotWrapperContracts),
		"ReusableResultLoopContracts":     len(c.ReusableResultLoopContracts),
		"RecorderResidualAddContracts":    len(c.RecorderResidualAddContracts),
		"RowLocalSparseGatherContracts":   len(c.RowLocalSparseGatherContracts),

		"TinySynchronousAcceleratorScreenContracts": len(c.TinySynchronousAcceleratorScreenContracts),
		"ForwardLossBackwardGraphContracts":         len(c.ForwardLossBackwardGraphContracts),
		"FragmentedAcceleratorObjectiveContracts":   len(c.FragmentedAcceleratorObjectiveContracts),
		"CrossStepAcceleratorResidencyContracts":    len(c.CrossStepAcceleratorResidencyContracts),
	}
	for name, n := range fields {
		if n == 0 {
			t.Errorf("example config leaves %s empty; the template should exercise every field", name)
		}
	}
	if c.Comment == "" || c.ReferenceBackendPkg == "" {
		t.Errorf("example config must populate _comment and referenceBackendPkg")
	}
	if c.CacheLineBytes <= 0 {
		t.Errorf("example config must populate cacheLineBytes")
	}
	if len(c.BoundedScratchFlowContracts) != 1 || !c.BoundedScratchFlowContracts[0].Valid() {
		t.Errorf("example config has invalid bounded-scratch contract: %+v", c.BoundedScratchFlowContracts)
	}
	if len(c.ReusableOneShotWrapperContracts) != 1 || !c.ReusableOneShotWrapperContracts[0].Valid() {
		t.Errorf("example config has invalid reusable one-shot wrapper contract: %+v", c.ReusableOneShotWrapperContracts)
	}
	if len(c.ReusableResultLoopContracts) != 1 || !c.ReusableResultLoopContracts[0].Valid() {
		t.Errorf("example config has invalid reusable-result loop contract: %+v", c.ReusableResultLoopContracts)
	}
	if len(c.RecorderResidualAddContracts) != 1 || !c.RecorderResidualAddContracts[0].Valid() {
		t.Errorf("example config has invalid recorder residual-add contract: %+v", c.RecorderResidualAddContracts)
	}
	if len(c.RowLocalSparseGatherContracts) != 1 || !c.RowLocalSparseGatherContracts[0].Valid() {
		t.Errorf("example config has invalid row-local sparse-gather contract: %+v", c.RowLocalSparseGatherContracts)
	}
	if len(c.TinySynchronousAcceleratorScreenContracts) != 1 || !c.TinySynchronousAcceleratorScreenContracts[0].Valid() {
		t.Errorf("example config has invalid tiny synchronous accelerator-screen contract: %+v", c.TinySynchronousAcceleratorScreenContracts)
	}
	if len(c.ForwardLossBackwardGraphContracts) != 1 || !c.ForwardLossBackwardGraphContracts[0].Valid() {
		t.Errorf("example config has invalid forward-loss-backward graph contract: %+v", c.ForwardLossBackwardGraphContracts)
	}
	if len(c.FragmentedAcceleratorObjectiveContracts) != 1 || !c.FragmentedAcceleratorObjectiveContracts[0].Valid() {
		t.Errorf("example config has invalid fragmented accelerator-objective contract: %+v", c.FragmentedAcceleratorObjectiveContracts)
	}
	if len(c.CrossStepAcceleratorResidencyContracts) != 1 || !c.CrossStepAcceleratorResidencyContracts[0].Valid() {
		t.Errorf("example config has invalid cross-step accelerator residency contract: %+v", c.CrossStepAcceleratorResidencyContracts)
	}
	if len(c.SchedulerTileGrainContracts) != 1 || !c.SchedulerTileGrainContracts[0].Valid() {
		t.Errorf("example config has invalid scheduler tile-grain contract: %+v", c.SchedulerTileGrainContracts)
	}
}
