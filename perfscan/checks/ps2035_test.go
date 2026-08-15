package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2035 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers every scalar arm (%v over each
// signed and unsigned width — with and without the widening wrapper,
// uintptr included — bool, float64 and float32), untyped constants,
// side-effecting operands kept verbatim, the strconv import being added
// exactly once, an aliased strconv qualifier being reused, the fmt-orphan
// file, and the advisory guards (a named []byte destination, a nil
// destination, a shadowed strconv name and comment-bearing scaffolding all
// stay unrewritten). See equiv_PS2035_test.go for the runtime proof that
// the rewrite appends the identical bytes on every input.
func TestPS2035(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2035.Analyzer, "ps2035")
}

// Negative shapes — NAMED operand types (which %v would print via their
// String()/Format() methods, so even an advisory would be wrong), strings
// and []byte (PS2141's territory), complex, literal text around the verb,
// other verbs (PS5015's territory), non-literal formats, spread calls,
// two-operand calls, and a shadowed fmt — must produce no diagnostics at
// all.
func TestPS2035Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2035.Analyzer, "ps2035neg")
}
