package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5015 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers every verb arm (%d/%b/%o/%x over
// each signed and unsigned width — with and without the widening wrapper —
// %t, %g over float64 and float32, %q over string and rune), untyped
// constants, side-effecting operands kept verbatim, the strconv import
// being added exactly once, an aliased strconv qualifier being reused, the
// fmt-orphan file, and the advisory guards (named operand types, a named
// []byte destination, a nil destination, a shadowed strconv name and
// comment-bearing scaffolding all stay unrewritten). See
// equiv_PS5015_test.go for the runtime proof that the rewrite appends the
// identical bytes on every input.
func TestPS5015(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5015.Analyzer, "ps5015")
}

// Negative shapes — literal text around the verb, flags/width/precision,
// %X/%e/%f/%v/%s, %x and %q over non-integer kinds, kind mismatches,
// non-literal formats, spread calls, two-verb formats, and a shadowed fmt —
// must produce no diagnostics at all.
func TestPS5015Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5015.Analyzer, "ps5015neg")
}
