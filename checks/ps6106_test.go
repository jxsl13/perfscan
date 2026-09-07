package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6106Source(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6106.Analyzer, "ps6106")
}

func TestPS6106Configured(t *testing.T) {
	t.Parallel()
	contract := config.BoundedScratchFlowContract{
		Name:                                  "device-bounded-gelu",
		Allocator:                             "ps6106contract.Device.Alloc",
		AllocatorCapacityArg:                  1,
		Producer:                              "ps6106contract.Device.Produce",
		ProducerBufferArg:                     1,
		ProducerActiveArg:                     2,
		Consumer:                              "ps6106contract.Device.Activate",
		ConsumerBufferArg:                     1,
		Observers:                             []config.BoundedScratchObserverContract{{Function: "ps6106contract.Device.Observe", BufferArg: 1, ActiveArg: 2}},
		ConfiguredCapacityElements:            1024,
		ConfiguredActiveElements:              16,
		ConfiguredRepeatCount:                 12,
		AllocatorReturnsFreshOwned:            true,
		ProducerWritesOnlyActivePrefix:        true,
		ConsumerTraversesFullCapacity:         true,
		ObserversReadOnlyActivePrefix:         true,
		CallsDoNotRetainBuffer:                true,
		CallsExecuteSynchronously:             true,
		InactiveTailNotRequired:               true,
		Replacement:                           "ps6106contract.Device.BiasGELU",
		ReplacementMatchesComposition:         true,
		ReplacementPreservesActivePrefixBits:  true,
		ReplacementLeavesInactiveTail:         true,
		ReplacementPreservesErrorsSideEffects: true,
		ReplacementPreservesProviderFallback:  true,
		ReplacementRejectsUnsupported:         true,
		ReplacementFailureUnmodified:          true,
	}
	analyzer := *PS6106.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6106WithContracts(pass, []config.BoundedScratchFlowContract{contract})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6106contract")
}

func TestPS6106NoConfigOrdinaryPackage(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6106.Analyzer, "ps6106noconfig")
}
