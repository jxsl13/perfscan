package checks

// PS3006's runtime differential: slices.SortStableFunc(s, cmp.Compare) —
// in both the func-value and the bare-closure spelling — must produce an
// output slice IDENTICAL to slices.Sort(s) for every input the auto-fix
// accepts (elements whose underlying type is an integer or string kind).
// Two claims are pinned here, against the REAL stdlib so a future stdlib
// change that breaks either fails CI:
//
//  1. ORDER: cmp.Compare(a, b) < 0 iff cmp.Less(a, b), the exact relation
//     slices.Sort is defined by, so both sides sort into the same ascending
//     order.
//  2. TIES: for integer/string elements, values that compare equal are
//     indistinguishable, so the stable algorithm's tie-order guarantee —
//     the ONLY semantic difference between SortStableFunc and Sort — is
//     unobservable and the slices are elementwise identical.
//
// The inputs are adversarial for BOTH algorithms: tie-heavy small domains
// (exercising the stable insertion runs and symMerge tie handling),
// already-sorted / reversed / all-equal / organ-pipe / sawtooth shapes
// (pdqsort's pattern-detection paths), and large uniform-random slices
// (full pdqsort machinery: pivot selection, partitioning, heap fallback).

import (
	"cmp"
	"math/rand"
	"slices"
	"testing"
)

func TestEquiv_PS3006SortStableFuncCmpCompare(t *testing.T) {
	type celsius int // named ordered element, like the fixture's type Celsius int

	check := func(t *testing.T, base []int) {
		t.Helper()
		a := slices.Clone(base)
		b := slices.Clone(base)
		c := slices.Clone(base)
		slices.SortStableFunc(a, func(x, y int) int { return cmp.Compare(x, y) })
		slices.Sort(b)
		slices.SortStableFunc(c, cmp.Compare) // the func-value spelling
		if !slices.Equal(a, b) || !slices.Equal(c, b) {
			t.Fatalf("slices.SortStableFunc(cmp.Compare) != slices.Sort on %v: %v / %v vs %v", base, a, c, b)
		}
	}

	// Deterministic adversarial shapes, at sizes spanning the stable
	// sort's insertion-run threshold and pdqsort's pattern shortcuts.
	for _, n := range []int{0, 1, 2, 3, 7, 12, 13, 20, 64, 257} {
		sorted := make([]int, n)
		reversed := make([]int, n)
		allEqual := make([]int, n)
		organPipe := make([]int, n)
		sawtooth := make([]int, n)
		for i := 0; i < n; i++ {
			sorted[i] = i
			reversed[i] = n - i
			allEqual[i] = 42
			if i < n/2 {
				organPipe[i] = i
			} else {
				organPipe[i] = n - i
			}
			sawtooth[i] = i % 5 // tie-heavy periodic pattern
		}
		for _, base := range [][]int{sorted, reversed, allEqual, organPipe, sawtooth} {
			check(t, base)
		}
	}

	// Randomized tie-heavy ints (small domain -> many ties, the stable
	// sort's specialty) and the named-element spelling.
	r := rand.New(rand.NewSource(3006))
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(80)
		base := make([]int, n)
		for i := range base {
			base[i] = r.Intn(6)
		}
		check(t, base)

		named := make([]celsius, n)
		for i := range base {
			named[i] = celsius(base[i])
		}
		na := slices.Clone(named)
		nb := slices.Clone(named)
		slices.SortStableFunc(na, func(x, y celsius) int { return cmp.Compare(x, y) })
		slices.Sort(nb)
		if !slices.Equal(na, nb) {
			t.Fatalf("named-type SortStableFunc(cmp.Compare) != slices.Sort on %v: %v vs %v", named, na, nb)
		}
	}

	// Strings from a duplicate-guaranteed pool: equal strings are
	// interchangeable values, so the tie arrangement is unobservable and
	// the outputs must be elementwise identical.
	pool := []string{"", "a", "aa", "ab", "b", "ba", "bb", "abc"}
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(40)
		base := make([]string, n)
		for i := range base {
			base[i] = pool[r.Intn(len(pool))]
		}
		a := slices.Clone(base)
		b := slices.Clone(base)
		c := slices.Clone(base)
		slices.SortStableFunc(a, func(x, y string) int { return cmp.Compare(x, y) })
		slices.Sort(b)
		slices.SortStableFunc(c, cmp.Compare)
		if !slices.Equal(a, b) || !slices.Equal(c, b) {
			t.Fatalf("string SortStableFunc(cmp.Compare) != slices.Sort on %v: %v / %v vs %v", base, a, c, b)
		}
	}

	// Big random ints too (not just tie-heavy): 4096 elements exercises
	// the full pdqsort machinery on one side and the run-merge cascade of
	// the stable sort on the other.
	for trial := 0; trial < 50; trial++ {
		base := make([]int, 4096)
		for i := range base {
			base[i] = r.Int()
		}
		check(t, base)
	}
}
