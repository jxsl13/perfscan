package ps2008

func perRow(n, d int) [][]float64 {
	rows := make([][]float64, n)
	for i := range rows {
		rows[i] = make([]float64, d) // want `rows\[i\] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one \(rows must die together and not be appended to\)`
	}
	return rows
}

func classicFor(n, d int) [][]int {
	rows := make([][]int, n)
	for i := 0; i < n; i++ {
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
