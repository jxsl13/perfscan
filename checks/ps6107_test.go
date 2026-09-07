package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6107(t *testing.T) {
	t.Parallel()
	contracts := []config.ReceiverStagingContract{
		ps6107FlatContract("flat", "flat-inline"),
		ps6107RowContract("rows", "rows"),
		ps6107NamedContract(),
	}
	tagged := ps6107FlatContract("tagged", "tagged")
	tagged.Consumer = "ps6107.consumeTagged"
	tagged.ConsumerArg = 1
	contracts = append(contracts, tagged)
	for _, method := range []string{
		"constant", "capacity", "conditional", "compound", "readsZero",
		"aliases", "stores", "laterUse", "twice", "async", "deferred",
		"wrongConsumer", "pointerElements", "valueReceiver",
	} {
		contracts = append(contracts, ps6107FlatContract(method, method))
	}
	methodExpression := ps6107FlatContract("methodExpression", "method-expression")
	methodExpression.Consumer = "ps6107.Device.UploadF32"
	methodExpression.ConsumerKind = config.ReceiverStagingCallMethod
	methodExpression.ConsumerArg = 1
	contracts = append(contracts, methodExpression)
	for _, method := range []string{
		"wrongRows", "wrongOverwriter", "wrongWidth", "rowAlias", "addressedExtent", "capturedExtent",
		"stringRows", "mapRows", "snapshotInput", "snapshotWidth", "addressedInput", "capturedWidth", "opaqueWidth",
		"wholeReceiverReset", "boxedReceiverMutation", "nestedReceiverMutation", "globalSnapshotMutation", "previouslyEscaped",
	} {
		contracts = append(contracts, ps6107RowContract(method, method))
	}
	contracts = append(contracts, ps6107RowContract("stableUnrelated", "stable-unrelated"))
	for _, method := range []string{"indexMutation", "hiddenSizeCall", "unreachable"} {
		contracts = append(contracts, ps6107FlatContract(method, method))
	}
	missingLifecycle := ps6107FlatContract("noMatchingLifecycle", "missing-lifecycle")
	missingLifecycle.LifecycleMethod = "ps6107.Other.Release"
	contracts = append(contracts, missingLifecycle)

	analyzer := *PS6107.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6107WithContracts(pass, contracts)
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6107")
}

func TestPS6107SilentWithoutContract(t *testing.T) {
	t.Parallel()
	analyzer := *PS6107.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6107WithContracts(pass, nil)
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6107silent")
}

func ps6107FlatContract(method, name string) config.ReceiverStagingContract {
	return config.ReceiverStagingContract{
		Name:                             name,
		CandidateMethod:                  "ps6107.Decoder." + method,
		Consumer:                         "ps6107.consumeF32",
		ConsumerKind:                     config.ReceiverStagingCallFunction,
		ConsumerArg:                      0,
		LifecycleMethod:                  "ps6107.Decoder.Release",
		MaxRetainedBytes:                 64 << 10,
		ReceiverCallsSequential:          true,
		ConsumerCompletesBeforeReturn:    true,
		ConsumerDoesNotRetainArgument:    true,
		LifecycleEndsReceiverUse:         true,
		ContentsMayPersistUntilLifecycle: true,
	}
}

func ps6107RowContract(method, name string) config.ReceiverStagingContract {
	contract := ps6107FlatContract(method, name)
	contract.Overwrite = "ps6107.Decoder.gatherInto"
	contract.OverwriteKind = config.ReceiverStagingCallMethod
	contract.OverwriteArg = 0
	contract.Consumer = "ps6107.Device.UploadF32"
	contract.ConsumerKind = config.ReceiverStagingCallMethod
	contract.OverwriteWritesAllBeforeRead = true
	contract.OverwriteCompletesBeforeReturn = true
	contract.OverwriteDoesNotRetainArgument = true
	contract.OverwriteAccessesOnlyArgumentLength = true
	contract.OverwriteIgnoresCapacityAndIdentity = true
	contract.OverwritePreservesExtentInputs = true
	contract.ExtentIsNonNegativeAndNonOverflowing = true
	return contract
}

func ps6107NamedContract() config.ReceiverStagingContract {
	contract := ps6107FlatContract("named", "named-inline")
	contract.Consumer = "ps6107.Device.UploadScalar"
	contract.ConsumerKind = config.ReceiverStagingCallMethod
	return contract
}
