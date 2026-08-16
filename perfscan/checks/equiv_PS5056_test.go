package checks

import (
	"encoding/hex"
	"math/rand"
	"testing"
)

// TestEquivPS5056 proves the rewrite is byte-identical INCLUDING nil-ness:
// []byte(hex.EncodeToString(b)) and hex.AppendEncode([]byte{}, b) produce the
// same bytes for every input, and the []byte{} destination makes the empty case
// non-nil to match []byte("") (a nil destination would return nil).
func TestEquivPS5056(t *testing.T) {
	same := func(a, b []byte) bool {
		if (a == nil) != (b == nil) || len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	chk := func(b []byte) {
		conv := []byte(hex.EncodeToString(b))
		app := hex.AppendEncode([]byte{}, b)
		if !same(conv, app) {
			t.Fatalf("diverge b=%x: conv=%q(nil=%v) app=%q(nil=%v)", b, conv, conv == nil, app, app == nil)
		}
	}
	for _, b := range [][]byte{nil, {}, {0}, {0xff}, {1, 2, 3}} {
		chk(b)
	}
	for i := 0; i < 50000; i++ {
		n := rand.Intn(24)
		x := make([]byte, n)
		for j := range x {
			x[j] = byte(rand.Intn(256))
		}
		chk(x)
	}
}
