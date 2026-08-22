package checks

// PS3029's runtime differential: slices.SortedStableFunc(seq, <hand-rolled
// three-way>) must return a BYTE-IDENTICAL slice to slices.Sorted(seq) for
// every input the auto-fix accepts (integer and string elements), and must
// consume the seq identically (same yields, same count — the deleted
// comparator is only referenced, never called, so evaluation must not
// change). Pinned against the REAL stdlib so a future stdlib change that
// breaks either claim fails CI:
//
//  1. COLLECTION: both entry points are literally Collect(seq) then sort, so
//     the seq is ranged once, to exhaustion, on both sides. The tests below
//     wrap every seq in a counter asserting the yield sequence is identical.
//  2. PERMUTATION: the hand-rolled ladder returns a value whose SIGN equals
//     cmp.Compare(a, b) on every pair (a<b -> negative, a>b -> positive,
//     equal -> 0), and cmp.Compare(a, b) < 0 iff cmp.Less(a, b) — the
//     relation slices.Sort's pdqsort branches on — so every comparison
//     answers the same on both sides and the sorted permutation is
//     identical up to ties.
//  3. TIES: the STABLE before-side keeps equal elements in yield order while
//     the unstable after-side may arrange them freely — but for the accepted
//     element kinds any two elements comparing equal are bitwise-identical
//     values, so the freedom is unobservable and the output is byte-for-byte
//     identical. Every matched comparator SPELLING (sequential ifs,
//     if/else-if with trailing return or final else, expressionless switch
//     with default clause or trailing return, swapped-arm/reversed-operand/
//     magnitude variants) is exercised, since the matcher accepts them all.
//  4. EMPTINESS: an empty seq yields the same empty (nil) result on both
//     sides — no panic divergence anywhere (neither entry point panics).
//
// The inputs are adversarial for the stable/unstable pairing: sizes crossing
// the insertion-sort cutoff and recursion thresholds, all-equal runs and
// duplicate plateaus (maximum tie-arrangement freedom, symMerge's hot path),
// already-sorted, reversed and organ-pipe shapes (pdqsort's pattern
// detectors), int extremes, and strings with shared prefixes, 0x00/0xFF
// bytes and invalid UTF-8 (byte-wise order). Float divergence (the ladder
// answers 0 for a NaN against anything while slices.Sort orders NaNs first,
// and the stable side keeps -0.0/+0.0 ties in yield order) is exactly why
// the fix excludes floats; that exclusion is the check's, not this test's,
// concern.

