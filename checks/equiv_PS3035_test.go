package checks

// PS3035's runtime differential: slices.MaxFunc/MinFunc with a SWAPPED
// hand-rolled three-way comparator (a>b -> negative, a<b -> positive — the
// REVERSED order) — in every spelling the check matches (if-chain, if/else-if
// with trailing return, fully chained if/else-if/else, switch with default,
// switch plus trailing return, flipped branch order, swapped operand order
// b<a/b>a, non-unit magnitudes) — must return a value BYTE-IDENTICAL to the
// OPPOSITE direct extremum, slices.Min/Max, for every input the auto-fix
// accepts (integer elements; strings are checked too because the check's
// advisory message claims bit-identity for them). Pinned against the REAL
// stdlib so a future stdlib change that breaks the claim fails CI:
//
//  1. VALUE: each matched comparator returns a value whose SIGN equals
//     cmp.Compare(b, a) on every pair (a>b -> negative, a<b -> positive,
//     equal -> 0). MaxFunc consumes only that sign (cmp(x[i], m) > 0), which
//     under the reversed order means x[i] < m — so MaxFunc updates exactly
//     when the new element is strictly SMALLER under the natural order,
//     selecting the elements slices.Min's builtin-min fold selects; MinFunc
//     reversed is the symmetric slices.Max.
//  2. TIES: both scans keep the EARLIER element of any tie (cmp == 0 never
//     updates; the builtin min/max keep the earlier operand), and for the
//     accepted element kinds any two elements comparing equal are
//     bitwise-identical values anyway, so tie selection is unobservable and
//     the returned value is byte-for-byte identical.
//  3. EMPTY: both spellings panic on an empty slice — verified explicitly.
//
// The inputs are adversarial for the extremum scan: the extrema first, last,
// middle, duplicated (tie stress from a tiny alphabet), all-equal runs,
// ascending/descending/organ-pipe shapes, int extremes among duplicates,
// unsigned and narrow kinds, and strings with shared prefixes, 0x00/0xFF
// bytes and invalid UTF-8. Float divergence (the ladder answers 0 for NaN
// against anything and calls -0.0/+0.0 a tie while the builtin min/max
// PROPAGATE NaN and order signed zeros) is exactly why the fix excludes
// floats; that exclusion is the check's, not this test's, concern.

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3035Comparators returns every SWAPPED comparator spelling the check
// matches, keyed by name for failure messages (the sign-mirrored PS3013
// matcher's shapes).
func ps3035Comparators[E stdcmp.Ordered]() map[string]func(E, E) int {
	return map[string]func(E, E) int{
		"if-chain": func(a, b E) int {
			if a > b {
				return -1
			}
			if a < b {
				return 1
			}
			return 0
		},
		"else-if": func(a, b E) int {
			if a > b {
				return -1
			} else if a < b {
				return 1
			}
			return 0
		},
		"else-chain": func(a, b E) int {
			if a < b {
				return 1
			} else if a > b {
				return -1
			} else {
				return 0
			}
		},
		"switch-default": func(a, b E) int {
			switch {
			case a > b:
				return -1
			case a < b:
				return 1
			default:
				return 0
			}
		},
		"switch-trailing": func(a, b E) int {
			switch {
			case a > b:
				return -1
			case a < b:
				return 1
			}
			return 0
		},
		"swapped-order-magnitudes": func(a, b E) int {
			if b < a {
				return -42
			}
			if b > a {
				return 7
			}
			return 0
		},
	}
}

// ps3035Check asserts MaxFunc under every matched REVERSED comparator equals
// slices.Min, and MinFunc equals slices.Max — the opposite-extremum rewrite.
func ps3035Check[E stdcmp.Ordered](t *testing.T, s []E) {
	t.Helper()
	if len(s) == 0 {
		return
	}
	wantMin := slices.Min(s)
	wantMax := slices.Max(s)
	for name, cmpFn := range ps3035Comparators[E]() {
		if got := slices.MaxFunc(s, cmpFn); got != wantMin {
			t.Fatalf("MaxFunc(%s swapped three-way) != Min on %v: %v vs %v", name, s, got, wantMin)
		}
		if got := slices.MinFunc(s, cmpFn); got != wantMax {
			t.Fatalf("MinFunc(%s swapped three-way) != Max on %v: %v vs %v", name, s, got, wantMax)
		}
	}
}

// ps3035MustPanic asserts fn panics — the empty-slice contract both
// spellings share.
func ps3035MustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s on an empty slice did not panic", what)
		}
	}()
	fn()
}

