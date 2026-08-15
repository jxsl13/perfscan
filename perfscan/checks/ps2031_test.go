package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2031 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both verbs (%s and %v), raw and
// parenthesized format literals, operand shapes that carry over
// byte-verbatim (concatenations, calls, selectors, indexes, '%'-laden
// constants, a comment INSIDE the operand — which survives), aliased
// fmt and errors imports, the errors-import injection, the fmt-orphan
// path, and the advisory shapes (a comment overlapping the deleted
// wrapper, a shadowed errors name). See equiv_PS2031_test.go for the
// runtime proof that fmt.Errorf("%s"|"%v", s) and errors.New(s) build
// byte-identical errors of the identical dynamic type on every input.
func TestPS2031(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2031.Analyzer, "ps2031")
}

// Negative shapes — decorated formats, non-literal formats, defined
// string types, []byte/error/Stringer operands, %w, wrong arity, a
// spread, a shadowed fmt, and fmt.Sprintf — must produce no
// diagnostics at all.
func TestPS2031Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2031.Analyzer, "ps2031neg")
}
