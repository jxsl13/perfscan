package checks

import (
	"math/rand"
	"strconv"
	"testing"
)

// TestEquiv_PS3021 pins the bit-identity of the PS3021 rewrite: the map
// double-lookup `if _, ok := m[k]; ok { use(m[k]) }` and its single-bind form
// `if v, ok := m[k]; ok { use(v) }` take the same branch and observe the same
// value for every input, PROVIDED the body does not mutate the map for that key
// (which the check requires). We model the body as a pure read and assert the
// (taken, value) pair agrees over exhaustive small keys plus randomized maps.
func TestEquiv_PS3021(t *testing.T) {
	// before models `if _, ok := m[k]; ok { return m[k], true }`.
	before := func(m map[string]int, k string) (int, bool) {
		if _, ok := m[k]; ok {
			return m[k], true
		}
		return 0, false
	}
	// after models `if v, ok := m[k]; ok { return v, true }`.
	after := func(m map[string]int, k string) (int, bool) {
		if v, ok := m[k]; ok {
			return v, true
		}
		return 0, false
	}

	check := func(m map[string]int, k string) {
		bv, bok := before(m, k)
		av, aok := after(m, k)
		if bv != av || bok != aok {
			t.Fatalf("divergence for key %q: before=(%d,%v) after=(%d,%v)", k, bv, bok, av, aok)
		}
	}

	// Exhaustive small keys against a fixed map (present, absent, zero-value).
	m0 := map[string]int{"a": 1, "b": 0, "c": -7, "": 42}
	for _, k := range []string{"a", "b", "c", "", "z", "aa", "A"} {
		check(m0, k)
	}
	// nil map.
	var mnil map[string]int
	for _, k := range []string{"", "x"} {
		check(mnil, k)
	}

	// Randomized maps and keys.
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		size := r.Intn(8)
		m := make(map[string]int, size)
		for j := 0; j < size; j++ {
			m[strconv.Itoa(r.Intn(12))] = r.Intn(1000) - 500
		}
		check(m, strconv.Itoa(r.Intn(16)))
	}
}
