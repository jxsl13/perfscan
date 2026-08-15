package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2036 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers every replacement arm (int with the
// int64 widening, int64/uint64 without a wrapper, the narrower signed and
// unsigned widths, uintptr, bool, float64, float32 with the float64
// widening), untyped-constant operands (int, rune, float, bool defaults),
// expression operands kept byte-verbatim, a named []byte destination and a
// nil-literal destination (both fixable: fmt.Append and strconv.Append* take
// and return the same unnamed []byte), an existing aliased strconv import
// reused by the rewrite (strconvalias.go), an aliased fmt import
// (aliased.go), the orphan-import path (orphan.go: fixing the file's only
// fmt reference orphans the import, which the fix pipeline prunes), the
// advisory guards (a shadowed strconv at the call site, comments inside the
// rewritten scaffolding), and the silent negatives (named scalar types with
// and without a String method, string operands — PS5033's — []byte, complex,
// nil, two operands, zero operands, a spread, Appendf/Appendln, a shadowed
// fmt). See equiv_PS2036_test.go for the runtime proof that fmt.Append and
// the strconv.Append* forms append identical bytes for every plain-scalar
// operand.
func TestPS2036(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2036.Analyzer, "ps2036")
}
