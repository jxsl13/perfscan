package ps2008

func perRow(n, d int) [][]float64 {
	rows := make([][]float64, n)
	for i := range rows {
		rows[i] = make([]float64, d) // want `rows\[i\] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one \(rows must die together and not be appended to\)`
	}
	return rows
}

type cfg struct{ dim int }

func selectorLen(n int, c cfg) [][]int {
	rows := make([][]int, n)
	for i := range rows {
		rows[i] = make([]int, c.dim) // want `rows\[i\] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one \(rows must die together and not be appended to\)`
	}
	return rows
}

func productLen(n, r, c int) [][]byte {
	rows := make([][]byte, n)
	for i := range rows {
		rows[i] = make([]byte, r*c) // want `rows\[i\] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one \(rows must die together and not be appended to\)`
	}
	return rows
}

// Counted-for loops stay advisory: reported, no automatic fix.
func classicFor(n, d int) [][]int {
	rows := make([][]int, n)
	for i := 0; i < n; i++ {
		rows[i] = make([]int, d) // want `rows\[i\] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one \(rows must die together and not be appended to\)`
	}
	return rows
}

// Extra statements in the body stay advisory: reported, no automatic fix.
func multiStmt(n, d int) [][]float64 {
	rows := make([][]float64, n)
	total := 0
	for i := range rows {
		rows[i] = make([]float64, d) // want `rows\[i\] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one \(rows must die together and not be appended to\)`
		total += d
	}
	_ = total
	return rows
}

// A make with an explicit capacity stays advisory: reported, no automatic fix.
func withCap(n, d int) [][]int {
	rows := make([][]int, n)
	for i := range rows {
		rows[i] = make([]int, d, d) // want `rows\[i\] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one \(rows must die together and not be appended to\)`
	}
	return rows
}

// A non-int length would make len(rows)*(d) a type mismatch: reported, no
// automatic fix.
func typedLen(n int, d int32) [][]int {
	rows := make([][]int, n)
	for i := range rows {
		rows[i] = make([]int, d) // want `rows\[i\] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one \(rows must die together and not be appended to\)`
	}
	return rows
}

// jagged rows vary with the loop variable: a slab cannot back them.
func jagged(n int) [][]byte {
	rows := make([][]byte, n)
	for i := range rows {
		rows[i] = make([]byte, i+1)
	}
	return rows
}

func slab(n, d int) [][]float64 {
	rows := make([][]float64, n)
	backing := make([]float64, n*d)
	for i := range rows {
		rows[i] = backing[i*d : (i+1)*d : (i+1)*d]
	}
	return rows
}
