package ps3005

import "sort"

func indirect(idx []int, m [][]float64, f int) {
	sort.Slice(idx, func(a, b int) bool { // want `the comparator sorting idx dereferences m\[idx\[…\]\]\[…\] on every comparison — a row-pointer load plus an index, O\(n log n\) times, for a value that depends only on the element; fill a flat id-indexed key column once and compare that \(same predicate, identical permutation\)`
		return m[idx[a]][f] < m[idx[b]][f]
	})
}

// A flat key is the fixed form: applying the advice clears the finding.
func flatKey(idx []int, key []float64) {
	sort.Slice(idx, func(a, b int) bool {
		return key[idx[a]] < key[idx[b]]
	})
}

// Sorting the data itself (not an index slice): no double dereference.
func direct(xs []float64) {
	sort.Slice(xs, func(a, b int) bool {
		return xs[a] < xs[b]
	})
}
