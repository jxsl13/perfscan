package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5072 is an AutoFix check. The main fixture covers pointer/value/expression
// receivers, aliases, comments, and the endianness/nil/static-type boundaries;
// the orphan fixture proves the last encoding/binary import is removed.
func TestPS5072(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5072.Analyzer, "ps5072", "ps5072alias", "ps5072orphan")
}
