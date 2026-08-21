package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2048 is an AutoFix check: the fixes are applied and diffed against a
// .go.golden fixture. The suite covers the two- and three-operand forms,
// string literals, untyped constants, index/slice/call/+ operands kept
// byte-verbatim, a selector writer, result use, and the import
// bookkeeping (io added, orphaned fmt dropped or swapped for io, aliased
// fmt still fixed). The advisory cases pin the withheld-fix guards: a
// local io shadow, a comment inside the rewritten scaffolding, an
// aliased io import, and fmt imported under two specs. Negatives pin the
// silent shapes: the single-operand form (PS2129's), any non-string
// operand (numbers, error, bool, nil — the spacing rule and fmt's
// formatting both diverge), named string types and Stringers, []byte, a
// variadic spread, Fprintln/Fprintf, a shadowed fmt, a dot-imported fmt,
// and the self-dispatch positions inside the writer's own Write and
// WriteString methods. See equiv_PS2048_test.go for the runtime proof
// that the rewrite writes byte-identical output with the same (n, err).
func TestPS2048(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2048.Analyzer, "ps2048")
}
