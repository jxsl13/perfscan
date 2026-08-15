package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2030 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers all six blankness-equivalent
// comparison shapes with the literal on either side of the operator,
// an aliased bytes import, arguments that carry over byte-verbatim
// (struct fields, index expressions, calls, appends, named []byte
// types, redundant parens), redundant parens around the scaffolding,
// the rewrite inside a larger condition and in a named-bool context,
// and comment-bearing scaffolding (which SURVIVES: the fix edits only
// the Fields identifier, so it is never withheld). See
// equiv_PS2030_test.go for the runtime proof that
// len(bytes.Fields(b)) == 0 and len(bytes.TrimSpace(b)) == 0 agree on
// every input, including non-ASCII spaces and invalid UTF-8.
func TestPS2030(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2030.Analyzer, "ps2030")
}

// Negative shapes — comparisons that genuinely need the field count,
// stored Fields slices, non-literal comparands, strings.Fields (PS2028
// territory), FieldsFunc, local/method Fields, shadowed bytes and
// shadowed len — must produce no diagnostics at all.
func TestPS2030Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2030.Analyzer, "ps2030neg")
}
