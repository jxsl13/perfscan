package ps3033

type holder struct {
	m map[string]int
	k string
}

func key() string { return "x" }

// Positives sit on one line so the fixture's want comments land OUTSIDE the
// rewritten span (a comment inside the span withholds the fix by design).

// POSITIVE: plain identifiers.
func p1(m map[string]int, k string) {
	if _, ok := m[k]; ok { delete(m, k) } // want `the presence guard around delete is pure overhead`
}

// POSITIVE: field-selector map and key.
func p2(h holder) {
	if _, ok := h.m[h.k]; ok { delete(h.m, h.k) } // want `the presence guard around delete is pure overhead`
}

// POSITIVE: literal string key.
func p3(m map[string]int) {
	if _, ok := m["gone"]; ok { delete(m, "gone") } // want `the presence guard around delete is pure overhead`
}

// POSITIVE: literal int key.
func p4(m map[int]string) {
	if _, ok := m[7]; ok { delete(m, 7) } // want `the presence guard around delete is pure overhead`
}

// POSITIVE: a differently named ok variable.
func p5(m map[string]int, k string) {
	if _, exists := m[k]; exists { delete(m, k) } // want `the presence guard around delete is pure overhead`
}

// POSITIVE: float64-keyed map (delete of a never-matching NaN key is a no-op
// on both sides, so floats are safe here, unlike the sort-family checks).
func p6(m map[float64]int, k float64) {
	if _, ok := m[k]; ok { delete(m, k) } // want `the presence guard around delete is pure overhead`
}

// ADVISORY: a comment inside the if would be deleted by the fix, so the
// report carries no automatic fix and the code is left as-is.
func a1(m map[string]int, k string) {
	if _, ok := m[k]; ok { // want `the presence guard around delete is pure overhead`
		// keep: audit trail for the removal
		delete(m, k)
	}
}

// ADVISORY: the guard is the else-branch of another if — a bare statement
// cannot follow else, so the fix is withheld.
func a2(m map[string]int, k string, cond bool) {
	if cond {
		m[k] = 1
	} else if _, ok := m[k]; ok { delete(m, k) } // want `the presence guard around delete is pure overhead`
}

// NEGATIVE: an else branch changes behavior on the absent path.
func n1(m map[string]int, k string) {
	if _, ok := m[k]; ok {
		delete(m, k)
	} else {
		m[k] = 0
	}
}

// NEGATIVE: extra statement in the body.
func n2(m map[string]int, k string) int {
	n := 0
	if _, ok := m[k]; ok {
		delete(m, k)
		n++
	}
	return n
}

// NEGATIVE: negated condition (delete only if ABSENT — not this pattern).
func n3(m map[string]int, k string) {
	if _, ok := m[k]; !ok {
		delete(m, k)
	}
}

// NEGATIVE: compound condition.
func n4(m map[string]int, k string, extra bool) {
	if _, ok := m[k]; ok && extra {
		delete(m, k)
	}
}

// NEGATIVE: the value is bound and used — the guard is not throwaway.
func n5(m map[string]int, k string) int {
	if v, ok := m[k]; ok {
		delete(m, k)
		return v
	}
	return 0
}

// NEGATIVE: different key in the delete.
func n6(m map[string]int, k, k2 string) {
	if _, ok := m[k]; ok {
		delete(m, k2)
	}
}

// NEGATIVE: different map in the delete.
func n7(m, m2 map[string]int, k string) {
	if _, ok := m[k]; ok {
		delete(m2, k)
	}
}

// NEGATIVE: call-valued key is not side-effect-free (the original evaluates
// it twice, the rewrite once).
func n8(m map[string]int) {
	if _, ok := m[key()]; ok {
		delete(m, key())
	}
}

// NEGATIVE: a shadowing user function named delete is NOT a no-op on absent
// keys, so the guard is semantically load-bearing.
func n9(m map[string]int, k string) {
	delete := func(m map[string]int, k string) { m[k] = -1 }
	if _, ok := m[k]; ok {
		delete(m, k)
	}
}

// NEGATIVE: comma-ok over a type assertion, not a map index.
func n10(x any) {
	if _, ok := x.(string); ok {
		_ = x
	}
}

// NEGATIVE: shadowed text-alike — the delete's k resolves to a different
// object than the guard's k (same spelling, different variable).
func n11(m map[string]int, k string) {
	if _, ok := m[k]; ok {
		k := "other"
		delete(m, k)
	}
}
