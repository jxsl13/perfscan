package checks

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
)

// TestEquivPS5065 proves the rewrite is byte-identical: hex encoding is
// order-preserving, so hex.EncodeToString(a) OP hex.EncodeToString(b) equals
// bytes.Compare(a, b) OP 0 for OP in <, <=, >, >=, over adversarial and
// randomized slices (prefixes, first-difference anywhere, nil/empty).
func TestEquivPS5065(t *testing.T) {
	corpus := [][]byte{
		nil, {}, {0}, {0, 0}, {1}, {0xff}, {0x0a}, {0x10},
		[]byte("abc"), []byte("abd"), []byte("ab"),
		{0x00, 0xff}, bytes.Repeat([]byte{0xab}, 5),
	}
	chk := func(a, b []byte) {
		ha, hb := hex.EncodeToString(a), hex.EncodeToString(b)
		c := bytes.Compare(a, b)
		if (ha < hb) != (c < 0) {
			t.Fatalf("< a=%x b=%x", a, b)
		}
		if (ha <= hb) != (c <= 0) {
			t.Fatalf("<= a=%x b=%x", a, b)
		}
		if (ha > hb) != (c > 0) {
			t.Fatalf("> a=%x b=%x", a, b)
		}
		if (ha >= hb) != (c >= 0) {
			t.Fatalf(">= a=%x b=%x", a, b)
		}
	}
	for _, a := range corpus {
		for _, b := range corpus {
			chk(a, b)
		}
	}
	rnd := func(seed int) []byte {
		r := rand.New(rand.NewSource(int64(seed)))
		x := make([]byte, r.Intn(8))
		for i := range x {
			x[i] = byte(r.Intn(4)) // small alphabet -> frequent prefixes/ties
		}
		return x
	}
	for i := 0; i < 40000; i++ {
		chk(rnd(i), rnd(i*2654435761+1))
	}
}
