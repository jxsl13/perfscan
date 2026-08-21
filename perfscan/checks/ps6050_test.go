package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6050(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6050.Analyzer, "ps6050")
}
