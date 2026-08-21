package benchmarks

import (
	"encoding/hex"
	"testing"
)

// PS2043 — len(hex.EncodeToString(b)) vs hex.EncodedLen(len(b)). The Before
// shape allocates a 2*len(b)-byte buffer, encodes every input byte into it and
// converts it to a string, purely to read the result's length; the After shape
// is one multiplication on len(b) — no allocation, no encode pass — and the
// stdlib guarantees the same integer by construction (EncodeToString sizes its
// buffer with exactly EncodedLen(len(src)) and always fills it). The gap
// scales linearly with len(b).
var (
	ps2043In   = make([]byte, 1024)
	ps2043Sink int
)

func BenchmarkPS2043_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2043Sink = len(hex.EncodeToString(ps2043In))
	}
}

func BenchmarkPS2043_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2043Sink = hex.EncodedLen(len(ps2043In))
	}
}
