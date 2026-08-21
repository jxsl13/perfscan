package checks

// PS3023's runtime differential: slices.CompareFunc with a bare cmp.Compare
// comparator — cmp.Compare passed directly AND the closure spelling the check
// matches — must return an int BYTE-IDENTICAL to slices.Compare on every
// input the auto-fix accepts. Pinned against the REAL stdlib so a future
// stdlib change that breaks the claim fails CI:
//
//  1. VALUE: slices.Compare's body IS CompareFunc's loop with the comparator
//     fixed to cmp.Compare(s1[i], s2[i]) in source order — same early-exit
//     index, same propagated cmp.Compare int, same ±1 length tie-breaks — so
//     every returned value agrees exactly, not merely in sign.
//  2. FLOATS: unlike the Max/Min family (built on the NaN-propagating builtin
//     max/min, which forced PS3111's float exclusion), both sides here run
//     cmp.Compare per pair, so NaN ordering (NaN first), NaN-vs-NaN ties and
//     -0.0 == +0.0 agree by construction; the fix therefore accepts floats
//     and this test hammers NaN/-0.0/Inf placements to pin that claim.
//  3. SHAPES: no panic paths, no writes, both slices read left to right — the
//     only observable is the returned int, compared exhaustively.
//
// The inputs are adversarial for a lexicographic early-exit loop: equal
// prefixes with a late divergence, one slice a proper prefix of the other,
// empties and nils on either side, first-element divergence, int extremes,
// unsigned and narrow kinds, named types, strings with shared prefixes,
// 0x00/0xFF bytes and invalid UTF-8, floats with NaN/-0.0/±Inf at every
// position, plus randomized draws from tiny alphabets (tie stress) and
// full-range values, in both argument orders.

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	stdcmp "cmp"
)

// ps3023Check pins both matched comparator spellings against slices.Compare
// on (a, b) AND (b, a) — the swapped call is a distinct code path through the
// length tie-breaks.
func ps3023Check[T stdcmp.Ordered](t *testing.T, a, b []T) {
	t.Helper()
	for _, pair := range [2][2][]T{{a, b}, {b, a}} {
		s1, s2 := pair[0], pair[1]
		want := slices.Compare(s1, s2)
		if got := slices.CompareFunc(s1, s2, stdcmp.Compare); got != want {
			t.Fatalf("CompareFunc(cmp.Compare) != Compare on %v vs %v: %d vs %d", s1, s2, got, want)
		}
		if got := slices.CompareFunc(s1, s2, func(x, y T) int { return stdcmp.Compare(x, y) }); got != want {
			t.Fatalf("CompareFunc(closure) != Compare on %v vs %v: %d vs %d", s1, s2, got, want)
		}
	}
}

