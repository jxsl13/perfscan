package checks

import (
	"bytes"
	"math/rand"
	"slices"
	"testing"
)

// TestEquivPS5055 proves the rewrite is byte-identical: slices.Equal/Compare and
// bytes.Equal/Compare use the same total order over bytes, so they agree for
// every pair of byte slices — nil, empty, equal, prefix, differing, and random.
func TestEquivPS5055(t *testing.T) {
	corpus := [][]byte{
		nil, {}, {0}, {0, 0}, {1}, []byte("abc"), []byte("abd"), []byte("ab"),
		{0xff, 0x00}, bytes.Repeat([]byte{0xab}, 40),
	}
	chk := func(a, b []byte) {
		if slices.Equal(a, b) != bytes.Equal(a, b) {
			t.Fatalf("Equal a=%x b=%x", a, b)
		}
		if slices.Compare(a, b) != bytes.Compare(a, b) {
			t.Fatalf("Compare a=%x b=%x: slices=%d bytes=%d", a, b, slices.Compare(a, b), bytes.Compare(a, b))
		}
	}
	for _, a := range corpus {
		for _, b := range corpus {
			chk(a, b)
		}
	}
	rnd := func(seed int) []byte {
		r := rand.New(rand.NewSource(int64(seed)))
		bs := make([]byte, r.Intn(12))
		for i := range bs {
			bs[i] = byte(r.Intn(4)) // small alphabet -> frequent ties/prefixes
		}
		return bs
	}
	for i := 0; i < 40000; i++ {
		chk(rnd(i), rnd(i*2654435761+1))
	}
}
