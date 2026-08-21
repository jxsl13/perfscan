package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6005(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6005.Analyzer, "ps6005", "ps6005alias")
}
