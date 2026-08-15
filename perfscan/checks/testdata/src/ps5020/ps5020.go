package ps5020

func chunk() string { return "chunk" }

type MyStr string
type Bytes []byte
type bs = []byte

// The canonical shape: the []byte conversion materializes a throwaway
// slice that append immediately copies out of. Rewritten to the builtin
// string-append special form.
func appendBasic(dst []byte, s string) []byte {
	return append(dst, []byte(s)...) // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
}

// An untyped string constant operand: append(dst, "lit"...) is legal
// and identical.
func appendLiteral(dst []byte) []byte {
	return append(dst, []byte("header: ")...) // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
}

// []uint8 spells the identical unnamed type, and an alias of []byte
// resolves to it.
func appendSpellings(dst []byte, s string) []byte {
	dst = append(dst, []uint8(s)...) // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	dst = append(dst, bs(s)...)      // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	return dst
}

// A named string operand and a named byte-slice destination: the
// bytestring special case accepts both, so the rewrite still
// type-checks.
func appendNamed(nb Bytes, m MyStr) Bytes {
	return append(nb, []byte(m)...) // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
}

// Operand expressions — including calls — keep the fix: s is evaluated
// exactly once in both forms, so side effects and evaluation order are
// untouched. The spread's ... covers the whole final argument, so even
// a binary expression needs no parentheses.
func appendExpr(dst []byte, name string, parts []string, i int) []byte {
	dst = append(dst, []byte(name+"!")...)     // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	dst = append(dst, []byte(chunk())...)      // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	dst = append(dst, []byte(parts[i])...)     // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	dst = append(dst, []byte(string(dst))...)  // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	return dst
}

// Parenthesized variants: the parens around the conversion or the
// operand survive verbatim; only the conversion scaffolding is deleted.
func appendParens(dst []byte, s string) []byte {
	dst = append(dst, ([]byte(s))...) // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	dst = append(dst, []byte((s))...) // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	dst = (append)(dst, []byte(s)...) // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	return dst
}

// A comment inside the deleted conversion scaffolding would be silently
// destroyed by the rewrite — advisory.
func appendCommented(dst []byte, s string) []byte {
	dst = append(dst, []byte( /* keep */ s)...) // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	dst = append(dst, []byte(s /* tail */)...)  // want `append\(dst, \[\]byte\(s\)\.\.\.\) converts s to a throwaway byte slice`
	return dst
}
