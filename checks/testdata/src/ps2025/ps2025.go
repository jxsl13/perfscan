package ps2025

import "unicode/utf8"

func basicForward(s string) bool {
	return utf8.Valid([]byte(s)) // want `utf8\.Valid\(\[\]byte\(s\)\) copies the whole string through a throwaway conversion just to validate it; utf8\.ValidString\(s\) runs the identical validation in place — same bool for every input, no copy, no allocation`
}

func basicReverse(b []byte) bool {
	return utf8.ValidString(string(b)) // want `utf8\.ValidString\(string\(b\)\) copies the whole \[\]byte through a throwaway conversion just to validate it; utf8\.Valid\(b\) runs the identical validation in place — same bool for every input, no copy, no allocation`
}

// []uint8 is the identical type — the spelling does not matter.
func uint8Spelling(s string) bool {
	return utf8.Valid([]uint8(s)) // want `utf8\.Valid\(\[\]uint8\(s\)\) copies the whole string`
}

// The operand carries over byte-verbatim: selectors, index expressions,
// calls with side effects (evaluated exactly once in both spellings), and
// untyped string constants all stay in place.
type payload struct{ body string }

func produce() string { return "x" }

func operands(p payload, ss []string, m map[string][]byte) {
	_ = utf8.Valid([]byte(p.body))       // want `utf8\.ValidString\(p\.body\) runs the identical validation in place`
	_ = utf8.Valid([]byte(ss[0]))        // want `utf8\.ValidString\(ss\[0\]\) runs the identical validation in place`
	_ = utf8.Valid([]byte(produce()))    // want `utf8\.ValidString\(produce\(\)\) runs the identical validation in place`
	_ = utf8.Valid([]byte("abc\xff"))    // want `utf8\.ValidString\("abc\\xff"\) runs the identical validation in place`
	_ = utf8.ValidString(string(m["k"])) // want `utf8\.Valid\(m\["k"\]\) runs the identical validation in place`
}

// Parenthesized shapes: the redundant parens around the conversion are
// deleted with it; parens inside the conversion stay verbatim.
func parens(s string, b []byte) {
	_ = utf8.Valid(([]byte(s)))       // want `utf8\.ValidString\(s\) runs the identical validation in place`
	_ = utf8.Valid([]byte((s)))       // want `utf8\.ValidString\(\(s\)\) runs the identical validation in place`
	_ = utf8.ValidString((string)(b)) // want `utf8\.Valid\(b\) runs the identical validation in place`
}

func inControlFlow(s string, b []byte) string {
	if !utf8.Valid([]byte(s)) { // want `utf8\.ValidString\(s\) runs the identical validation in place`
		return ""
	}
	for utf8.ValidString(string(b)) { // want `utf8\.Valid\(b\) runs the identical validation in place`
		break
	}
	return s
}

// A comment inside the conversion syntax the fix would delete withholds
// the automatic fix: the report stays advisory and the comment survives.
func commentAdvisory(s string) bool {
	return utf8.Valid([]byte(s /* raw */)) // want `a comment inside the conversion syntax withholds the automatic fix — rewrite by hand`
}
