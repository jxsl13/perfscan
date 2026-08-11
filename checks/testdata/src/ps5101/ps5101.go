package ps5101

import (
	"bytes"
	stdbytes "bytes"
	"strings"
)

func eq(a, b []byte) bool {
	return bytes.Compare(a, b) == 0 // want `bytes\.Compare\(\.\.\.\) == 0 tests equality only; bytes\.Equal\(a, b\) is faster and computes no ordering`
}

func neq(a, b []byte) bool {
	return bytes.Compare(a, b) != 0 // want `bytes\.Compare\(\.\.\.\) != 0 tests equality only; !bytes\.Equal\(a, b\) is faster and computes no ordering`
}

func zeroOnLeft(a, b []byte) bool {
	return 0 == bytes.Compare(a, b) // want `bytes\.Compare\(\.\.\.\) == 0 tests equality only; bytes\.Equal\(a, b\) is faster and computes no ordering`
}

func zeroOnLeftNeq(a, b []byte) bool {
	return 0 != bytes.Compare(a, b) // want `bytes\.Compare\(\.\.\.\) != 0 tests equality only; !bytes\.Equal\(a, b\) is faster and computes no ordering`
}

// An aliased import keeps its qualifier: only the selected name changes.
func aliased(a, b []byte) bool {
	return stdbytes.Compare(a, b) == 0 // want `bytes\.Compare\(\.\.\.\) == 0 tests equality only; bytes\.Equal\(a, b\) is faster and computes no ordering`
}

// Composed into a larger condition: unary ! binds tighter than &&, so the
// rewrite needs no extra parentheses.
func composed(a, b, c []byte) bool {
	return len(c) > 0 && bytes.Compare(a, b) != 0 // want `bytes\.Compare\(\.\.\.\) != 0 tests equality only; !bytes\.Equal\(a, b\) is faster and computes no ordering`
}

// --- guards: none of the following may be reported or rewritten ---

// Ordering operators genuinely need the three-way result.
func ordering(a, b []byte) bool {
	return bytes.Compare(a, b) < 0
}

func orderingGE(a, b []byte) bool {
	return bytes.Compare(a, b) >= 0
}

// A variable holding 0 is not the literal constant 0.
func zeroVar(a, b []byte) bool {
	zero := 0
	return bytes.Compare(a, b) == zero
}

// A named constant is not the literal 0 either.
const namedZero = 0

func zeroConst(a, b []byte) bool {
	return bytes.Compare(a, b) == namedZero
}

// The ordering result used as a value must survive.
func threeWay(a, b []byte) int {
	return bytes.Compare(a, b)
}

// strings.Compare is not bytes.Compare.
func stringsCompare(a, b string) bool {
	return strings.Compare(a, b) == 0
}

// A local function named Compare on a value called bytes must not match.
type fakeBytes struct{}

func (fakeBytes) Compare(a, b []byte) int { return 0 }

func localCompare(a, b []byte) bool {
	var bytes fakeBytes
	return bytes.Compare(a, b) == 0
}
