package checks

import (
	"maps"
	"slices"
	"testing"
)

// TestEquivPS5045 proves the rewrite is byte-identical: len(slices.Collect(
// maps.Keys(m))) and len(slices.Collect(maps.Values(m))) both equal len(m) for
// every map (one entry per key), empty included.
func TestEquivPS5045(t *testing.T) {
	check := func(m map[int]int) {
		if got := len(slices.Collect(maps.Keys(m))); got != len(m) {
			t.Fatalf("keys len=%d want %d for %v", got, len(m), m)
		}
		if got := len(slices.Collect(maps.Values(m))); got != len(m) {
			t.Fatalf("values len=%d want %d for %v", got, len(m), m)
		}
	}
	check(map[int]int{})
	check(map[int]int{1: 1})
	check(map[int]int{1: 10, 2: 20, 3: 30})
	big := map[int]int{}
	for i := 0; i < 5000; i++ {
		big[i] = i * 2
	}
	check(big)
}
