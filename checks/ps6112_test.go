package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6112(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6112TestAnalyzer(ps6112Contracts("ps6112")), "ps6112")
}

func TestPS6112AlignedGrainAndFinalRemainderStaySilent(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6112TestAnalyzer(ps6112Contracts("ps6112aligned")[:1]), "ps6112aligned")
}

func TestPS6112SilentWithoutContracts(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6112TestAnalyzer(nil), "ps6112silent")
}

func TestPS6112UnreachableAndShadowedKernelRoutesStaySilent(t *testing.T) {
	t.Parallel()
	contracts := ps6112Contracts("ps6112routernegative")[:1]
	fake := contracts[0]
	fake.Name = "shadowed-router"
	fake.Scheduler = "ps6112routernegative.fakeSchedule"
	fake.Band = "ps6112routernegative.fakeBand"
	fake.KernelEntry = "ps6112routernegative.fakeEntry"
	fake.Variants = ps6112CloneVariants(contracts[0].Variants)
	for index := range fake.Variants {
		fake.Variants[index].TileRouter = "ps6112routernegative.fakeRouter"
	}
	analysistest.Run(t, analysistest.TestData(), ps6112TestAnalyzer(append(contracts, fake)), "ps6112routernegative")
}

func ps6112TestAnalyzer(contracts []config.SchedulerTileGrainContract) *analysis.Analyzer {
	analyzer := *PS6112.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6112WithContracts(pass, contracts)
	}
	return &analyzer
}

