package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6071(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6071.Analyzer, "ps6071/...")
}
