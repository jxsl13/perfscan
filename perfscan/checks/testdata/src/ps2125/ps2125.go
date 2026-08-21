package ps2125

func process(int) {}

// Basic assignment; the file has no unicode/utf8 import yet, so the
// first rune-form fix adds it.
func runeCount(s string) int {
	n := len([]rune(s)) // want `len\(\[\]rune\(s\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\(s\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
	return n
}

// Inside a larger expression: the replacement is a call — a primary
// expression — so no parentheses are ever needed.
func runeCountExpr(s string) int {
	n := len([]rune(s)) + 1 // want `len\(\[\]rune\(s\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\(s\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
	return n
}

// Call-argument context.
func runeArg(s string) {
	process(len([]rune(s))) // want `len\(\[\]rune\(s\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\(s\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
}

// An untyped string constant operand defaults to string.
func runeLit() int {
	return len([]rune("héllo wörld")) // want `len\(\[\]rune\("héllo wörld"\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\("héllo wörld"\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
}

// []int32 spells the identical []rune type.
func runeInt32(s string) int {
	return len([]int32(s)) // want `len\(\[\]int32\(s\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\(s\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
}

// []byte form: only the conversion is dropped, the outer len stays.
func byteCount(s string) int {
	return len([]byte(s)) // want `len\(\[\]byte\(s\)\) spells a byte length as a throwaway \[\]byte conversion; len\(s\) is the direct, bit-identical byte count \(allocation-free on every toolchain\)`
}

// []byte inside a larger expression: still just len(s), the surrounding
// len call already delimits it.
func byteExpr(s string) int {
	return 2 * len([]byte(s)) // want `len\(\[\]byte\(s\)\) spells a byte length as a throwaway \[\]byte conversion; len\(s\) is the direct, bit-identical byte count \(allocation-free on every toolchain\)`
}

// []uint8 spells the identical []byte type.
func byteUint8(s string) int {
	return len([]uint8(s)) // want `len\(\[\]uint8\(s\)\) spells a byte length as a throwaway \[\]byte conversion; len\(s\) is the direct, bit-identical byte count \(allocation-free on every toolchain\)`
}

// Reported but not fixed: the rewrite would make this a constant expression.
func byteLiteral() int {
	return len([]byte("literal")) // want `len\(\[\]byte\("literal"\)\) spells a byte length as a throwaway \[\]byte conversion; len\("literal"\) is the direct, bit-identical byte count \(allocation-free on every toolchain\)`
}

// Reported but NOT fixed: a comment inside the rewritten scaffolding
// would be destroyed by the edits.
func commentedRune(s string) int {
	return len( // keep me // want `len\(\[\]rune\(s\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\(s\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
		[]rune(s))
}

func commentedByte(s string) int {
	return len([]byte( // keep me // want `len\(\[\]byte\(s\)\) spells a byte length as a throwaway \[\]byte conversion; len\(s\) is the direct, bit-identical byte count \(allocation-free on every toolchain\)`
		s))
}

// The utf8 qualifier is shadowed at the call site: the rewrite could
// not reference the package — reported, no fix.
func shadowedUtf8(s string) int {
	utf8 := 0
	_ = utf8
	return len([]rune(s)) // want `len\(\[\]rune\(s\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\(s\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
}

// --- guards: none of the following may be reported ---

// Identity conversion of a []byte: the operand is not a string, there
// is no string copy to drop.
func notStringByte(b []byte) int {
	return len([]byte(b))
}

// Identity conversion of a []rune likewise.
func notStringRune(r []rune) int {
	return len([]rune(r))
}

// A named string type is skipped deliberately: only exactly string.
type myString string

func namedString(m myString) int {
	return len([]rune(m))
}

// Plain len of a string is already minimal.
func plainLen(s string) int {
	return len(s)
}

// The conversion stored in a variable first may have other consumers.
func storedFirst(s string) int {
	r := []rune(s)
	return len(r)
}

// A named slice type is not the canonical []rune spelling.
type runes []rune

func namedSliceType(s string) int {
	return len(runes(s))
}

// The outer call is not the builtin len.
func notLen(s string) int {
	return cap([]rune(s))
}

// A shadowed len is not the builtin.
func shadowedLen(s string) int {
	len := func([]rune) int { return 0 }
	return len([]rune(s))
}
