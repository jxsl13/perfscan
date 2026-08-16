package checks

import (
	"bytes"
	"math/rand"
	"slices"
	"testing"
)

// TestEquivPS5062 proves the rewrite iterates the identical runes: the []rune
// that bytes.Runes(b) yields (and the before-loop ranges) equals the runes a
// string range visits, which []rune(string(b)) materializes — for every input,
// including invalid UTF-8 (each bad byte becomes U+FFFD consuming one byte in
// both).
func TestEquivPS5062(t *testing.T) {
	chk := func(b []byte) {
		viaRunes := bytes.Runes(b)     // what "for range bytes.Runes(b)" iterates
		viaString := []rune(string(b)) // the runes "for range string(b)" visits
		if !slices.Equal(viaRunes, viaString) {
			t.Fatalf("b=%q: runes=%v string=%v", b, viaRunes, viaString)
		}
	}
	for _, b := range [][]byte{nil, {}, []byte("hello"), []byte("héllo€"), {0xff, 0xfe}, []byte("a\xffb"), []byte("😀x")} {
		chk(b)
	}
	for i := 0; i < 20000; i++ {
		n := rand.Intn(24)
		x := make([]byte, n)
		for j := range x {
			x[j] = byte(rand.Intn(256))
		}
		chk(x)
	}
}
