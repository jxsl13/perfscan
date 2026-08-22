package checks

// PS3008's runtime differential: sort.IntsAreSorted(x) and
// sort.StringsAreSorted(x) must return a bool IDENTICAL to
// slices.IsSorted(x) for EVERY input — pinned against the REAL stdlib so a
// future stdlib change that breaks the claim fails CI. Two claims:
//
//  1. PREDICATE: both spellings answer "no adjacent pair is descending
//     under <". On go1.21 sort.IntsAreSorted routes through
//     sort.IsSorted(sort.IntSlice(x)) whose Less is the identical < that
//     slices.IsSorted's cmp.Less compiles to for int/string; from go1.22
//     on the sort helpers ARE one-line slices.IsSorted wrappers.
//  2. NO REORDERING: the call is a pure read-only scan returning a bool,
//     so every tie/stability hazard of the sort-family REORDERING fixes is
//     structurally absent; the differential also verifies the input slice
//     is left byte-identical by both sides.
//
// The inputs are adversarial for a sortedness scan: exactly-sorted runs
// with a single violation planted at every position (first pair, last
// pair, middle), all-equal and tie-heavy slices (ties are NOT violations
// under a non-strict <= order), len 0/1/2 boundaries, int extremes
// (MinInt/MaxInt wraparound-adjacent pairs), and strings exercising
// lexicographic byte order including the empty string and prefix pairs
// ("a" < "aa").

import (
	"math"
	"math/rand"
	"slices"
	"sort"
	"testing"
)

func TestEquiv_PS3008AreSortedSlicesIsSorted(t *testing.T) {
	type celsius int // named ordered element, like the fixture's intList

	checkInts := func(t *testing.T, base []int) {
		t.Helper()
		a := slices.Clone(base)
		b := slices.Clone(base)
		got := sort.IntsAreSorted(a)
		want := slices.IsSorted(b)
		if got != want {
			t.Fatalf("sort.IntsAreSorted(%v) = %v, slices.IsSorted = %v", base, got, want)
		}
		if !slices.Equal(a, base) || !slices.Equal(b, base) {
			t.Fatalf("a sortedness predicate mutated its input: %v / %v from %v", a, b, base)
		}
	}
	checkStrings := func(t *testing.T, base []string) {
		t.Helper()
		a := slices.Clone(base)
		b := slices.Clone(base)
		got := sort.StringsAreSorted(a)
		want := slices.IsSorted(b)
		if got != want {
			t.Fatalf("sort.StringsAreSorted(%v) = %v, slices.IsSorted = %v", base, got, want)
		}
		if !slices.Equal(a, base) || !slices.Equal(b, base) {
			t.Fatalf("a sortedness predicate mutated its input: %v / %v from %v", a, b, base)
		}
	}

	// Boundaries: len 0/1/2 (both return true for len < 2, nil included).
	checkInts(t, nil)
	checkInts(t, []int{})
	checkInts(t, []int{7})
	checkInts(t, []int{1, 2})
	checkInts(t, []int{2, 1})
	checkInts(t, []int{2, 2})
	checkStrings(t, nil)
	checkStrings(t, []string{})
	checkStrings(t, []string{"x"})
	checkStrings(t, []string{"", ""})

	// Int extremes: MinInt/MaxInt adjacency is where a buggy subtraction-
	// based comparison would wrap; < does not, on either side.
	checkInts(t, []int{math.MinInt, -1, 0, 1, math.MaxInt})
	checkInts(t, []int{math.MaxInt, math.MinInt})
	checkInts(t, []int{math.MinInt, math.MaxInt})
	checkInts(t, []int{math.MaxInt, math.MaxInt, math.MinInt})

	// A single violation planted at EVERY adjacent position of an
	// otherwise sorted tie-heavy slice: both scans must spot it no matter
	// where it hides (first pair, last pair, anywhere between).
	for _, n := range []int{2, 3, 7, 16, 64} {
		base := make([]int, n)
		for i := range base {
			base[i] = i / 2 // sorted with ties: ties are NOT violations
		}
		checkInts(t, base)
		for pos := 0; pos+1 < n; pos++ {
			bad := slices.Clone(base)
			bad[pos], bad[pos+1] = bad[pos+1]+1, bad[pos] // strict descent at pos
			checkInts(t, bad)
		}
	}

	// Named ordered element: the generic side infers through ~[]E and
	// compares underlying ints — no method is ever consulted.
	named := []celsius{-3, -3, 0, 4, 4, 9}
	if got, want := slices.IsSorted(named), true; got != want {
		t.Fatalf("slices.IsSorted(%v) = %v, want %v", named, got, want)
	}
	if got, want := slices.IsSorted([]celsius{5, 4}), false; got != want {
		t.Fatalf("slices.IsSorted([]celsius{5, 4}) = %v, want %v", got, want)
	}

	// Randomized ints from a tiny domain (tie-heavy: near-sorted slices
	// where the verdict often hinges on a single late pair) and from the
	// full range.
	r := rand.New(rand.NewSource(3008))
	for trial := 0; trial < 5000; trial++ {
		n := r.Intn(20)
		small := make([]int, n)
		wide := make([]int, n)
		for i := range small {
			small[i] = r.Intn(3)
			wide[i] = r.Int()
		}
		checkInts(t, small)
		checkInts(t, wide)
		sorted := slices.Clone(small)
		slices.Sort(sorted)
		checkInts(t, sorted) // must agree on true, not only on false
	}

	// Strings: lexicographic byte order with empty strings, prefix pairs
	// ("a" < "aa"), and non-ASCII bytes; plus every 2-permutation of the
	// pool, sorted and unsorted.
	pool := []string{"", "a", "aa", "ab", "b", "ba", "\x00", "\xff", "abc", "ÿ"}
	for _, x := range pool {
		for _, y := range pool {
			checkStrings(t, []string{x, y})
			checkStrings(t, []string{x, y, x})
		}
	}
	for trial := 0; trial < 5000; trial++ {
		n := r.Intn(12)
		base := make([]string, n)
		for i := range base {
			base[i] = pool[r.Intn(len(pool))]
		}
		checkStrings(t, base)
		sorted := slices.Clone(base)
		slices.Sort(sorted)
		checkStrings(t, sorted)
	}
}
