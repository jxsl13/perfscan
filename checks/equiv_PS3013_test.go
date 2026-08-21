package checks

// PS3013's runtime differential: slices.SortFunc with a hand-rolled
// three-way comparator — in every spelling the check matches (if-chain,
// if/else-if with trailing return, fully chained if/else-if/else, switch
// with default, switch plus trailing return, swapped if order, swapped
// operand order b>a/b<a, non-unit magnitudes) — must leave the slice
// BYTE-IDENTICAL to slices.Sort for every input the auto-fix accepts
// (integer and string elements). Pinned against the REAL stdlib so a future
// stdlib change that breaks the claim fails CI:
//
//  1. PERMUTATION: each matched comparator returns a value whose SIGN equals
//     cmp.Compare(a, b) on every pair (a<b -> negative, a>b -> positive,
//     equal -> 0), and slices.SortFunc consumes only that sign;
//     cmp.Compare(a, b) < 0 iff cmp.Less(a, b) — the relation slices.Sort's
//     pdqsort branches on — so every comparison answers the same on both
//     sides and the sorted permutation is identical.
//  2. TIES: for the accepted element kinds any two elements comparing equal
//     are bitwise-identical values, so even the unstable sort's freedom to
//     arrange ties is unobservable: the output is byte-for-byte identical.
//
// The inputs are adversarial for an unstable pdqsort: sizes crossing the
// insertion-sort cutoff and recursion thresholds, all-equal runs and
// duplicate plateaus (tie-arrangement freedom), already-sorted, reversed and
// organ-pipe shapes (pdqsort's pattern detectors), int extremes, and strings
// with shared prefixes, 0x00/0xFF bytes and invalid UTF-8 (byte-wise order).
// Float divergence (NaN comparing neither < nor >, -0.0/+0.0 ties) is
// exactly why the fix excludes floats; that exclusion is the check's, not
// this test's, concern.

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3013Comparators returns every comparator spelling the check matches,
// keyed by name for failure messages.
func ps3013Comparators[E stdcmp.Ordered]() map[string]func(E, E) int {
	return map[string]func(E, E) int{
		"if-chain": func(a, b E) int {
			if a < b {
				return -1
			}
			if a > b {
				return 1
			}
			return 0
		},
		"else-if": func(a, b E) int {
			if a < b {
				return -1
			} else if a > b {
				return 1
			}
			return 0
		},
		"else-chain": func(a, b E) int {
			if a < b {
				return -1
			} else if a > b {
				return 1
			} else {
				return 0
			}
		},
		"switch-default": func(a, b E) int {
			switch {
			case a < b:
				return -1
			case a > b:
				return 1
			default:
				return 0
			}
		},
		"switch-trailing": func(a, b E) int {
			switch {
			case a < b:
				return -1
			case a > b:
				return 1
			}
			return 0
		},
		"swapped-order-magnitudes": func(a, b E) int {
			if b < a {
				return 42
			}
			if b > a {
				return -7
			}
			return 0
		},
	}
}

func ps3013Check[E stdcmp.Ordered](t *testing.T, s []E) {
	t.Helper()
	want := slices.Clone(s)
	slices.Sort(want)
	for name, cmpFn := range ps3013Comparators[E]() {
		got := slices.Clone(s)
		slices.SortFunc(got, cmpFn)
		if !slices.Equal(got, want) {
			t.Fatalf("SortFunc(%s three-way) != Sort on %v: %v vs %v", name, s, got, want)
		}
	}
}

func TestEquiv_PS3013HandRolledThreeWay(t *testing.T) {
	// Structured integer shapes at pdqsort-adversarial sizes: below/at/above
	// the insertion-sort cutoff (12) and well beyond.
	sizes := []int{0, 1, 2, 3, 7, 12, 13, 31, 64, 257, 1024}
	for _, n := range sizes {
		sorted := make([]int, n)
		for i := range sorted {
			sorted[i] = i - n/2
		}
		ps3013Check(t, sorted)

		rev := slices.Clone(sorted)
		slices.Reverse(rev)
		ps3013Check(t, rev)

		allEqual := make([]int, n)
		for i := range allEqual {
			allEqual[i] = 42
		}
		ps3013Check(t, allEqual)

		organ := make([]int, n)
		for i := range organ {
			organ[i] = min(i, n-1-i) // organ pipe: ascending then descending
		}
		ps3013Check(t, organ)

		fewDistinct := make([]int, n)
		for i := range fewDistinct {
			fewDistinct[i] = i % 3 // duplicate plateaus
		}
		ps3013Check(t, fewDistinct)
	}

	// Int extremes among duplicates — inputs on which the NOT-matched
	// subtraction comparator would overflow; the matched three-way answers
	// them exactly like slices.Sort.
	ps3013Check(t, []int{math.MaxInt, math.MinInt, 0, -1, 1, math.MaxInt, math.MinInt, 0, 0})

	// Unsigned and narrow integer kinds (named types are the same underlying
	// story; the classifier accepts any integer kind).
	ps3013Check(t, []uint8{255, 0, 128, 1, 255, 0, 127, 128})
	ps3013Check(t, []uint64{math.MaxUint64, 0, 1, math.MaxUint64, 42})
	ps3013Check(t, []int8{-128, 127, 0, -1, -128, 127})

	// Randomized integer draws: many duplicates from a tiny alphabet (tie
	// stress) and full-range values (pattern-free), at random sizes.
	r := rand.New(rand.NewSource(3013))
	for trial := 0; trial < 1000; trial++ {
		n := r.Intn(80)
		small := make([]int, n)
		big := make([]int, n)
		for i := 0; i < n; i++ {
			small[i] = r.Intn(4)
			big[i] = int(r.Uint64()) // full-range, sign bit included
		}
		ps3013Check(t, small)
		ps3013Check(t, big)
	}

	// Strings: byte-lexicographic edge cases — empty, prefix pairs
	// ("prefix" < "prefixx"), 0x00/0xFF bytes, invalid UTF-8, high-bit vs
	// ASCII — with duplicates.
	pool := []string{
		"", "a", "aa", "aaa", "ab", "b", "ba",
		"\x00", "\x00a", "a\x00", "\xff", "\xff\xfe",
		"\x80world", "world", "wörld", "wo\xc3",
		"prefix", "prefixx", "prefix\x00",
	}
	ps3013Check(t, pool)
	for trial := 0; trial < 1000; trial++ {
		n := r.Intn(60)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		ps3013Check(t, s)
	}

	// Random byte strings (arbitrary, frequently invalid UTF-8) with a tiny
	// alphabet forcing long shared prefixes and frequent exact ties.
	for trial := 0; trial < 500; trial++ {
		n := r.Intn(50)
		s := make([]string, n)
		for i := range s {
			b := make([]byte, r.Intn(8))
			for j := range b {
				b[j] = []byte{0x00, 'a', 0x7f, 0x80, 0xc3, 0xff}[r.Intn(6)]
			}
			s[i] = string(b)
		}
		ps3013Check(t, s)
	}

	// A big shuffled slice: full pdqsort recursion depth.
	bigSlice := make([]int, 4096)
	for i := range bigSlice {
		bigSlice[i] = i % 1000 // duplicates at scale
	}
	r.Shuffle(len(bigSlice), func(i, j int) { bigSlice[i], bigSlice[j] = bigSlice[j], bigSlice[i] })
	ps3013Check(t, bigSlice)
}
