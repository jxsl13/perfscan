package checks

// PS3012's runtime differential: slices.SortedFunc(seq, cmp.Compare) — in
// both the func-value and the bare-closure spelling — must return a
// BYTE-IDENTICAL slice to slices.Sorted(seq) for every input the auto-fix
// accepts (integer and string elements), and must consume the seq
// identically (same yields, same count — the deleted comparator is only
// referenced, never called, so evaluation must not change). Pinned against
// the REAL stdlib so a future stdlib change that breaks either claim fails
// CI:
//
//  1. COLLECTION: both entry points are literally Collect(seq) then sort, so
//     the seq is ranged once, to exhaustion, on both sides. The tests below
//     wrap every seq in a counter asserting the yield sequence is identical.
//  2. PERMUTATION: cmp.Compare(a, b) < 0 iff cmp.Less(a, b) — the relation
//     slices.Sort's pdqsort branches on — so every comparison answers the
//     same on both sides and the sorted permutation is identical. For the
//     accepted element kinds any two elements comparing equal are
//     bitwise-identical values, so even the unstable sort's freedom to
//     arrange ties is unobservable: the output is byte-for-byte identical.
//
// The inputs are adversarial for an unstable pdqsort: sizes crossing the
// insertion-sort cutoff and recursion thresholds, all-equal runs and
// duplicate plateaus (tie-arrangement freedom), already-sorted, reversed and
// organ-pipe shapes (pdqsort's pattern detectors), int extremes, and strings
// with shared prefixes, 0x00/0xFF bytes and invalid UTF-8 (byte-wise order).
// Float divergence (-0.0/+0.0, NaN payloads) is exactly why the fix excludes
// floats; that exclusion is the check's, not this test's, concern.

import (
	"iter"
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3012Seq wraps s in an iter.Seq that records every yielded element, so
// two runs can be compared for identical consumption.
func ps3012Seq[E any](s []E, log *[]E) iter.Seq[E] {
	return func(yield func(E) bool) {
		for _, v := range s {
			*log = append(*log, v)
			if !yield(v) {
				return
			}
		}
	}
}

func ps3012Check[E stdcmp.Ordered](t *testing.T, s []E) {
	t.Helper()
	var wantLog, gotLog1, gotLog2 []E
	want := slices.Sorted(ps3012Seq(s, &wantLog))
	got1 := slices.SortedFunc(ps3012Seq(s, &gotLog1), func(a, b E) int { return stdcmp.Compare(a, b) })
	got2 := slices.SortedFunc(ps3012Seq(s, &gotLog2), stdcmp.Compare)
	if !slices.Equal(got1, want) || !slices.Equal(got2, want) {
		t.Fatalf("SortedFunc(cmp.Compare) != Sorted on %v: %v / %v vs %v", s, got1, got2, want)
	}
	if !slices.Equal(gotLog1, wantLog) || !slices.Equal(gotLog2, wantLog) {
		t.Fatalf("seq consumed differently on %v: %v / %v vs %v", s, gotLog1, gotLog2, wantLog)
	}
	// Byte-identity also means the nil/empty distinction agrees: both sides
	// are the same slices.Collect result.
	if (want == nil) != (got1 == nil) || (want == nil) != (got2 == nil) {
		t.Fatalf("nilness differs on %v: want nil=%v, got %v/%v", s, want == nil, got1 == nil, got2 == nil)
	}
}

func TestEquiv_PS3012SortedFuncCmpCompare(t *testing.T) {
	// Structured integer shapes at pdqsort-adversarial sizes: below/at/above
	// the insertion-sort cutoff (12) and well beyond.
	sizes := []int{0, 1, 2, 3, 7, 12, 13, 31, 64, 257, 1024}
	for _, n := range sizes {
		sorted := make([]int, n)
		for i := range sorted {
			sorted[i] = i - n/2
		}
		ps3012Check(t, sorted)

		rev := slices.Clone(sorted)
		slices.Reverse(rev)
		ps3012Check(t, rev)

		allEqual := make([]int, n)
		for i := range allEqual {
			allEqual[i] = 42
		}
		ps3012Check(t, allEqual)

		organ := make([]int, n)
		for i := range organ {
			organ[i] = min(i, n-1-i) // organ pipe: ascending then descending
		}
		ps3012Check(t, organ)

		fewDistinct := make([]int, n)
		for i := range fewDistinct {
			fewDistinct[i] = i % 3 // duplicate plateaus
		}
		ps3012Check(t, fewDistinct)
	}

	// Int extremes among duplicates.
	ps3012Check(t, []int{math.MaxInt, math.MinInt, 0, -1, 1, math.MaxInt, math.MinInt, 0, 0})

	// Unsigned and narrow integer kinds (named types are the same underlying
	// story; the classifier accepts any integer kind).
	ps3012Check(t, []uint8{255, 0, 128, 1, 255, 0, 127, 128})
	ps3012Check(t, []uint64{math.MaxUint64, 0, 1, math.MaxUint64, 42})
	ps3012Check(t, []int8{-128, 127, 0, -1, -128, 127})

	// Randomized integer draws: many duplicates from a tiny alphabet (tie
	// stress) and full-range values (pattern-free), at random sizes.
	r := rand.New(rand.NewSource(3012))
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(80)
		small := make([]int, n)
		big := make([]int, n)
		for i := 0; i < n; i++ {
			small[i] = r.Intn(4)
			big[i] = int(r.Uint64()) // full-range, sign bit included
		}
		ps3012Check(t, small)
		ps3012Check(t, big)
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
	ps3012Check(t, pool)
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(60)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		ps3012Check(t, s)
	}

	// Random byte strings (arbitrary, frequently invalid UTF-8) with a tiny
	// alphabet forcing long shared prefixes and frequent exact ties.
	for trial := 0; trial < 1000; trial++ {
		n := r.Intn(50)
		s := make([]string, n)
		for i := range s {
			b := make([]byte, r.Intn(8))
			for j := range b {
				b[j] = []byte{0x00, 'a', 0x7f, 0x80, 0xc3, 0xff}[r.Intn(6)]
			}
			s[i] = string(b)
		}
		ps3012Check(t, s)
	}

	// A big shuffled slice: full pdqsort recursion depth.
	bigSlice := make([]int, 4096)
	for i := range bigSlice {
		bigSlice[i] = i % 1000 // duplicates at scale
	}
	r.Shuffle(len(bigSlice), func(i, j int) { bigSlice[i], bigSlice[j] = bigSlice[j], bigSlice[i] })
	ps3012Check(t, bigSlice)
}
