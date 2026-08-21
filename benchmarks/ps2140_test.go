package benchmarks

import (
	"math"
	"testing"
)

// PS2140 — an operation that allocates and returns its output buffer vs an
// Into variant that writes a caller-owned destination. Both compute the same
// elementwise SiLU; the allocating form pays a fresh len(x)-sized []float64
// allocation on every call (immediate garbage for a caller running it in a
// loop), while the Into form reuses one preallocated destination. The win is
// the entire output payload — here 4096*8 = 32 KB/op — plus the GC pressure it
// creates in inference loops.

var ps2140In = func() []float64 {
	x := make([]float64, 4096)
	for i := range x {
		x[i] = float64(i%17) - 8
	}
	return x
}()

var ps2140Dst = make([]float64, 4096)

func ps2140SiLU(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[i] = x[i] / (1 + math.Exp(-x[i]))
	}
	return out
}

func ps2140SiLUInto(dst, x []float64) {
	for i := range dst {
		dst[i] = x[i] / (1 + math.Exp(-x[i]))
	}
}

func BenchmarkPS2140_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkSF = ps2140SiLU(ps2140In)
	}
}

func BenchmarkPS2140_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2140SiLUInto(ps2140Dst, ps2140In)
	}
	sinkSF = ps2140Dst
}

var sinkSF []float64
