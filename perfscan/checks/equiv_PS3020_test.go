package checks

import (
	"cmp"
	"math"
	"math/rand"
	"slices"
	"testing"
)

// TestEquiv_PS3020 pins the bit-identity of the PS3020 rewrite for integer
// elements: slices.MaxFunc under the SWAPPED cmp.Compare(b, a) comparator
// selects the maximum of the REVERSED order — the natural minimum — exactly as
// slices.Min does, and symmetrically MinFunc-swapped matches slices.Max. The
// scan updates only on a strictly smaller (resp. larger) element, so both
// sides keep the earlier of any tie; for integers equal values are
// bitwise-identical anyway, so value equality IS byte identity. Verified over
// adversarial fixed inputs (single element, all-equal, sorted both ways,
// extreme values including math.MinInt64/MaxInt64, ties in every position) and
// randomized slices, plus the empty-slice panic on both sides.
func TestEquiv_PS3020(t *testing.T) {
	beforeMin := func(xs []int) int {
		return slices.MaxFunc(xs, func(a, b int) int { return cmp.Compare(b, a) })
	}
	afterMin := func(xs []int) int {
		return slices.Min(xs)
	}
	beforeMax := func(xs []int) int {
		return slices.MinFunc(xs, func(a, b int) int { return cmp.Compare(b, a) })
	}
	afterMax := func(xs []int) int {
		return slices.Max(xs)
	}

	check := func(xs []int) {
		t.Helper()
		if b, a := beforeMin(xs), afterMin(xs); b != a {
			t.Fatalf("min divergence for %v: MaxFunc(swapped)=%d slices.Min=%d", xs, b, a)
		}
		if b, a := beforeMax(xs), afterMax(xs); b != a {
			t.Fatalf("max divergence for %v: MinFunc(swapped)=%d slices.Max=%d", xs, b, a)
		}
	}

	fixed := [][]int{
		{0},
		{-1},
		{math.MinInt64},
		{math.MaxInt64},
		{math.MinInt64, math.MaxInt64},
		{math.MaxInt64, math.MinInt64},
		{7, 7, 7, 7},
		{0, 0, -1, 0, 0},
		{1, 2, 3, 4, 5},
		{5, 4, 3, 2, 1},
		{3, -3, 3, -3},
		{math.MinInt64, 0, math.MinInt64},
		{math.MaxInt64, 0, math.MaxInt64},
		{-0, 0}, // integer -0 is 0; ties everywhere
	}
	for _, xs := range fixed {
		check(xs)
	}

	// Randomized: small alphabets force ties in every position.
	r := rand.New(rand.NewSource(3020))
	for i := 0; i < 20000; i++ {
		n := 1 + r.Intn(12)
		xs := make([]int, n)
		span := []int{2, 3, 1 << 30}[r.Intn(3)]
		for j := range xs {
			xs[j] = r.Intn(span) - span/2
		}
		check(xs)
	}

	// Both sides panic on an empty slice (each indexes s[0] up front).
	mustPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s did not panic on an empty slice", name)
			}
		}()
		f()
	}
	mustPanic("slices.MaxFunc(swapped)", func() { beforeMin(nil) })
	mustPanic("slices.Min", func() { afterMin(nil) })
	mustPanic("slices.MinFunc(swapped)", func() { beforeMax(nil) })
	mustPanic("slices.Max", func() { afterMax(nil) })
}

// TestEquiv_PS3020_FloatDivergence pins WHY float elements stay advisory: with
// a NaN present, MinFunc under the swapped comparator computes the maximum of
// the natural cmp order, and cmp.Compare orders NaN FIRST — so the scan keeps
// a real number — while slices.Max propagates NaN via the builtin max. The two
// disagree outright, so the rewrite would change behavior.
func TestEquiv_PS3020_FloatDivergence(t *testing.T) {
	xs := []float64{1.5, math.NaN(), 2.5}
	before := slices.MinFunc(xs, func(a, b float64) int { return cmp.Compare(b, a) })
	after := slices.Max(xs)
	if !math.IsNaN(after) {
		t.Fatalf("expected slices.Max to propagate NaN, got %v", after)
	}
	if math.IsNaN(before) {
		t.Fatalf("expected the swapped MinFunc scan to skip NaN, got NaN")
	}
	// Signed zeros: the builtin min prefers -0.0, the swapped scan keeps the
	// earlier of the cmp.Compare tie.
	zs := []float64{math.Copysign(0, +1), math.Copysign(0, -1)}
	bmin := slices.MaxFunc(zs, func(a, b float64) int { return cmp.Compare(b, a) })
	amin := slices.Min(zs)
	if math.Signbit(bmin) == math.Signbit(amin) {
		t.Fatalf("expected a signed-zero divergence, both picked %v/signbit=%v", bmin, math.Signbit(bmin))
	}
}
