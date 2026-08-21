package benchmarks

import (
	"bytes"
	"encoding/hex"
	"testing"
)

var ps5064X = []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}
var ps5064R bool

// BenchmarkPS5064Before is hex.EncodeToString(x) == "<const>": x is encoded to
// a hex string just to compare it against the constant.
func BenchmarkPS5064Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5064R = hex.EncodeToString(ps5064X) == "deadbeef000102030405060708090a0b0c"
	}
}

// BenchmarkPS5064After is bytes.Equal(x, <decoded>): the raw bytes are compared
// directly, no encode pass.
func BenchmarkPS5064After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5064R = bytes.Equal(ps5064X, []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c})
	}
}
