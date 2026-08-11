package ps2106

func combineTwo(x []string, name string) []string {
	x = append(x, "--verbose") // want `2 consecutive appends to x can be combined into one append call — a single length/growth check instead of 2`
	x = append(x, name)
	return x
}

func combineThree(x []int, a, b int) []int {
	x = append(x, a, b) // want `3 consecutive appends to x can be combined into one append call — a single length/growth check instead of 3`
	x = append(x, 1)
	x = append(x, b+1)
	return x
}

type pt struct{ a, b int }

func fieldArgs(x []int, p pt) []int {
	x = append(x, p.a) // want `2 consecutive appends to x can be combined into one append call — a single length/growth check instead of 2`
	x = append(x, p.b)
	return x
}

// A later argument with a call could observe x through a capture or a
// global; the run is reported but the fix is withheld.
func advisoryCalls(x []string, f func() string) []string {
	x = append(x, f()) // want `2 consecutive appends to x can be combined into one append call — a single length/growth check instead of 2 \(left advisory: an appended argument may have side effects or read shared memory; combine manually only if it cannot observe x\)`
	x = append(x, f())
	return x
}

// A later argument selecting through a pointer both may panic and may
// read memory aliasing x's backing array; advisory only.
func ptrFieldArgs(x []int, p *pt) []int {
	x = append(x, p.a) // want `2 consecutive appends to x can be combined into one append call — a single length/growth check instead of 2 \(left advisory: an appended argument may have side effects or read shared memory; combine manually only if it cannot observe x\)`
	x = append(x, p.b)
	return x
}

// len(x) must see the slice after the first append; combining would change
// its value, so the run ends and nothing is reported.
func mentionsX(x []int) []int {
	x = append(x, 1)
	x = append(x, len(x))
	return x
}

// Spread appends are never combined.
func spread(x, y []int) []int {
	x = append(x, 1)
	x = append(x, y...)
	return x
}

// A statement between the appends breaks adjacency.
func notAdjacent(x []int) ([]int, int) {
	x = append(x, 1)
	n := len(x)
	x = append(x, 2)
	return x, n
}

// Different slices never combine.
func differentSlices(x, y []int) ([]int, []int) {
	x = append(x, 1)
	y = append(y, 2)
	return x, y
}

// A shadowing local named append is not the builtin.
func shadowedAppend(x []int) []int {
	append := func(s []int, v ...int) []int { return s }
	x = append(x, 1)
	x = append(x, 2)
	return x
}

// The := form declares a new variable; only plain assignments combine.
func defineForm(x []int) []int {
	y := append(x, 1)
	y = append(y, 2)
	return y
}
