package checks

// PS3010's runtime differential: slices.IsSortedFunc(s, cmp.Compare) — in both
// the func-value and the bare-closure spelling — must return a bool IDENTICAL
// to slices.IsSorted(s) for EVERY input of EVERY ordered element type,
// floats included. The claim pinned here, against the REAL stdlib so a future
// stdlib change that breaks it fails CI:
//
//	cmp.Compare(a, b) < 0  iff  cmp.Less(a, b)   for ALL a, b of any cmp.Ordered type.
//
// slices.IsSorted is the descending scan `if cmp.Less(x[i], x[i-1]) return
// false`; IsSortedFunc is the same loop with `cmp(x[i], x[i-1]) < 0`. With the
// equivalence above every iteration answers identically, so the loops return
// the identical bool at the identical index — and because the output is a pure
// bool there is NO tie arrangement that could expose -0.0/+0.0 bit patterns or
// NaN payloads (the concern that keeps the sort-family float rewrites
// advisory). The float edge table, case by case (Less = (isNaN(a) && !isNaN(b)) || a < b;
// Compare returns -1 exactly when isNaN(a)&&!isNaN(b), or neither NaN and a < b):
//
//	NaN vs non-NaN -> both true; non-NaN vs NaN -> both false;
//	NaN vs NaN     -> both false; -0.0 vs +0.0  -> both false (they compare equal).
//
// The float differential is EXHAUSTIVE: every sequence of length 0..5 over an
// adversarial pool {NaN, -Inf, -1.5, -0.0, +0.0, 1.5, +Inf} — 19k+ slices
// covering every ordering, every NaN position, mixed ±0.0 plateaus and the
// early-return point at every index. Integers, strings (invalid UTF-8
// included), named types and a generic instantiation are covered by
// deterministic shapes plus randomized draws.

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3010Check asserts the three spellings agree on one input slice.
func ps3010Check[E stdcmp.Ordered](t *testing.T, base []E) {
	t.Helper()
	want := slices.IsSorted(base)
	if got := slices.IsSortedFunc(base, stdcmp.Compare[E]); got != want {
		t.Fatalf("IsSortedFunc(cmp.Compare) = %v, IsSorted = %v on %v", got, want, base)
	}
	if got := slices.IsSortedFunc(base, func(a, b E) int { return stdcmp.Compare(a, b) }); got != want {
		t.Fatalf("IsSortedFunc(literal cmp.Compare) = %v, IsSorted = %v on %v", got, want, base)
	}
}

func TestEquiv_PS3010IsSortedFuncCmpCompare(t *testing.T) {
	// FLOATS, exhaustively: every sequence of length 0..5 over the edge pool.
	// NaN (quiet), infinities, signed zeros and an ordinary value in both
	// signs. 7^5 = 16807 five-element slices alone; every possible position
	// of the first out-of-order pair — and thus every early-return point —
	// occurs, as do all-NaN, NaN-first (sorted: cmp orders NaN first),
	// NaN-last (unsorted on both sides) and ±0.0-only plateaus.
	pool := []float64{math.NaN(), math.Inf(-1), -1.5, math.Copysign(0, -1), 0, 1.5, math.Inf(1)}
	var enumerate func(prefix []float64, n int)
	enumerate = func(prefix []float64, n int) {
		ps3010Check(t, prefix)
		if n == 0 {
			return
		}
		for _, v := range pool {
			enumerate(append(prefix, v), n-1)
		}
	}
	enumerate(nil, 5)

	// Larger float slices with NaNs scattered by a deterministic RNG: the
	// exhaustive proof above already covers the semantics; this adds length.
	r := rand.New(rand.NewSource(3010))
	for trial := 0; trial < 2000; trial++ {
		n := r.Intn(64)
		base := make([]float64, n)
		for i := range base {
			base[i] = pool[r.Intn(len(pool))]
		}
		if r.Intn(2) == 0 {
			slices.SortFunc(base, stdcmp.Compare) // sorted (NaNs first) half the time
		}
		ps3010Check(t, base)
	}

	// INTEGERS: deterministic shapes (sorted, reversed, all-equal, organ
	// pipe, sawtooth, single out-of-order swap at each index) plus random.
	for _, n := range []int{0, 1, 2, 3, 7, 64, 257} {
		sorted := make([]int, n)
		allEq := make([]int, n)
		organ := make([]int, n)
		saw := make([]int, n)
		for i := 0; i < n; i++ {
			sorted[i] = i / 3 // ascending with tie plateaus
			allEq[i] = 42
			organ[i] = min(i, n-i)
			saw[i] = i % 5
		}
		reversed := slices.Clone(sorted)
		slices.Reverse(reversed)
		for _, base := range [][]int{sorted, reversed, allEq, organ, saw} {
			ps3010Check(t, base)
		}
		// One swap at every adjacent position: pins the early-return index.
		for i := 1; i < n; i++ {
			swapped := slices.Clone(sorted)
			swapped[i-1], swapped[i] = swapped[i]+1, swapped[i-1]
			ps3010Check(t, swapped)
		}
	}
	for trial := 0; trial < 2000; trial++ {
		base := make([]int, r.Intn(80))
		for i := range base {
			base[i] = r.Intn(10) // tie-heavy
		}
		ps3010Check(t, base)
	}

	// STRINGS: byte-lexicographic edge cases — empty, shared prefixes, 0x00
	// and 0xFF bytes, invalid UTF-8 — sorted and shuffled.
	spool := []string{
		"", "a", "aa", "ab", "b",
		"\x00", "a\x00", "\xff", "\xff\xfe",
		"\x80world", "world", "wörld", "wo\xc3",
		"prefix", "prefixx", "prefix\x00",
	}
	ps3010Check(t, spool)
	sortedS := slices.Clone(spool)
	slices.Sort(sortedS)
	ps3010Check(t, sortedS)
	for trial := 0; trial < 2000; trial++ {
		base := make([]string, r.Intn(40))
		for i := range base {
			base[i] = spool[r.Intn(len(spool))]
		}
		ps3010Check(t, base)
	}

	// NAMED float type and a generic call path: the same equivalence must
	// hold through a defined type (ordered operators only, methods never
	// consulted) and through a type-parameter instantiation.
	type Temp float64
	for trial := 0; trial < 500; trial++ {
		base := make([]Temp, r.Intn(16))
		for i := range base {
			base[i] = Temp(pool[r.Intn(len(pool))])
		}
		ps3010Check(t, base)
	}
}
