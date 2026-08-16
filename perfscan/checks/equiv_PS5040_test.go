package checks

import (
	"fmt"
	"math"
	"testing"
	"unicode/utf8"
)

// TestEquivPS5040 proves the rewrite is byte-identical: for every operand
// value, fmt.Appendf(dst, "%c", r) and utf8.AppendRune(dst, r) append the
// same bytes. "%c" formats the integer as a Unicode code point (U+FFFD for
// negative, > MaxRune, or surrogate values); utf8.AppendRune applies exactly
// that mapping. The gate admits only int32-lossless operand kinds, so rune(r)
// never truncates.
func TestEquivPS5040(t *testing.T) {
	check := func(r rune) {
		a := fmt.Appendf([]byte("seed"), "%c", r)
		b := utf8.AppendRune([]byte("seed"), r)
		if string(a) != string(b) {
			t.Fatalf("r=%d (0x%x): fmt=%q utf8=%q", r, r, a, b)
		}
	}
	// Boundaries and special code points.
	for _, r := range []rune{
		0, 1, 'A', 0x7F, 0x80, 0x7FF, 0x800, 0xFFFD, 0xFFFF, 0x10000,
		utf8.MaxRune, utf8.MaxRune + 1, 0xD7FF, 0xD800, 0xDBFF, 0xDC00, 0xDFFF, 0xE000,
		-1, -128, math.MinInt32, math.MaxInt32,
	} {
		check(r)
	}
	// Exhaustive over the whole BMP + the surrogate and astral edges.
	for r := rune(0); r <= 0x11000; r++ {
		check(r)
	}
	// Strided sweep across the full int32 range (invalid runes -> U+FFFD both).
	for v := int64(math.MinInt32); v <= math.MaxInt32; v += 65413 {
		check(rune(v))
	}
	// The narrower widths the fix wraps as rune(x): every value is in range.
	for i := 0; i < 256; i++ {
		check(rune(byte(i)))
		check(rune(int8(i)))
	}
	for i := int32(math.MinInt16); i <= math.MaxInt16; i += 7 {
		check(rune(int16(i)))
		check(rune(uint16(i & 0xFFFF)))
	}
}
