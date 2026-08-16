package checks

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

// TestEquivPS5061 proves the rewrite is byte-identical: fmt.Appendf of one %c
// verb spliced into literal text produces exactly what the nested
// append/utf8.AppendRune chain writes, over every rune including the surrogate
// range and out-of-range values (fmt and utf8.AppendRune both emit U+FFFD).
func TestEquivPS5061(t *testing.T) {
	seed := []byte("seed>")
	chk := func(r rune) {
		want := fmt.Appendf(append([]byte(nil), seed...), "[%c]", r)
		got := append(utf8.AppendRune(append(append([]byte(nil), seed...), "["...), r), "]"...)
		if string(want) != string(got) {
			t.Fatalf("r=%d: appendf=%q chain=%q", r, want, got)
		}
	}
	for _, r := range []rune{0, 'A', '€', 0x10FFFF, 0x110000, -1, 0xD800, 0xDFFF, utf8.RuneError} {
		chk(r)
	}
	for r := rune(0); r < 0x11100; r++ {
		chk(r)
	}
}
