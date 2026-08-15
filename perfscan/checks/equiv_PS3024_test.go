package checks

// PS3024's runtime differential: sort.SliceIsSorted(x, func(i, j int) bool
// { return x[i] < x[j] }) must return a bool IDENTICAL to slices.IsSorted(x)
// for EVERY non-float ordered input — pinned against the REAL stdlib so a
// future stdlib change that breaks the claim fails CI. Two claims:
//
//  1. PREDICATE: both run the same backwards scan — sort.SliceIsSorted
//     returns false iff less(i, i-1) for some i (with this closure:
//     x[i] < x[i-1]), slices.IsSorted returns false iff
//     cmp.Less(x[i], x[i-1]) for some i. cmp.Less differs from < ONLY on
//     NaN, so for the non-float element types the fix accepts the two
//     decide false at exactly the same first out-of-order pair and true
//     otherwise, len < 2 (nil included) returning true on both sides.
//  2. NO REORDERING / NO MUTATION: the call is a pure read-only scan
//     returning a bool; the differential also verifies the input slice is
//     left byte-identical by both sides.
//
// The inputs are adversarial for a sortedness scan: exactly-sorted
// tie-heavy runs with a single violation planted at EVERY adjacent
// position, all-equal slices (ties are NOT violations under the strict <
// closure), len 0/1/2 boundaries, nil, int/uint extremes
// (MinInt/MaxInt/MaxUint64 adjacency, where a subtraction-based comparison
// would wrap), named element and named slice types (the generic side
// infers through ~[]E and compares underlying values), and strings
// exercising lexicographic byte order including empty strings, prefix
// pairs ("a" < "aa") and non-ASCII bytes.

import (
	"math"
	"math/rand"
	"slices"
	"sort"
	"testing"
)

func TestEquiv_PS3024SliceIsSortedClosureSlicesIsSorted(t *testing.T) {
	type celsius int   // named ordered element, like the fixture's uint64 field
	type intList []int // named slice type, inferred through ~[]E

	checkInts := func(t *testing.T, base []int) {
		t.Helper()
		a := slices.Clone(base)
		b := slices.Clone(base)
		got := sort.SliceIsSorted(a, func(i, j int) bool { return a[i] < a[j] })
		want := slices.IsSorted(b)
		if got != want {
			t.Fatalf("sort.SliceIsSorted(%v, <closure) = %v, slices.IsSorted = %v", base, got, want)
		}
		if !slices.Equal(a, base) || !slices.Equal(b, base) {
			t.Fatalf("a sortedness predicate mutated its input: %v / %v from %v", a, b, base)
		}
	}
	checkStrings := func(t *testing.T, base []string) {
		t.Helper()
		a := slices.Clone(base)
		b := slices.Clone(base)
		got := sort.SliceIsSorted(a, func(i, j int) bool { return a[i] < a[j] })
		want := slices.IsSorted(b)
		if got != want {
			t.Fatalf("sort.SliceIsSorted(%v, <closure) = %v, slices.IsSorted = %v", base, got, want)
		}
		if !slices.Equal(a, base) || !slices.Equal(b, base) {
			t.Fatalf("a sortedness predicate mutated its input: %v / %v from %v", a, b, base)
		}
	}
	checkUints := func(t *testing.T, base []uint64) {
		t.Helper()
		a := slices.Clone(base)
		b := slices.Clone(base)
		got := sort.SliceIsSorted(a, func(i, j int) bool { return a[i] < a[j] })
		want := slices.IsSorted(b)
		if got != want {
			t.Fatalf("sort.SliceIsSorted(%v, <closure) = %v, slices.IsSorted = %v", base, got, want)
		}
	}

	// Boundaries: len 0/1/2 — both return true for len < 2, nil included
	// (a nil slice boxed into sort's any parameter is a typed non-nil
	// interface whose reflected Len is 0, never a panic).
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

	// Int/uint extremes: MinInt/MaxInt (and MaxUint64) adjacency is where a
	// buggy subtraction-based comparison would wrap; < does not, on either
	// side.
	checkInts(t, []int{math.MinInt, -1, 0, 1, math.MaxInt})
	checkInts(t, []int{math.MaxInt, math.MinInt})
	checkInts(t, []int{math.MinInt, math.MaxInt})
	checkInts(t, []int{math.MaxInt, math.MaxInt, math.MinInt})
	checkUints(t, []uint64{0, 1, math.MaxUint64})
	checkUints(t, []uint64{math.MaxUint64, 0})
	checkUints(t, []uint64{math.MaxUint64, math.MaxUint64})

	// A single violation planted at EVERY adjacent position of an otherwise
	// sorted tie-heavy slice: both scans must spot it no matter where it
	// hides (first pair, last pair, anywhere between).
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

	// Named element AND named slice types: the closure compares the named
	// values with < (underlying int), and the generic side infers S=intList
	// / E=celsius through ~[]E and compares with the identical cmp.Less —
	// no method is ever consulted on either side.
	named := []celsius{-3, -3, 0, 4, 4, 9}
	if got, want := sort.SliceIsSorted(named, func(i, j int) bool { return named[i] < named[j] }), slices.IsSorted(named); got != want {
		t.Fatalf("named element: sort.SliceIsSorted = %v, slices.IsSorted = %v", got, want)
	}
	rev := []celsius{5, 4}
	if got, want := sort.SliceIsSorted(rev, func(i, j int) bool { return rev[i] < rev[j] }), slices.IsSorted(rev); got != want {
		t.Fatalf("named element (unsorted): sort.SliceIsSorted = %v, slices.IsSorted = %v", got, want)
	}
	nl := intList{1, 1, 2, 3}
	if got, want := sort.SliceIsSorted(nl, func(i, j int) bool { return nl[i] < nl[j] }), slices.IsSorted(nl); got != want {
		t.Fatalf("named slice: sort.SliceIsSorted = %v, slices.IsSorted = %v", got, want)
	}

	// Randomized ints from a tiny domain (tie-heavy: near-sorted slices
	// where the verdict often hinges on a single late pair) and from the
	// full range; the sorted clone must agree on true, not only on false.
	r := rand.New(rand.NewSource(3024))
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
		checkInts(t, sorted)
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
