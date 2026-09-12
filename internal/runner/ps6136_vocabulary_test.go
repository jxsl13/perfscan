package runner

import (
	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
	"testing"
)

func TestPS6136StarvedVocabulary(t *testing.T) {
	t.Parallel()
	for _, cfg := range []config.Config{{}, {OutputWorkspaceContracts: []config.OutputWorkspaceContract{{OwnerType: "project.Owner", WorkspaceField: "output"}}}} {
		if missing := missingVocab(checks.PS6136, &cfg); len(missing) != 1 || missing[0] != "outputWorkspaceContracts" {
			t.Fatalf("missing %v", missing)
		}
	}
}
