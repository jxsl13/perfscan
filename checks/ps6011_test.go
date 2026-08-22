package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6011(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6011.Analyzer, "ps6011", "ps6011alias")
}
