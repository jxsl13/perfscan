package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
)

func TestPS6137StarvedVocabulary(t *testing.T) {
	t.Parallel()
	for _, cfg := range []config.Config{{}, {NativeSnapshotReuseContracts: []config.NativeSnapshotReuseContract{{CandidateMethod: "project.Recorder.Profile"}}}} {
		if missing := missingVocab(checks.PS6137, &cfg); len(missing) != 1 || missing[0] != "nativeSnapshotReuseContracts" {
			t.Fatalf("missing=%v", missing)
		}
	}
}
