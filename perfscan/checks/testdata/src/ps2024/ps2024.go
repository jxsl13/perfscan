package ps2024

import "unicode/utf8"

func process(int) {}

// Basic assignment: the string direction.
func runeCount(s string) int {
	n := utf8.RuneCount([]byte(s)) // want `utf8\.RuneCount\(\[\]byte\(s\)\) copies s into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\(s\) is the bit-identical, zero-copy count`
	return n
}

// Inside a larger expression: the replacement is a call — a primary
// expression — so no parentheses are ever needed.
func runeCountExpr(s string) int {
	return utf8.RuneCount([]byte(s)) + 1 // want `utf8\.RuneCount\(\[\]byte\(s\)\) copies s into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\(s\) is the bit-identical, zero-copy count`
}

// Call-argument context.
func runeArg(s string) {
	process(utf8.RuneCount([]byte(s))) // want `utf8\.RuneCount\(\[\]byte\(s\)\) copies s into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\(s\) is the bit-identical, zero-copy count`
}

// An untyped string constant operand defaults to string.
func runeLit() int {
	return utf8.RuneCount([]byte("héllo wörld")) // want `utf8\.RuneCount\(\[\]byte\("héllo wörld"\)\) copies "héllo wörld" into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\("héllo wörld"\) is the bit-identical, zero-copy count`
}

// []uint8 spells the identical []byte type.
func runeUint8(s string) int {
	return utf8.RuneCount([]uint8(s)) // want `utf8\.RuneCount\(\[\]uint8\(s\)\) copies s into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\(s\) is the bit-identical, zero-copy count`
}

// Statement contexts stay legal: the replacement is itself a call.
func stmtCtx(s string) {
	defer utf8.RuneCount([]byte(s)) // want `utf8\.RuneCount\(\[\]byte\(s\)\) copies s into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\(s\) is the bit-identical, zero-copy count`
}

// Mirror direction: RuneCountInString(string(b)) -> RuneCount(b).
func mirror(b []byte) int {
	return utf8.RuneCountInString(string(b)) // want `utf8\.RuneCountInString\(string\(b\)\) copies b into a throwaway string just to count its runes; utf8\.RuneCount\(b\) is the bit-identical, zero-copy count`
}

// Mirror inside a larger expression.
func mirrorExpr(b []byte) int {
	return 2 * utf8.RuneCountInString(string(b)) // want `utf8\.RuneCountInString\(string\(b\)\) copies b into a throwaway string just to count its runes; utf8\.RuneCount\(b\) is the bit-identical, zero-copy count`
}

// Reported but NOT fixed: a comment inside the deleted scaffolding
// would be destroyed by the edits.
func commented(s string) int {
	return utf8.RuneCount([]byte( // keep me // want `utf8\.RuneCount\(\[\]byte\(s\)\) copies s into a throwaway \[\]byte just to count its runes; utf8\.RuneCountInString\(s\) is the bit-identical, zero-copy count`
		s))
}

type MyString string
type MyBytes []byte

// NAMED string operand: skipped entirely — utf8.RuneCountInString(s)
// would not compile without a kept no-op conversion.
func namedString(s MyString) int {
	return utf8.RuneCount([]byte(s))
}

// NAMED byte-slice operand in the mirror: skipped for the same reason.
func namedBytes(b MyBytes) int {
	return utf8.RuneCountInString(string(b))
}

// A conversion to a NAMED slice type never matches.
func namedConv(s string) int {
	return utf8.RuneCount(MyBytes(s))
}

// No conversion: both calls are already in their direct form.
func direct(b []byte, s string) int {
	return utf8.RuneCount(b) + utf8.RuneCountInString(s)
}

// A conversion result stored in a variable first may have other
// consumers and is out of scope.
func stored(s string) int {
	bs := []byte(s)
	return utf8.RuneCount(bs)
}

// A same-named local function is not unicode/utf8's.
func RuneCount(p []byte) int { return len(p) }

func localFunc(s string) int {
	return RuneCount([]byte(s))
}

// string([]rune) in the mirror is not a []byte conversion.
func runeSlice(rs []rune) int {
	return utf8.RuneCountInString(string(rs))
}

// string(rune) in the mirror is not a []byte conversion either.
func oneRune(r rune) int {
	return utf8.RuneCountInString(string(r))
}
