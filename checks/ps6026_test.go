package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6026(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6026.Analyzer, "ps6026")
}
