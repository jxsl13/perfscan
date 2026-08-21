package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6030(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6030.Analyzer, "ps6030")
}
