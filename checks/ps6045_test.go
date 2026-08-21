package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6045(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6045.Analyzer, "ps6045")
}
