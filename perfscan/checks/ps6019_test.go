package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/perfscan/config"
)

func TestPS6019(t *testing.T) {
	defer config.SetForTesting(config.Config{TopKSelectorFuncs: []string{"topKIndices", "argmaxIndex"}})()
	analysistest.Run(t, analysistest.TestData(), PS6019.Analyzer, "ps6019")
}

func TestPS6019SilentWithoutVocabulary(t *testing.T) {
	defer config.SetForTesting(config.Config{})()
	analysistest.Run(t, analysistest.TestData(), PS6019.Analyzer, "ps6019silent")
}
