package checks

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
)

// TestEquivPS5064 proves the rewrite is byte-identical: for a valid lowercase
// even-length hex constant C, hex.EncodeToString(x) == C equals
// bytes.Equal(x, decode(C)) for every x. It also pins why uppercase and
// odd/invalid constants are excluded (they can never equal an EncodeToString
// result, so the == is unconditionally false while bytes.Equal of the decode is
// not).
func TestEquivPS5064(t *testing.T) {
	consts := []string{"", "00", "ab", "deadbeef", "4142", "00ff", "cafe"}
	chk := func(x []byte, c string) {
		dec, _ := hex.DecodeString(c)
		if (hex.EncodeToString(x) == c) != bytes.Equal(x, dec) {
			t.Fatalf("x=%x c=%q", x, c)
		}
	}
	for _, c := range consts {
		for _, x := range [][]byte{nil, {}, {0}, {0xab}, {0xde, 0xad, 0xbe, 0xef}, {0x41, 0x42}} {
			chk(x, c)
		}
		for i := 0; i < 5000; i++ {
			n := rand.Intn(6)
			b := make([]byte, n)
			for j := range b {
				b[j] = byte(rand.Intn(256))
			}
			chk(b, c)
		}
	}
	// Excluded uppercase: EncodeToString never uppercase, so == is always false,
	// but the decode of "AB" is {0xab} which bytes.Equal would match.
	if (hex.EncodeToString([]byte{0xab}) == "AB") == func() bool { d, _ := hex.DecodeString("AB"); return bytes.Equal([]byte{0xab}, d) }() {
		t.Fatal("expected uppercase divergence")
	}
}
