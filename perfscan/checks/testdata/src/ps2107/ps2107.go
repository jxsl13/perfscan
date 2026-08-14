package ps2107

import (
	"fmt"
)

// %d over a plain int: fixed to strconv.Itoa.
func decimal(i int) string {
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// %d over a narrower signed type: fixed to strconv.FormatInt.
func narrow(i int16) string {
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// %d over an unsigned type: fixed to strconv.FormatUint.
func wide(u uint64) string {
	return fmt.Sprintf("%d", u) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// %t over a plain bool: fixed to strconv.FormatBool.
func truth(b bool) string {
	return fmt.Sprintf("%t", b) // want `fmt\.Sprintf of a single %t value boxes the argument and walks fmt's formatter state machine; strconv\.FormatBool converts it directly`
}

// %x over a plain []byte: fixed to hex.EncodeToString.
func dump(bs []byte) string {
	return fmt.Sprintf("%x", bs) // want `fmt\.Sprintf of a single %x \[\]byte value boxes the argument and walks fmt's formatter state machine; hex\.EncodeToString converts it directly`
}

// %s over a plain string is an IDENTITY, not a conversion: PS2107 leaves it to
// PS2130 (which owns fmt.Sprintf("%s"/"%v", s) and fmt.Sprint). Silent here.
func same(s string) string {
	return fmt.Sprintf("%s", s)
}

type port int

// Named integer type: it could implement fmt.Formatter, which fmt honors
// for %d and strconv would not — reported, no fix.
func named(p port) string {
	return fmt.Sprintf("%d", p) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

type blob []byte

// Named byte-slice type: could implement fmt.Formatter and is not
// assignable to hex's []byte parameter — reported, no fix.
func namedBytes(b blob) string {
	return fmt.Sprintf("%x", b) // want `fmt\.Sprintf of a single %x \[\]byte value boxes the argument and walks fmt's formatter state machine; hex\.EncodeToString converts it directly`
}

// strconv is shadowed at the call site: the rewrite could not reference
// the package — reported, no fix.
func shadowed(i int) string {
	strconv := i
	_ = strconv
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// Two verbs need real formatting: silent.
func pair(a, b int) string {
	return fmt.Sprintf("%d-%d", a, b)
}

// Literal text around the verb is not a bare conversion: silent.
func labeled(i int) string {
	return fmt.Sprintf("id=%d", i)
}

// %x over a string is out of scope: silent.
func hexString(s string) string {
	return fmt.Sprintf("%x", s)
}

// %g over a plain float64: fixed to strconv.FormatFloat with bitSize 64.
func fg(f float64) string {
	return fmt.Sprintf("%g", f) // want `fmt\.Sprintf of a single %g float value boxes the argument and walks fmt's formatter state machine; strconv\.FormatFloat converts it directly`
}

// %g over a plain float32: fixed to strconv.FormatFloat with a float64
// widening and bitSize 32.
func fg32(f float32) string {
	return fmt.Sprintf("%g", f) // want `fmt\.Sprintf of a single %g float value boxes the argument and walks fmt's formatter state machine; strconv\.FormatFloat converts it directly`
}

type Celsius float64

// Named float type: it could implement fmt.Formatter, which fmt honors for
// %g and strconv.FormatFloat would not — reported, no fix.
func fgn(c Celsius) string {
	return fmt.Sprintf("%g", c) // want `fmt\.Sprintf of a single %g float value boxes the argument and walks fmt's formatter state machine; strconv\.FormatFloat converts it directly`
}

// %e defaults to 6 digits, not the shortest form FormatFloat(-1) prints:
// silent.
func fe(f float64) string {
	return fmt.Sprintf("%e", f)
}

// A precision makes %g no longer a bare verb: silent.
func fgPrec(f float64) string {
	return fmt.Sprintf("%.3g", f)
}

// %g over a non-float argument is out of scope: silent.
func fgInt() string {
	return fmt.Sprintf("%g", 42)
}

// Literal text around the verb is not a bare conversion: silent.
func fgLabeled(f float64) string {
	return fmt.Sprintf("x=%g", f)
}

// %b over an int: base-2 via FormatInt.
func fb(i int) string {
	return fmt.Sprintf("%b", i) // want `fmt\.Sprintf of a single %b value boxes the argument and walks fmt's formatter state machine; strconv\.FormatInt/FormatUint with base 2 converts it directly`
}

// %b over a uint64: base-2 via FormatUint.
func fbu(u uint64) string {
	return fmt.Sprintf("%b", u) // want `fmt\.Sprintf of a single %b value boxes the argument and walks fmt's formatter state machine; strconv\.FormatInt/FormatUint with base 2 converts it directly`
}

// %o over an int: base-8 via FormatInt.
func fo(i int) string {
	return fmt.Sprintf("%o", i) // want `fmt\.Sprintf of a single %o value boxes the argument and walks fmt's formatter state machine; strconv\.FormatInt/FormatUint with base 8 converts it directly`
}

// %q over a string: strconv.Quote.
func fq(s string) string {
	return fmt.Sprintf("%q", s) // want `fmt\.Sprintf\("%q", s\) on a string boxes the argument and walks fmt's formatter state machine; strconv\.Quote converts it directly`
}

// %q over a rune: strconv.QuoteRune.
func fqr(r rune) string {
	return fmt.Sprintf("%q", r) // want `fmt\.Sprintf\("%q", r\) on a rune boxes the argument and walks fmt's formatter state machine; strconv\.QuoteRune converts it directly`
}

type Mode int

// Named integer type could implement fmt.Formatter: advisory, no fix.
func fbn(m Mode) string {
	return fmt.Sprintf("%b", m) // want `fmt\.Sprintf of a single %b value boxes the argument and walks fmt's formatter state machine; strconv\.FormatInt/FormatUint with base 2 converts it directly`
}

// %q over a []byte is out of scope: silent.
func fqb(b []byte) string {
	return fmt.Sprintf("%q", b)
}

// %#o has a flag, not a bare verb: silent.
func foFlag(i int) string {
	return fmt.Sprintf("%#o", i)
}

// %c over a rune: string(r).
func fc(r rune) string {
	return fmt.Sprintf("%c", r) // want `fmt\.Sprintf\("%c", r\) boxes the argument and walks fmt's formatter state machine; string\(r\) converts the rune directly`
}

type Glyph rune

// Named rune type could implement fmt.Formatter: advisory, no fix.
func fcn(g Glyph) string {
	return fmt.Sprintf("%c", g) // want `fmt\.Sprintf\("%c", r\) boxes the argument and walks fmt's formatter state machine; string\(r\) converts the rune directly`
}

// %c over a plain int is out of scope (a value beyond the rune range would be
// truncated by rune(), diverging from %c's U+FFFD): silent.
func fcInt(i int) string {
	return fmt.Sprintf("%c", i)
}

// %q over a string(...) conversion expression (not a bare ident): the argument
// is spliced verbatim. Observed on the Go stdlib (text/scanner: %q over
// string(tok)).
func fqConv(tok []byte) string {
	return fmt.Sprintf("%q", string(tok)) // want `fmt\.Sprintf\("%q", s\) on a string boxes the argument and walks fmt's formatter state machine; strconv\.Quote converts it directly`
}

// FORMATTING-DIRECTIVE NEGATIVES: a width, flag, or precision means the verb is
// no longer a BARE verb — a strconv rewrite would silently drop the
// padding/sign/alt-form and change the output, so these must stay silent. (%#o
// and a precision-%g are covered above; these add the common width, explicit
// sign, and alternate-hex forms a user is most likely to hit.)
func fWidthPad(n int) string { return fmt.Sprintf("%5d", n) } // minimum-width padding
func fPlusSign(n int) string { return fmt.Sprintf("%+d", n) } // explicit sign flag
func fAltHex(n int) string   { return fmt.Sprintf("%#x", n) } // alternate form (0x prefix)

// COMMENT-SAFETY NEGATIVE: a comment INSIDE the call's replaced range blocks the
// fix — a strconv rewrite would splice out the /* verb */ comment, silently
// deleting it, so PS2107 still REPORTS but attaches no SuggestedFix and the
// Sprintf is left byte-for-byte intact (comment preserved). Pins the
// ps2107CommentsOverlap guard.
func fCommentInCall(n int) string {
	return fmt.Sprintf("%d" /* verb */, n) // want `fmt\.Sprintf of a single %d value boxes the argument`
}

// %x over a plain int: base-16 via FormatInt (%x on a negative prints the
// sign then the hex magnitude, exactly as FormatInt does).
func fx(i int) string {
	return fmt.Sprintf("%x", i) // want `fmt\.Sprintf of a single %x integer value boxes the argument and walks fmt's formatter state machine; strconv\.FormatInt/FormatUint with base 16 \(hexadecimal\) converts it directly`
}

// %x over an unsigned type: base-16 via FormatUint.
func fxu(u uint) string {
	return fmt.Sprintf("%x", u) // want `fmt\.Sprintf of a single %x integer value boxes the argument and walks fmt's formatter state machine; strconv\.FormatInt/FormatUint with base 16 \(hexadecimal\) converts it directly`
}

type Hex int

// Named integer type could implement fmt.Formatter, which fmt honors for %x
// and strconv would not: advisory, no fix.
func fxn(h Hex) string {
	return fmt.Sprintf("%x", h) // want `fmt\.Sprintf of a single %x integer value boxes the argument and walks fmt's formatter state machine; strconv\.FormatInt/FormatUint with base 16 \(hexadecimal\) converts it directly`
}

// %X prints UPPERCASE hex digits; strconv.FormatInt is lowercase-only, so the
// uppercase verb must never be rewritten: silent.
func fxUpper(i int) string {
	return fmt.Sprintf("%X", i)
}
