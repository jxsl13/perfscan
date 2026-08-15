package benchmarks

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// PS3011 — slices.BinarySearchFunc with a bare strings.Compare comparator vs
// slices.BinarySearch. Both make the identical log n probes over the same
// sorted []string and return the identical (index, found); BinarySearchFunc
// pays an indirect comparator call (plus the strings.Compare hop inside the
// literal) on each probe, which the monomorphized slices.BinarySearch drops —
// the byte comparison itself is a runtime cmpstring call on both sides.
// 8-byte keys share a 4-byte prefix so every probe does real byte work;
// targets cycle across the whole range so every search runs a full log n
// probes. 0 allocs either way.
var ps3011Data = func() []string {
	a := make([]string, 4096)
	for i := range a {
		a[i] = fmt.Sprintf("key %04d", i) // sorted, distinct, shared prefix
	}
	return a
}()

var (
	ps3011Idx   int
	ps3011Found bool
)

func BenchmarkPS3011_Before(b *testing.B) {
	b.ReportAllocs()
	for n := range b.N {
		ps3011Idx, ps3011Found = slices.BinarySearchFunc(ps3011Data, ps3011Data[n%4096], func(a, b string) int { return strings.Compare(a, b) })
	}
}

func BenchmarkPS3011_After(b *testing.B) {
	b.ReportAllocs()
	for n := range b.N {
		ps3011Idx, ps3011Found = slices.BinarySearch(ps3011Data, ps3011Data[n%4096])
	}
}
