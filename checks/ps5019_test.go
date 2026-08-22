package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5019 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the rewrite forms (identifiers,
// slice/index expressions, field selectors, byte-slice literals, a
// parenthesized Count, nesting inside larger expressions, an aliased
// bytes import) and the advisory guards (operands containing
// calls/conversions/receives, comments in the replaced punctuation).
// See equiv_PS5019_test.go for the runtime proof that
// n == Count(b, old) makes the rewrite byte-identical on every input.
func TestPS5019(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5019.Analyzer, "ps5019")
}

// Negative shapes — a Count over a different haystack or needle, an
// unrelated n, a same-named method set, swapped Count arguments, the
// strings package spelling (that is PS5012's territory) — must produce
// no diagnostics at all.
func TestPS5019Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5019.Analyzer, "ps5019neg")
}
