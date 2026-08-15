package checks

// PS3014's runtime differential: slices.IsSortedFunc(s, strings.Compare) —
// in both the func-value and the bare-closure spelling — must return the
// IDENTICAL bool as slices.IsSorted(s) for every input the auto-fix accepts
// (string elements). The claim is pinned against the REAL stdlib so a future
// stdlib change that breaks it fails CI:
//
// slices.IsSorted is defined as the descending scan
// 'if cmp.Less(x[i], x[i-1]) return false'; IsSortedFunc is the same loop
// with 'cmp(x[i], x[i-1]) < 0'. With cmp = strings.Compare,
// strings.Compare(a, b) < 0 iff a < b byte-lexicographically — precisely
// cmp.Less on string — so every iteration answers identically and both
// sides return the identical bool at the identical adjacent pair. Invalid
// UTF-8 changes nothing: both order by raw bytes, never runes. The result
// is a pure bool, so no tie-arrangement freedom exists at all.
//
// The inputs are adversarial for the byte-lexicographic order: duplicate
// plateaus (ties must count as sorted), shared prefixes and the prefix rule
// ("prefix" < "prefixx"), 0x00 and 0xFF bytes, high-bit bytes vs ASCII,
// truncated UTF-8 runes, empty strings, nil and single-element slices,
// exactly-sorted slices, slices broken at the first/last/only adjacent pair,
// and fully random (usually unsorted) draws.

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestEquiv_PS3014IsSortedFuncStringsCompare(t *testing.T) {
	check := func(t *testing.T, s []string) {
		t.Helper()
		want := slices.IsSorted(s)
		got1 := slices.IsSortedFunc(s, func(a, b string) int { return strings.Compare(a, b) })
		got2 := slices.IsSortedFunc(s, strings.Compare)
		if got1 != want || got2 != want {
			t.Fatalf("IsSortedFunc(strings.Compare) = %v/%v, IsSorted = %v on %q", got1, got2, want, s)
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

	// Deterministic edges: nil, empty, singletons, every ordered pair from
	// the pool (sorted or not — both sides must agree either way).
	check(t, nil)
	check(t, []string{})
	for _, x := range pool {
		check(t, []string{x})
		for _, y := range pool {
			check(t, []string{x, y})
			check(t, []string{x, y, x})
		}
	}

	// Exactly-sorted multisets with duplicate plateaus at boundary sizes,
	// then the same slice broken at the first, middle and last adjacent
	// pair (early return points on both sides must coincide).
	ordered := slices.Clone(pool)
	slices.Sort(ordered)
	for _, n := range []int{2, 3, 7, 16, 64, 257} {
		s := make([]string, n)
		for i := 0; i < n; i++ {
			s[i] = ordered[i*len(ordered)/n] // sorted with tie plateaus
		}
		check(t, s)
		for _, at := range []int{0, (n - 2) / 2, n - 2} {
			broken := slices.Clone(s)
			broken[at], broken[at+1] = broken[at+1], broken[at]
			check(t, broken)
		}
	}

	// Randomized draws from the pool: sorted (many duplicates), sorted-then-
	// perturbed, and raw unsorted.
	r := rand.New(rand.NewSource(3014))
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(50)
		s := make([]string, n)
		for i := range s {
			s[i] = pool[r.Intn(len(pool))]
		}
		check(t, s) // usually unsorted
		slices.Sort(s)
		check(t, s) // sorted with plateaus
		if n > 1 {
			i, j := r.Intn(n), r.Intn(n)
			s[i], s[j] = s[j], s[i]
			check(t, s) // possibly broken at a random spot
		}
	}

	// Random byte strings (arbitrary, frequently invalid UTF-8) with a tiny
	// alphabet forcing long shared prefixes.
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(40)
		s := make([]string, n)
		for i := range s {
			b := make([]byte, r.Intn(10))
			for j := range b {
				b[j] = []byte{0x00, 'a', 0x7f, 0x80, 0xc3, 0xff}[r.Intn(6)]
			}
			s[i] = string(b)
		}
		check(t, s)
		slices.Sort(s)
		check(t, s)
	}

	// A big sorted slice of fully random byte strings: the full O(n) pass,
	// then one perturbation.
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
		check(t, s)
		i := r.Intn(len(s) - 1)
		s[i], s[i+1] = s[i+1], s[i]
		check(t, s)
	}
}
