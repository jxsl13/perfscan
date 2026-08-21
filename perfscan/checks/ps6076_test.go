package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/perfscan/config"
)

func TestPS6076(t *testing.T) {
	defer config.SetForTesting(config.Config{FanOutHelpers: []string{"parallelFor", "bandPool.Range"}})()
	analysistest.Run(t, analysistest.TestData(), PS6076.Analyzer, "ps6076")
}

func TestPS6076SilentWithoutVocabulary(t *testing.T) {
	defer config.SetForTesting(config.Config{})()
	analysistest.Run(t, analysistest.TestData(), PS6076.Analyzer, "ps6076silent")
}
