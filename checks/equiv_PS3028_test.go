package checks

// PS3028's runtime differential: slices.BinarySearchFunc with a hand-rolled
// three-way comparator (a<b -> negative, a>b -> positive, else 0) must return
// an (index, found) pair BYTE-IDENTICAL to slices.BinarySearch on every input
// the auto-fix accepts (integer and string element kinds). Pinned against the
// REAL stdlib so a future stdlib change that breaks the claim fails CI:
//
//  1. SIGN-ONLY: BinarySearchFunc consumes the comparator result only as
//     cmp(x[h], target) < 0 (each probe) and cmp(x[i], target) == 0 (the
//     found check), while slices.BinarySearch branches on cmp.Less(x[h],
//     target) and reports found via x[i] == target. For an integer or string
//     element the chain's sign equals cmp.Compare on every pair, so every
//     probe branches identically and the pair is identical — even on
//     UNSORTED input, where the answer is garbage but still deterministic.
//  2. MAGNITUDE: unlike slices.CompareFunc's verbatim value propagation
//     (PS3027), only the sign is consumed here, so the family's sign-only
//     relaxation (PS3013/PS3022) IS sound: a -7/+42 chain must agree too,
//     and a subtest proves it.
//  3. FLOATS: the chain answers 0 for a NaN against anything ('<' and '>'
//     both false), so BinarySearchFunc reports found=true at a NaN probe
//     while slices.BinarySearch orders NaN first — pinned divergence
//     subtests prove the float carve-out is load-bearing in BOTH directions
//     (NaN element, NaN target).
//  4. SHAPES: no panic paths, no writes, the slice and target are evaluated
//     once on both sides — the only observable is the returned pair,
//     compared exhaustively.
//
// The inputs are adversarial for a binary search: empties and nils, single
// elements, targets below/above/at every boundary, long duplicate runs (the
// leftmost-index invariant), int extremes on which a subtraction comparator
// would overflow, unsigned and narrow kinds, named types, strings with shared
// prefixes, the empty string and invalid UTF-8, unsorted garbage, and
// randomized sorted draws with hit-and-miss targets.

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3028Ladder is the shape the auto-fix accepts: the hand-rolled ascending
// three-way over the two bare parameters (unit magnitudes).
func ps3028Ladder[T stdcmp.Ordered](a, b T) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// ps3028Check pins the matched chain spellings against slices.BinarySearch
// for one (slice, target) pair — the named generic chain, the fresh-literal
// spelling the check actually matches at call sites, and a non-unit-magnitude
// chain (sign-only consumption makes magnitudes irrelevant here).
func ps3028Check[T stdcmp.Ordered](t *testing.T, x []T, target T) {
	t.Helper()
	wantI, wantOK := slices.BinarySearch(x, target)
	if i, ok := slices.BinarySearchFunc(x, target, ps3028Ladder[T]); i != wantI || ok != wantOK {
		t.Fatalf("BinarySearchFunc(ladder) != BinarySearch on %v / %v: (%d,%v) vs (%d,%v)", x, target, i, ok, wantI, wantOK)
	}
	i, ok := slices.BinarySearchFunc(x, target, func(a, b T) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	if i != wantI || ok != wantOK {
		t.Fatalf("BinarySearchFunc(literal ladder) != BinarySearch on %v / %v: (%d,%v) vs (%d,%v)", x, target, i, ok, wantI, wantOK)
	}
	// Swapped arm order, swapped operand spellings, non-unit magnitudes —
	// everything the sign-only matcher accepts.
	i, ok = slices.BinarySearchFunc(x, target, func(a, b T) int {
		if b < a {
			return 42
		}
		if b > a {
			return -7
		}
		return 0
	})
	if i != wantI || ok != wantOK {
		t.Fatalf("BinarySearchFunc(-7/+42 ladder) != BinarySearch on %v / %v: (%d,%v) vs (%d,%v)", x, target, i, ok, wantI, wantOK)
	}
}

func TestEquiv_PS3028BinarySearchFuncHandRolled(t *testing.T) {
	// Empties and nils: the loop never probes, found is decided at i == n.
	ps3028Check[int](t, nil, 0)
	ps3028Check(t, []int{}, math.MinInt)
	ps3028Check(t, []int{7}, 6)
	ps3028Check(t, []int{7}, 7)
	ps3028Check(t, []int{7}, 8)

	// Sorted distinct values at assorted sizes (odd, even, powers of two ±1):
	// every element as a hit target, plus a miss on either side of every
	// boundary.
	for _, n := range []int{1, 2, 3, 7, 8, 13, 64, 255, 256, 257, 1024} {
		x := make([]int, n)
		for i := range x {
			x[i] = i*3 - n // sorted, distinct, negative and positive
		}
		for _, v := range x {
			ps3028Check(t, x, v)
			ps3028Check(t, x, v-1)
			ps3028Check(t, x, v+1)
		}
		ps3028Check(t, x, math.MinInt)
		ps3028Check(t, x, math.MaxInt)
	}

	// Long duplicate runs: BinarySearch returns the LEFTMOST position where
	// target could appear — both sides must agree on it.
	dup := []int{1, 1, 1, 2, 2, 2, 2, 5, 5, 9, 9, 9, 9, 9}
	for target := 0; target <= 10; target++ {
		ps3028Check(t, dup, target)
	}
	all := make([]int, 100)
	for i := range all {
		all[i] = 4
	}
	ps3028Check(t, all, 3)
	ps3028Check(t, all, 4)
	ps3028Check(t, all, 5)

	// Int extremes among the elements and as targets — inputs on which a
	// subtraction comparator would overflow; the chain and BinarySearch
	// answer identically.
	ext := []int{math.MinInt, math.MinInt + 1, -1, 0, 1, math.MaxInt - 1, math.MaxInt}
	for _, v := range ext {
		ps3028Check(t, ext, v)
	}

	// Unsigned and narrow integer kinds, and a named integer type — the
	// classifier accepts any integer-underlying element.
	ps3028Check(t, []uint8{0, 1, 128, 254, 255}, uint8(255))
	ps3028Check(t, []uint8{0, 1, 128, 254, 255}, uint8(2))
	ps3028Check(t, []uint64{0, 1, math.MaxUint64 - 1, math.MaxUint64}, uint64(math.MaxUint64))
	ps3028Check(t, []int8{-128, -1, 0, 127}, int8(-128))
	ps3028Check(t, []uintptr{0, 1, ^uintptr(0)}, ^uintptr(0)-1)
	type rank int
	ps3028Check(t, []rank{1, 2, 3, 5, 8}, rank(4))
	ps3028Check(t, []rank{1, 2, 3, 5, 8}, rank(5))

	// Strings: byte-lexicographic on both sides — shared prefixes, the empty
	// string, a proper-prefix target, and invalid UTF-8 (raw bytes, never
	// runes).
	ss := []string{"", "a", "ab", "abc", "ab\xff", "b", "ba"}
	slices.Sort(ss)
	for _, target := range append(slices.Clone(ss), "aa", "ab\xfe", "\xff", "zzz") {
		ps3028Check(t, ss, target)
	}

	// UNSORTED garbage: the answer is meaningless but both sides make the
	// identical probe sequence with identical branch answers, so the pair
	// must still be identical.
	garbage := []int{9, -3, 7, 7, 0, math.MaxInt, math.MinInt, 4}
	for _, target := range []int{-3, 0, 4, 5, 7, 9, math.MinInt, math.MaxInt} {
		ps3028Check(t, garbage, target)
	}

	// Randomized sorted draws: tiny alphabets force long duplicate runs and
	// frequent hits; full-range values exercise deep misses.
	r := rand.New(rand.NewSource(3028))
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(50)
		x := make([]int, n)
		for i := range x {
			x[i] = r.Intn(7)
		}
		slices.Sort(x)
		ps3028Check(t, x, r.Intn(9)-1)

		y := make([]int, 1+r.Intn(64))
		for i := range y {
			y[i] = int(r.Uint64())
		}
		slices.Sort(y)
		if r.Intn(2) == 0 {
			ps3028Check(t, y, y[r.Intn(len(y))]) // guaranteed hit
		} else {
			ps3028Check(t, y, int(r.Uint64())) // near-certain miss
		}
	}
}

