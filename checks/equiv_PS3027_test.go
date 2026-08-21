package checks

// PS3027's runtime differential: slices.CompareFunc with a hand-rolled
// three-way comparator returning EXACTLY -1/+1/0 must return an int
// BYTE-IDENTICAL to slices.Compare on every input the auto-fix accepts
// (integer element kinds). Pinned against the REAL stdlib so a future stdlib
// change that breaks the claim fails CI:
//
//  1. VALUE: slices.CompareFunc returns the comparator's first non-zero
//     result VERBATIM, then falls back to the ±1 length tie-breaks. For an
//     integer element the exact -1/+1/0 ladder IS cmp.Compare (no NaN, so
//     cmp.Compare's NaN prelude is dead and its answers are the ladder's),
//     and slices.Compare is the same loop with cmp.Compare fixed per pair —
//     so every returned value agrees exactly, not merely in sign.
//  2. MAGNITUDE GUARD: the sign-only relaxation the sort/extremum siblings
//     (PS3013/PS3022) allow is UNSOUND here — a ladder returning -2/+3 makes
//     CompareFunc yield -2/+3 where slices.Compare yields -1/+1. A pinned
//     divergence subtest proves that guard is load-bearing.
//  3. FLOATS: the ladder answers 0 for a NaN against anything ('<' and '>'
//     both false) while slices.Compare orders NaN first via cmp.Compare — a
//     pinned divergence subtest proves the float carve-out is load-bearing.
//  4. SHAPES: no panic paths, no writes, both slices read left to right — the
//     only observable is the returned int, compared exhaustively.
//
// The inputs are adversarial for a lexicographic early-exit loop: equal
// prefixes with a late divergence, one slice a proper prefix of the other,
// empties and nils on either side, first-element divergence, int extremes on
// which a subtraction comparator would overflow, unsigned and narrow kinds,
// named types, plus randomized draws from tiny alphabets (tie stress) and
// full-range values, in both argument orders.

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3027Ladder is the exact spelling the auto-fix accepts: the hand-rolled
// ascending three-way returning exactly -1/+1/0.
func ps3027Ladder[T stdcmp.Ordered](a, b T) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// ps3027Check pins the matched ladder spelling against slices.Compare on
// (a, b) AND (b, a) — the swapped call is a distinct code path through the
// length tie-breaks.
func ps3027Check[T stdcmp.Ordered](t *testing.T, a, b []T) {
	t.Helper()
	for _, pair := range [2][2][]T{{a, b}, {b, a}} {
		s1, s2 := pair[0], pair[1]
		want := slices.Compare(s1, s2)
		if got := slices.CompareFunc(s1, s2, ps3027Ladder[T]); got != want {
			t.Fatalf("CompareFunc(ladder) != Compare on %v vs %v: %d vs %d", s1, s2, got, want)
		}
		// The fresh-literal spelling the check actually matches at call sites.
		got := slices.CompareFunc(s1, s2, func(x, y T) int {
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
			return 0
		})
		if got != want {
			t.Fatalf("CompareFunc(literal ladder) != Compare on %v vs %v: %d vs %d", s1, s2, got, want)
		}
	}
}

