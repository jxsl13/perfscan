package runner

import (
	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
	"testing"
)

func TestPS6132StarvedVocabulary(t *testing.T) {
	t.Parallel()
	for _, cfg := range []config.Config{{}, {ContextTransientWorkspaceContracts: []config.ContextTransientWorkspaceContract{{AllocationMethod: "project.Decoder.allocate"}}}} {
		if missing := missingVocab(checks.PS6132, &cfg); len(missing) != 1 || missing[0] != "contextTransientWorkspaceContracts" {
			t.Fatalf("missing=%v", missing)
		}
	}
}