// TestEquiv_PS3028FloatCarveOutLoadBearing pins WHY floats stay advisory: the
// chain answers 0 for a NaN against anything, so BinarySearchFunc's probe
// treats a NaN element as equal to the target and its found check reports
// true there, while slices.BinarySearch orders NaN first via cmp.Less and
// reports found only for a real match (or NaN searched among NaNs). Both
// directions — a NaN element and a NaN target — are pinned.
func TestEquiv_PS3028FloatCarveOutLoadBearing(t *testing.T) {
	nan := math.NaN()

	// NaN element, real target: the chain answers 0 at the NaN probe and
	// "finds" 1 there, while cmp.Less orders the NaN BEFORE the target and
	// walks past it — the index AND the found bit both diverge.
	gotI, gotOK := slices.BinarySearchFunc([]float64{nan}, 1.0, ps3028Ladder[float64])
	wantI, wantOK := slices.BinarySearch([]float64{nan}, 1.0)
	if gotI == wantI && gotOK == wantOK {
		t.Fatalf("expected the float ladder to DIVERGE from slices.BinarySearch on a NaN element, both returned (%d,%v) — the float carve-out may be obsolete", gotI, gotOK)
	}
	if gotI != 0 || !gotOK || wantI != 1 || wantOK {
		t.Fatalf("divergence shape changed: BinarySearchFunc=(%d,%v) (want (0,true)), BinarySearch=(%d,%v) (want (1,false))", gotI, gotOK, wantI, wantOK)
	}

	// Real elements, NaN target: the chain "finds" NaN at index 0.
	gotI, gotOK = slices.BinarySearchFunc([]float64{1, 2}, nan, ps3028Ladder[float64])
	wantI, wantOK = slices.BinarySearch([]float64{1, 2}, nan)
	if gotI == wantI && gotOK == wantOK {
		t.Fatalf("expected the float ladder to DIVERGE from slices.BinarySearch on a NaN target, both returned (%d,%v) — the float carve-out may be obsolete", gotI, gotOK)
	}
	if gotI != 0 || !gotOK || wantI != 0 || wantOK {
		t.Fatalf("divergence shape changed: BinarySearchFunc=(%d,%v) (want (0,true)), BinarySearch=(%d,%v) (want (0,false))", gotI, gotOK, wantI, wantOK)
	}
}