func TestEquiv_PS3035SwappedHandRolledThreeWay(t *testing.T) {
	// Both spellings panic on the empty slice, so the rewrite preserves the
	// failure mode too.
	ladder := ps3035Comparators[int]()["if-chain"]
	ps3035MustPanic(t, "slices.Min", func() { _ = slices.Min([]int{}) })
	ps3035MustPanic(t, "slices.MaxFunc", func() { _ = slices.MaxFunc([]int{}, ladder) })
	ps3035MustPanic(t, "slices.Max", func() { _ = slices.Max([]int{}) })
	ps3035MustPanic(t, "slices.MinFunc", func() { _ = slices.MinFunc([]int{}, ladder) })

	// Structured integer shapes: extremum first/last/middle, duplicated
	// extremum, all-equal, monotone and organ-pipe runs, at assorted sizes.
	sizes := []int{1, 2, 3, 7, 13, 64, 257, 1024}
	for _, n := range sizes {
		asc := make([]int, n)
		for i := range asc {
			asc[i] = i - n/2 // max last, min first
		}
		ps3035Check(t, asc)

		desc := slices.Clone(asc)
		slices.Reverse(desc) // max first, min last
		ps3035Check(t, desc)

		allEqual := make([]int, n)
		for i := range allEqual {
			allEqual[i] = 42 // every element ties for both extrema
		}
		ps3035Check(t, allEqual)

		organ := make([]int, n)
		for i := range organ {
			organ[i] = min(i, n-1-i) // max in the middle, min at both ends
		}
		ps3035Check(t, organ)

		fewDistinct := make([]int, n)
		for i := range fewDistinct {
			fewDistinct[i] = i % 3 // duplicated extrema throughout
		}
		ps3035Check(t, fewDistinct)
	}

	// Int extremes among duplicates — inputs on which the NOT-matched
	// reversed subtraction comparator (b - a) would overflow; the matched
	// swapped three-way answers them exactly like the builtin min/max.
	ps3035Check(t, []int{math.MaxInt, math.MinInt, 0, -1, 1, math.MaxInt, math.MinInt, 0, 0})

	// Unsigned and narrow integer kinds (named types are the same underlying
	// story; the classifier accepts any integer kind).
	ps3035Check(t, []uint8{255, 0, 128, 1, 255, 0, 127, 128})
	ps3035Check(t, []uint64{math.MaxUint64, 0, 1, math.MaxUint64, 42})
	ps3035Check(t, []int8{-128, 127, 0, -1, -128, 127})

	// Randomized integer draws: many duplicates from a tiny alphabet (tie
	// stress) and full-range values (pattern-free), at random sizes.
	r := rand.New(rand.NewSource(3035))
	for trial := 0; trial < 1000; trial++ {
		n := 1 + r.Intn(80)
		small := make([]int, n)
		big := make([]int, n)
		for i := 0; i < n; i++ {
			small[i] = r.Intn(4)
			big[i] = int(r.Uint64()) // full-range, sign bit included
		}
		ps3035Check(t, small)
		ps3035Check(t, big)
	}

	// Strings: the check's ADVISORY message claims bit-identity (the fix is
	// withheld for perf only), so pin that claim too — byte-lexicographic
	// edge cases: empty, prefix pairs ("prefix" < "prefixx"), 0x00/0xFF
	// bytes, invalid UTF-8, high-bit vs ASCII — with duplicates.
	pool := []string{
		"", "a", "aa", "aaa", "ab", "b", "ba",
		"\x00", "\x00a", "a\x00", "\xff", "\xff\xfe",
		"\x80world", "world", "wörld", "wo\xc3",
		"prefix", "prefixx", "prefix\x00",
	}
	ps3035Check(t, pool)
	for trial := 0; trial < 1000; trial++ {
		n := 1 + r.Intn(60)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		ps3035Check(t, s)
	}

	// Random byte strings (arbitrary, frequently invalid UTF-8) with a tiny
	// alphabet forcing long shared prefixes and frequent exact ties.
	for trial := 0; trial < 500; trial++ {
		n := 1 + r.Intn(50)
		s := make([]string, n)
		for i := range s {
			b := make([]byte, r.Intn(8))
			for j := range b {
				b[j] = []byte{0x00, 'a', 0x7f, 0x80, 0xc3, 0xff}[r.Intn(6)]
			}
			s[i] = string(b)
		}
		ps3035Check(t, s)
	}

	// A big scattered slice: full-length single-pass scan.
	bigSlice := make([]int, 4096)
	for i := range bigSlice {
		bigSlice[i] = i % 1000 // duplicated extrema at scale
	}
	r.Shuffle(len(bigSlice), func(i, j int) { bigSlice[i], bigSlice[j] = bigSlice[j], bigSlice[i] })
	ps3035Check(t, bigSlice)
}
