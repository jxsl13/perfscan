package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2040 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the plain-string positives (2- and
// 3-operand calls, an arbitrary first operand — call and conversion — untyped
// constants, a package-qualified identifier, a trailing comma, a preserved
// between-buf-and-first-operand comment), the advisory guards (a NAMED string
// operand that could carry String()/Format(), a NAMED []byte destination, a
// non-inert LATER operand — call and index — and comments inside the rewritten
// scaffolding), the orphan-import guard (orphan.go: fixing the file's only fmt
// reference would orphan the import — golden identical), and the negatives
// (single-operand — PS5033's shape — and zero-operand calls, []byte/int/any/
// error/nil operands, spread arguments, Appendln, a shadowed fmt). See
// equiv_PS2040_test.go for the runtime proof that fmt.Append over all-string
// operands and the nested append chain return identical bytes — and for the
// reproduced divergences behind each fix gate.
func TestPS2040(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2040.Analyzer, "ps2040")
}
