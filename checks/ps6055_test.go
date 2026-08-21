package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6055(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6055.Analyzer, "ps6055")
}
