package ps2011

import (
	str "strings"
	"strings"
)

func sideEffect() string { return "x" }

// The plain shape: constant seed and count.
func constBoth() []byte {
	return []byte(strings.Repeat("ab", 64)) // want `\[\]byte\(strings\.Repeat\(s, n\)\) allocates the repeated string and then a second \[\]byte buffer it is copied into; bytes\.Repeat\(\[\]byte\(s\), n\) fills a single buffer — one allocation and one copy instead of two \(advisory: the result's capacity and the panic message can differ\)`
}

// Variable seed and count: the same double-build pattern.
func variables(s string, n int) []byte {
	return []byte(strings.Repeat(s, n)) // want `\[\]byte\(strings\.Repeat\(s, n\)\) allocates the repeated string and then a second \[\]byte buffer it is copied into; bytes\.Repeat\(\[\]byte\(s\), n\) fills a single buffer — one allocation and one copy instead of two \(advisory: the result's capacity and the panic message can differ\)`
}

// A side-effecting seed is still the pattern (the advisory rewrite keeps
// it evaluated once, in the same order).
func seedWithCall() []byte {
	return []byte(strings.Repeat(sideEffect(), 3)) // want `\[\]byte\(strings\.Repeat\(s, n\)\) allocates the repeated string and then a second \[\]byte buffer it is copied into; bytes\.Repeat\(\[\]byte\(s\), n\) fills a single buffer — one allocation and one copy instead of two \(advisory: the result's capacity and the panic message can differ\)`
}

// The parenthesized conversion form is the same conversion.
func parenType() []byte {
	return ([]byte)(strings.Repeat("=", 40)) // want `\[\]byte\(strings\.Repeat\(s, n\)\) allocates the repeated string and then a second \[\]byte buffer it is copied into; bytes\.Repeat\(\[\]byte\(s\), n\) fills a single buffer — one allocation and one copy instead of two \(advisory: the result's capacity and the panic message can differ\)`
}

// A parenthesized operand is the same call.
func parenOperand() []byte {
	return []byte((strings.Repeat("-", 12))) // want `\[\]byte\(strings\.Repeat\(s, n\)\) allocates the repeated string and then a second \[\]byte buffer it is copied into; bytes\.Repeat\(\[\]byte\(s\), n\) fills a single buffer — one allocation and one copy instead of two \(advisory: the result's capacity and the panic message can differ\)`
}

// An ALIASED strings import still matches: the qualifier resolves to the
// strings package.
func aliasQualifier() []byte {
	return []byte(str.Repeat("ab", 4)) // want `\[\]byte\(strings\.Repeat\(s, n\)\) allocates the repeated string and then a second \[\]byte buffer it is copied into; bytes\.Repeat\(\[\]byte\(s\), n\) fills a single buffer — one allocation and one copy instead of two \(advisory: the result's capacity and the panic message can differ\)`
}

// --- silent shapes: no report at all ---

type definedByteSlice []byte

// A DEFINED slice conversion target is a different value — silent.
func definedTarget() definedByteSlice {
	return definedByteSlice(strings.Repeat("a", 2))
}

// The repetition used as a string (no []byte conversion) is PS2003's
// territory, not PS2011's — silent.
func noConversion() string {
	return strings.Repeat("a", 2)
}

// A []rune conversion is a different (decoding) operation — silent.
func runeConversion() []rune {
	return []rune(strings.Repeat("a", 2))
}

type fakeStrings struct{}

func (fakeStrings) Repeat(s string, n int) string { return s }

// A shadowed strings identifier does not resolve to the package — silent.
func shadowedStrings() []byte {
	strings := fakeStrings{}
	return []byte(strings.Repeat("a", 2))
}

// The operand must be DIRECTLY the Repeat call: a derived expression is a
// different pattern — silent.
func derivedOperand(s string) []byte {
	return []byte(strings.Repeat(s, 1) + "!")
}
