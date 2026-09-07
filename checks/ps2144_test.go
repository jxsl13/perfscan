package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2144(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		t.Parallel()
		analysistest.Run(t, analysistest.TestData(), PS2144.Analyzer, "ps2144")
	})
	t.Run("negative", func(t *testing.T) {
		t.Parallel()
		analysistest.Run(t, analysistest.TestData(), PS2144.Analyzer, "ps2144neg")
	})
	t.Run("generic-layout", func(t *testing.T) {
		t.Parallel()
		// Valid generic make shapes must fail closed rather than asking
		// go/types for an unavailable concrete layout.
		analysistest.Run(t, analysistest.TestData(), PS2144.Analyzer, "ps2144generic")
	})
}
