package ps2045

import "bytes"

// Both operators on value-typed (provably non-nil) receivers are
// rewritten; the whole comparison becomes one bytes.Equal call
// (negated for !=).
func forms() {
	var a, b bytes.Buffer
	_ = a.String() == b.String()  // want `a\.String\(\) == b\.String\(\) copies both whole bytes\.Buffers just to compare them; bytes\.Equal\(a\.Bytes\(\), b\.Bytes\(\)\) tests the same bytes with no copy and no allocation`
	_ = a.String() != b.String()  // want `a\.String\(\) != b\.String\(\) copies both whole bytes\.Buffers just to compare them; !bytes\.Equal\(a\.Bytes\(\), b\.Bytes\(\)\) tests`
	if a.String() == b.String() { // want `a\.String\(\) == b\.String\(\) copies both whole bytes\.Buffers`
		return
	}
}

// The receivers carry over byte-verbatim: struct fields, slice and
// array elements, in any combination.
type holder struct{ b bytes.Buffer }

func receivers(h holder, bufs []bytes.Buffer, arr [2]bytes.Buffer) {
	if h.b.String() == bufs[0].String() { // want `h\.b\.String\(\) == bufs\[0\]\.String\(\) copies both whole bytes\.Buffers just to compare them; bytes\.Equal\(h\.b\.Bytes\(\), bufs\[0\]\.Bytes\(\)\) tests`
		return
	}
	for arr[1].String() != h.b.String() { // want `arr\[1\]\.String\(\) != h\.b\.String\(\) copies both whole bytes\.Buffers just to compare them; !bytes\.Equal\(arr\[1\]\.Bytes\(\), h\.b\.Bytes\(\)\) tests`
		break
	}
}

// Provably non-nil pointer receivers — an address-of expression or a
// direct new(bytes.Buffer) — are rewritten too, on either or both
// sides.
func provablyNonNil() {
	var a, b bytes.Buffer
	_ = (&a).String() == (&b).String()           // want `\(&a\)\.String\(\) == \(&b\)\.String\(\) copies both whole bytes\.Buffers just to compare them; bytes\.Equal\(\(&a\)\.Bytes\(\), \(&b\)\.Bytes\(\)\) tests`
	_ = new(bytes.Buffer).String() != a.String() // want `new\(bytes\.Buffer\)\.String\(\) != a\.String\(\) copies both whole bytes\.Buffers`
	_ = a.String() == new(bytes.Buffer).String() // want `a\.String\(\) == new\(bytes\.Buffer\)\.String\(\) copies both whole bytes\.Buffers`
}

// The same receiver on both sides is still the pattern (a tautology
// the rewrite preserves: both sides read the same unread window).
func selfCompare(a bytes.Buffer) {
	_ = a.String() == a.String() // want `a\.String\(\) == a\.String\(\) copies both whole bytes\.Buffers`
}

// An alias of bytes.Buffer is the identical type — the spelling does
// not matter, on either side.
type bufAlias = bytes.Buffer

func aliased(x bufAlias, y bytes.Buffer) bool {
	return x.String() == y.String() // want `x\.String\(\) == y\.String\(\) copies both whole bytes\.Buffers`
}

// Parenthesized shapes: parens around either String() call vanish with
// the replaced comparison.
func parens() {
	var a, b bytes.Buffer
	_ = (a.String()) == (b.String()) // want `\(a\.String\(\)\) == \(b\.String\(\)\) copies both whole bytes\.Buffers just to compare them; bytes\.Equal\(a\.Bytes\(\), b\.Bytes\(\)\) tests`
}

// Embedded in a larger boolean expression: only the comparison is
// replaced, and the prefixed ! of the != form binds tighter than &&,
// so no parentheses are needed.
func embedded(other bool) bool {
	var a, b bytes.Buffer
	return other && a.String() != b.String() // want `a\.String\(\) != b\.String\(\) copies both whole bytes\.Buffers`
}
