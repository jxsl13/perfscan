package checks

import (
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6114(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6114TestAnalyzer(ps6114TestContracts()), "ps6114")
}

func TestPS6114ContractOrderIndependent(t *testing.T) {
	t.Parallel()
	contracts := ps6114TestContracts()
	slices.Reverse(contracts)
	analysistest.Run(t, analysistest.TestData(), ps6114TestAnalyzer(contracts), "ps6114")
}

func TestPS6114SilentWithoutConfig(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6114TestAnalyzer(nil), "ps6114silent")
}

func TestPS6114DuplicateContractsAreAmbiguous(t *testing.T) {
	t.Parallel()
	contract := ps6114TestContract("ps6114.ownerRange")
	duplicate := contract
	duplicate.Name = "second-name"
	if got := ps6114Contracts([]config.RowLocalSparseGatherContract{contract, duplicate}); len(got) != 0 {
		t.Fatalf("duplicate shape produced %d usable contracts", len(got))
	}
}

func TestPS6114DuplicateSitesAreAmbiguous(t *testing.T) {
	t.Parallel()
	contract := ps6114TestContract("ps6114.ownerRange")
	second := contract
	second.Name = "second-name"
	second.Transform = "ps6114.OtherTransform.Forward"
	if got := ps6114Contracts([]config.RowLocalSparseGatherContract{contract, second}); len(got) != 0 {
		t.Fatalf("duplicate configured site produced %d usable contracts", len(got))
	}
}

func ps6114TestAnalyzer(contracts []config.RowLocalSparseGatherContract) *analysis.Analyzer {
	analyzer := *PS6114.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) { return runPS6114WithContracts(pass, contracts) }
	return &analyzer
}

func ps6114TestContracts() []config.RowLocalSparseGatherContract {
	sites := []string{
		"ownerRange", "Model.ownerMethodRange", "assignmentClassic", "extraTransformUse", "wrongAxis", "wrongEnd", "wrongIndex",
		"collectionAppend", "conditionalLoop", "breakLoop", "swallowedGatherError", "interfaceTransform",
		"currentFusedFallback", "unstableStride", "strideOne", "zeroBatch", "deadPath",
		"postTransformAlias", "postCollectionEscape", "wrongConcatAxis", "wrongConcatOperation", "capturedBatch",
		"receiverExposure", "terminalSideEffect", "wrongErrorReturn", "constantGeometry", "overflowGeometry",
		"globalGeometry", "safeOldVersionRead", "preAddressOutput", "preCaptureOutput", "packageOutputCandidate",
	}
	contracts := make([]config.RowLocalSparseGatherContract, 0, len(sites))
	for _, site := range sites {
		contract := ps6114TestContract("ps6114." + site)
		contract.Name = site
		if site == "interfaceTransform" {
			contract.Transform = "ps6114.rowTransform.Forward"
		}
		if site == "currentFusedFallback" {
			contract.ExistingFusedCapabilityFallback = true
		}
		contracts = append(contracts, contract)
	}
	return contracts
}

func ps6114TestContract(site string) config.RowLocalSparseGatherContract {
	return config.RowLocalSparseGatherContract{
		Name: "row-local", ConfiguredSite: site,
		Transform: "ps6114.Transform.Forward", Gather: "ps6114.gather", Concat: "ps6114.concat",
		SliceOperation: "ps6114.sliceOp", SliceOperationValue: "1",
		ConcatOperation: "ps6114.concatOp", ConcatOperationValue: "2",
		SliceAttrsType: "ps6114.SliceAttrs", SliceAxisField: "ps6114.SliceAttrs.Axis",
		SliceStartField: "ps6114.SliceAttrs.Start", SliceEndField: "ps6114.SliceAttrs.End",
		ConcatAttrsType: "ps6114.ConcatAttrs", ConcatAxisField: "ps6114.ConcatAttrs.Axis",
		TransformInputArgument: 2, GatherOperationArgument: 2, GatherAttrsArgument: 3,
		GatherInputArgument: 4, ConcatOperationArgument: 2, ConcatCollectionArgument: 3, ConcatAttrsArgument: 4,
		PackedRowsEqualBatchTimesStride: true, BatchPositive: true, StrideGreaterThanOne: true,
		NativeIntArithmeticNoOverflow: true, TransformRowsIndependent: true,
		TransformPreservesRowOrderAndWidth: true, TransformParametersImmutable: true,
		TransformDoesNotMutateInput: true, TransformInputOutputDoNotAlias: true,
		TransformDoesNotRetainArguments: true, CallsExecuteSynchronously: true,
		DiscardedRowsHaveNoEffectsStateOrRNG: true, GatherDeterministicAndValueIndependent: true,
		GatherAndConcatDoNotMutateOrRetain: true, SelectedFirstEquivalent: true, FusedForwardParity: true,
		FloatingPointPolicyPreserved: true, ErrorAndPanicParity: true, PartialOutputParity: true,
		RecorderOrderParity: true, VJPAllInputGradientsParity: true, SupportedDTypesLayoutsBackends: true,
		EquivalentFallbackUnlessForwardAndBackwardAvailable: true,
	}
}
