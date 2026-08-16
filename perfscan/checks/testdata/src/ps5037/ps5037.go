package ps5037

import (
	"unicode/utf8"
)

// --- the six emptiness forms on a string ---

func emptyEQ(s string) bool {
	return utf8.RuneCountInString(s) == 0 // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(s\) == 0 is the bit-identical O\(1\) test`
}

func emptyNE(s string) bool {
	return utf8.RuneCountInString(s) != 0 // want `utf8\.RuneCountInString\(\.\.\.\) != 0 scans the entire string just to test emptiness; len\(s\) != 0 is the bit-identical O\(1\) test`
}

func emptyGT(s string) bool {
	return utf8.RuneCountInString(s) > 0 // want `utf8\.RuneCountInString\(\.\.\.\) > 0 scans the entire string just to test emptiness; len\(s\) > 0 is the bit-identical O\(1\) test`
}

func emptyLE(s string) bool {
	return utf8.RuneCountInString(s) <= 0 // want `utf8\.RuneCountInString\(\.\.\.\) <= 0 scans the entire string just to test emptiness; len\(s\) <= 0 is the bit-identical O\(1\) test`
}

func emptyGE1(s string) bool {
	return utf8.RuneCountInString(s) >= 1 // want `utf8\.RuneCountInString\(\.\.\.\) >= 1 scans the entire string just to test emptiness; len\(s\) >= 1 is the bit-identical O\(1\) test`
}

func emptyLT1(s string) bool {
	return utf8.RuneCountInString(s) < 1 // want `utf8\.RuneCountInString\(\.\.\.\) < 1 scans the entire string just to test emptiness; len\(s\) < 1 is the bit-identical O\(1\) test`
}

// --- the []byte twin ---

func bytesEQ(b []byte) bool {
	return utf8.RuneCount(b) == 0 // want `utf8\.RuneCount\(\.\.\.\) == 0 scans the entire \[\]byte just to test emptiness; len\(b\) == 0 is the bit-identical O\(1\) test`
}

func bytesGT(b []byte) bool {
	return utf8.RuneCount(b) > 0 // want `utf8\.RuneCount\(\.\.\.\) > 0 scans the entire \[\]byte just to test emptiness; len\(b\) > 0 is the bit-identical O\(1\) test`
}

// Literal on the left: `0 == count` is `count == 0`, `0 < count` is
// `count > 0`, `1 > count` is `count < 1`, `1 <= count` is `count >= 1`.
// The fix only renames the callee, so no edit depends on the side.

func revEQ(s string) bool {
	return 0 == utf8.RuneCountInString(s) // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(s\) == 0 is the bit-identical O\(1\) test`
}

func revGT(s string) bool {
	return 0 < utf8.RuneCountInString(s) // want `utf8\.RuneCountInString\(\.\.\.\) > 0 scans the entire string just to test emptiness; len\(s\) > 0 is the bit-identical O\(1\) test`
}

func revLT1(s string) bool {
	return 1 > utf8.RuneCountInString(s) // want `utf8\.RuneCountInString\(\.\.\.\) < 1 scans the entire string just to test emptiness; len\(s\) < 1 is the bit-identical O\(1\) test`
}

func revGE1(b []byte) bool {
	return 1 <= utf8.RuneCount(b) // want `utf8\.RuneCount\(\.\.\.\) >= 1 scans the entire \[\]byte just to test emptiness; len\(b\) >= 1 is the bit-identical O\(1\) test`
}

// A parenthesized literal or call still matches; the callee rename
// leaves the parentheses in place ((len)(s) and (0) are both legal).
func parenLit(s string) bool {
	return utf8.RuneCountInString(s) != (0) // want `utf8\.RuneCountInString\(\.\.\.\) != 0 scans the entire string just to test emptiness; len\(s\) != 0 is the bit-identical O\(1\) test`
}

func parenCall(s string) bool {
	return (utf8.RuneCountInString(s)) == 0 // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(s\) == 0 is the bit-identical O\(1\) test`
}

// A hex spelling of the literal is still the literal.
func hexLit(b []byte) bool {
	return utf8.RuneCount(b) != 0x0 // want `utf8\.RuneCount\(\.\.\.\) != 0 scans the entire \[\]byte just to test emptiness; len\(b\) != 0 is the bit-identical O\(1\) test`
}

// A named byte-slice argument is fine: len reads the same underlying
// bytes (utf8.RuneCount's parameter is the unnamed []byte, so the value
// was assignable to it unchanged).
type payload []byte

func namedBytes(p payload) bool {
	return utf8.RuneCount(p) == 0 // want `utf8\.RuneCount\(\.\.\.\) == 0 scans the entire \[\]byte just to test emptiness; len\(p\) == 0 is the bit-identical O\(1\) test`
}

// A named string type only ever arrives through an explicit conversion
// (it is not assignable to the plain string parameter); the conversion
// text stays verbatim under len, evaluated exactly once.
type header string

