package checks

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
)

// TestEquivPS5054 proves the rewrite is byte-identical: hex encoding is
// injective, so hex.EncodeToString(a) == hex.EncodeToString(b) equals
// bytes.Equal(a, b) (and != the negation) for every pair of slices, including
// nil, empty, differing lengths, NUL bytes, and invalid UTF-8.
func TestEquivPS5054(t *testing.T) {
	corpus := [][]byte{
		nil, {}, {0}, {0, 0}, {1}, []byte("abc"), []byte("abd"),
		{0xff, 0x00, 0x7f}, bytes.Repeat([]byte{0xab}, 33),
	}
	chk := func(a, b []byte) {
		if (hex.EncodeToString(a) == hex.EncodeToString(b)) != bytes.Equal(a, b) {
			t.Fatalf("== a=%x b=%x", a, b)
		}
		if (hex.EncodeToString(a) != hex.EncodeToString(b)) != !bytes.Equal(a, b) {
			t.Fatalf("!= a=%x b=%x", a, b)
		}
	}
	for _, a := range corpus {
		for _, b := range corpus {
			chk(a, b)
		}
	}
	rnd := func(seed int) []byte {
		r := rand.New(rand.NewSource(int64(seed)))
		bs := make([]byte, r.Intn(10))
		r.Read(bs)
		return bs
	}
	for i := 0; i < 30000; i++ {
		chk(rnd(i), rnd(i*2654435761+1))
	}
}
