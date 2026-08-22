package ps2108

func direct(s string) string {
	return string([]byte(s)) // want `string\(\[\]byte\(x\)\) round-trips a string through a throwaway byte slice \(two copies\); the expression equals x — use the string directly`
}

// A call argument is evaluated exactly once either way.
func viaCall(f func() string) string {
	return string([]byte(f())) // want `string\(\[\]byte\(x\)\) round-trips a string through a throwaway byte slice \(two copies\); the expression equals x — use the string directly`
}

// A non-primary argument keeps its grouping: the call was a primary
// expression, so the replacement is parenthesized before being indexed.
func concatIndexed(a, b string) byte {
	return string([]byte(a + b))[0] // want `string\(\[\]byte\(x\)\) round-trips a string through a throwaway byte slice \(two copies\); the expression equals x — use the string directly`
}

// An alias of []byte is the identical type; the round-trip is the same.
type raw = []byte

func aliased(s string) string {
	return string(raw(s)) // want `string\(\[\]byte\(x\)\) round-trips a string through a throwaway byte slice \(two copies\); the expression equals x — use the string directly`
}

// A constant argument is advisory only: substituting a constant expression
// for a non-constant one can change compile-time properties (duplicate
// switch cases), so no fix is offered and the code stays as written.
const greeting = "hello"

func constant() string {
	return string([]byte(greeting)) // want `string\(\[\]byte\(x\)\) round-trips a string through a throwaway byte slice \(two copies\); the expression equals x — use the string directly`
}

// --- guards: none of the following may be reported or rewritten ---

// A plain string(bs) over a byte slice is a genuine conversion.
func genuine(bs []byte) string {
	return string(bs)
}

// The reverse direction []byte(string(b)) makes a defensive copy that
// un-aliases the slice — it may be relied upon and is never touched.
func reverse(b []byte) []byte {
	return []byte(string(b))
}

// A NAMED string argument: the round-trip converts named -> string, so
// removing it would change the expression's static type.
type id string

func named(v id) string {
	return string([]byte(v))
}

// A named result type is not the predeclared string conversion.
type myStr string

func namedOuter(s string) myStr {
	return myStr([]byte(s))
}

// A defined (not aliased) byte-slice type is not the []byte conversion.
type blob []byte

func definedSlice(s string) string {
	return string(blob(s))
}

// A shadowed identifier spelled string is not the predeclared conversion.
func shadowed(s string) []byte {
	string := func(b []byte) []byte { return b }
	return string([]byte(s))
}
