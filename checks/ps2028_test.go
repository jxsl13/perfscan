package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2028 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers all six blankness-equivalent
// comparison shapes with the literal on either side of the operator,
// an aliased strings import, arguments that carry over byte-verbatim
// (struct fields, index expressions, calls, concatenations, redundant
// parens), redundant parens around the scaffolding, the rewrite inside
// a larger condition and in a named-bool context, and the advisory
// guard (comment-bearing scaffolding). See equiv_PS2028_test.go for
// the runtime proof that len(strings.Fields(s)) == 0 and
// strings.TrimSpace(s) == "" agree on every input, including non-ASCII
// spaces and invalid UTF-8.
func TestPS2028(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2028.Analyzer, "ps2028")
}

// Negative shapes — comparisons that genuinely need the field count,
// stored Fields slices, non-literal comparands, bytes.Fields,
// FieldsFunc, local/method Fields, shadowed strings and shadowed len —
// must produce no diagnostics at all.
func TestPS2028Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2028.Analyzer, "ps2028neg")
}
