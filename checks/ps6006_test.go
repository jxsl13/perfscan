package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6006(t *testing.T) {
	defer config.SetForTesting(config.Config{SelectorPromotionSymbols: []string{
		"useSplitK",
		"chooseSplitK",
		"UseSplitK",
	}})()
	analysistest.Run(t, analysistest.TestData(), PS6006.Analyzer, "ps6006")
}

func TestPS6006SilentWithoutVocabulary(t *testing.T) {
	defer config.SetForTesting(config.Config{})()
	analysistest.Run(t, analysistest.TestData(), PS6006.Analyzer, "ps6006silent")
}
