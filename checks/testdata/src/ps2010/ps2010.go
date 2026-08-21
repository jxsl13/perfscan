package ps2010

import "bytes"

func use(bool) {}

// Basic condition: both sides are []byte conversions of plain strings.
func basic(a, b string) {
	if bytes.Equal([]byte(a), []byte(b)) { // want `bytes\.Equal`
		use(true)
	}
}

// Negation: == binds looser than !, so the fix parenthesizes.
func negated(a, b string) bool {
	return !bytes.Equal([]byte(a), []byte(b)) // want `bytes\.Equal`
}

// && and || bind looser than ==: no parentheses needed.
func logical(a, b string, ok bool) bool {
	return bytes.Equal([]byte(a), []byte(b)) && ok // want `bytes\.Equal`
}

// Comparison parent: the LEFT operand needs no parentheses (== is
// left-associative); the RIGHT operand does.
func cmpParent(a, b string, ok bool) bool {
	l := bytes.Equal([]byte(a), []byte(b)) == ok // want `bytes\.Equal`
	r := ok == bytes.Equal([]byte(a), []byte(b)) // want `bytes\.Equal`
	return l != r
}

// Delimited contexts: call argument and map index need no parentheses.
func delimited(a, b string, m map[bool]int) int {
	use(bytes.Equal([]byte(a), []byte(b)))      // want `bytes\.Equal`
	return m[bytes.Equal([]byte(a), []byte(b))] // want `bytes\.Equal`
}

// Argument shapes spliced verbatim: selector and call result.
type pair struct{ x, y string }

func shapes(p pair, f func() string) bool {
	return bytes.Equal([]byte(p.x), []byte(f())) // want `bytes\.Equal`
}

// Side effects are preserved: each operand is still evaluated exactly
// once, left to right, in the rewritten form.
func sideEffects(f, g func() string) bool {
	return bytes.Equal([]byte(f()), []byte(g())) // want `bytes\.Equal`
}

// ONE constant operand is fine — the comparison stays non-constant.
func oneConst(s string) bool {
	return bytes.Equal([]byte("key"), []byte(s)) // want `bytes\.Equal`
}

// An alias of []byte is the identical type and matches.
type bs = []byte

func alias(a, b string) bool {
	return bytes.Equal(bs(a), bs(b)) // want `bytes\.Equal`
}

// Parenthesized conversions still match; the parens are absorbed by the
// replacement.
func parens(a, b string) bool {
	return bytes.Equal(([]byte(a)), []byte(b)) // want `bytes\.Equal`
}

// NEGATIVE (advisory): both operands constant — s1 == s2 would be a
// compile-time constant while bytes.Equal(...) is not. Report, no fix.
func bothConst() bool {
	return bytes.Equal([]byte("a"), []byte("b")) // want `bytes\.Equal`
}

// NEGATIVE (advisory): a comment inside the replaced call would be
// dropped by the rewrite — the fix is withheld.
func commented(a, b string) bool {
	return bytes.Equal([]byte(a) /* keep me */, []byte(b)) // want `bytes\.Equal`
}

// NEGATIVE (advisory): a bare expression statement, go, and defer
// syntactically require a call — `a == b` would not compile there, so
// the fix is withheld (including behind parentheses).
func stmtOnly(a, b string) {
	bytes.Equal([]byte(a), []byte(b))       // want `bytes\.Equal`
	(bytes.Equal([]byte(a), []byte(b)))     // want `bytes\.Equal`
	go bytes.Equal([]byte(a), []byte(b))    // want `bytes\.Equal`
	defer bytes.Equal([]byte(a), []byte(b)) // want `bytes\.Equal`
}

// NEGATIVE: a named string type is not reported at all — mixed named and
// unnamed strings are not comparable, and even same-named operands belong
// to the named type's own semantics.
type name string

func named(a, b name) bool {
	return bytes.Equal([]byte(a), []byte(b))
}

func mixedNamed(a name, b string) bool {
	return bytes.Equal([]byte(a), []byte(b))
}

// NEGATIVE: a defined byte-slice type is not the predeclared []byte.
type buf []byte

func namedSlice(a, b string) bool {
	return bytes.Equal(buf(a), []byte(b))
}

// NEGATIVE: operands that are not string conversions — a []byte value,
// nil, or []byte of a []byte (a conversion, but not from string).
func notConv(a []byte, s string) bool {
	if bytes.Equal(a, []byte(s)) {
		return true
	}
	if bytes.Equal(nil, []byte(s)) {
		return true
	}
	return bytes.Equal([]byte(a), []byte(s))
}

// NEGATIVE: a shadowed bytes does not resolve to the stdlib function.
type fakeBytes struct{}

func (fakeBytes) Equal(a, b []byte) bool { return false }

func shadowed(a, b string) bool {
	bytes := fakeBytes{}
	return bytes.Equal([]byte(a), []byte(b))
}
