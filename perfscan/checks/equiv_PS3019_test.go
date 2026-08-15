package checks

// PS3019's runtime differential: slices.IsSortedFunc with a hand-rolled
// three-way comparator — in every spelling the check matches (if-chain,
// if/else-if with trailing return, fully chained if/else-if/else, switch
// with default, switch plus trailing return, swapped if order, swapped
// operand order b>a/b<a, non-unit magnitudes) — must answer EXACTLY the
// bool slices.IsSorted answers for every input the auto-fix accepts
// (integer and string elements). Pinned against the REAL stdlib so a
// future stdlib change that breaks the claim fails CI:
//
//  1. PREDICATE: each matched comparator returns a value whose SIGN equals
//     cmp.Compare(a, b) on every pair (a<b -> negative, a>b -> positive,
//     equal -> 0), and IsSortedFunc consumes only 'cmp(x[i], x[i-1]) < 0';
//     for integer/string elements that predicate is 'x[i] < x[i-1]' —
//     precisely the cmp.Less predicate slices.IsSorted's descending scan
//     branches on — so every adjacent pair answers the same on both sides
//     and the scan returns the identical bool at the identical point.
//  2. There is no tie-arrangement dimension at all: the output is a single
//     bool, not a reordered slice, so byte-identity IS bool equality.
//
// The inputs are adversarial for the scan: empty and single-element
// slices, sorted runs, all-equal runs and duplicate plateaus (equal
// neighbors must read as in-order), a violation planted at EVERY position
// (first pair, middle, last pair — the early return must trip at the same
// point), reversed and organ-pipe shapes, int extremes (where the
// NOT-matched subtraction comparator would overflow), unsigned and narrow
// kinds, and strings with shared prefixes, 0x00/0xFF bytes and invalid
// UTF-8 (byte-wise order on both sides).
//
// Float divergence is pinned too — as the reason the fix EXCLUDES floats:
// a NaN compares neither '<' nor '>', so the hand-rolled chain calls NaN
// equal to everything and the scan sails past it, while slices.IsSorted
// orders NaNs first. On [1.0, NaN] the two sides provably disagree.

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3019Comparators returns every comparator spelling the check matches,
// keyed by name for failure messages (the same spelling set as PS3013's —
// the matcher is shared).
func ps3019Comparators[E stdcmp.Ordered]() map[string]func(E, E) int {
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

func ps3019Check[E stdcmp.Ordered](t *testing.T, s []E) {
	t.Helper()
	want := slices.IsSorted(s)
	for name, cmpFn := range ps3019Comparators[E]() {
		if got := slices.IsSortedFunc(s, cmpFn); got != want {
			t.Fatalf("IsSortedFunc(%s three-way) = %v, IsSorted = %v on %v", name, got, want, s)
		}
	}
}

func TestEquiv_PS3019HandRolledThreeWay(t *testing.T) {
	// Structured integer shapes: sorted (the full O(n) pass), reversed,
	// all-equal and few-distinct plateaus (equal neighbors are in-order on
	// both sides), organ pipe (sorted until the peak), and a violation
	// planted at every single position of a sorted run — the early return
	// must answer identically wherever the first out-of-order pair sits.
	sizes := []int{0, 1, 2, 3, 7, 13, 64, 257}
	for _, n := range sizes {
		sorted := make([]int, n)
		for i := range sorted {
			sorted[i] = i - n/2
		}
		ps3019Check(t, sorted)

		rev := slices.Clone(sorted)
		slices.Reverse(rev)
		ps3019Check(t, rev)

		allEqual := make([]int, n)
		for i := range allEqual {
			allEqual[i] = 42
		}
		ps3019Check(t, allEqual)

		organ := make([]int, n)
		for i := range organ {
			organ[i] = min(i, n-1-i)
		}
		ps3019Check(t, organ)

		fewDistinct := make([]int, n)
		for i := range fewDistinct {
			fewDistinct[i] = i % 3
		}
		ps3019Check(t, fewDistinct)

		for at := 0; at < n; at++ {
			planted := slices.Clone(sorted)
			planted[at] = math.MaxInt // one spike: unsorted unless at == n-1
			ps3019Check(t, planted)
			planted[at] = math.MinInt // one dip: unsorted unless at == 0
			ps3019Check(t, planted)
		}
	}

	// Int extremes among duplicates — inputs on which the NOT-matched
	// subtraction comparator would overflow; the matched three-way answers
	// them exactly like slices.IsSorted.
	ps3019Check(t, []int{math.MinInt, math.MinInt, -1, 0, 0, 1, math.MaxInt, math.MaxInt})
	ps3019Check(t, []int{math.MaxInt, math.MinInt, 0, -1, 1, math.MaxInt, math.MinInt, 0, 0})

	// Unsigned and narrow integer kinds (named types are the same
	// underlying story; the classifier accepts any integer kind).
	ps3019Check(t, []uint8{0, 1, 127, 128, 255})
	ps3019Check(t, []uint8{255, 0, 128, 1, 255, 0, 127, 128})
	ps3019Check(t, []uint64{0, 1, 42, math.MaxUint64, math.MaxUint64})
	ps3019Check(t, []uint64{math.MaxUint64, 0, 1, math.MaxUint64, 42})
	ps3019Check(t, []int8{-128, -128, -1, 0, 127, 127})
	ps3019Check(t, []int8{127, -128, 0, -1, -128, 127})

	// Randomized integer draws: many duplicates from a tiny alphabet
	// (near-sorted with tie plateaus, both verdicts frequent) and
	// full-range values, at random sizes.
	r := rand.New(rand.NewSource(3019))
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(40)
		small := make([]int, n)
		big := make([]int, n)
		for i := 0; i < n; i++ {
			small[i] = r.Intn(3)
			big[i] = int(r.Uint64()) // full-range, sign bit included
		}
		ps3019Check(t, small)
		slices.Sort(small) // sorted with duplicates: verdict true
		ps3019Check(t, small)
		ps3019Check(t, big)
	}

	// Strings: byte-lexicographic edge cases — empty, prefix pairs
	// ("prefix" < "prefixx"), 0x00/0xFF bytes, invalid UTF-8, high-bit vs
	// ASCII — with duplicates, unsorted, sorted, and near-sorted with one
	// planted violation.
	pool := []string{
		"", "a", "aa", "aaa", "ab", "b", "ba",
		"\x00", "\x00a", "a\x00", "\xff", "\xff\xfe",
		"\x80world", "world", "wörld", "wo\xc3",
		"prefix", "prefixx", "prefix\x00",
	}
	ps3019Check(t, pool)
	sortedPool := slices.Clone(pool)
	slices.Sort(sortedPool)
	ps3019Check(t, sortedPool)
	for at := range sortedPool {
		planted := slices.Clone(sortedPool)
		planted[at] = "\xff\xff" // one spike
		ps3019Check(t, planted)
		planted[at] = "" // one dip
		ps3019Check(t, planted)
	}
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(30)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		ps3019Check(t, s)
		slices.Sort(s)
		ps3019Check(t, s)
	}

	// Random byte strings (arbitrary, frequently invalid UTF-8) with a
	// tiny alphabet forcing long shared prefixes and frequent exact ties.
	for trial := 0; trial < 500; trial++ {
		n := r.Intn(25)
		s := make([]string, n)
		for i := range s {
			b := make([]byte, r.Intn(8))
			for j := range b {
				b[j] = []byte{0x00, 'a', 0x7f, 0x80, 0xc3, 0xff}[r.Intn(6)]
			}
			s[i] = string(b)
		}
		ps3019Check(t, s)
		slices.Sort(s)
		ps3019Check(t, s)
	}

	// A big sorted slice with tie plateaus: the full O(n) pass at scale.
	bigSorted := make([]int, 4096)
	for i := range bigSorted {
		bigSorted[i] = i / 2
	}
	ps3019Check(t, bigSorted)
}

