package ps2107

import "fmt"

// %s-over-[]byte cases live in their own file so PS2130 (which owns %s over a
// STRING) is irrelevant: every %s below has a []byte operand. The rewrite is
// the builtin conversion string(bs) — bit-identical to %s over a []byte
// (raw bytes verbatim, invalid UTF-8 preserved, nil -> "") and needs no import.

// %s over a plain []byte identifier: fixed to string(b).
func sBytes(b []byte) string {
	return fmt.Sprintf("%s", b) // want `fmt\.Sprintf of a single %s \[\]byte value boxes the argument and walks fmt's formatter state machine; the builtin conversion string\(bs\) yields the bytes directly`
}

type payload struct {
	body []byte
}

// %s over a []byte-typed selector: the selector is spliced verbatim.
func sBytesField(p payload) string {
	return fmt.Sprintf("%s", p.body) // want `fmt\.Sprintf of a single %s \[\]byte value boxes the argument and walks fmt's formatter state machine; the builtin conversion string\(bs\) yields the bytes directly`
}

// %s over a []byte built by a conversion expression: still an unnamed []byte,
// the whole conversion is spliced as the string() operand.
func sBytesConv(s string) string {
	return fmt.Sprintf("%s", []byte(s)) // want `fmt\.Sprintf of a single %s \[\]byte value boxes the argument and walks fmt's formatter state machine; the builtin conversion string\(bs\) yields the bytes directly`
}

type Raw []byte

// String makes Raw a fmt.Stringer: %s prints "raw", NOT the bytes, so a
// string(r) rewrite would change behavior.
func (Raw) String() string { return "raw" }

// NAMED []byte type with a String() method: %s honors the Stringer and
// string(r) would not — reported, NO fix (golden identical). Pins the
// unnamed-slice guard (same reasoning as the %x []byte arm).
func sNamedStringer(r Raw) string {
	return fmt.Sprintf("%s", r) // want `fmt\.Sprintf of a single %s \[\]byte value boxes the argument and walks fmt's formatter state machine; the builtin conversion string\(bs\) yields the bytes directly`
}

// %v over a []byte prints the NUMERIC slice form ("[104 105]"), NOT the
// string, so it is NOT string(b) and must stay silent and unfixed here.
func vBytes(b []byte) string {
	return fmt.Sprintf("%v", b)
}
