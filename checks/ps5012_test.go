package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5012 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms (identifiers,
// literals, field selectors, index expressions, a parenthesized Count,
// nesting inside larger expressions, an aliased strings import) and the
// advisory guards (operands containing calls/conversions/receives,
// comments in the replaced punctuation). See equiv_PS5012_test.go for
// the runtime proof that n == Count(s, old) makes the rewrite
// byte-identical on every input.
func TestPS5012(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5012.Analyzer, "ps5012")
}

// Negative shapes — a Count over a different haystack or needle, an
// unrelated n, a same-named method set, swapped Count arguments — must
// produce no diagnostics at all.
func TestPS5012Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5012.Analyzer, "ps5012neg")
}
