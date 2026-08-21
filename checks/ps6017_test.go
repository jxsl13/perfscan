package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6017(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6017.Analyzer, "ps6017")
}
