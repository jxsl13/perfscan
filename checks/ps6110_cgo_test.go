package checks

import (
	"go/build"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6110RealCgoNames(t *testing.T) {
	t.Parallel()
	if !build.Default.CgoEnabled {
		t.Skip("cgo disabled")
	}
	contract := config.NativeSnapshotStringCopyContract{
		Name:                                 "cgo-labels",
		CandidateCallable:                    "ps6110cgo.recorder.Profile",
		CandidateKind:                        config.NativeSnapshotCallMethod,
		DestinationResultPosition:            1,
		DestinationStringField:               "Label",
		AcquireCallable:                      "C.profile_snapshot",
		AcquireKind:                          config.NativeSnapshotCallCgo,
		RecordsOutArgumentPosition:           1,
		CountOutArgumentPosition:             2,
		AcquireStatusResultPosition:          1,
		NativeStringField:                    "label",
		CopyKind:                             config.NativeSnapshotCopyGoString,
		CopyPointerArgumentPosition:          1,
		CopyStringResultPosition:             1,
		LifecycleCallable:                    "ps6110cgo.recorder.Free",
		LifecycleKind:                        config.NativeSnapshotCallMethod,
		SnapshotStableThroughCandidateReturn: true,
		SnapshotNotMutatedDuringExtraction:   true,
		ExtractionIsSynchronous:              true,
		CopyReturnsExactOwnedString:          true,
		ReturnedStringsOutliveLifecycle:      true,
		ExactContentCheckRequired:            true,
	}
	contractN := contract
	contractN.Name = "cgo-labels-n"
	contractN.CandidateCallable = "ps6110cgo.recorder.ProfileN"
	contractN.NativeLengthField = "length"
	contractN.CopyKind = config.NativeSnapshotCopyGoStringN
	contractN.CopyLengthArgumentPosition = 2
	analysistest.Run(t, analysistest.TestData(), ps6110TestAnalyzer([]config.NativeSnapshotStringCopyContract{contract, contractN}), "ps6110cgo")
}
