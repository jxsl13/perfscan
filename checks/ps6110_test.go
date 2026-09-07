package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6110(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6110TestAnalyzer(ps6110Contracts("ps6110")), "ps6110")
}

func TestPS6110SilentWithoutContracts(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), ps6110TestAnalyzer(nil), "ps6110silent")
}

func ps6110TestAnalyzer(contracts []config.NativeSnapshotStringCopyContract) *analysis.Analyzer {
	analyzer := *PS6110.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6110WithContracts(pass, contracts)
	}
	return &analyzer
}

func ps6110Contracts(packagePath string) []config.NativeSnapshotStringCopyContract {
	base := config.NativeSnapshotStringCopyContract{
		Name:                                 "labels",
		CandidateCallable:                    packagePath + ".Recorder.Profile",
		CandidateKind:                        config.NativeSnapshotCallMethod,
		DestinationResultPosition:            1,
		DestinationSliceField:                "Events",
		DestinationStringField:               "Label",
		AcquireCallable:                      packagePath + ".snapshot",
		AcquireKind:                          config.NativeSnapshotCallFunction,
		RecordsOutArgumentPosition:           2,
		CountOutArgumentPosition:             3,
		AcquireStatusResultPosition:          1,
		NativeStringField:                    "label",
		CopyKind:                             config.NativeSnapshotCallFunction,
		CopyCallable:                         packagePath + ".ownedString",
		CopyPointerArgumentPosition:          1,
		CopyStringResultPosition:             1,
		LifecycleCallable:                    packagePath + ".Recorder.Free",
		LifecycleKind:                        config.NativeSnapshotCallMethod,
		SnapshotStableThroughCandidateReturn: true,
		SnapshotNotMutatedDuringExtraction:   true,
		ExtractionIsSynchronous:              true,
		CopyReturnsExactOwnedString:          true,
		ReturnedStringsOutliveLifecycle:      true,
		ExactContentCheckRequired:            true,
	}
	profileN := base
	profileN.Name = "labels-n"
	profileN.CandidateCallable = packagePath + ".Recorder.ProfileN"
	profileN.DestinationSliceField = ""
	profileN.NativeLengthField = "length"
	profileN.CopyCallable = packagePath + ".ownedStringN"
	profileN.CopyLengthArgumentPosition = 2
	direct := base
	direct.Name = "direct-count"
	direct.CandidateCallable = packagePath + ".Recorder.DirectCount"
	direct.DestinationSliceField = ""
	return append([]config.NativeSnapshotStringCopyContract{base, profileN, direct}, ps6110NegativeContracts(base, packagePath)...)
}

func ps6110NegativeContracts(base config.NativeSnapshotStringCopyContract, packagePath string) []config.NativeSnapshotStringCopyContract {
	names := []string{"ParameterSnapshot", "OffsetIndex", "NoSingleFastPath", "EscapedDestination", "EarlyFree", "ExposedSnapshot", "CapturedDestination", "LabelAlias", "MutatedCanonicalCount", "ConditionalCopy", "DuplicateCopy", "GlobalDestination", "AliasedNative", "MutatedIndex", "DeadLoop", "SkippedCopy", "ParameterOutArguments", "GlobalAcquisition", "ClobberedStatus", "OverwrittenOutput", "NativeReturn", "CompositeNativeEscape"}
	contracts := make([]config.NativeSnapshotStringCopyContract, 0, len(names)+3)
	for _, name := range names {
		contract := base
		contract.CandidateCallable = packagePath + ".Recorder." + name
		contract.DestinationSliceField = ""
		contracts = append(contracts, contract)
	}
	fake := base
	fake.CandidateCallable = packagePath + ".Recorder.FakeC"
	fake.DestinationSliceField = ""
	fake.CopyKind = config.NativeSnapshotCopyGoString
	fake.CopyCallable = ""
	contracts = append(contracts, fake)
	generated := base
	generated.CandidateCallable = packagePath + ".Recorder.FakeGenerated"
	generated.DestinationSliceField = ""
	generated.CopyKind = config.NativeSnapshotCopyGoString
	generated.CopyCallable = ""
	contracts = append(contracts, generated)
	adjusted := base
	adjusted.CandidateCallable = packagePath + ".Recorder.FakeAdjustedFilename"
	adjusted.DestinationSliceField = ""
	adjusted.NativeLengthField = "length"
	adjusted.CopyKind = config.NativeSnapshotCopyGoStringN
	adjusted.CopyCallable = ""
	adjusted.CopyLengthArgumentPosition = 2
	contracts = append(contracts, adjusted)
	composite := base
	composite.Name = "composite-good"
	composite.CandidateCallable = packagePath + ".Recorder.CompositeGood"
	composite.DestinationSliceField = ""
	contracts = append(contracts, composite)
	unresolved := base
	unresolved.CandidateCallable = packagePath + ".Recorder.UnresolvedLifecycle"
	unresolved.DestinationSliceField = ""
	unresolved.LifecycleCallable = packagePath + ".Recorder.MissingLifecycle"
	contracts = append(contracts, unresolved)
	return contracts
}