import (
	"iter"
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3029Seq wraps s in an iter.Seq that records every yielded element, so
// two runs can be compared for identical consumption.
func ps3029Seq[E any](s []E, log *[]E) iter.Seq[E] {
	return func(yield func(E) bool) {
		for _, v := range s {
			*log = append(*log, v)
			if !yield(v) {
				return
			}
		}
	}
}

// ps3029Comparators returns every hand-rolled three-way SPELLING the matcher
// accepts — each must induce the identical result.
func ps3029Comparators[E stdcmp.Ordered]() []func(a, b E) int {
	return []func(a, b E) int{
		// Two sequential ifs plus a trailing return.
		func(a, b E) int {
			if a < b {
				return -1
			}
			if a > b {
				return 1
			}
			return 0
		},
		// if/else-if with the default as the trailing return.
		func(a, b E) int {
			if a < b {
				return -1
			} else if a > b {
				return 1
			}
			return 0
		},
		// Fully chained if/else-if/else.
		func(a, b E) int {
			if a < b {
				return -1
			} else if a > b {
				return 1
			} else {
				return 0
			}
		},
		// Expressionless switch with a default clause.
		func(a, b E) int {
			switch {
			case a < b:
				return -1
			case a > b:
				return 1
			default:
				return 0
			}
		},
		// Switch without a default plus the trailing return.
		func(a, b E) int {
			switch {
			case a < b:
				return -1
			case a > b:
				return 1
			}
			return 0
		},
		// Greater arm first, reversed operands, magnitudes other than 1 —
		// only the SIGN is consumed.
		func(a, b E) int {
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

func ps3029Check[E stdcmp.Ordered](t *testing.T, s []E) {
	t.Helper()
	var wantLog []E
	want := slices.Sorted(ps3029Seq(s, &wantLog))
	for i, cmpFn := range ps3029Comparators[E]() {
		var gotLog []E
		got := slices.SortedStableFunc(ps3029Seq(s, &gotLog), cmpFn)
		if !slices.Equal(got, want) {
			t.Fatalf("comparator %d: SortedStableFunc(three-way) != Sorted on %v: %v vs %v", i, s, got, want)
		}
		if !slices.Equal(gotLog, wantLog) {
			t.Fatalf("comparator %d: seq consumed differently on %v: %v vs %v", i, s, gotLog, wantLog)
		}
		// Byte-identity also means the nil/empty distinction agrees: both
		// sides are the same slices.Collect result.
		if (want == nil) != (got == nil) {
			t.Fatalf("comparator %d: nilness differs on %v: want nil=%v, got nil=%v", i, s, want == nil, got == nil)
		}
	}
}

func TestEquiv_PS3029SortedStableFuncHandRolledThreeWay(t *testing.T) {
	// Structured integer shapes at sort-adversarial sizes: below/at/above
	// the insertion-sort cutoffs (12 unstable, 20 stable) and well beyond,
	// crossing the stable sort's run-merge boundaries.
	sizes := []int{0, 1, 2, 3, 7, 12, 13, 20, 21, 31, 64, 257, 1024}
	for _, n := range sizes {
		sorted := make([]int, n)
		for i := range sorted {
			sorted[i] = i - n/2
		}
		ps3029Check(t, sorted)

		rev := slices.Clone(sorted)
		slices.Reverse(rev)
		ps3029Check(t, rev)

		allEqual := make([]int, n)
		for i := range allEqual {
			allEqual[i] = 42
		}
		ps3029Check(t, allEqual)

		organ := make([]int, n)
		for i := range organ {
			organ[i] = min(i, n-1-i) // organ pipe: ascending then descending
		}
		ps3029Check(t, organ)

		fewDistinct := make([]int, n)
		for i := range fewDistinct {
			fewDistinct[i] = i % 3 // duplicate plateaus: maximum tie freedom
		}
		ps3029Check(t, fewDistinct)
	}

	// Int extremes among duplicates.
	ps3029Check(t, []int{math.MaxInt, math.MinInt, 0, -1, 1, math.MaxInt, math.MinInt, 0, 0})

	// Unsigned and narrow integer kinds (named types are the same underlying
	// story; the classifier accepts any integer kind).
	ps3029Check(t, []uint8{255, 0, 128, 1, 255, 0, 127, 128})
	ps3029Check(t, []uint64{math.MaxUint64, 0, 1, math.MaxUint64, 42})
	ps3029Check(t, []int8{-128, 127, 0, -1, -128, 127})

	// Randomized integer draws: many duplicates from a tiny alphabet (tie
	// stress for the stable/unstable difference) and full-range values
	// (pattern-free), at random sizes.
	r := rand.New(rand.NewSource(3029))
	for trial := 0; trial < 1000; trial++ {
		n := r.Intn(80)
		small := make([]int, n)
		big := make([]int, n)
		for i := 0; i < n; i++ {
			small[i] = r.Intn(4)
			big[i] = int(r.Uint64()) // full-range, sign bit included
		}
		ps3029Check(t, small)
		ps3029Check(t, big)
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
	ps3029Check(t, pool)
	for trial := 0; trial < 1000; trial++ {
		n := r.Intn(60)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		ps3029Check(t, s)
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
		ps3029Check(t, s)
	}

	// A big shuffled slice: full recursion depth on both algorithms, with
	// duplicates at scale so the stable merge actually has ties to keep.
	bigSlice := make([]int, 4096)
	for i := range bigSlice {
		bigSlice[i] = i % 1000
	}
	r.Shuffle(len(bigSlice), func(i, j int) { bigSlice[i], bigSlice[j] = bigSlice[j], bigSlice[i] })
	ps3029Check(t, bigSlice)
}
