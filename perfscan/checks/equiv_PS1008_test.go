package checks

// PS1008's runtime differential: slices.EqualFunc(a, b, func(x, y T) bool
// { return x == y }) must return a bool IDENTICAL to slices.Equal(a, b)
// for EVERY input — pinned against the REAL stdlib so a future stdlib
// change that breaks the claim fails CI. The claims:
//
//  1. LENGTH FIRST: both return false immediately on a length mismatch
//     (nil and empty are equal to each other on both sides).
//  2. SHORT-CIRCUIT SCAN: both scan left-to-right and stop at the first
//     differing pair, comparing with the identical == — including the
//     float edges (NaN != NaN on both sides, -0.0 == +0.0 on both sides)
//     and the swapped operand order (== is symmetric).
//  3. READ-ONLY: neither side mutates its inputs; the differential
//     verifies both slices are left byte-identical.
//  4. PANIC PARITY on interface elements: both sides return false without
//     panicking when a pair's dynamic types differ, and both panic with
//     the same "comparing uncomparable type" runtime error when a pair's
//     identical dynamic type is uncomparable.
//
// The inputs are adversarial for an equality scan: length mismatches with
// equal prefixes, a single differing pair planted at every position
// (first, last, middle), all-equal slices, len 0/1/2 boundaries, int
// extremes, strings with empty/prefix/non-ASCII shapes, float NaN and
// signed-zero edges, named element types, comparable structs and
// pointers, and interface elements with equal, unequal, type-mismatched
// and uncomparable pairs.

import (
	"math"
	"math/rand"
	"runtime"
	"slices"
	"testing"
)

