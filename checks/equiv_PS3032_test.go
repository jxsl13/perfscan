package checks

// PS3032's runtime differential: sort.IsSorted(sort.IntSlice(x)) and
// sort.IsSorted(sort.StringSlice(x)) must return a bool IDENTICAL to
// slices.IsSorted(x) for EVERY input — pinned against the REAL stdlib so a
// future stdlib change that breaks the claim fails CI. Three claims:
//
//  1. PREDICATE: both spellings answer "no adjacent pair is descending
//     under <". sort.IntSlice.Less / sort.StringSlice.Less are the plain
//     x[i] < x[j], and slices.IsSorted's cmp.Less compiles to that
//     identical < for int and string (its NaN clause is constant-false for
//     non-float types) — and unlike the sort.IntsAreSorted helper (a
//     one-line slices.IsSorted wrapper since go1.22, PS3008's subject),
//     the adapter spelling really does scan through the sort.Interface,
//     so this differential pins two genuinely distinct code paths.
//  2. SCAN DIRECTION IS UNOBSERVABLE: the comparison is side-effect-free,
//     so whichever adjacent pair either implementation happens to test
//     first cannot change the returned bool — sortedness is a total
//     predicate over all adjacent pairs.
//  3. NO REORDERING: the call is a pure read-only scan returning a bool,
//     and the conversion is a zero-copy reinterpretation of the same
//     slice header; the differential also verifies the input slice is
//     left byte-identical by both sides.
//
// The inputs are adversarial for a sortedness scan: exactly-sorted runs
// with a single violation planted at every position (first pair, last
// pair, middle), all-equal and tie-heavy slices (ties are NOT violations
// under a non-strict <= order), len 0/1/2 boundaries, int extremes
// (MinInt/MaxInt wraparound-adjacent pairs), and strings exercising
// lexicographic byte order including the empty string, prefix pairs
// ("a" < "aa"), and non-ASCII bytes.

import (
	"math"
	"math/rand"
	"slices"
	"sort"
	"testing"
)

func TestEquiv_PS3032IsSortedAdapterSlicesIsSorted(t *testing.T) {
	type intList []int // named slice type, like the fixture's intList

	checkInts := func(t *testing.T, base []int) {
		t.Helper()
		a := slices.Clone(base)
		b := slices.Clone(base)
		got := sort.IsSorted(sort.IntSlice(a)) // the BEFORE shape, verbatim
		want := slices.IsSorted(b)             // the AFTER shape, verbatim
		if got != want {
			t.Fatalf("sort.IsSorted(sort.IntSlice(%v)) = %v, slices.IsSorted = %v", base, got, want)
		}
		if !slices.Equal(a, base) || !slices.Equal(b, base) {
			t.Fatalf("a sortedness predicate mutated its input: %v / %v from %v", a, b, base)
		}
	}
	checkStrings := func(t *testing.T, base []string) {
		t.Helper()
		a := slices.Clone(base)
		b := slices.Clone(base)
		got := sort.IsSorted(sort.StringSlice(a))
		want := slices.IsSorted(b)
		if got != want {
			t.Fatalf("sort.IsSorted(sort.StringSlice(%v)) = %v, slices.IsSorted = %v", base, got, want)
		}
		if !slices.Equal(a, base) || !slices.Equal(b, base) {
			t.Fatalf("a sortedness predicate mutated its input: %v / %v from %v", a, b, base)
		}
	}

	// Boundaries: len 0/1/2 (both return true for len < 2, nil included —
	// note the BEFORE shape accepts a typed nil []int through the
	// conversion; the untyped-nil spelling is excluded by the matcher).
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

	// Named slice type: the BEFORE shape converts it into the adapter
	// (legal — same underlying []int, zero-copy over the same backing
	// array), the AFTER shape infers through ~[]E and compares underlying
	// ints — no method is ever consulted; the bools must agree.
	named := intList{-3, -3, 0, 4, 4, 9}
	if got, want := sort.IsSorted(sort.IntSlice(named)), slices.IsSorted(named); got != want {
		t.Fatalf("adapter over named slice type = %v, slices.IsSorted = %v", got, want)
	}
	unsorted := intList{5, 4}
	if got, want := sort.IsSorted(sort.IntSlice(unsorted)), slices.IsSorted(unsorted); got != want {
		t.Fatalf("adapter over named slice type = %v, slices.IsSorted = %v", got, want)
	}

	// Randomized ints from a tiny domain (tie-heavy: near-sorted slices
	// where the verdict often hinges on a single late pair) and from the
	// full range.
	r := rand.New(rand.NewSource(3032))
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
