package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2114(t *testing.T) {
	t.Parallel()
	// Advisory check: no SuggestedFixes, analysistest.Run validates the
	// // want diagnostics only.
	analysistest.Run(t, analysistest.TestData(), PS2114.Analyzer, "ps2114")
}

func TestPS2114OwnershipTokens(t *testing.T) {
	t.Parallel()
	// The owned path returns its original pointer token without a diagnostic;
	// the raw-slice fallback remains a reported boxing boundary.
	analysistest.Run(t, analysistest.TestData(), PS2114.Analyzer, "ps2114ownership")
}

func TestPS2114OwnershipDocumentation(t *testing.T) {
	t.Parallel()

	wants := []string{
		"Allocate the wrapper only on a cold miss",
		"return the same pointer to Put",
		"GC reclamation and per-P behavior",
		"raw slice is an explicit fallback boundary",
		"not a universal wall-time improvement",
		"paired measurements",
		"made F32 1.170x slower",
		"made F64 1.121x slower",
	}
	text := strings.Join(strings.Fields(PS2114.Doc.Text+"\n"+PS2114.Doc.MeasuredWin), " ")
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("PS2114 documentation does not contain %q", want)
		}
	}
}
