package ps3021

type kb struct{ f string }

func f() string { return "x" }

// POSITIVE: single body read.
func p1(m map[string]int, k string) int {
	if _, ok := m[k]; ok { // want `map is looked up twice for the same key`
		return m[k]
	}
	return 0
}

// POSITIVE: multiple body reads, both rewritten.
func p2(m map[string]int, k string) int {
	if _, ok := m[k]; ok { // want `map is looked up twice for the same key`
		return m[k] + m[k]
	}
	return 0
}

// POSITIVE: field-selector key.
func p3(m map[string]int, o kb) int {
	if _, ok := m[o.f]; ok { // want `map is looked up twice for the same key`
		return m[o.f]
	}
	return 0
}

// POSITIVE: read nested in an inner block (same function scope, no mutation).
func p4(m map[string]int, k string) int {
	if _, ok := m[k]; ok { // want `map is looked up twice for the same key`
		if m[k] > 3 {
			return m[k]
		}
	}
	return 0
}

// NEGATIVE: zero body reads — the _ form is already correct.
func n1(m map[string]int, k string) int {
	if _, ok := m[k]; ok {
		return 1
	}
	return 0
}

// NEGATIVE: body writes m[k] — a re-read could differ.
func n2(m map[string]int, k string) int {
	if _, ok := m[k]; ok {
		m[k] = 5
		return m[k]
	}
	return 0
}

// NEGATIVE: body deletes from m.
func n3(m map[string]int, k string) int {
	if _, ok := m[k]; ok {
		v := m[k]
		delete(m, k)
		return v
	}
	return 0
}

// NEGATIVE: body reassigns m.
func n4(m map[string]int, k string) int {
	if _, ok := m[k]; ok {
		m = nil
		return len(m)
	}
	return 0
}

// NEGATIVE: call-valued key is not side-effect-free.
func n5(m map[string]int) int {
	if _, ok := m[f()]; ok {
		return m[f()]
	}
	return 0
}

// NEGATIVE: different key in the body.
func n6(m map[string]int, k, k2 string) int {
	if _, ok := m[k]; ok {
		return m[k2]
	}
	return 0
}

// NEGATIVE: value name is used (not the blank form) — out of scope here.
func n7(m map[string]int, k string) int {
	if v, ok := m[k]; ok {
		return v + m[k]
	}
	return 0
}

// NEGATIVE: qualifying read hidden in a closure that may outlive the guard.
func n8(m map[string]int, k string) func() int {
	if _, ok := m[k]; ok {
		return func() int { return m[k] }
	}
	return nil
}

// NEGATIVE: comma-ok over a channel receive, not a map index.
func n9(ch chan int) int {
	if _, ok := <-ch; ok {
		return 1
	}
	return 0
}
