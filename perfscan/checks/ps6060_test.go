package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6060(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6060.Analyzer, "ps6060", "ps6060native")
}
