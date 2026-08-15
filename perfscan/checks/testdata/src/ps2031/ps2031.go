package ps2031

import "bytes"

// Both operators in both operand orders on a value-typed (provably
// non-nil) receiver are rewritten; the whole comparison becomes one
// bytes.Equal call (negated for !=).
func forms() {
	var buf bytes.Buffer
	_ = buf.String() == "OK"  // want `buf\.String\(\) == "OK" copies the whole bytes\.Buffer just to compare it against a constant string; bytes\.Equal\(buf\.Bytes\(\), \[\]byte\("OK"\)\) tests the same bytes with no copy and no allocation`
	_ = buf.String() != "OK"  // want `buf\.String\(\) != "OK" copies the whole bytes\.Buffer just to compare it against a constant string; !bytes\.Equal\(buf\.Bytes\(\), \[\]byte\("OK"\)\) tests`
	_ = "OK" == buf.String()  // want `"OK" == buf\.String\(\) copies the whole bytes\.Buffer just to compare it against a constant string; bytes\.Equal\(buf\.Bytes\(\), \[\]byte\("OK"\)\) tests`
	_ = "OK" != buf.String()  // want `"OK" != buf\.String\(\) copies the whole bytes\.Buffer`
	if buf.String() == "ready\n" { // want `buf\.String\(\) == "ready\\n" copies the whole bytes\.Buffer`
		return
	}
}

// The constant operand carries over verbatim: named constants, raw
// string literals, escape sequences (including invalid UTF-8 and NUL
// bytes — both sides are pure byte compares), and constant
// concatenations.
const ok = "OK"

func constants(buf bytes.Buffer) {
	_ = buf.String() == ok             // want `buf\.String\(\) == ok copies the whole bytes\.Buffer just to compare it against a constant string; bytes\.Equal\(buf\.Bytes\(\), \[\]byte\(ok\)\) tests`
	_ = buf.String() == `raw "quoted"` // want "buf\\.String\\(\\) == `raw \"quoted\"` copies the whole bytes\\.Buffer"
	_ = buf.String() == "\xff\x00b"    // want `copies the whole bytes\.Buffer`
	_ = buf.String() != "a" + "b"      // want `copies the whole bytes\.Buffer`
}

// The receiver carries over byte-verbatim: struct fields, slice and
// array elements.
type holder struct{ b bytes.Buffer }

func receivers(h holder, bufs []bytes.Buffer, arr [2]bytes.Buffer) {
	if h.b.String() == "OK" { // want `h\.b\.String\(\) == "OK" copies the whole bytes\.Buffer just to compare it against a constant string; bytes\.Equal\(h\.b\.Bytes\(\), \[\]byte\("OK"\)\) tests`
		return
	}
	for bufs[0].String() != "done" { // want `bufs\[0\]\.String\(\) != "done" copies the whole bytes\.Buffer just to compare it against a constant string; !bytes\.Equal\(bufs\[0\]\.Bytes\(\), \[\]byte\("done"\)\) tests`
		break
	}
	_ = arr[1].String() == "x" // want `arr\[1\]\.String\(\) == "x" copies the whole bytes\.Buffer`
}

// Provably non-nil pointer receivers — an address-of expression or a
// direct new(bytes.Buffer) — are rewritten too.
func provablyNonNil() {
	var buf bytes.Buffer
	_ = (&buf).String() == "OK"           // want `\(&buf\)\.String\(\) == "OK" copies the whole bytes\.Buffer just to compare it against a constant string; bytes\.Equal\(\(&buf\)\.Bytes\(\), \[\]byte\("OK"\)\) tests`
	_ = new(bytes.Buffer).String() != "x" // want `new\(bytes\.Buffer\)\.String\(\) != "x" copies the whole bytes\.Buffer`
}

// An alias of bytes.Buffer is the identical type — the spelling does
// not matter.
type bufAlias = bytes.Buffer

func aliased(buf bufAlias) bool {
	return buf.String() == "OK" // want `buf\.String\(\) == "OK" copies the whole bytes\.Buffer`
}

// Parenthesized shapes: parens around the String() call or the
// constant vanish with the replaced comparison.
func parens() {
	var buf bytes.Buffer
	_ = (buf.String()) == "OK" // want `\(buf\.String\(\)\) == "OK" copies the whole bytes\.Buffer`
	_ = buf.String() == ("OK") // want `buf\.String\(\) == \("OK"\) copies the whole bytes\.Buffer just to compare it against a constant string; bytes\.Equal\(buf\.Bytes\(\), \[\]byte\("OK"\)\) tests`
}

// Embedded in a larger boolean expression: only the comparison is
// replaced, and the prefixed ! of the != form binds tighter than &&,
// so no parentheses are needed.
func embedded(other bool) bool {
	var buf bytes.Buffer
	return other && buf.String() != "OK" // want `buf\.String\(\) != "OK" copies the whole bytes\.Buffer`
}
