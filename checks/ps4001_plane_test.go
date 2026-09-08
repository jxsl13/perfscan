package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS4001FixedEndianPlane(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS4001.Analyzer, "ps4001plane", "ps4001planealias")
}
