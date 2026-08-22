package ps3005

// The sort package is imported under an ALIAS: PkgFuncCall matches aliased
// stdlib imports. Only the comparator body is rewritten — the s.Slice call
// itself REMAINS — so the aliased import stays used and is NOT pruned.

import s "sort"

func indirectAliased(idx []int, m [][]float64, f int) {
	s.Slice(idx, func(a, b int) bool { // want `the comparator sorting idx dereferences m\[idx\[…\]\]\[…\] on every comparison — a row-pointer load plus an index, O\(n log n\) times, for a value that depends only on the element; fill a flat id-indexed key column once and compare that \(same predicate, identical permutation\)`
		return m[idx[a]][f] < m[idx[b]][f]
	})
}
