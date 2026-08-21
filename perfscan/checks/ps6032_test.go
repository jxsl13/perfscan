package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6032(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6032.Analyzer, "ps6032")
}
