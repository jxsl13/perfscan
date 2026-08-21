package ps5023

import (
	"bytes"
	"strings"
)

// --- the basic shapes: a constant ASCII rune literal through IndexRune in
// both packages. The fix renames the callee and leaves the rune literal
// token completely untouched — an untyped rune constant in [0, 0x80) is
// representable as byte and converts implicitly.

func basics(s string, b []byte) int {
	i := strings.IndexRune(s, 'z') // want `strings\.IndexRune of the constant ASCII rune 'z' pays a non-inlined range-check wrapper before delegating to the byte scan; strings\.IndexByte\(s, 'z'\) jumps straight to the same scan — identical index for every input`
	j := bytes.IndexRune(b, '=') // want `bytes\.IndexRune of the constant ASCII rune '=' pays a non-inlined range-check wrapper before delegating to the byte scan; bytes\.IndexByte\(b, '='\) jumps straight to the same scan — identical index for every input`
	return i + j
}

// Every ASCII spelling carries over verbatim — escape sequences, NUL, the
// 0x7f upper bound, backslash and quote escapes, \u and octal escapes of
// ASCII code points: only the value bound [0, 0x80) matters, and the
// literal token itself is never rewritten.
func escapes(s string, b []byte) {
	_ = strings.IndexRune(s, '\t') // want `strings\.IndexByte\(s, '\\t'\) jumps straight to the same scan`
	_ = strings.IndexRune(s, '\x00') // want `strings\.IndexByte\(s, '\\x00'\) jumps straight to the same scan`
	_ = strings.IndexRune(s, '\x7f') // want `strings\.IndexByte\(s, '\\x7f'\) jumps straight to the same scan`
	_ = strings.IndexRune(s, '\\') // want `strings\.IndexByte\(s, '\\\\'\) jumps straight to the same scan`
	_ = strings.IndexRune(s, '\'') // want `strings\.IndexByte\(s, '\\''\) jumps straight to the same scan`
	_ = strings.IndexRune(s, '\u0041') // want `strings\.IndexByte\(s, '\\u0041'\) jumps straight to the same scan`
	_ = strings.IndexRune(s, '\101') // want `strings\.IndexByte\(s, '\\101'\) jumps straight to the same scan`
	_ = bytes.IndexRune(b, '\n') // want `bytes\.IndexByte\(b, '\\n'\) jumps straight to the same scan`
}

// A plain integer literal is an untyped constant too: 47 converts to byte
// in IndexByte's argument position exactly as it converts to rune in
// IndexRune's, so the rename alone is still a complete fix.
func integerSpellings(s string, b []byte) {
	_ = strings.IndexRune(s, 47) // want `strings\.IndexByte\(s, 47\) jumps straight to the same scan`
	_ = strings.IndexRune(s, 0x2f) // want `strings\.IndexByte\(s, 0x2f\) jumps straight to the same scan`
	_ = bytes.IndexRune(b, 0) // want `bytes\.IndexByte\(b, 0\) jumps straight to the same scan`
}

// The replacement stays a call returning the same int, so it drops into
// every syntactic position — parenthesized arguments and calls,
// comparisons, index expressions, even go/defer statements — with no
// parenthesization, statement, or splicing concerns.
func contexts(s string, b []byte, m map[int]int) {
	_ = strings.IndexRune(s, ('!')) // want `strings\.IndexByte\(s, '!'\) jumps straight to the same scan`
	_ = (bytes.IndexRune(b, ';')) // want `bytes\.IndexByte\(b, ';'\) jumps straight to the same scan`
	if strings.IndexRune(s, ',') >= 0 { // want `strings\.IndexByte\(s, ','\) jumps straight to the same scan`
		_ = m[bytes.IndexRune(b, ':')] // want `bytes\.IndexByte\(b, ':'\) jumps straight to the same scan`
	}
	go strings.IndexRune(s, 'g') // want `strings\.IndexByte\(s, 'g'\) jumps straight to the same scan`
	defer bytes.IndexRune(b, 'd') // want `bytes\.IndexByte\(b, 'd'\) jumps straight to the same scan`
}

// --- advisory: reported but never rewritten ---

// A named constant keeps its symbolic name — and a TYPED rune constant
// would additionally need an inserted byte(...) conversion to compile —
// so these sites are advisory-only: write IndexByte(s, byte(c)) by hand
// (the conversion is unnecessary when c is untyped).
const slash = '/'

const tab rune = '\t'

func named(s string, b []byte) {
	_ = strings.IndexRune(s, slash) // want `the rune is a constant expression, not a literal — rewrite to strings\.IndexByte by hand`
	_ = bytes.IndexRune(b, tab) // want `the rune is a constant expression, not a literal — rewrite to bytes\.IndexByte by hand`
	_ = strings.IndexRune(s, 'a'+1) // want `the rune is a constant expression, not a literal — rewrite to strings\.IndexByte by hand`
	_ = strings.IndexRune(s, rune('z')) // want `the rune is a constant expression, not a literal — rewrite to strings\.IndexByte by hand`
}
