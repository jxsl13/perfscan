package checks

import (
	"encoding/base32"
	"encoding/base64"
	"math/rand"
	"testing"
)

// TestEquivPS5057 proves the rewrite is byte-identical INCLUDING nil-ness across
// the base64 (Std/URL/Raw) and base32 (Std/Hex) encoders: the []byte{}
// destination makes the empty case non-nil to match []byte(""), and the encoder
// receiver is the same on both sides so the alphabet and padding agree.
func TestEquivPS5057(t *testing.T) {
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
	b64 := []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding}
	b32 := []*base32.Encoding{base32.StdEncoding, base32.HexEncoding}

	chk := func(b []byte) {
		for _, e := range b64 {
			if !same([]byte(e.EncodeToString(b)), e.AppendEncode([]byte{}, b)) {
				t.Fatalf("base64 diverge b=%x", b)
			}
		}
		for _, e := range b32 {
			if !same([]byte(e.EncodeToString(b)), e.AppendEncode([]byte{}, b)) {
				t.Fatalf("base32 diverge b=%x", b)
			}
		}
	}
	for _, b := range [][]byte{nil, {}, {0}, {0xff}, {1, 2, 3, 4, 5}} {
		chk(b)
	}
	for i := 0; i < 30000; i++ {
		n := rand.Intn(24)
		x := make([]byte, n)
		for j := range x {
			x[j] = byte(rand.Intn(256))
		}
		chk(x)
	}
}
