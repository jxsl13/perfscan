package ps5103

import (
	"strings"
	stdstrings "strings"
)

func eqLower(a, b string) bool {
	return strings.ToLower(a) == strings.ToLower(b) // want `strings\.ToLower\(a\) == strings\.ToLower\(b\) allocates two converted strings; strings\.EqualFold\(a, b\) is allocation-free`
}

func neqLower(a, b string) bool {
	return strings.ToLower(a) != strings.ToLower(b) // want `strings\.ToLower\(a\) != strings\.ToLower\(b\) allocates two converted strings; !strings\.EqualFold\(a, b\) is allocation-free`
}

func eqUpper(a, b string) bool {
	return strings.ToUpper(a) == strings.ToUpper(b) // want `strings\.ToUpper\(a\) == strings\.ToUpper\(b\) allocates two converted strings; strings\.EqualFold\(a, b\) is allocation-free`
}

func neqUpper(a, b string) bool {
	return strings.ToUpper(a) != strings.ToUpper(b) // want `strings\.ToUpper\(a\) != strings\.ToUpper\(b\) allocates two converted strings; !strings\.EqualFold\(a, b\) is allocation-free`
}

// An aliased stdlib import is still the strings package.
func aliased(a, b string) bool {
	return stdstrings.ToLower(a) == stdstrings.ToLower(b) // want `strings\.ToLower\(a\) == strings\.ToLower\(b\) allocates two converted strings`
}

// Parenthesized operands are still the same pattern.
func parenthesized(a, b string) bool {
	return (strings.ToLower(a)) == (strings.ToLower(b)) // want `strings\.ToLower\(a\) == strings\.ToLower\(b\) allocates two converted strings`
}

// Composed into a larger condition.
func composed(a, b, c string) bool {
	return c != "" && strings.ToLower(a) == strings.ToLower(b) // want `strings\.ToLower\(a\) == strings\.ToLower\(b\) allocates two converted strings`
}

// --- guards: none of the following may be reported ---

// Ordering genuinely needs the converted strings; EqualFold answers only
// equality.
func ordering(a, b string) bool {
	return strings.ToLower(a) < strings.ToLower(b)
}

// One-sided: the right operand is not a conversion of the same kind.
func oneSided(a, b string) bool {
	return strings.ToLower(a) == b
}

// Mixed ToLower vs ToUpper is not a case-insensitive equality test.
func mixed(a, b string) bool {
	return strings.ToLower(a) == strings.ToUpper(b)
}

type fakeStrings struct{}

func (fakeStrings) ToLower(s string) string { return s }

// A shadowed `strings` identifier is not the stdlib package.
func shadowed(a, b string) bool {
	var strings fakeStrings
	return strings.ToLower(a) == strings.ToLower(b)
}

// A local function named ToLower selected through a struct field is not
// strings.ToLower.
type table struct {
	ToLower func(string) string
}

func viaField(t table, a, b string) bool {
	return t.ToLower(a) == t.ToLower(b)
}
