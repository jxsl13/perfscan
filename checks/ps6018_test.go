package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6018(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6018.Analyzer, "ps6018")
}
