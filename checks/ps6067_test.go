package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6067(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6067.Analyzer, "ps6067", "ps6067alias")
}

func TestPS6067TimingInfoMerge(t *testing.T) {
	left := ps6067TimingInfo{kind: ps6067Measurement, source: "elapsed"}
	right := ps6067TimingInfo{kind: ps6067Measurement, source: "control", statistical: true}
	got := ps6067MergeTiming(left, right)
	if got.kind != ps6067Measurement || got.source != "elapsed" || !got.statistical {
		t.Fatalf("unexpected merge: %s", ps6067DebugTiming(got))
	}
}
