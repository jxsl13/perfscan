package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2142(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2142.Analyzer, "ps2142", "ps2142alias")
}
