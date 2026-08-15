package checks

// PS3011's runtime differential: slices.BinarySearchFunc(s, target,
// strings.Compare) — in both the func-value and the bare-closure spelling —
// must return the IDENTICAL (index, found) pair as slices.BinarySearch(s,
// target) for every input the auto-fix accepts (string elements). Two claims
// are pinned here, against the REAL stdlib so a future stdlib change that
// breaks either fails CI:
//
//  1. ORDER: strings.Compare(a, b) < 0 iff a < b byte-lexicographically —
//     precisely cmp.Less on string, the relation slices.BinarySearch branches
//     on at every probe — so both sides make the identical probe sequence.
//     Invalid UTF-8 changes nothing: both order by raw bytes, never runes.
//  2. FOUND: strings.Compare(a, b) == 0 iff the strings are byte-equal —
//     precisely the == slices.BinarySearch's found-check uses — so the final
//     bool agrees too. A binary search is a deterministic function of the
//     comparator alone (no tie-arrangement freedom like the unstable sorts),
//     so the pair is bit-identical EVEN ON UNSORTED input, where the answer
//     is meaningless but still must match.
//
// The inputs are adversarial for the byte-lexicographic order: duplicate
// plateaus (leftmost-insertion semantics), shared prefixes and the prefix
// rule ("prefix" < "prefixx"), 0x00 and 0xFF bytes, high-bit bytes vs ASCII,
// truncated UTF-8 runes, empty strings, nil and single-element slices, and —
// per claim 2 — deliberately UNSORTED slices. Targets are drawn both from the
// slice (present) and from perturbations (absent, below-first, above-last).

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS3011BinarySearchFuncStringsCompare(t *testing.T) {
	check := func(t *testing.T, s []string, target string) {
		t.Helper()
		wantI, wantOK := slices.BinarySearch(s, target)
		gotI1, gotOK1 := slices.BinarySearchFunc(s, target, func(a, b string) int { return strings.Compare(a, b) })
		gotI2, gotOK2 := slices.BinarySearchFunc(s, target, strings.Compare)
		if gotI1 != wantI || gotOK1 != wantOK || gotI2 != wantI || gotOK2 != wantOK {
			t.Fatalf("BinarySearchFunc(strings.Compare) != BinarySearch on %q target %q: (%d,%v)/(%d,%v) vs (%d,%v)",
				s, target, gotI1, gotOK1, gotI2, gotOK2, wantI, wantOK)
		}
	}

	// A byte-lexicographic edge-case alphabet: empty string, prefix pairs,
	// 0x00/0xFF bytes, invalid UTF-8, high-bit vs ASCII, truncated runes.
	pool := []string{
		"", "a", "aa", "aaa", "ab", "b", "ba",
		"\x00", "\x00a", "a\x00", "\xff", "\xff\xfe",
		"\x80world", "world", "wörld", "wo\xc3",
		"prefix", "prefixx", "prefix\x00",
	}

	// Every target against every SORTED multiset drawn from the pool at
	// boundary sizes, with duplicate plateaus (i*len/n repeats elements).
	// Targets include every pool element (present or absent) plus
	// perturbations probing the prefix rule and the extremes.
	targets := slices.Clone(pool)
	for _, p := range pool {
		targets = append(targets, p+"\x00", p+"\xff", "\x00"+p)
	}
	ordered := slices.Clone(pool)
	slices.Sort(ordered)
	for _, n := range []int{0, 1, 2, 3, 7, 16, 64, 257} {
		s := make([]string, n)
		for i := 0; i < n; i++ {
			s[i] = ordered[i*len(ordered)/max(n, 1)] // sorted with tie plateaus
		}
		for _, target := range targets {
			check(t, s, target)
		}
	}
	// Nil slice (not just empty).
	check(t, nil, "x")
	check(t, nil, "")

	// Randomized sorted draws from the pool: many duplicates, leftmost-index
	// semantics under stress.
	r := rand.New(rand.NewSource(3011))
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(50)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		slices.Sort(s)
		check(t, s, pool[r.Intn(len(pool))])
		check(t, s, targets[r.Intn(len(targets))])
	}

	// Random byte strings (arbitrary, frequently invalid UTF-8) with a tiny
	// alphabet forcing long shared prefixes; the target is drawn from the
	// same distribution, or from the slice itself (guaranteed present).
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(40)
		s := make([]string, n)
		mk := func() string {
			b := make([]byte, r.Intn(10))
			for j := range b {
				b[j] = []byte{0x00, 'a', 0x7f, 0x80, 0xc3, 0xff}[r.Intn(6)]
			}
			return string(b)
		}
		for i := range s {
			s[i] = mk()
		}
		slices.Sort(s)
		check(t, s, mk())
		if n > 0 {
			check(t, s, s[r.Intn(n)])
		}
	}

	// UNSORTED slices: the contract answer is meaningless, but both entry
	// points run the identical probe loop on the identical comparison signs,
	// so the pair must STILL be bit-identical — the strongest form of the
	// rewrite's safety claim.
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(30)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		check(t, s, pool[r.Intn(len(pool))])
	}

	// A big sorted slice of random byte strings: full log n probe depth.
	for trial := 0; trial < 10; trial++ {
		s := make([]string, 4096)
		for i := range s {
			b := make([]byte, 1+r.Intn(12))
			for j := range b {
				b[j] = byte(r.Intn(256))
			}
			s[i] = string(b)
		}
		slices.Sort(s)
		for k := 0; k < 64; k++ {
			check(t, s, s[r.Intn(len(s))])
			b := make([]byte, r.Intn(12))
			for j := range b {
				b[j] = byte(r.Intn(256))
			}
			check(t, s, string(b))
		}
	}
}
