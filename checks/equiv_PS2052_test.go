package checks

import "testing"

// TestEquivPS2052 proves the rewrite is byte-identical when the map is not
// mutated between the reads (the only shape the check fixes): the value bound
// once in the if-init equals every re-read of m[k], including the absent-key
// zero value. The bound form and the double-lookup form return the same result.
func TestEquivPS2052(t *testing.T) {
	m := map[string]int{"present": 7, "zero": 0, "neg": -3}
	keys := []string{"present", "zero", "neg", "absent"}

	// Shape: if m[k] > 0 { total += m[k] } (double lookup) vs
	//        if v := m[k]; v > 0 { total += v } (bound once).
	doubleGuard := func(k string) int {
		total := 100
		if m[k] > 0 {
			total += m[k]
		}
		return total
	}
	boundGuard := func(k string) int {
		total := 100
		if v := m[k]; v > 0 {
			total += v
		}
		return total
	}
	// Shape: if m[k] > 0 && m[k] < 10 { ... } (two condition reads).
	doubleCond := func(k string) bool { return m[k] > 0 && m[k] < 10 }
	boundCond := func(k string) bool { v := m[k]; return v > 0 && v < 10 }

	for _, k := range keys {
		if doubleGuard(k) != boundGuard(k) {
			t.Fatalf("guard k=%q: double=%d bound=%d", k, doubleGuard(k), boundGuard(k))
		}
		if doubleCond(k) != boundCond(k) {
			t.Fatalf("cond k=%q: double=%v bound=%v", k, doubleCond(k), boundCond(k))
		}
	}
}
