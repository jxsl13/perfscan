package checks

import (
	"math/rand"
	"testing"
	"unicode/utf8"
)

// TestEquivPS5067 proves the rewrite is byte-identical: utf8.FullRune and
// utf8.FullRuneInString return the same bool for the same bytes, over valid,
// invalid, truncated, and randomized inputs.
func TestEquivPS5067(t *testing.T) {
	corpus := []string{
		"", "a", "é", "€", "😀", "\xff", "a\xff",
		"\xe2", "\xe2\x82", "\xe2\x82\xac", // truncated then full €
		"\xf0\x9f\x98", "\xf0\x9f\x98\x80", // truncated then full 😀
		"\x80", "\xc0", "\xed\xa0\x80", // continuation, invalid lead, surrogate
	}
	for _, s := range corpus {
		b := []byte(s)
		if utf8.FullRune(b) != utf8.FullRuneInString(s) {
			t.Fatalf("corpus %q: FullRune=%v FullRuneInString=%v", s, utf8.FullRune(b), utf8.FullRuneInString(s))
		}
	}
	for i := 0; i < 50000; i++ {
		r := rand.New(rand.NewSource(int64(i)))
		b := make([]byte, r.Intn(6))
		for j := range b {
			b[j] = byte(r.Intn(256))
		}
		if utf8.FullRune(b) != utf8.FullRuneInString(string(b)) {
			t.Fatalf("rand %x", b)
		}
	}
}
