package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2143(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2143.Analyzer, "ps2143", "ps2143alias")
}