func namedString(h header) bool {
	return utf8.RuneCountInString(string(h)) == 0 // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(string\(h\)\) == 0 is the bit-identical O\(1\) test`
}

// A side-effecting argument passes through verbatim: one evaluation on
// either side, in the same position.
var calls int

func next() string { calls++; return "x" }

func sideEffect() bool {
	return utf8.RuneCountInString(next()) != 0 // want `utf8\.RuneCountInString\(\.\.\.\) != 0 scans the entire string just to test emptiness; len\(next\(\)\) != 0 is the bit-identical O\(1\) test`
}

// In an if condition — the canonical shape — and composed into a larger
// condition (the comparison node is untouched, so no parentheses move).
func inIf(s string) int {
	if utf8.RuneCountInString(s) == 0 { // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(s\) == 0 is the bit-identical O\(1\) test`
		return 0
	}
	return 1
}

func composed(s, t string) bool {
	return len(t) > 0 && utf8.RuneCountInString(s) > 0 // want `utf8\.RuneCountInString\(\.\.\.\) > 0 scans the entire string just to test emptiness; len\(s\) > 0 is the bit-identical O\(1\) test`
}

// A NAMED bool context is fine — the comparison node (whose type the
// context shaped) is untouched; only the callee inside it changes.
type flag bool

func namedBool(s string) flag {
	var f flag = utf8.RuneCountInString(s) == 0 // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(s\) == 0 is the bit-identical O\(1\) test`
	return f
}

// --- advisory: matched, but the fix is withheld ---

// A CONSTANT string argument would make len(...) a compile-time
// constant while the call is not (duplicate-switch-case hazard) — the
// report stays advisory.
func constArg() bool {
	return utf8.RuneCountInString("héllo") > 0 // want `utf8\.RuneCountInString\(\.\.\.\) > 0 scans the entire string just to test emptiness; len\("héllo"\) > 0 is the bit-identical O\(1\) test`
}

// A comment inside the replaced callee text would be destroyed by the
// edit — advisory.
func commentInCallee(s string) bool {
	return utf8. /* why? */ RuneCountInString(s) == 0 // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(s\) == 0 is the bit-identical O\(1\) test`
}

// A local declaration shadowing the builtin len would capture the
// rewrite — advisory.
func shadowedLen(s string) bool {
	len := func(string) int { return 0 }
	_ = len("")
	return utf8.RuneCountInString(s) == 0 // want `utf8\.RuneCountInString\(\.\.\.\) == 0 scans the entire string just to test emptiness; len\(s\) == 0 is the bit-identical O\(1\) test`
}

// --- negatives: not emptiness tests, or not this check's site ---

const zero = 0

var limit int

func negatives(s string, b []byte) {
	_ = utf8.RuneCountInString(s) == 1  // exactly one RUNE — len cannot answer that
	_ = utf8.RuneCountInString(s) != 1  // ditto
	_ = utf8.RuneCountInString(s) > 1   // multi-rune — uses the count
	_ = utf8.RuneCountInString(s) <= 1  // at most one rune — uses the count
	_ = utf8.RuneCountInString(s) >= 0  // constant true — not an emptiness test
	_ = utf8.RuneCountInString(s) < 0   // constant false
	_ = utf8.RuneCountInString(s) == 2  // uses the count
	_ = utf8.RuneCountInString(s) == zero  // named constant, not the literal
	_ = utf8.RuneCountInString(s) > limit // non-constant comparand
	n := utf8.RuneCountInString(s)
	_ = n == 0 // bound first — the variable may have other consumers
	_ = utf8.RuneCountInString(s)+1 == 1 // arithmetic context — not a direct operand
	_ = utf8.RuneCountInString(s) == utf8.RuneCountInString(s[1:]) // no literal side
	_ = utf8.RuneLen('x') == 0                                     // a different utf8 function
	_ = b
}

// PS2024's throwaway-conversion shapes are its sites: it rewrites the
// call to the zero-copy sibling first; this check stays silent here and
// picks the sibling up on a later pass.
func ps2024Owned(s string, b []byte) {
	_ = utf8.RuneCount([]byte(s)) == 0
	_ = utf8.RuneCountInString(string(b)) > 0
}

// utf8.RuneCount(nil): len(nil) does not compile, and the comparison is
// statically decidable — silent.
func nilArg() bool {
	return utf8.RuneCount(nil) == 0
}

// A same-named local function does not resolve to unicode/utf8.
func localFunc(s string) bool {
	RuneCountInString := func(string) int { return 0 }
	return RuneCountInString(s) == 0
}

// A shadowed utf8 identifier does not resolve to the package.
type fakeUtf8 struct{}

func (fakeUtf8) RuneCountInString(string) int { return 0 }

func shadowedPkg(s string) bool {
	utf8 := fakeUtf8{}
	return utf8.RuneCountInString(s) == 0
}
