package benchmarks

import (
	"slices"
	"testing"
)

// PS3019 — slices.IsSortedFunc with a hand-rolled three-way if-chain
// comparator vs the monomorphized slices.IsSorted. Both scan a sorted
// 4096-element []int once (sorted input forces the full O(n) pass — no
// early return); IsSortedFunc pays an indirect comparator call plus up to
// two relational comparisons inside the literal on every adjacent pair,
// which slices.IsSorted inlines into a single direct '<'. The bool is
// identical (the chain's sign is negative iff a < b, exactly the cmp.Less
// predicate IsSorted branches on); 0 allocs either way, so the delta is
// pure comparison overhead — the same character as PS3010's cmp.Compare
// spelling of the identical anti-pattern.
var ps3019Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = i / 2 // ascending with tie plateaus
	}
	return a
}()

var ps3019Sink bool

func BenchmarkPS3019_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3019Sink = slices.IsSortedFunc(ps3019Data, func(x, y int) int {
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
			return 0
		})
	}
}

func BenchmarkPS3019_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3019Sink = slices.IsSorted(ps3019Data)
	}
}
