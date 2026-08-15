package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2041 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the plain-string positives (one, two
// and three operands, a call as the FIRST operand, untyped constants and
// package-qualified identifiers, a trailing comma, a preserved comment after
// buf), the advisory guards (a NAMED string operand that could carry
// String()/Format(), a NAMED []byte destination, a non-inert LATER operand —
// call and index — and comments inside the rewritten scaffolding), the
// orphan-import guard (orphan.go: fixing the file's only fmt reference would
// orphan the import — golden identical), and the negatives (zero operands,
// int/[]byte/any/error/nil operands, spread arguments, fmt.Append and
// fmt.Sprintln, a shadowed fmt). See equiv_PS2041_test.go for the runtime
// proof that fmt.Appendln(buf, a, b) and the nested append chain return
// identical bytes for every plain-string operand, plus the pinned divergence
// witnesses behind each guard.
func TestPS2041(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2041.Analyzer, "ps2041")
}
