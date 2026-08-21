package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6008(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6008.Analyzer, "ps6008", "ps6008alias")
}
