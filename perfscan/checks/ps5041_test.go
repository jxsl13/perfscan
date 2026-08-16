package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5041 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers the plain string form, a constant
// string, a side-effecting operand passing through once, and reuse of the
// file's existing strconv qualifier. Advisory cases pin the withheld-fix
// guards: a NAMED byte-slice dst and a comment inside the rewritten
// scaffolding. Negatives stay SILENT: a named string type with a String
// method ("%q" would quote String()), the "%+q" ASCII form, and a
// non-string operand. See equiv_PS5041_test.go for the runtime proof.
func TestPS5041(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5041.Analyzer, "ps5041")
}

// TestPS5041Add pins the import-add path: the first fix inserts strconv.
func TestPS5041Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5041.Analyzer, "ps5041add")
}