func TestEquiv_PS1008EqualFuncBareEqSlicesEqual(t *testing.T) {
	checkInts := func(t *testing.T, a, b []int) {
		t.Helper()
		ca, cb := slices.Clone(a), slices.Clone(b)
		got := slices.EqualFunc(ca, cb, func(x, y int) bool { return x == y })
		want := slices.Equal(ca, cb)
		if got != want {
			t.Fatalf("EqualFunc(%v, %v, ==) = %v, slices.Equal = %v", a, b, got, want)
		}
		// The swapped operand order is the same equality (== is symmetric).
		if swapped := slices.EqualFunc(ca, cb, func(x, y int) bool { return y == x }); swapped != want {
			t.Fatalf("EqualFunc(%v, %v, swapped ==) = %v, slices.Equal = %v", a, b, swapped, want)
		}
		if !slices.Equal(ca, a) || !slices.Equal(cb, b) {
			t.Fatalf("an equality scan mutated its input: %v / %v from %v / %v", ca, cb, a, b)
		}
	}
	checkStrings := func(t *testing.T, a, b []string) {
		t.Helper()
		ca, cb := slices.Clone(a), slices.Clone(b)
		got := slices.EqualFunc(ca, cb, func(x, y string) bool { return x == y })
		want := slices.Equal(ca, cb)
		if got != want {
			t.Fatalf("EqualFunc(%q, %q, ==) = %v, slices.Equal = %v", a, b, got, want)
		}
		if !slices.Equal(ca, a) || !slices.Equal(cb, b) {
			t.Fatalf("an equality scan mutated its input: %q / %q from %q / %q", ca, cb, a, b)
		}
	}
	checkFloats := func(t *testing.T, a, b []float64) {
		t.Helper()
		got := slices.EqualFunc(a, b, func(x, y float64) bool { return x == y })
		want := slices.Equal(a, b)
		if got != want {
			t.Fatalf("EqualFunc(%v, %v, ==) = %v, slices.Equal = %v", a, b, got, want)
		}
	}
	checkAnys := func(t *testing.T, a, b []any) {
		t.Helper()
		got := slices.EqualFunc(a, b, func(x, y any) bool { return x == y })
		want := slices.Equal(a, b)
		if got != want {
			t.Fatalf("EqualFunc(%v, %v, ==) = %v, slices.Equal = %v", a, b, got, want)
		}
	}

	// Boundaries: len 0/1/2, nil vs empty (equal on both sides), length
	// mismatches with equal prefixes (the pre-scan length check).
	checkInts(t, nil, nil)
	checkInts(t, nil, []int{})
	checkInts(t, []int{}, nil)
	checkInts(t, []int{}, []int{})
	checkInts(t, []int{7}, []int{7})
	checkInts(t, []int{7}, []int{8})
	checkInts(t, []int{7}, nil)
	checkInts(t, nil, []int{7})
	checkInts(t, []int{1, 2}, []int{1, 2})
	checkInts(t, []int{1, 2}, []int{1, 2, 3})
	checkInts(t, []int{1, 2, 3}, []int{1, 2})

	// Int extremes.
	checkInts(t, []int{math.MinInt, 0, math.MaxInt}, []int{math.MinInt, 0, math.MaxInt})
	checkInts(t, []int{math.MinInt}, []int{math.MaxInt})
	checkInts(t, []int{math.MaxInt, math.MinInt}, []int{math.MinInt, math.MaxInt})

	// A single differing pair planted at EVERY position of an otherwise
	// equal pair of slices: both scans must spot it no matter where it
	// hides (first element, last element, anywhere between).
	for _, n := range []int{1, 2, 3, 7, 16, 64} {
		base := make([]int, n)
		for i := range base {
			base[i] = i * 3
		}
		checkInts(t, base, slices.Clone(base))
		for pos := 0; pos < n; pos++ {
			bad := slices.Clone(base)
			bad[pos]++
			checkInts(t, base, bad)
			checkInts(t, bad, base)
		}
	}

	// Aliasing: the same slice on both sides.
	same := []int{3, 1, 4, 1, 5}
	checkInts(t, same, same)

	// Randomized: tiny domains produce equal-prefix pairs whose verdict
	// hinges on one late element; wide domains produce early exits.
	r := rand.New(rand.NewSource(1008))
	for trial := 0; trial < 5000; trial++ {
		n := r.Intn(20)
		m := n
		if r.Intn(4) == 0 {
			m = r.Intn(20)
		}
		a := make([]int, n)
		b := make([]int, m)
		for i := range a {
			a[i] = r.Intn(3)
		}
		for i := range b {
			b[i] = r.Intn(3)
		}
		checkInts(t, a, b)
		checkInts(t, a, slices.Clone(a))
	}

	// Strings: empty strings, prefix pairs ("a" vs "aa"), non-ASCII bytes.
	pool := []string{"", "a", "aa", "ab", "b", "\x00", "\xff", "abc", "ÿ"}
	for _, x := range pool {
		for _, y := range pool {
			checkStrings(t, []string{x}, []string{y})
			checkStrings(t, []string{x, y}, []string{x, y})
			checkStrings(t, []string{x, y}, []string{y, x})
		}
	}
	for trial := 0; trial < 3000; trial++ {
		n := r.Intn(8)
		a := make([]string, n)
		b := make([]string, n)
		for i := range a {
			a[i] = pool[r.Intn(len(pool))]
			b[i] = pool[r.Intn(len(pool))]
		}
		checkStrings(t, a, b)
		checkStrings(t, a, slices.Clone(a))
	}

	// Floats: NaN != NaN makes BOTH sides answer false even against an
	// otherwise identical slice — including a NaN compared with itself in
	// an aliased scan — and -0.0 == +0.0 makes both answer true.
	nan := math.NaN()
	checkFloats(t, []float64{nan}, []float64{nan})
	fs := []float64{1, nan, 3}
	checkFloats(t, fs, fs) // aliased: still false on both sides
	checkFloats(t, []float64{math.Copysign(0, -1)}, []float64{0})
	checkFloats(t, []float64{0}, []float64{math.Copysign(0, -1)})
	checkFloats(t, []float64{1, math.Copysign(0, -1), 2}, []float64{1, 0, 2})
	checkFloats(t, []float64{math.Inf(1), math.Inf(-1)}, []float64{math.Inf(1), math.Inf(-1)})
	checkFloats(t, []float64{math.Inf(1)}, []float64{math.Inf(-1)})
	checkFloats(t, []float64{math.SmallestNonzeroFloat64}, []float64{0})

	// Named element types: == compares underlying values on both sides.
	type id int64
	ids1, ids2 := []id{1, 2, 3}, []id{1, 2, 4}
	if got, want := slices.EqualFunc(ids1, ids2, func(x, y id) bool { return x == y }), slices.Equal(ids1, ids2); got != want {
		t.Fatalf("named element: EqualFunc = %v, Equal = %v", got, want)
	}

	// Comparable structs and pointers.
	type pt struct{ X, Y int }
	ps1, ps2 := []pt{{1, 2}, {3, 4}}, []pt{{1, 2}, {3, 5}}
	if got, want := slices.EqualFunc(ps1, ps2, func(x, y pt) bool { return x == y }), slices.Equal(ps1, ps2); got != want {
		t.Fatalf("struct element: EqualFunc = %v, Equal = %v", got, want)
	}
	n1, n2 := &pt{}, &pt{}
	ptrs1, ptrs2 := []*pt{n1, n2}, []*pt{n1, n2}
	if got, want := slices.EqualFunc(ptrs1, ptrs2, func(x, y *pt) bool { return x == y }), slices.Equal(ptrs1, ptrs2); got != want {
		t.Fatalf("pointer element: EqualFunc = %v, Equal = %v", got, want)
	}

	// Interface elements: equal pairs, unequal pairs, and pairs whose
	// dynamic types DIFFER (false without a panic on both sides — the
	// dynamic-type comparison comes first).
	checkAnys(t, []any{1, "a", 2.5}, []any{1, "a", 2.5})
	checkAnys(t, []any{1, "a"}, []any{1, "b"})
	checkAnys(t, []any{1}, []any{"1"})
	checkAnys(t, []any{nil}, []any{nil})
	checkAnys(t, []any{nil}, []any{0})
	checkAnys(t, []any{1, []int{1}}, []any{2, []int{1}}) // early exit BEFORE the uncomparable pair

	// PANIC PARITY: when the scan reaches a pair whose identical dynamic
	// type is uncomparable, BOTH sides panic with the same runtime error.
	mustPanic := func(t *testing.T, name string, f func() bool) (msg string) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("%s: expected a runtime panic on an uncomparable pair", name)
			}
			err, ok := r.(runtime.Error)
			if !ok {
				t.Fatalf("%s: panic %v (%T) is not a runtime.Error", name, r, r)
			}
			msg = err.Error()
		}()
		f()
		return ""
	}
	ua := []any{1, []int{1}}
	ub := []any{1, []int{2}}
	before := mustPanic(t, "EqualFunc", func() bool {
		return slices.EqualFunc(ua, ub, func(x, y any) bool { return x == y })
	})
	after := mustPanic(t, "Equal", func() bool {
		return slices.Equal(ua, ub)
	})
	if before != after {
		t.Fatalf("panic messages diverge: EqualFunc %q vs Equal %q", before, after)
	}

	// Generic context: a comparable-constrained type parameter element —
	// the closure's x == y and Equal's E comparable resolve to the same
	// comparison.
	if g, w := ps1008EqBoth([]int{1, 2}, []int{1, 2}); g != w {
		t.Fatalf("generic ints: %v vs %v", g, w)
	}
	if g, w := ps1008EqBoth([]string{"a"}, []string{"b"}); g != w {
		t.Fatalf("generic strings: %v vs %v", g, w)
	}
}

// ps1008EqBoth runs the before (EqualFunc + bare == closure) and after
// (Equal) spellings on a comparable-constrained type parameter element.
func ps1008EqBoth[T comparable](a, b []T) (bool, bool) {
	return slices.EqualFunc(a, b, func(x, y T) bool { return x == y }), slices.Equal(a, b)
}