func TestEquiv_PS3023CompareFuncCmpCompare(t *testing.T) {
	// Nil/empty on either side: the length tie-breaks alone decide.
	ps3023Check[int](t, nil, nil)
	ps3023Check(t, nil, []int{})
	ps3023Check(t, []int{}, []int{})
	ps3023Check(t, nil, []int{1})
	ps3023Check(t, []int{}, []int{0})

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
		ps3023Check(t, base, equal) // fully equal -> 0 both ways

		first := slices.Clone(base)
		first[0]++ // diverges at index 0
		ps3023Check(t, base, first)

		last := slices.Clone(base)
		last[n-1]-- // equal prefix, diverges at the last pair
		ps3023Check(t, base, last)

		if n > 1 {
			mid := slices.Clone(base)
			mid[n/2] = math.MaxInt // diverges in the middle, extreme value
			ps3023Check(t, base, mid)
		}

		ps3023Check(t, base, base[:n-n/3]) // proper prefix: +1/-1 by length
		ps3023Check(t, base[:0], base)
	}

	// Int extremes among duplicates — inputs on which a subtraction
	// comparator would overflow; cmp.Compare and Compare answer identically.
	ps3023Check(t,
		[]int{math.MaxInt, math.MinInt, 0, -1, 1},
		[]int{math.MaxInt, math.MinInt, 0, -1, 2})
	ps3023Check(t,
		[]int{math.MinInt, math.MaxInt},
		[]int{math.MaxInt, math.MinInt})

	// Unsigned and narrow integer kinds (named types are the same underlying
	// story; the classifier accepts any ordered element).
	ps3023Check(t, []uint8{255, 0, 128}, []uint8{255, 0, 129})
	ps3023Check(t, []uint64{math.MaxUint64, 0}, []uint64{math.MaxUint64, 1})
	ps3023Check(t, []int8{-128, 127, 0}, []int8{-128, 126, 0})
	type rank int
	ps3023Check(t, []rank{3, 1, 2}, []rank{3, 1})

	// FLOATS — the fix accepts them, so pin the claim hard: NaN at every
	// position on either side, NaN-vs-NaN ties, -0.0/+0.0 ties (equal, so
	// comparison must continue past them), ±Inf, and denormals.
	nan := math.NaN()
	negZero := math.Copysign(0, -1)
	floatPool := [][]float64{
		nil,
		{},
		{nan},
		{nan, nan},
		{nan, 1},
		{1, nan},
		{negZero},
		{0},
		{negZero, 0},
		{0, negZero},
		{negZero, 1},
		{0, 1},
		{nan, negZero, 0},
		{math.Inf(1)},
		{math.Inf(-1)},
		{math.Inf(1), math.Inf(-1), nan, 0, negZero, 1e300, -1e300, 5e-324},
		{1, 2, 3},
		{1, 2, nan},
		{1, 2, 4},
	}
	for _, a := range floatPool {
		for _, b := range floatPool {
			ps3023Check(t, a, b)
		}
	}
	ps3023Check(t,
		[]float32{float32(math.NaN()), 0, float32(negZero)},
		[]float32{float32(math.NaN()), float32(negZero), 0})

	// Strings: byte-lexicographic edge cases — empty, prefix pairs, 0x00/0xFF
	// bytes, invalid UTF-8, high-bit vs ASCII.
	strPool := []string{
		"", "a", "aa", "ab", "b",
		"\x00", "\x00a", "a\x00", "\xff", "\xff\xfe",
		"\x80world", "world", "wörld", "wo\xc3",
		"prefix", "prefixx", "prefix\x00",
	}
	for i := range strPool {
		for j := range strPool {
			ps3023Check(t, []string{strPool[i]}, []string{strPool[j]})
			ps3023Check(t,
				[]string{strPool[i], strPool[j]},
				[]string{strPool[j], strPool[i]})
		}
	}

	// Randomized draws: tiny alphabets force long equal prefixes and frequent
	// full ties; independent lengths exercise every length tie-break.
	r := rand.New(rand.NewSource(3023))
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
		ps3023Check(t, a, b)

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
		ps3023Check(t, p, q)
	}
	for trial := 0; trial < 1000; trial++ {
		na, nb := r.Intn(8), r.Intn(8)
		a := make([]string, na)
		b := make([]string, nb)
		for i := range a {
			a[i] = strPool[r.Intn(len(strPool))]
		}
		for i := range b {
			b[i] = strPool[r.Intn(len(strPool))]
		}
		ps3023Check(t, a, b)
	}
	for trial := 0; trial < 1000; trial++ {
		vals := []float64{nan, negZero, 0, 1, -1, math.Inf(1), math.Inf(-1)}
		na, nb := r.Intn(6), r.Intn(6)
		a := make([]float64, na)
		b := make([]float64, nb)
		for i := range a {
			a[i] = vals[r.Intn(len(vals))]
		}
		for i := range b {
			b[i] = vals[r.Intn(len(vals))]
		}
		ps3023Check(t, a, b)
	}

	// A big pair diverging only at the very last element: full-length scan.
	big := make([]int, 4096)
	for i := range big {
		big[i] = i * 7
	}
	bigTail := slices.Clone(big)
	bigTail[len(bigTail)-1]--
	ps3023Check(t, big, bigTail)
	ps3023Check(t, big, big[:4095])
}
