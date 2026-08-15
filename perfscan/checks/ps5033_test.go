package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5033 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the plain-string positives (variables,
// call results, untyped constants, conversions, a trailing comma, a preserved
// between-args comment), the advisory guards (a NAMED string operand that
// could carry String()/Format(), a NAMED []byte destination, a comment inside
// the rewritten scaffolding), the orphan-import guard (orphan.go: fixing the
// file's only fmt reference would orphan the import — golden identical), and
// the negatives (multi-operand and zero-operand calls, []byte/int/any/error/
// nil operands, spread arguments, Appendln, a shadowed fmt). See
// equiv_PS5033_test.go for the runtime proof that fmt.Append(buf, s) and
// append(buf, s...) return identical bytes — and identical slice growth — for
// every plain-string operand.
func TestPS5033(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5033.Analyzer, "ps5033")
}
