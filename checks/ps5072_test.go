package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5072 is an AutoFix check. The main fixture covers pointer/value/expression
// receivers, aliases, comments, and the endianness/nil/static-type boundaries;
// the orphan fixture proves the last encoding/binary import is removed.
func TestPS5072(t *testing.T) {
	t.Parallel()
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5072.Analyzer, "ps5072", "ps5072alias", "ps5072orphan")
}

func TestPS5072GroupedOrphanFix(t *testing.T) {
	t.Parallel()
	results := analysistest.Run(t, analysistest.TestData(), PS5072.Analyzer, "ps5072orphan")
	var diagnostics []analysis.Diagnostic
	for _, result := range results {
		diagnostics = append(diagnostics, result.Diagnostics...)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %d, want 2", len(diagnostics))
	}
	fixes := 0
	for _, diagnostic := range diagnostics {
		if len(diagnostic.SuggestedFixes) == 0 {
			continue
		}
		fixes += len(diagnostic.SuggestedFixes)
		if edits := len(diagnostic.SuggestedFixes[0].TextEdits); edits != 5 {
			t.Fatalf("grouped fix edits = %d, want 5 (two sites plus import)", edits)
		}
	}
	if fixes != 1 {
		t.Fatalf("suggested fixes = %d, want one atomic file-wide fix", fixes)
	}
}
