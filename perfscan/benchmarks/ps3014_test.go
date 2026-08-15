package benchmarks

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// PS3014 — slices.IsSortedFunc with a bare strings.Compare comparator vs
// slices.IsSorted. Both scan a sorted 4096-element []string once (sorted
// input forces the full O(n) pass — no early return) and return the
// identical bool; IsSortedFunc pays an indirect comparator call (plus the
// strings.Compare hop inside the literal) on every adjacent pair, which the
// monomorphized slices.IsSorted inlines — the byte comparison itself is a
// runtime string-compare call on both sides. 8-byte keys share a 4-byte
// prefix so every pair does real byte work. 0 allocs either way.
var ps3014Data = func() []string {
	a := make([]string, 4096)
	for i := range a {
		a[i] = fmt.Sprintf("key %04d", i) // sorted, distinct, shared prefix
	}
	return a
}()

var ps3014Sink bool

func BenchmarkPS3014_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3014Sink = slices.IsSortedFunc(ps3014Data, func(a, b string) int { return strings.Compare(a, b) })
	}
}

func BenchmarkPS3014_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3014Sink = slices.IsSorted(ps3014Data)
	}
}
