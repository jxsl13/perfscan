package checks

import (
	"math"
	"testing"
	"unicode/utf8"
)

// TestEquivPS2050 proves the rewrite is byte-identical: string(utf8.AppendRune(
// nil, r)) and string(r) are the same bytes for every rune, including invalid
// ones (both emit the same 1-4 byte UTF-8 or the RuneError encoding U+FFFD).
func TestEquivPS2050(t *testing.T) {
	check := func(r rune) {
		a := string(utf8.AppendRune(nil, r))
		b := string(r)
		if a != b {
			t.Fatalf("r=%d (0x%x): appendrune=%q string=%q", r, r, a, b)
		}
	}
	for _, r := range []rune{
		0, 1, 'A', 0x7F, 0x80, 0x7FF, 0x800, 0xFFFD, 0xFFFF, 0x10000,
		utf8.MaxRune, utf8.MaxRune + 1, 0xD7FF, 0xD800, 0xDBFF, 0xDC00, 0xDFFF, 0xE000,
		-1, -128, math.MinInt32, math.MaxInt32,
	} {
		check(r)
	}
	for r := rune(0); r <= 0x11000; r++ {
		check(r)
	}
	for v := int64(math.MinInt32); v <= math.MaxInt32; v += 65413 {
		check(rune(v))
	}
}
