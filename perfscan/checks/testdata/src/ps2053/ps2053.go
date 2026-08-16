package ps2053

func use(int)     {}
func useP(*int)   {}
func sideEffect() {}

type Rec struct{ N int }

// --- POSITIVES ---

func simple(counts map[string]int) int {
	total := 0
	for k := range counts { // want `map is ranged by key then re-indexed for the same key`
		total += counts[k]
	}
	return total
}

func multiRead(m map[string]int) (int, int) {
	a, b := 0, 0
	for k := range m { // want `map is ranged by key then re-indexed for the same key`
		a += m[k]
		b += m[k]
	}
	return a, b
}

// A pointer value copies a word — always a win.
func pointerVal(m map[string]*int) {
	for k := range m { // want `map is ranged by key then re-indexed for the same key`
		useP(m[k])
	}
}

// v is taken in the body, so the fix picks the next fresh name.
func freshName(m map[string]int) {
	for k := range m { // want `map is ranged by key then re-indexed for the same key`
		v := 1
		use(v + m[k])
	}
}

// --- ADVISORY: reported, no fix ---

func commentInRead(m map[string]int) int {
	total := 0
	for k := range m { // want `map is ranged by key then re-indexed for the same key`
		total += m[ /* keep */ k]
	}
	return total
}

// --- NEGATIVES: silent ---

// A struct value would be copied every iteration.
func structValue(m map[string]Rec) int {
	total := 0
	for k := range m {
		total += m[k].N
	}
	return total
}

// The body writes the entry.
func bodyMutates(m map[string]int) {
	for k := range m {
		m[k] = 0
		use(m[k])
	}
}

func bodyDeletes(m map[string]int) {
	for k := range m {
		delete(m, k)
		use(m[k])
	}
}

// No re-read of m[k].
func noRead(m map[string]int) int {
	n := 0
	for range m {
		n++
	}
	return n
}

// Already binds the value.
func alreadyBound(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

// Only a comma-ok read, which cannot become the single value v.
func commaOkOnly(m map[string]int) {
	for k := range m {
		if _, ok := m[k]; ok {
			sideEffect()
		}
	}
}

// A read hidden in a closure could run after m changes.
func closureRead(m map[string]int) {
	for k := range m {
		go func() { use(m[k]) }()
	}
}

// Ranging a slice, not a map.
func sliceRange(s []int) int {
	total := 0
	for i := range s {
		total += s[i]
	}
	return total
}
