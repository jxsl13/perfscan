package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5039 is an AutoFix check: the fixes are applied and diffed against a
// .go.golden fixture. The suite covers the plain rune form (with the
// unicode/utf8 import added once per file), an int32 operand kept
// verbatim, a named rune type and every narrower width (byte, int8,
// int16, uint16) wrapped as rune(x), a parenthesized conversion, a
// side-effecting operand passing through verbatim, an indexed dst
// expression, and an aliased utf8 import reused as the qualifier. The
// advisory cases pin the withheld-fix guards: a NAMED byte-slice dst
// (the rewrite would change the expression's static type), a comment
// inside the rewritten scaffolding, and a shadowed utf8 qualifier.
// Negatives pin the divergence shapes that must stay SILENT: constant
// runes (compile-time folded), the wider integer types (int, int64,
// uint32, uintptr — string(x) converts the full value where rune(x)
// would truncate), a named string type's conversion, a plain string
// spread, string([]rune), a []byte conversion spread, a non-spread
// append, and a three-argument append. See equiv_PS5039_test.go for the
// runtime proof that every fixed shape is byte-identical.
func TestPS5039(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5039.Analyzer, "ps5039")
}

// TestPS5039Dot pins the dot-import path: with no usable qualifier the
// fix is withheld and the report stays advisory.
func TestPS5039Dot(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5039.Analyzer, "ps5039dot")
}
