package ps2116

type point struct{ X, Y int }

type buf []byte

// --- positives: each loop is exactly a zero-fill and becomes clear(s) ---

func zeroIntsRange(s []int) {
	for i := range s { // want `the loop writes the zero value to every element of s; clear\(s\) states that directly \(Go 1\.21\)`
		s[i] = 0
	}
}

func zeroBytesCounted(p []byte) {
	for i := 0; i < len(p); i++ { // want `the loop writes the zero value to every element of p; clear\(p\) states that directly \(Go 1\.21\)`
		p[i] = 0
	}
}

func zeroStrings(names []string) {
	for i := range names { // want `the loop writes the zero value to every element of names; clear\(names\) states that directly \(Go 1\.21\)`
		names[i] = ""
	}
}

func zeroBools(flags []bool) {
	for i := range flags { // want `the loop writes the zero value to every element of flags; clear\(flags\) states that directly \(Go 1\.21\)`
		flags[i] = false
	}
}

func zeroPointers(ps []*point) {
	for i := range ps { // want `the loop writes the zero value to every element of ps; clear\(ps\) states that directly \(Go 1\.21\)`
		ps[i] = nil
	}
}

// nil IS the zero value of an interface element.
func zeroAnys(vs []any) {
	for i := range vs { // want `the loop writes the zero value to every element of vs; clear\(vs\) states that directly \(Go 1\.21\)`
		vs[i] = nil
	}
}

// An empty struct literal is the struct's zero value.
func zeroStructs(pts []point) {
	for i := range pts { // want `the loop writes the zero value to every element of pts; clear\(pts\) states that directly \(Go 1\.21\)`
		pts[i] = point{}
	}
}

// A named slice type clears fine (its core type is a slice).
func zeroNamed(b buf) {
	for i := range b { // want `the loop writes the zero value to every element of b; clear\(b\) states that directly \(Go 1\.21\)`
		b[i] = 0
	}
}

func zeroFloats(fs []float64) {
	for i := 0; i < len(fs); i++ { // want `the loop writes the zero value to every element of fs; clear\(fs\) states that directly \(Go 1\.21\)`
		fs[i] = 0
	}
}

// A complex slice zeroed with 0 (real=imag=0) is clear-equivalent; exercises
// ps2116ConstZero's complex handling on the firing side.
func zeroComplex(cs []complex128) {
	for i := range cs { // want `the loop writes the zero value to every element of cs; clear\(cs\) states that directly \(Go 1\.21\)`
		cs[i] = 0
	}
}

// --- report-only: the loop is a zero-fill but the fix is suppressed ---

// A local `clear` captures the injected call: report, no fix.
func shadowedClear(s []int) {
	clear := func([]int) {}
	_ = clear
	for i := range s { // want `the loop writes the zero value to every element of s; clear\(s\) states that directly \(Go 1\.21\)`
		s[i] = 0
	}
}

// An interior comment would be deleted by the rewrite: report, no fix.
func interiorComment(s []int) {
	for i := range s { // want `the loop writes the zero value to every element of s; clear\(s\) states that directly \(Go 1\.21\)`
		// zeroed so the pool can reuse the backing array
		s[i] = 0
	}
}

// --- guards: none of the following may be reported or rewritten ---

// A non-zero fill has no clear equivalent; the loop IS the remedy.
func fillOnes(s []int) {
	for i := range s {
		s[i] = 1
	}
}

// A non-zero IMAGINARY fill is not the complex zero value: ps2116ConstZero's
// complex branch checks the sign of both the real AND imaginary parts, so
// 1i (real 0, imag 1) must NOT fire. A branch that only tested the real part
// would wrongly rewrite this to clear (which stores 0+0i).
func fillImaginary(cs []complex128) {
	for i := range cs {
		cs[i] = 1i
	}
}

// A runtime value is not provably the zero value.
func fillVar(s []int, v int) {
	for i := range s {
		s[i] = v
	}
}

// A named constant keeps its own meaning even when it happens to be zero.
const zeroC = 0

func fillNamedConst(s []int) {
	for i := range s {
		s[i] = zeroC
	}
}

// 0 assigned to an interface element boxes a non-nil int — NOT the zero
// value of the element type.
func boxedZero(vs []any) {
	for i := range vs {
		vs[i] = 0
	}
}

// []int{} is a non-nil empty slice, not the nil zero value.
func emptyNotNil(rows [][]int) {
	for i := range rows {
		rows[i] = []int{}
	}
}

// Zeroing MAP VALUES is not clear(m): clear on a map deletes the keys.
func zeroMapValues(m map[string]int) {
	for k := range m {
		m[k] = 0
	}
}

// Arrays are not clearable.
func zeroArray() [8]int {
	var a [8]int
	for i := range a {
		a[i] = 0
	}
	return a
}

// Neither are pointers-to-array.
func zeroArrayPtr(pa *[8]int) {
	for i := range pa {
		pa[i] = 0
	}
}

// With plain = the index survives the loop with an observable value.
func indexEscapes(s []int) int {
	var i int
	for i = range s {
		s[i] = 0
	}
	return i
}

// A second statement in the body is not a pure zero-fill.
func extraStatement(s []int) (n int) {
	for i := range s {
		s[i] = 0
		n++
	}
	return n
}

// The body writes a DIFFERENT slice than the one ranged over.
func otherTarget(s, t []int) {
	for i := range s {
		t[i] = 0
	}
}

// An integer range bound is not the slice's length.
func intRange(s []int, n int) {
	for i := range n {
		s[i] = 0
	}
}

// Counted-loop shape guards: wrong start, wrong bound, wrong step.
func startAtOne(s []int) {
	for i := 1; i < len(s); i++ {
		s[i] = 0
	}
}

func boundOtherSlice(s, t []int) {
	for i := 0; i < len(t); i++ {
		s[i] = 0
	}
}

func stepTwo(s []int) {
	for i := 0; i < len(s); i += 2 {
		s[i] = 0
	}
}

// A shadowing local len is not the builtin.
func shadowedLen(s []int) {
	len := func([]int) int { return 0 }
	for i := 0; i < len(s); i++ {
		s[i] = 0
	}
}
