package ps2144

type scalar float32

// Two adjacent slices, both live after the final make.
func pair(n int) float32 {
	a := make([]float32, n) // want `adjacent same-shape scratch slices remain live together`
	b := make([]float32, n)
	for i := range a {
		a[i] = float32(i)
		b[i] = -a[i]
	}
	return a[0] + b[0]
}

// Three lanes of a named, non-zero-sized element type. Direct len, copy, and
// clear uses are local and neither expose capacity nor let a slice escape.
func triple(n int, src []scalar) int {
	a := make([]scalar, n) // want `one overflow-guarded backing allocation with full-capacity non-overlapping lanes`
	b := make([]scalar, n)
	c := make([]scalar, n)
	copy(a, src)
	copy(b, a)
	clear(c)
	return len(a) + len(b) + int(c[0])
}

// Positive constants are stable; equivalent constant expressions share a
// value key. Nested lexical blocks are scanned independently.
func constants() int {
	if true {
		a := make([]int, 4+4) // want `adjacent same-shape scratch slices remain live together`
		b := make([]int, 8)
		a[0] = 1
		b[0] = 2
		return a[0] + b[0]
	}
	return 0
}

// Parentheses are transparent around documented direct-index and len uses.
func parenthesized(n int) int {
	a := make([]int, n) // want `adjacent same-shape scratch slices remain live together`
	b := make([]int, n)
	(a)[0], (b)[0] = 1, 2
	return len((a)) + len((b))
}

// Parentheses are also transparent for the other allowed whole-slice
// builtins and range.
func parenthesizedBuiltins(n int, src []int) int {
	a := make([]int, n) // want `adjacent same-shape scratch slices remain live together`
	b := make([]int, n)
	copy((a), src)
	clear((b))
	total := 0
	for i := range a {
		total = total + a[i] + b[i]
	}
	return total
}
