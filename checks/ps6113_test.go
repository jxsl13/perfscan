package checks

import (
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6113(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6113TestAnalyzer(ps6113TestContracts()), "ps6113")
}

func TestPS6113ContractOrderDoesNotChangeFindings(t *testing.T) {
	t.Parallel()
	contracts := ps6113TestContracts()
	slices.Reverse(contracts)
	analysistest.Run(t, analysistest.TestData(), ps6113TestAnalyzer(contracts), "ps6113")
}

func TestPS6113SilentWithoutConfig(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6113TestAnalyzer(nil), "ps6113silent")
}

func TestPS6113RejectsInterfaceClaimedAsConcreteImplementation(t *testing.T) {
	t.Parallel()
	contract := ps6113InterfaceContract()
	contract.Name = "bad-interface-implementation"
	contract.Projection = "ps6113badimpl.projection.record"
	contract.Accumulate = "ps6113badimpl.projection.recordAdd"
	contract.RecorderBinary = "ps6113badimpl.recorder.Binary"
	contract.EagerSequence = "ps6113badimpl.firstErr"
	contract.AddOperation = "ps6113badimpl.binaryAdd"
	contract.ConfiguredSite = "ps6113badimpl.candidate"
	contract.Implementations = []config.RecorderResidualAddImplementation{{
		Projection: "ps6113badimpl.projection.record",
		Accumulate: "ps6113badimpl.projection.recordAdd",
	}}
	analysistest.Run(t, analysistest.TestData(), ps6113TestAnalyzer([]config.RecorderResidualAddContract{contract}), "ps6113badimpl")
}

func ps6113TestAnalyzer(contracts []config.RecorderResidualAddContract) *analysis.Analyzer {
	analyzer := *PS6113.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6113WithContracts(pass, contracts)
	}
	return &analyzer
}

func ps6113TestContracts() []config.RecorderResidualAddContract {
	return []config.RecorderResidualAddContract{
		ps6113InterfaceContract(),
		ps6113DirectContract(),
		ps6113ReceiverPrefixContract(),
		ps6113RecursiveOverlapContract(),
	}
}

func ps6113InterfaceContract() config.RecorderResidualAddContract {
	contract := ps6113Contract("ps6113.projection.record", "ps6113.projection.recordAdd", "ps6113.recorder.Binary")
	contract.Name = "owner-interface-residual"
	contract.EagerSequence = "ps6113.firstErr"
	contract.ConfiguredSite = "ps6113.ownerEager"
	contract.EagerSequenceReturnsFirstError = true
	contract.AllDynamicProjectionTypesCovered = true
	contract.Implementations = []config.RecorderResidualAddImplementation{
		{Projection: "ps6113.concreteProjection.record", Accumulate: "ps6113.concreteProjection.recordAdd"},
		{Projection: "ps6113.secondProjection.record", Accumulate: "ps6113.secondProjection.recordAdd"},
	}
	return contract
}

func ps6113DirectContract() config.RecorderResidualAddContract {
	contract := ps6113Contract("ps6113.directProjection.record", "ps6113.directProjection.recordAdd", "ps6113.directRecorder.Binary")
	contract.Name = "direct-residual"
	return contract
}

func ps6113ReceiverPrefixContract() config.RecorderResidualAddContract {
	contract := ps6113Contract("ps6113.receiverProjection.record", "ps6113.receiverProjection.recordAdd", "ps6113.directRecorder.Binary")
	contract.Name = "receiver-prefix-negative"
	return contract
}

func ps6113RecursiveOverlapContract() config.RecorderResidualAddContract {
	contract := ps6113Contract("ps6113.recursiveProjection.record", "ps6113.recursiveProjection.recordAdd", "ps6113.recursiveRecorder.Binary")
	contract.Name = "recursive-role-overlap-negative"
	contract.AccumulateDestinationLengthArgument = 0
	return contract
}

func ps6113Contract(projection, accumulate, binary string) config.RecorderResidualAddContract {
	return config.RecorderResidualAddContract{
		Name:                                              "residual",
		Projection:                                        projection,
		Accumulate:                                        accumulate,
		RecorderBinary:                                    binary,
		AddOperation:                                      "ps6113.binaryAdd",
		AddOperationValue:                                 "1",
		ProjectionRecorderArgument:                        1,
		ProjectionSourceArgument:                          2,
		ProjectionTemporaryArgument:                       3,
		ProjectionExtentArguments:                         []int{4},
		BinaryDestinationArgument:                         1,
		BinaryTemporaryArgument:                           2,
		BinaryOutputArgument:                              3,
		BinaryOperationArgument:                           4,
		AccumulateRecorderArgument:                        1,
		AccumulateSourceArgument:                          2,
		AccumulateTemporaryArgument:                       3,
		AccumulateDestinationArgument:                     4,
		AccumulateExtentArguments:                         []int{5},
		AccumulateDestinationLengthArgument:               6,
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
	}
}
