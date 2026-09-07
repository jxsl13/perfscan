package benchmarks

import "testing"

// PS2144 — three same-length scratch slices as three backing allocations vs
// three full-capacity, non-overlapping lanes of one guarded backing slice. The
// pair deliberately keeps total payload bytes and element work identical; it
// isolates object count. Timing remains allocator/GC/workload dependent.

var ps2144Sink float64

//go:noinline
func ps2144Before(n int) float64 {
	a := make([]float64, n)
	b := make([]float64, n)
	c := make([]float64, n)
	for i := range a {
		a[i] = float64(i)
		b[i] = a[i] + 1
		c[i] = b[i] * 2
	}
	return a[n-1] + b[n-1] + c[n-1]
}

//go:noinline
func ps2144After(n int) float64 {
	const lanes = 3
	if n < 0 || n > int(^uint(0)>>1)/lanes {
		panic("scratch size out of range")
	}
	scratch := make([]float64, lanes*n)
	a := scratch[:n:n]
	b := scratch[1*n : 2*n : 2*n]
	c := scratch[2*n : 3*n : 3*n]
	for i := range a {
		a[i] = float64(i)
		b[i] = a[i] + 1
		c[i] = b[i] * 2
	}
	return a[n-1] + b[n-1] + c[n-1]
}

func BenchmarkPS2144_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2144Sink = ps2144Before(4096)
	}
}

func BenchmarkPS2144_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2144Sink = ps2144After(4096)
	}
}
