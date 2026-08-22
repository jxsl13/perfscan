package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6056(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6056.Analyzer, "ps6056")
}
