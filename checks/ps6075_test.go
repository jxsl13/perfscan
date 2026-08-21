package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6075(t *testing.T) {
	defer config.SetForTesting(config.Config{CacheLineBytes: 128})()
	analysistest.Run(t, analysistest.TestData(), PS6075.Analyzer, "ps6075")
}

func TestPS6075ConfiguredCacheLineThreshold(t *testing.T) {
	defer config.SetForTesting(config.Config{CacheLineBytes: 64})()
	analysistest.Run(t, analysistest.TestData(), PS6075.Analyzer, "ps6075line")
}
