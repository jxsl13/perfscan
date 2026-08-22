package checks

import (
	"math"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
)

// TestEquiv_PS3033 pins the bit-identity of the PS3033 rewrite: the guarded
// map delete `if _, ok := m[k]; ok { delete(m, k) }` and the unconditional
// `delete(m, k)` leave the map in the same state for every input — present
// key, absent key, nil map, and (for float keys) a stored-but-unmatchable NaN
// key — because the builtin delete of an absent key, and of any key on a nil
// map, is a spec-guaranteed no-op. We run both forms on independently built
// equal maps and assert the full final map state agrees.
func TestEquiv_PS3033(t *testing.T) {
	before := func(m map[string]int, k string) {
		//lint:ignore S1033 deliberately the guarded "before" form — this test pins that it equals the plain delete
		if _, ok := m[k]; ok {
			delete(m, k)
		}
	}
	after := func(m map[string]int, k string) {
		delete(m, k)
	}

	clone := func(m map[string]int) map[string]int {
		if m == nil {
			return nil
		}
		c := make(map[string]int, len(m))
		for k, v := range m {
			c[k] = v
		}
		return c
	}

	check := func(m map[string]int, k string) {
		mb, ma := clone(m), clone(m)
		before(mb, k)
		after(ma, k)
		if !reflect.DeepEqual(mb, ma) || len(mb) != len(ma) {
			t.Fatalf("divergence for key %q on %v: before->%v after->%v", k, m, mb, ma)
		}
	}

	// Present, absent, zero-value, and empty-string keys against a fixed map.
	m0 := map[string]int{"a": 1, "b": 0, "c": -7, "": 42}
	for _, k := range []string{"a", "b", "c", "", "z", "aa", "A"} {
		check(m0, k)
	}

	// nil map: reading is legal (ok=false) and delete is a no-op — neither
	// side may panic and both must leave the map nil.
	var mnil map[string]int
	for _, k := range []string{"", "x"} {
		check(mnil, k)
	}

	// NaN float keys: a stored NaN key can never be matched, so the guard is
	// always false AND the unconditional delete matches nothing — the state
	// (including the unreachable NaN entry) must be identical.
	t.Run("nan-keys", func(t *testing.T) {
		nan := math.NaN()
		build := func() map[float64]int {
			return map[float64]int{nan: 1, 2.5: 2, 0: 3}
		}
		fbefore := func(m map[float64]int, k float64) {
			//lint:ignore S1033 deliberately the guarded "before" form under test
			if _, ok := m[k]; ok {
				delete(m, k)
			}
		}
		fafter := func(m map[float64]int, k float64) {
			delete(m, k)
		}
		for _, k := range []float64{nan, 2.5, 0, math.Copysign(0, -1), 99} {
			mb, ma := build(), build()
			fbefore(mb, k)
			fafter(ma, k)
			if len(mb) != len(ma) {
				t.Fatalf("divergence for float key %v: before len=%d after len=%d", k, len(mb), len(ma))
			}
			// Compare the non-NaN entries pairwise; NaN entries are counted
			// via the length (DeepEqual cannot look up NaN keys either).
			for k2, v2 := range mb {
				if !math.IsNaN(k2) {
					if av, ok := ma[k2]; !ok || av != v2 {
						t.Fatalf("divergence for float key %v at %v: before=%d after=(%d,%v)", k, k2, v2, av, ok)
					}
				}
			}
		}
	})

	// Randomized maps and keys, including repeated deletes of the same key.
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		size := r.Intn(8)
		m := make(map[string]int, size)
		for j := 0; j < size; j++ {
			m[strconv.Itoa(r.Intn(12))] = r.Intn(1000) - 500
		}
		k := strconv.Itoa(r.Intn(16))
		check(m, k)
		// Deleting twice in a row must also agree (second delete is absent).
		mb, ma := clone(m), clone(m)
		before(mb, k)
		before(mb, k)
		after(ma, k)
		after(ma, k)
		if !reflect.DeepEqual(mb, ma) {
			t.Fatalf("double-delete divergence for key %q on %v: before->%v after->%v", k, m, mb, ma)
		}
	}
}