// TestEquiv_PS3019FloatDivergence pins WHY float elements are advisory-only:
// the hand-rolled chain calls a NaN equal to everything ('<' and '>' are
// both false), so IsSortedFunc scans past it and answers true, while
// slices.IsSorted — defined via cmp.Less, which orders NaNs first — answers
// false for a NaN after a non-NaN. The two spellings provably disagree, so
// the auto-fix must never touch float elements. (This is the check's guard
// under test, not a bug: the fixture advisory.go keeps floats report-only.)
func TestEquiv_PS3019FloatDivergence(t *testing.T) {
	nan := math.NaN()
	chain := func(a, b float64) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
	for _, s := range [][]float64{
		{1.0, nan},
		{1.0, nan, 2.0},
		{nan, 2.0, nan},
	} {
		got := slices.IsSortedFunc(s, chain)
		want := slices.IsSorted(s)
		if got == want {
			t.Fatalf("expected divergence on %v: IsSortedFunc(chain) = %v, IsSorted = %v — if the stdlib changed, revisit PS3019's float guard", s, got, want)
		}
	}
	// And -0.0/+0.0 in either order agree on BOTH sides (they compare
	// equal); the float guard exists solely for NaN, but stays a blanket
	// float exclusion because a static check cannot rule NaN out.
	negZero := math.Copysign(0, -1)
	for _, s := range [][]float64{{negZero, 0.0}, {0.0, negZero}} {
		got := slices.IsSortedFunc(s, chain)
		want := slices.IsSorted(s)
		if got != want || !got {
			t.Fatalf("±0.0 must agree (both true) on %v: IsSortedFunc(chain) = %v, IsSorted = %v", s, got, want)
		}
	}
}
