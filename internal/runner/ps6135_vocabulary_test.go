package runner

import (
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
)

func TestPS6135StarvedVocabulary(t *testing.T) {
	t.Parallel()
	for _, cfg := range []config.Config{{}, {ActiveBoundFallbackContracts: []config.ActiveBoundFallbackContract{{WrapperMethod: "project.Decoder.binElem"}}}} {
		if missing := missingVocab(checks.PS6135, &cfg); len(missing) != 1 || missing[0] != "activeBoundFallbackContracts" {
			t.Fatalf("missing=%v", missing)
		}
	}
}
