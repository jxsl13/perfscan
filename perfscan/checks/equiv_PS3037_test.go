package checks

import (
	"math"
	"slices"
	"sort"
	"testing"
)

// TestEquivPS3037 proves the rewrite is byte-identical for ints and strings:
// sort.SearchInts / SearchStrings return the same insertion index as
// slices.BinarySearch across exhaustive targets. It also pins WHY
// sort.SearchFloat64s is excluded: it disagrees with slices.BinarySearch on NaN.
func TestEquivPS3037(t *testing.T) {
	ints := []int{-5, -1, 0, 2, 2, 7, 7, 7, 100}
	for x := -10; x <= 110; x++ {
		i := sort.SearchInts(ints, x)
		j, _ := slices.BinarySearch(ints, x)
		if i != j {
			t.Fatalf("SearchInts x=%d: sort=%d slices=%d", x, i, j)
		}
	}
	strs := []string{"", "a", "aa", "ab", "b", "banana", "cherry"}
	for _, x := range []string{"", "a", "aa", "ab", "b", "ba", "banana", "c", "cherry", "z"} {
		i := sort.SearchStrings(strs, x)
		j, _ := slices.BinarySearch(strs, x)
		if i != j {
			t.Fatalf("SearchStrings x=%q: sort=%d slices=%d", x, i, j)
		}
	}

	// Excluded case: floats with a NaN target diverge, which is why
	// SearchFloat64s is never rewritten.
	fs := []float64{1, 2, 3}
	sortIdx := sort.SearchFloat64s(fs, math.NaN())
	slicesIdx, _ := slices.BinarySearch(fs, math.NaN())
	if sortIdx == slicesIdx {
		t.Fatalf("expected NaN divergence, both returned %d", sortIdx)
	}
}