func TestEquiv_PS3027CompareFuncHandRolled(t *testing.T) {
	// Nil/empty on either side: the length tie-breaks alone decide.
	ps3027Check[int](t, nil, nil)
	ps3027Check(t, nil, []int{})
	ps3027Check(t, []int{}, []int{})
	ps3027Check(t, nil, []int{1})
	ps3027Check(t, []int{}, []int{0})

	// Structured integer shapes: first-element divergence, late divergence
	// after an equal prefix, proper prefixes, full equality, at assorted
	// sizes.
	sizes := []int{1, 2, 3, 7, 13, 64, 257, 1024}
	for _, n := range sizes {
		base := make([]int, n)
		for i := range base {
			base[i] = i - n/2
		}
		equal := slices.Clone(base)
		ps3027Check(t, base, equal) // fully equal -> 0 both ways

		first := slices.Clone(base)
		first[0]++ // diverges at index 0
		ps3027Check(t, base, first)

		last := slices.Clone(base)
		last[n-1]-- // equal prefix, diverges at the last pair
		ps3027Check(t, base, last)

		if n > 1 {
			mid := slices.Clone(base)
			mid[n/2] = math.MaxInt // diverges in the middle, extreme value
			ps3027Check(t, base, mid)
		}

		ps3027Check(t, base, base[:n-n/3]) // proper prefix: +1/-1 by length
		ps3027Check(t, base[:0], base)
	}

	// Int extremes among duplicates — inputs on which a subtraction
	// comparator would overflow; the ladder and Compare answer identically.
	ps3027Check(t,
		[]int{math.MaxInt, math.MinInt, 0, -1, 1},
		[]int{math.MaxInt, math.MinInt, 0, -1, 2})
	ps3027Check(t,
		[]int{math.MinInt, math.MaxInt},
		[]int{math.MaxInt, math.MinInt})

	// Unsigned and narrow integer kinds, and a named integer type — the
	// classifier accepts any integer-underlying element.
	ps3027Check(t, []uint8{255, 0, 128}, []uint8{255, 0, 129})
	ps3027Check(t, []uint64{math.MaxUint64, 0}, []uint64{math.MaxUint64, 1})
	ps3027Check(t, []int8{-128, 127, 0}, []int8{-128, 126, 0})
	ps3027Check(t, []uintptr{0, ^uintptr(0)}, []uintptr{0, ^uintptr(0) - 1})
	type rank int
	ps3027Check(t, []rank{3, 1, 2}, []rank{3, 1})

	// Randomized draws: tiny alphabets force long equal prefixes and frequent
	// full ties; independent lengths exercise every length tie-break.
	r := rand.New(rand.NewSource(3027))
	for trial := 0; trial < 2000; trial++ {
		na, nb := r.Intn(12), r.Intn(12)
		a := make([]int, na)
		b := make([]int, nb)
		for i := range a {
			a[i] = r.Intn(3) - 1
		}
		for i := range b {
			b[i] = r.Intn(3) - 1
		}
		ps3027Check(t, a, b)

		// Full-range values, shared random prefix with a late tail.
		n := 1 + r.Intn(40)
		p := make([]int, n)
		for i := range p {
			p[i] = int(r.Uint64())
		}
		q := slices.Clone(p)
		if r.Intn(2) == 0 {
			q[r.Intn(n)] = int(r.Uint64())
		} else {
			q = q[:r.Intn(n+1)]
		}
		ps3027Check(t, p, q)
	}

	// A big pair diverging only at the very last element: full-length scan.
	big := make([]int, 4096)
	for i := range big {
		big[i] = i * 7
	}
	bigTail := slices.Clone(big)
	bigTail[len(bigTail)-1]--
	ps3027Check(t, big, bigTail)
	ps3027Check(t, big, big[:4095])
}

// TestEquiv_PS3027MagnitudeGuardLoadBearing pins WHY the exact ±1 magnitude
// requirement exists: slices.CompareFunc propagates the comparator's first
// non-zero result verbatim, so a -2/+3 ladder — which the sign-only
// PS3013/PS3022 matchers would happily accept — must NOT be rewritten to
// slices.Compare. If the stdlib ever started canonicalizing the propagated
// value this test would fail and the guard could be revisited.
func TestEquiv_PS3027MagnitudeGuardLoadBearing(t *testing.T) {
	loose := func(a, b int) int {
		if a < b {
			return -2
		}
		if a > b {
			return 3
		}
		return 0
	}
	if got, want := slices.CompareFunc([]int{5}, []int{9}, loose), slices.Compare([]int{5}, []int{9}); got == want {
		t.Fatalf("expected the -2/+3 ladder to DIVERGE from slices.Compare (verbatim propagation), got %d == %d — the unit-magnitude guard may be obsolete", got, want)
	} else if got != -2 || want != -1 {
		t.Fatalf("divergence shape changed: CompareFunc=%d (want -2), Compare=%d (want -1)", got, want)
	}
	if got, want := slices.CompareFunc([]int{9}, []int{5}, loose), slices.Compare([]int{9}, []int{5}); got != 3 || want != 1 {
		t.Fatalf("divergence shape changed: CompareFunc=%d (want 3), Compare=%d (want 1)", got, want)
	}
}

// TestEquiv_PS3027FloatCarveOutLoadBearing pins WHY floats stay advisory: the
// ladder answers 0 for a NaN against anything, so CompareFunc scans past NaN
// positions as ties, while slices.Compare orders NaN first via cmp.Compare.
func TestEquiv_PS3027FloatCarveOutLoadBearing(t *testing.T) {
	nan := math.NaN()
	a, b := []float64{nan}, []float64{1}
	got := slices.CompareFunc(a, b, ps3027Ladder[float64])
	want := slices.Compare(a, b)
	if got == want {
		t.Fatalf("expected the float ladder to DIVERGE from slices.Compare on a NaN element, both returned %d — the float carve-out may be obsolete", got)
	}
	if got != 0 || want != -1 {
		t.Fatalf("divergence shape changed: CompareFunc=%d (want 0), Compare=%d (want -1)", got, want)
	}
}
