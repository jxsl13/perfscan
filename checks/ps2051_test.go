package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2051 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. Positives cover both directions (Match([]byte(s)) ->
// MatchString(s) and MatchString(string(b)) -> Match(b)). Advisory: named
// string/[]byte operands (convertible but not assignable to the twin param)
// and a comment in the unwrapped scaffolding. Negatives stay SILENT: no
// conversion, an identity []byte([]byte) conversion, the two-arg package-level
// regexp.Match, and a different method. See equiv_PS2051_test.go.
func TestPS2051(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2051.Analyzer, "ps2051")
}
