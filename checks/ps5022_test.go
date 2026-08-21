package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5022 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both rewritten functions in both
// packages (strings/bytes IndexAny -> IndexByte as a drop-in call,
// ContainsAny -> IndexByte >= 0 with the comparison spliced in), the
// literal wrap over escape sequences, NUL, 0x7f and raw backquoted
// literals, every operator-introduction context for the ContainsAny form
// (plain boolean positions bare, a leading ! absorbed into < 0,
// comparison operands and a stacked ! parenthesized), the aliased-import
// qualifiers, and the advisory guards (named-constant and
// constant-expression cutsets, go/defer and bare-statement ContainsAny
// positions, and a comment between ! and the call stay unrewritten). See
// equiv_PS5022_test.go for the runtime proof that the rewrite returns
// the identical index/boolean on every input — and for the divergence
// witness pinning why the < 0x80 cutset bound is load-bearing.
func TestPS5022(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5022.Analyzer, "ps5022")
}

// Negative shapes — multi-byte, empty, non-constant, and (crucially)
// single NON-ASCII-byte cutsets, the substring-form siblings owned by
// PS5007/PS5013/PS5016/PS5014, LastIndexAny, the Rune/Byte members, a
// shadowing identifier, a func value, and a dot import — must produce no
// diagnostics at all.
func TestPS5022Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5022.Analyzer, "ps5022neg")
}