func ps6112Contracts(packagePath string) []config.SchedulerTileGrainContract {
	base := config.SchedulerTileGrainContract{
		Name:               "attention-forward-bands",
		Scheduler:          packagePath + ".schedule",
		GrainConstant:      packagePath + ".bandRows",
		Band:               packagePath + ".band",
		BandRowsArgument:   2,
		KernelEntry:        packagePath + ".kernelEntry",
		KernelRowsArgument: 3,
		RepeatedFullTasks:  true,
		Variants: []config.SchedulerTileGrainVariant{
			{
				Name:                               "amd64-default",
				GOOS:                               "linux",
				GOARCH:                             "amd64",
				TileHeight:                         6,
				TileRouter:                         packagePath + ".kernelEntry",
				TileRouterRowsArgument:             3,
				TiledKernel:                        packagePath + ".tile6",
				ScalarFallback:                     packagePath + ".scalarTail",
				ScalarFallbackRowsArgument:         3,
				KernelEntryRoutesFullTiles:         true,
				ScalarFallbackHandlesTileRemainder: true,
			},
			{
				Name:                               "apple-arm64-simd",
				GOOS:                               "darwin",
				GOARCH:                             "arm64",
				BuildTags:                          []string{"ps6112simd"},
				TileHeight:                         4,
				TileRouter:                         packagePath + ".kernelRouter",
				TileRouterRowsArgument:             3,
				TiledKernel:                        packagePath + ".tile4",
				ScalarFallback:                     packagePath + ".scalarTail",
				ScalarFallbackRowsArgument:         3,
				KernelEntryRoutesFullTiles:         true,
				ScalarFallbackHandlesTileRemainder: true,
			},
		},
	}
	contracts := []config.SchedulerTileGrainContract{base}
	owner := base
	owner.Name = "owner-dynamic-scheduler"
	owner.Scheduler = packagePath + ".ownerSchedule"
	owner.SynchronousRunner = packagePath + ".parallel"
	owner.SynchronousRunnerWorkArgument = 1
	owner.SynchronousRunnerExecutesBeforeReturn = true
	owner.Variants = ps6112CloneVariants(base.Variants)
	contracts = append(contracts, owner)
	for _, name := range []string{"arbitraryTaskSchedule", "localGrainShadow", "shiftedStart", "zeroTripStart", "staticallyPartialOnly", "skippingPost", "inclusiveCondition", "storedSchedulerClosure", "returnedSchedulerClosure", "retainedSchedulerClosure", "appendedSchedulerClosure", "immediateSchedulerClosure", "reboundExtent", "addressedExtent", "aliasedExtent", "closureMutatedExtent", "rangeAssignedExtent", "snapshotExtent", "loopCarriedExtent", "stableLoopCarriedExtent", "intentional", "profiled", "bareMarker", "deadSchedule", "noCeiling", "falseCeiling", "noFinalClamp", "falseClamp", "fakeBand", "fakeMin", "aliasedBandRows"} {
		candidate := base
		candidate.Name = name
		candidate.Scheduler = packagePath + "." + name
		candidate.Variants = ps6112CloneVariants(base.Variants)
		contracts = append(contracts, candidate)
	}
	wrongRunnerSlot := base
	wrongRunnerSlot.Name = "wrong-synchronous-runner-slot"
	wrongRunnerSlot.Scheduler = packagePath + ".wrongRunnerSlot"
	wrongRunnerSlot.SynchronousRunner = packagePath + ".parallelLabeled"
	wrongRunnerSlot.SynchronousRunnerWorkArgument = 1
	wrongRunnerSlot.SynchronousRunnerExecutesBeforeReturn = true
	wrongRunnerSlot.Variants = ps6112CloneVariants(base.Variants)
	contracts = append(contracts, wrongRunnerSlot)
	typed := base
	typed.Name = "typed-int-grain"
	typed.Scheduler = packagePath + ".typedIntSchedule"
	typed.GrainConstant = packagePath + ".typedBandRows"
	typed.Variants = ps6112CloneVariants(base.Variants)
	contracts = append(contracts, typed)
	aliasedKernel := base
	aliasedKernel.Name = "aliased-kernel-rows"
	aliasedKernel.Scheduler = packagePath + ".aliasedKernelRows"
	aliasedKernel.Band = packagePath + ".bandAlias"
	aliasedKernel.Variants = ps6112CloneVariants(base.Variants)
	contracts = append(contracts, aliasedKernel)
	narrow := base
	narrow.Name = "narrow-overflow"
	narrow.Scheduler = packagePath + ".narrowSchedule"
	narrow.GrainConstant = packagePath + ".narrowBandRows"
	narrow.Band = packagePath + ".narrowBand"
	narrow.KernelEntry = packagePath + ".narrowKernelEntry"
	narrow.Variants = ps6112CloneVariants(base.Variants)
	contracts = append(contracts, narrow)
	incomplete := base
	incomplete.Name = "incomplete-variant-set"
	incomplete.Scheduler = packagePath + ".incompleteVariantSchedule"
	incomplete.Variants = ps6112CloneVariants(base.Variants)
	incomplete.Variants[0].TiledKernel = packagePath + ".missingTile"
	contracts = append(contracts, incomplete)
	stored := base
	stored.Name = "stored-forwarder"
	stored.Scheduler = packagePath + ".storedForwardSchedule"
	stored.Band = packagePath + ".storedBand"
	stored.Variants = ps6112CloneVariants(base.Variants)
	contracts = append(contracts, stored)
	missingFallback := base
	missingFallback.Name = "missing-fallback"
	missingFallback.Scheduler = packagePath + ".missingFallback"
	missingFallback.Variants = ps6112CloneVariants(base.Variants)
	missingFallback.Variants[1].ScalarFallback = packagePath + ".missingScalar"
	contracts = append(contracts, missingFallback)
	dynamic := base
	dynamic.Name = "mutable-grain"
	dynamic.Scheduler = packagePath + ".dynamicGrain"
	dynamic.GrainConstant = packagePath + ".dynamicBandRows"
	dynamic.Variants = ps6112CloneVariants(base.Variants)
	contracts = append(contracts, dynamic)
	overlap := base
	overlap.Name = "ambiguous-build-selection"
	overlap.Scheduler = packagePath + ".overlapSchedule"
	overlap.GrainConstant = packagePath + ".overlapRows"
	overlap.Variants = ps6112CloneVariants(base.Variants)
	overlap.Variants[1].BuildTags = []string{"ps6112overlap"}
	overlap.Variants[1].TiledKernel = packagePath + ".tile6"
	contracts = append(contracts, overlap)
	return contracts
}

func ps6112CloneVariants(variants []config.SchedulerTileGrainVariant) []config.SchedulerTileGrainVariant {
	result := make([]config.SchedulerTileGrainVariant, len(variants))
	copy(result, variants)
	for index := range result {
		result[index].BuildTags = append([]string(nil), result[index].BuildTags...)
	}
	return result
}
