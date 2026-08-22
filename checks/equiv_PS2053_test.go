package checks

import "testing"

// TestEquivPS2053 proves the rewrite is byte-identical when the map is not
// mutated for the current key within the iteration (the only shape the check
// fixes): the value the range binds to v equals every re-read of m[k], so the
// key-only range with re-index and the key/value range produce the same result.
func TestEquivPS2053(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3, "z": 0, "neg": -4}

	rehash := func() (int, int) {
		sum, cnt := 0, 0
		for k := range m {
			sum += m[k]
			if m[k] > 0 {
				cnt++
			}
		}
		return sum, cnt
	}
	bound := func() (int, int) {
		sum, cnt := 0, 0
		for k, v := range m {
			_ = k
			sum += v
			if v > 0 {
				cnt++
			}
		}
		return sum, cnt
	}

	rs, rc := rehash()
	bs, bc := bound()
	if rs != bs || rc != bc {
		t.Fatalf("diverge: rehash=(%d,%d) bound=(%d,%d)", rs, rc, bs, bc)
	}
}
