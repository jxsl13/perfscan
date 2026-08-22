package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5025 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both packages (strings/bytes
// LastIndexAny -> LastIndexByte as a drop-in call), the literal wrap over
// escape sequences, NUL, 0x7f and raw backquoted literals, syntactic
// contexts (conditions, index expressions, parenthesized literals and
// calls, go/defer — the replacement stays a call, so every position is
// safe), the aliased-import qualifiers, and the advisory guards
// (named-constant and constant-expression cutsets stay unrewritten). See
// equiv_PS5025_test.go for the runtime proof that the rewrite returns
// the identical index on every input — and for the divergence witness
// pinning why the < 0x80 cutset bound is load-bearing.
func TestPS5025(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5025.Analyzer, "ps5025")
}

// Negative shapes — multi-byte, empty, non-constant, and (crucially)
// single NON-ASCII-byte cutsets, the substring-form siblings owned by
// PS5007/PS5013, the forward IndexAny/ContainsAny direction owned by
// PS5022, the Byte members, a shadowing identifier, a func value, and a
// dot import — must produce no diagnostics at all.
func TestPS5025Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5025.Analyzer, "ps5025neg")
}
