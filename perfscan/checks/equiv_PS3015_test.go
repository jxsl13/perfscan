package checks

// PS3015's runtime differential: slices.SortedFunc(seq, strings.Compare) —
// in both the func-value and the bare-closure spelling — must return a
// BYTE-IDENTICAL slice to slices.Sorted(seq) for every string input, and
// must consume the seq identically (same yields, same count — the deleted
// comparator is only referenced, never called, so evaluation must not
// change). Pinned against the REAL stdlib so a future stdlib change that
// breaks either claim fails CI:
//
//  1. COLLECTION: both entry points are literally Collect(seq) then sort, so
//     the seq is ranged once, to exhaustion, on both sides. Every seq below
//     is wrapped in a counter asserting the yield sequence is identical.
//  2. PERMUTATION: strings.Compare(a, b) < 0 iff a < b
//     byte-lexicographically — precisely cmp.Less on string, the relation
//     slices.Sort's pdqsort branches on — so every comparison answers the
//     same on both sides and the sorted permutation is identical. Strings
//     that compare equal are byte-equal values, so even the unstable sort's
//     freedom to arrange ties is unobservable: the output is byte-for-byte
//     identical.
//
// The inputs are adversarial for an unstable pdqsort and for byte-wise
// string ordering: sizes crossing the insertion-sort cutoff and recursion
// thresholds, all-equal runs and duplicate plateaus (tie-arrangement
// freedom), already-sorted, reversed and organ-pipe shapes (pdqsort's
// pattern detectors), empties, shared-prefix pairs, 0x00/0xFF bytes,
// high-bit-vs-ASCII order and invalid UTF-8 (both sides order raw bytes,
// never runes). The float divergences of the cmp.Compare family cannot
// arise: strings.Compare pins the element type to string.

import (
	"iter"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// ps3015Seq wraps s in an iter.Seq that records every yielded element, so
// two runs can be compared for identical consumption.
func ps3015Seq(s []string, log *[]string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, v := range s {
			*log = append(*log, v)
			if !yield(v) {
				return
			}
		}
	}
}

func ps3015Check(t *testing.T, s []string) {
	t.Helper()
	var wantLog, gotLog1, gotLog2 []string
	want := slices.Sorted(ps3015Seq(s, &wantLog))
	got1 := slices.SortedFunc(ps3015Seq(s, &gotLog1), func(a, b string) int { return strings.Compare(a, b) })
	got2 := slices.SortedFunc(ps3015Seq(s, &gotLog2), strings.Compare)
	if !slices.Equal(got1, want) || !slices.Equal(got2, want) {
		t.Fatalf("SortedFunc(strings.Compare) != Sorted on %q: %q / %q vs %q", s, got1, got2, want)
	}
	if !slices.Equal(gotLog1, wantLog) || !slices.Equal(gotLog2, wantLog) {
		t.Fatalf("seq consumed differently on %q: %q / %q vs %q", s, gotLog1, gotLog2, wantLog)
	}
	// Byte-identity also means the nil/empty distinction agrees: both sides
	// are the same slices.Collect result.
	if (want == nil) != (got1 == nil) || (want == nil) != (got2 == nil) {
		t.Fatalf("nilness differs on %q: want nil=%v, got %v/%v", s, want == nil, got1 == nil, got2 == nil)
	}
}

func TestEquiv_PS3015SortedFuncStringsCompare(t *testing.T) {
	// Byte-lexicographic edge cases — empty, prefix pairs
	// ("prefix" < "prefixx"), 0x00/0xFF bytes, invalid UTF-8, high-bit vs
	// ASCII — with duplicates.
	pool := []string{
		"", "a", "aa", "aaa", "ab", "b", "ba",
		"\x00", "\x00a", "a\x00", "\xff", "\xff\xfe",
		"\x80world", "world", "wörld", "wo\xc3",
		"prefix", "prefixx", "prefix\x00",
	}
	ps3015Check(t, nil)
	ps3015Check(t, []string{})
	ps3015Check(t, pool)

	// Structured shapes at pdqsort-adversarial sizes: below/at/above the
	// insertion-sort cutoff (12) and well beyond.
	sizes := []int{0, 1, 2, 3, 7, 12, 13, 31, 64, 257, 1024}
	for _, n := range sizes {
		sorted := make([]string, n)
		for i := range sorted {
			sorted[i] = strings.Repeat("k", i%8) + string(rune('a'+i%26))
		}
		slices.Sort(sorted)
		ps3015Check(t, sorted)

		rev := slices.Clone(sorted)
		slices.Reverse(rev)
		ps3015Check(t, rev)

		allEqual := make([]string, n)
		for i := range allEqual {
			allEqual[i] = "same\x00tie"
		}
		ps3015Check(t, allEqual)

		organ := make([]string, n)
		for i := range organ {
			organ[i] = strings.Repeat("x", min(i, n-1-i)%10) // organ pipe by length
		}
		ps3015Check(t, organ)

		fewDistinct := make([]string, n)
		for i := range fewDistinct {
			fewDistinct[i] = pool[i%3] // duplicate plateaus
		}
		ps3015Check(t, fewDistinct)
	}

	// Randomized draws from the adversarial pool: many exact ties.
	r := rand.New(rand.NewSource(3015))
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(80)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		ps3015Check(t, s)
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
		ps3015Check(t, s)
	}

	// A big shuffled slice: full pdqsort recursion depth, duplicates at
	// scale.
	big := make([]string, 4096)
	for i := range big {
		big[i] = "w-" + strings.Repeat("a", i%3) + string(rune('a'+i%26)) + pool[i%len(pool)]
	}
	r.Shuffle(len(big), func(i, j int) { big[i], big[j] = big[j], big[i] })
	ps3015Check(t, big)
}
