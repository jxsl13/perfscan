package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6036(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6036.Analyzer, "ps6036")
}
