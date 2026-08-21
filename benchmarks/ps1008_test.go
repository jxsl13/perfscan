package benchmarks

import (
	"slices"
	"testing"
)

// PS1008 — slices.EqualFunc with a bare x == y equality closure vs
// slices.Equal. Two EQUAL 4096-element []int slices are scanned per op —
// the worst case: no early exit, so every element pair goes through the
// equality. Both sides length-check first and short-circuit on the first
// mismatch, comparing with the identical ==, so the result bool is the
// same on every input; both do 0 allocs. Measured PARITY on gc 1.26/arm64
// (~1.4 µs/op both): gc inlines slices.EqualFunc (cost 50) and
// devirtualizes the literal closure, so both sides compile to the same
// direct-comparison loop — the empirical demonstration (same policy as
// PS2125/PS3102) that the rewrite's win on current gc is source-level
// robustness, while toolchains without closure devirtualization drop a
// real per-pair indirect call.
var ps1008A, ps1008B = func() ([]int, []int) {
	a := make([]int, 4096)
	for i := range a {
		a[i] = i * 7
	}
	return a, slices.Clone(a)
}()

var ps1008Sink bool

func BenchmarkPS1008_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps1008Sink = slices.EqualFunc(ps1008A, ps1008B, func(x, y int) bool { return x == y })
	}
}

func BenchmarkPS1008_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps1008Sink = slices.Equal(ps1008A, ps1008B)
	}
}
