package checks

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"math/rand"
	"testing"
)

// TestEquivPS5058 proves the rewrite is byte-identical: for a FIXED base64/base32
// encoder, EncodeToString is injective, so enc.EncodeToString(a) ==
// enc.EncodeToString(b) equals bytes.Equal(a, b) (and != the negation). It also
// pins WHY the same encoder is required: Std and URL base64 disagree on some
// bytes, so a cross-encoder compare is not bytes.Equal.
func TestEquivPS5058(t *testing.T) {
	b64 := []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding}
	b32 := []*base32.Encoding{base32.StdEncoding, base32.HexEncoding}
	rnd := func(seed int) []byte {
		r := rand.New(rand.NewSource(int64(seed)))
		x := make([]byte, r.Intn(12))
		for i := range x {
			x[i] = byte(r.Intn(6)) // small alphabet -> frequent equal pairs
		}
		return x
	}
	for i := 0; i < 60000; i++ {
		a, b := rnd(i), rnd(i*2654435761+1)
		for _, e := range b64 {
			if (e.EncodeToString(a) == e.EncodeToString(b)) != bytes.Equal(a, b) {
				t.Fatalf("base64 a=%x b=%x", a, b)
			}
		}
		for _, e := range b32 {
			if (e.EncodeToString(a) == e.EncodeToString(b)) != bytes.Equal(a, b) {
				t.Fatalf("base32 a=%x b=%x", a, b)
			}
		}
	}
	// Cross-encoder trap the check excludes: Std vs URL differ on 0xFB/0xFF.
	x := []byte{0xfb, 0xff}
	if base64.StdEncoding.EncodeToString(x) == base64.URLEncoding.EncodeToString(x) {
		t.Fatal("expected Std and URL base64 to differ")
	}
}
