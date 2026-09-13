package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
)

func TestPS6140StarvedVocabulary(t *testing.T) {
	t.Parallel()
	for _, cfg := range []config.Config{{}, {UnusedProjectionScratchContracts: []config.UnusedProjectionScratchContract{{OwnerType: "project.Owner", WorkspaceField: "scratch"}}}} {
		if missing := missingVocab(checks.PS6140, &cfg); len(missing) != 1 || missing[0] != "unusedProjectionScratchContracts" {
			t.Fatalf("missing vocabulary=%v", missing)
		}
	}
}
