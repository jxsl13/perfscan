package checks

import (
	"math"
	"math/rand"
	"slices"
	"testing"
)

// TestEquivPS5063 proves the rewrite is byte-identical for the matched element
// types (integers, strings): slices.Compare(a, b) == 0 equals slices.Equal(a, b)
// for every pair. It also pins WHY float elements are excluded: two NaNs are
// ordered equal by cmp.Compare but unequal by ==.
func TestEquivPS5063(t *testing.T) {
	ints := [][]int{nil, {}, {0}, {1}, {1, 2}, {1, 2, 3}, {1, 3}, {2}}
	for _, a := range ints {
		for _, b := range ints {
			if (slices.Compare(a, b) == 0) != slices.Equal(a, b) {
				t.Fatalf("int a=%v b=%v", a, b)
			}
		}
	}
	strs := [][]string{nil, {}, {"a"}, {"a", "b"}, {"a", "c"}, {"b"}}
	for _, a := range strs {
		for _, b := range strs {
			if (slices.Compare(a, b) == 0) != slices.Equal(a, b) {
				t.Fatalf("str a=%v b=%v", a, b)
			}
		}
	}
	// randomized small-alphabet int slices (frequent equal pairs)
	rnd := func(seed int) []int {
		r := rand.New(rand.NewSource(int64(seed)))
		x := make([]int, r.Intn(6))
		for i := range x {
			x[i] = r.Intn(3)
		}
		return x
	}
	for i := 0; i < 40000; i++ {
		a, b := rnd(i), rnd(i*2654435761+1)
		if (slices.Compare(a, b) == 0) != slices.Equal(a, b) {
			t.Fatalf("rand a=%v b=%v", a, b)
		}
	}
	// Excluded float case: NaN diverges.
	nan := []float64{math.NaN()}
	if (slices.Compare(nan, nan) == 0) == slices.Equal(nan, nan) {
		t.Fatal("expected NaN divergence between Compare==0 and Equal")
	}
}
