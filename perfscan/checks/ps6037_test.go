package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6037(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6037.Analyzer, "ps6037")
}
