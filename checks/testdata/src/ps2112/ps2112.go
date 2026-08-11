package ps2112

// Nil-conversion base: the chain's result is nil exactly when
// slices.Concat's is — fixed, adding the slices import.
func concat2(a, b []int) []int {
	return append(append([]int(nil), a...), b...) // want `chained spread appends onto an empty base hand-roll a concatenation, growing the result per append; slices\.Concat sizes and allocates it once`
}

// Three spread segments collapse into one Concat call.
func concat3(a, b, c []string) []string {
	return append(append(append([]string(nil), a...), b...), c...) // want `chained spread appends onto an empty base hand-roll a concatenation, growing the result per append; slices\.Concat sizes and allocates it once`
}

// Parenthesized conversion form of the nil base: fixed too.
func parenConv(a, b []int) []int {
	return append(append(([]int)(nil), a...), b...) // want `chained spread appends onto an empty base hand-roll a concatenation, growing the result per append; slices\.Concat sizes and allocates it once`
}

// Empty literal base: empty but NON-nil — with all-empty operands the
// chain returns a non-nil empty slice where Concat returns nil, and
// nil-ness is observable. Reported, no fix.
func litBase(a, b []int) []int {
	return append(append([]int{}, a...), b...) // want `chained spread appends onto an empty base hand-roll a concatenation, growing the result per append; slices\.Concat sizes and allocates it once`
}

// make with zero length: same non-nil empty base — reported, no fix.
func makeBase(a, b []int) []int {
	return append(append(make([]int, 0), a...), b...) // want `chained spread appends onto an empty base hand-roll a concatenation, growing the result per append; slices\.Concat sizes and allocates it once`
}

// slices is shadowed at the call site: the rewrite could not reference
// the package — reported, no fix.
func shadowed(a, b []int) []int {
	slices := 0
	_ = slices
	return append(append([]int(nil), a...), b...) // want `chained spread appends onto an empty base hand-roll a concatenation, growing the result per append; slices\.Concat sizes and allocates it once`
}

// Non-empty base variable: append may extend dst's backing array in
// place — the normal way to extend a slice, and Concat would not. Silent.
func extend(dst, a, b []int) []int {
	return append(append(dst, a...), b...)
}

// make with an explicit capacity: the capacity exists to be reused by
// the appends (and by later ones) — silent.
func capBase(a, b []int) []int {
	return append(append(make([]int, 0, len(a)+len(b)), a...), b...)
}

// A single-element append in the chain is not a concat: silent.
func mixed(a []int, v int) []int {
	return append(append([]int(nil), a...), v)
}

// String spread into []byte: slices.Concat cannot take a string — silent.
func byteString(bs []byte, s string) []byte {
	return append(append([]byte(nil), bs...), s...)
}

// A single spread append is a clone, not a concat chain: silent.
func clone(a []int) []int {
	return append([]int(nil), a...)
}

type ints []int

// Named slice operand: Concat would infer a different result type (or
// fail to infer one at all) — silent.
func named(a ints, b []int) []int {
	return append(append([]int(nil), a...), b...)
}
