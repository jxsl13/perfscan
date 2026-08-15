package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5030 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both rewritten functions
// (strings.IndexAny -> IndexRune, strings.ContainsAny -> ContainsRune —
// both call-for-call with the same result type, so every syntactic
// position is fixable, go/defer and bare statements included), the rune
// literal rendered from 2-, 3- and 4-byte encodings, escape and raw
// source spellings all landing on the same rune literal, a non-printable
// rune rendered as a backslash-u escape, the aliased-import qualifier,
// and the advisory guard (named-constant and constant-expression cutsets
// stay unrewritten). See equiv_PS5030_test.go for the runtime proof that
// the rewrite returns the identical index/boolean on every input — and
// for the pins on the guards' load-bearing exclusions.
func TestPS5030(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5030.Analyzer, "ps5030")
}

// Negative shapes — ASCII single-byte cutsets (PS5022's), empty,
// multi-rune and rune-plus-garbage cutsets, lone non-ASCII bytes,
// truncated/overlong/surrogate sequences, the real U+FFFD cutset in both
// spellings, non-constant cutsets, the bytes-package twins, LastIndexAny,
// the Rune members themselves, a shadowing identifier, a func value, and
// a dot import — must produce no diagnostics at all.
func TestPS5030Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5030.Analyzer, "ps5030neg")
}
