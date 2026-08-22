package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6022(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6022.Analyzer, "ps6022")
}
