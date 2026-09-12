package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
)

func TestPS6133StarvedVocabulary(t *testing.T) {
	t.Parallel()
	for _, cfg := range []config.Config{{}, {RowLocalStridedGuardContracts: []config.RowLocalStridedGuardContract{{WrapperMethod: "project.Recorder.RoPEPair"}}}} {
		if missing := missingVocab(checks.PS6133, &cfg); len(missing) != 1 || missing[0] != "rowLocalStridedGuardContracts" {
			t.Fatalf("missing=%v", missing)
		}
	}
}
