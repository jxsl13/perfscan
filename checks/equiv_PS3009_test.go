package checks

// PS3009's runtime differential: slices.SortFunc(s, strings.Compare) and
// slices.SortStableFunc(s, strings.Compare) — in both the func-value and
// the bare-closure spelling — must produce an output slice IDENTICAL to
// slices.Sort(s) for every input the auto-fix accepts (string elements).
// Two claims are pinned here, against the REAL stdlib so a future stdlib
// change that breaks either fails CI:
//
//  1. ORDER: strings.Compare(a, b) < 0 iff a < b byte-lexicographically —
//     precisely cmp.Less on string, the exact relation slices.Sort is
//     defined by — so both sides sort into the same ascending order.
//     Invalid UTF-8 changes nothing: both sides order by raw bytes, never
//     runes.
//  2. TIES: strings that compare equal are byte-equal, hence
//     interchangeable, so the unstable pdqsort's freedom to arrange ties
//     (and SortStableFunc's guarantee to keep them — the ONLY semantic
//     difference between the stable and unstable entry points) is
//     unobservable and the slices are elementwise identical.
//
// The inputs are adversarial for BOTH algorithms: tie-heavy small pools
// (exercising the stable insertion runs and symMerge tie handling),
// already-sorted / reversed / all-equal / organ-pipe / sawtooth shapes
// (pdqsort's pattern-detection paths), shared-prefix and invalid-UTF-8
// byte strings (the byte-lexicographic order's edge cases: prefix rule,
// 0x00 and 0xFF bytes, high-bit bytes vs ASCII), and large random slices
// (full pdqsort machinery: pivot selection, partitioning, heap fallback).

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS3009SortFuncStringsCompare(t *testing.T) {
	check := func(t *testing.T, base []string) {
		t.Helper()
		want := slices.Clone(base)
		slices.Sort(want)
		got := [][]string{
			slices.Clone(base), slices.Clone(base),
			slices.Clone(base), slices.Clone(base),
		}
		slices.SortFunc(got[0], func(a, b string) int { return strings.Compare(a, b) })
		slices.SortFunc(got[1], strings.Compare)
		slices.SortStableFunc(got[2], func(a, b string) int { return strings.Compare(a, b) })
		slices.SortStableFunc(got[3], strings.Compare)
		for i, g := range got {
			if !slices.Equal(g, want) {
				t.Fatalf("variant %d: Sort(Stable)Func(strings.Compare) != slices.Sort on %q: %q vs %q", i, base, g, want)
			}
		}
	}

	// Deterministic adversarial shapes over a byte-lexicographic edge-case
	// alphabet, at sizes spanning the stable sort's insertion-run
	// threshold and pdqsort's pattern shortcuts.
	pool := []string{
		"", "a", "aa", "aaa", "ab", "b", "ba",
		"\x00", "\x00a", "a\x00", "\xff", "\xff\xfe", // 0x00/0xFF bytes, invalid UTF-8
		"\x80world", "world", "wörld", "wo\xc3", // high-bit vs ASCII, truncated rune
		"prefix", "prefixx", "prefix\x00",
	}
	for _, n := range []int{0, 1, 2, 3, 7, 12, 13, 20, 64, 257} {
		ordered := slices.Clone(pool)
		slices.Sort(ordered)
		sorted := make([]string, n)
		allEqual := make([]string, n)
		organPipe := make([]string, n)
		sawtooth := make([]string, n)
		for i := 0; i < n; i++ {
			sorted[i] = ordered[i*len(ordered)/max(n, 1)] // ascending with tie plateaus
			allEqual[i] = "tie"
			organPipe[i] = ordered[min(i, n-i)%len(ordered)]
			sawtooth[i] = ordered[i%5] // tie-heavy periodic pattern
		}
		reversed := slices.Clone(sorted)
		slices.Reverse(reversed)
		for _, base := range [][]string{sorted, reversed, allEqual, organPipe, sawtooth} {
			check(t, base)
		}
	}

	// Randomized tie-heavy draws from the edge-case pool (small domain ->
	// many byte-equal ties, the stable sort's specialty). Equal strings
	// here are DISTINCT headers built at different times, so a tie swap
	// would still be invisible — exactly the interchangeability claim.
	r := rand.New(rand.NewSource(3009))
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(80)
		base := make([]string, n)
		for i := range base {
			base[i] = pool[r.Intn(len(pool))]
		}
		check(t, base)
	}

	// Random byte strings (arbitrary, frequently invalid UTF-8) with a
	// tiny alphabet forcing long shared prefixes — the byte-lexicographic
	// prefix rule under stress.
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(60)
		base := make([]string, n)
		for i := range base {
			b := make([]byte, r.Intn(12))
			for j := range b {
				b[j] = []byte{0x00, 'a', 0x7f, 0x80, 0xc3, 0xff}[r.Intn(6)]
			}
			base[i] = string(b)
		}
		check(t, base)
	}

	// Big random string slices too (not just tie-heavy): 4096 elements
	// exercises the full pdqsort machinery on one side and the run-merge
	// cascade of the stable sort on the other.
	for trial := 0; trial < 20; trial++ {
		base := make([]string, 4096)
		for i := range base {
			b := make([]byte, 1+r.Intn(16))
			for j := range b {
				b[j] = byte(r.Intn(256))
			}
			base[i] = string(b)
		}
		check(t, base)
	}
}
