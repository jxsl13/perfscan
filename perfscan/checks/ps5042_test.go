package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5042 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers int and int64 (AppendInt, the latter
// bare), uint and uint64 (AppendUint), a narrower width (byte) widened as
// uint64(x), and a side-effecting operand passing through once, plus reuse
// of the file's existing strconv qualifier. Advisory cases pin the guards: a
// NAMED byte-slice dst and a comment inside the scaffolding. Negatives stay
// SILENT: a named integer type with a Format method ("%d" formats via it), a
// width flag, a different verb, and a float operand. See equiv_PS5042_test.go.
func TestPS5042(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5042.Analyzer, "ps5042")
}

// TestPS5042Add pins the import-add path: the first fix inserts strconv.
func TestPS5042Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5042.Analyzer, "ps5042add")
}
