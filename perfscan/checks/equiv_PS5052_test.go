package checks

import (
	"maps"
	"slices"
	"testing"
)

// TestEquivPS5052 proves the rewrite is byte-identical: sorting permutes but
// never changes length, and a map has one entry per key, so
// len(slices.Sorted(maps.Keys/Values(m))) == len(m) for every map size,
// including empty.
func TestEquivPS5052(t *testing.T) {
	for _, size := range []int{0, 1, 2, 5, 17, 128, 1000} {
		m := make(map[int]int, size)
		for i := 0; i < size; i++ {
			m[i*7-3] = i
		}
		if got := len(slices.Sorted(maps.Keys(m))); got != len(m) {
			t.Fatalf("keys size %d: len(Sorted)=%d len(m)=%d", size, got, len(m))
		}
		if got := len(slices.Sorted(maps.Values(m))); got != len(m) {
			t.Fatalf("values size %d: len(Sorted)=%d len(m)=%d", size, got, len(m))
		}
	}
}
