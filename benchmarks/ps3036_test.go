package benchmarks

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// PS3036 — slices.CompareFunc with a bare strings.Compare comparator vs
// slices.Compare. Both compare two 4096-element []string slices that agree on
// every element but the last — a full-length lexicographic scan — but
// CompareFunc pays an indirect comparator call (plus the strings.Compare hop
// inside the literal) on every element pair, which the monomorphized
// slices.Compare drops; the byte comparison itself is a runtime cmpstring
// call on both sides. 8-byte keys share a 4-byte prefix so every pair does
// real byte work. 0 allocs either way.
var ps3036A, ps3036B = func() ([]string, []string) {
	a := make([]string, 4096)
	for i := range a {
		a[i] = fmt.Sprintf("key %04d", i)
	}
	b := slices.Clone(a)
	b[len(b)-1] += "!" // equal prefix, divergence at the very last pair
	return a, b
}()

var ps3036Sink int

func BenchmarkPS3036_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3036Sink = slices.CompareFunc(ps3036A, ps3036B, func(x, y string) int { return strings.Compare(x, y) })
	}
}

func BenchmarkPS3036_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3036Sink = slices.Compare(ps3036A, ps3036B)
	}
}
