package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2047 is an AutoFix check: the fixes are applied and diffed against a
// .go.golden fixture. The suite covers every verb of the shared PS2136
// table (Itoa with the int64(...) wrap and explicit base 10, FormatInt/
// Uint/Float/Bool, and the Quote family), a whole-expression Itoa
// argument, a parenthesized spread argument, an aliased strconv import
// whose qualifier the rewrite keeps, a side-effecting argument passing
// through verbatim, and an indexed dst expression. The advisory cases
// pin the withheld-fix guards: a NAMED byte-slice dst and a generic
// ~[]byte dst (the rewrite would change the expression's static type),
// and comments inside the rewritten scaffolding. Negatives pin the
// silent shapes: FormatComplex (no Append* twin), a plain string spread,
// a []byte conversion spread, a non-spread append, and a local value
// shadowing the strconv package. See equiv_PS2047_test.go for the
// runtime proof that every fixed shape appends byte-identical output.
func TestPS2047(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2047.Analyzer, "ps2047")
}
