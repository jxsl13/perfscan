package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6073(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6073.Analyzer, "ps6073")
}
