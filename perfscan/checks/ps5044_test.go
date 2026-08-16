package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5044 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. Positives cover a plain string variable, a call
// operand, and an untyped string constant. Advisory cases pin the
// withheld-fix guards: a NAMED string type (may implement Stringer/
// Formatter), a NAMED []byte destination, and a comment in the
// scaffolding. Negatives stay SILENT: a []byte operand ("%v" prints its
// element list), an int operand, and the "%s" verb (PS2141's). See
// equiv_PS5044_test.go for the runtime byte-identity proof.
func TestPS5044(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5044.Analyzer, "ps5044")
}
